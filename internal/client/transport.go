package client

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/paulGUZU/fsak/pkg/config"
	"github.com/paulGUZU/fsak/pkg/crypto"
)

const (
	uploadFlagFirst     byte = 1
	uploadFrameHeader        = 5 // [seq(4)][flags(1)]
	uploadPipelineLimit      = 4

	minUploadChunkSize     = 16 * 1024
	initialUploadChunkSize = 64 * 1024
	maxUploadChunkSize     = 512 * 1024

	downloadNoDataBackoff = 120 * time.Millisecond
)

type Transport struct {
	Config *config.Config
	Pool   *AddressPool
	Client *http.Client

	outboundInterface string
	secretKey         [32]byte
	framePool         sync.Pool
}

func NewTransport(cfg *config.Config, pool *AddressPool) *Transport {
	httpTransport := newHTTPTransport("")
	return &Transport{
		Config:    cfg,
		Pool:      pool,
		Client:    &http.Client{Timeout: 30 * time.Second, Transport: httpTransport},
		secretKey: crypto.DeriveKey(cfg.Secret),
		framePool: sync.Pool{
			New: func() any {
				return make([]byte, aes.BlockSize+maxUploadChunkSize+uploadFrameHeader+256)
			},
		},
	}
}

func newHTTPTransport(outboundInterface string) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	if control := interfaceDialerControl(strings.TrimSpace(outboundInterface)); control != nil {
		dialer.Control = control
	}
	return &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		DisableKeepAlives:   false,
		DialContext:         dialer.DialContext,
	}
}

func (t *Transport) SetOutboundInterface(name string) {
	name = strings.TrimSpace(name)
	if name == t.outboundInterface {
		return
	}
	t.outboundInterface = name
	t.Client.Transport = newHTTPTransport(name)
}

func (t *Transport) Tunnel(target string, clientConn net.Conn) error {
	serverIP := t.Pool.PickBest()
	sessionID := newSessionID()

	destURL := fmt.Sprintf("%s://%s:%d", t.scheme(), serverIP, t.Config.Port)
	hostHeader := t.Config.Host

	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	var doneOnce sync.Once
	stop := func() {
		doneOnce.Do(func() {
			close(done)
			cancel()
		})
	}
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		t.uploadLoop(ctx, destURL, hostHeader, sessionID, target, serverIP, clientConn, done, stop)
	}()
	go func() {
		defer wg.Done()
		t.downloadLoop(ctx, destURL, hostHeader, sessionID, clientConn, done, stop)
	}()
	wg.Wait()
	return nil
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (t *Transport) scheme() string {
	if t.Config.TLS {
		return "https"
	}
	return "http"
}

func (t *Transport) uploadLoop(ctx context.Context, baseURL, host, id, target, serverIP string, clientConn net.Conn, done chan struct{}, stop func()) {
	targetBytes := []byte(target)
	if len(targetBytes) > 65535 {
		fmt.Printf("Upload chunk failed: target address too long\n")
		stop()
		return
	}

	readBuf := make([]byte, maxUploadChunkSize)
	sizer := newAdaptiveChunkSizer(initialUploadChunkSize, minUploadChunkSize, maxUploadChunkSize)

	var seq uint32
	firstPacket := true
	var sendWG sync.WaitGroup
	sem := make(chan struct{}, uploadPipelineLimit)
	defer sendWG.Wait()

readLoop:
	for {
		select {
		case <-done:
			break readLoop
		default:
		}

		chunkSize := sizer.Next()
		if chunkSize < minUploadChunkSize {
			chunkSize = minUploadChunkSize
		}
		if chunkSize > maxUploadChunkSize {
			chunkSize = maxUploadChunkSize
		}

		_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := clientConn.Read(readBuf[:chunkSize])
		if n > 0 {
			body, backing, errBuild := t.buildUploadChunk(seq, firstPacket, targetBytes, readBuf[:n])
			if errBuild != nil {
				fmt.Printf("Upload chunk failed: %v\n", errBuild)
				stop()
				break readLoop
			}

			select {
			case sem <- struct{}{}:
			case <-done:
				t.putFrameBuffer(backing)
				break readLoop
			}

			sendWG.Add(1)
			go func(payload []byte, backing []byte) {
				defer sendWG.Done()
				defer func() {
					<-sem
					t.putFrameBuffer(backing)
				}()

				dur, sendErr := t.sendChunk(ctx, baseURL, host, id, payload)
				sizer.Observe(dur, sendErr == nil)
				t.Pool.ReportRuntimeResult(serverIP, sendErr == nil, dur)
				if sendErr != nil {
					fmt.Printf("Upload chunk failed: %v\n", sendErr)
					stop()
				}
			}(body, backing)

			firstPacket = false
			seq++
		}

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			stop()
			break
		}
	}
	_ = clientConn.SetReadDeadline(time.Time{})
}

func (t *Transport) buildUploadChunk(seq uint32, first bool, target []byte, data []byte) (body []byte, backing []byte, err error) {
	plainSize := uploadFrameHeader + len(data)
	if first {
		plainSize += 2 + len(target)
	}
	totalSize := aes.BlockSize + plainSize

	backing = t.getFrameBuffer(totalSize)
	body = backing[:totalSize]
	iv := body[:aes.BlockSize]
	if _, err := rand.Read(iv); err != nil {
		t.putFrameBuffer(backing)
		return nil, nil, err
	}

	plain := body[aes.BlockSize:]
	binary.BigEndian.PutUint32(plain[0:4], seq)
	if first {
		plain[4] = uploadFlagFirst
	} else {
		plain[4] = 0
	}

	offset := uploadFrameHeader
	if first {
		binary.BigEndian.PutUint16(plain[offset:offset+2], uint16(len(target)))
		offset += 2
		copy(plain[offset:offset+len(target)], target)
		offset += len(target)
	}
	copy(plain[offset:offset+len(data)], data)
	plain = plain[:offset+len(data)]

	if err := crypto.XORCTRInPlace(t.secretKey, iv, plain); err != nil {
		t.putFrameBuffer(backing)
		return nil, nil, err
	}
	return body[:aes.BlockSize+len(plain)], backing, nil
}

func (t *Transport) getFrameBuffer(size int) []byte {
	v := t.framePool.Get()
	if v == nil {
		return make([]byte, size)
	}
	buf := v.([]byte)
	if cap(buf) < size {
		return make([]byte, size)
	}
	return buf[:size]
}

func (t *Transport) putFrameBuffer(buf []byte) {
	if buf == nil {
		return
	}
	if cap(buf) > (aes.BlockSize+maxUploadChunkSize+uploadFrameHeader+2048)*2 {
		return
	}
	t.framePool.Put(buf[:0])
}

func (t *Transport) sendChunk(ctx context.Context, baseURL, host, id string, data []byte) (time.Duration, error) {
	start := time.Now()
	url := fmt.Sprintf("%s/upload?session_id=%s", baseURL, id)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	req.Host = host
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := t.Client.Do(req)
	if err != nil {
		return time.Since(start), err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return time.Since(start), fmt.Errorf("upload failed with status %s", resp.Status)
	}
	return time.Since(start), nil
}

func (t *Transport) downloadLoop(ctx context.Context, baseURL, host, id string, clientConn net.Conn, done chan struct{}, stop func()) {
	url := fmt.Sprintf("%s/download?session_id=%s", baseURL, id)

	for {
		select {
		case <-done:
			return
		default:
		}

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Host = host

		resp, err := t.Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			select {
			case <-done:
				return
			case <-time.After(downloadNoDataBackoff):
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(200 * time.Millisecond)
			continue
		}

		iv := make([]byte, aes.BlockSize)
		if _, err := io.ReadFull(resp.Body, iv); err != nil {
			resp.Body.Close()
			continue
		}

		reader, err := crypto.NewCryptoReaderWithKey(resp.Body, t.secretKey, iv)
		if err != nil {
			resp.Body.Close()
			continue
		}
		if _, err := io.Copy(clientConn, reader); err != nil {
			resp.Body.Close()
			stop()
			return
		}
		resp.Body.Close()
	}
}

type adaptiveChunkSizer struct {
	mu  sync.Mutex
	cur int
	min int
	max int
}

func newAdaptiveChunkSizer(initial, min, max int) *adaptiveChunkSizer {
	if initial < min {
		initial = min
	}
	if initial > max {
		initial = max
	}
	return &adaptiveChunkSizer{
		cur: initial,
		min: min,
		max: max,
	}
}

func (s *adaptiveChunkSizer) Next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *adaptiveChunkSizer) Observe(rtt time.Duration, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !ok {
		s.cur /= 2
		if s.cur < s.min {
			s.cur = s.min
		}
		return
	}

	switch {
	case rtt < 180*time.Millisecond:
		s.cur += 16 * 1024
	case rtt > 1200*time.Millisecond:
		s.cur /= 2
	case rtt > 700*time.Millisecond:
		s.cur -= 8 * 1024
	}

	if s.cur < s.min {
		s.cur = s.min
	}
	if s.cur > s.max {
		s.cur = s.max
	}
}
