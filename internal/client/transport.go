package client

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paulGUZU/fsak/pkg/config"
	"github.com/paulGUZU/fsak/pkg/crypto"
)

type Transport struct {
	Config *config.Config
	Pool   *AddressPool
	Client *http.Client
}

func NewTransport(cfg *config.Config, pool *AddressPool) *Transport {
	return &Transport{
		Config: cfg,
		Pool:   pool,
		Client: &http.Client{
			Timeout: 30 * time.Second, // Timeout for individual chunk requests
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				DisableKeepAlives:   false,
			},
		},
	}
}

func (t *Transport) Tunnel(target string, clientConn net.Conn) error {
	serverIP := t.Pool.PickBest()
	sessionID := uuid.New().String()
	
	destURL := fmt.Sprintf("%s://%s:%d", t.scheme(), serverIP, t.Config.Port)
	hostHeader := t.Config.Host
	
	var wg sync.WaitGroup
	wg.Add(2)
	
	// Control
	done := make(chan struct{})
	
	// Upload Loop
	go func() {
		defer wg.Done()
		t.uploadLoop(destURL, hostHeader, sessionID, target, clientConn, done)
	}()

	// Download Loop
	go func() {
		defer wg.Done()
		t.downloadLoop(destURL, hostHeader, sessionID, clientConn, done)
	}()

	wg.Wait()
	return nil
}

func (t *Transport) scheme() string {
	if t.Config.TLS {
		return "https"
	}
	return "http"
}

func (t *Transport) uploadLoop(baseURL, host, id, target string, clientConn net.Conn, done chan struct{}) {
	// First request must contain target address
	firstPacket := true
	
	buf := make([]byte, 512*1024) // 512KB chunks to improve throughput
	
	for {
		select {
		case <-done:
			return
		default:
		}

		n, err := clientConn.Read(buf)
		if n > 0 {
			// Construct Body: [IV][EncryptedPayload]
			body := new(bytes.Buffer)
			
			// IV
			iv := make([]byte, 16)
			io.ReadFull(rand.Reader, iv)
			body.Write(iv)
			
			writer, _ := crypto.NewCryptoWriter(body, t.Config.Secret, iv)
			
			if firstPacket {
				l := uint16(len(target))
				lBuf := make([]byte, 2)
				binary.BigEndian.PutUint16(lBuf, l)
				writer.Write(lBuf)
				writer.Write([]byte(target))
				firstPacket = false
			}
			
			writer.Write(buf[:n])
			
			// Send Request
			t.sendChunk(baseURL, host, id, body.Bytes())
		}
		
		if err != nil {
			close(done)
			return
		}
	}
}

func (t *Transport) sendChunk(baseURL, host, id string, data []byte) {
	url := fmt.Sprintf("%s/upload?session_id=%s", baseURL, id)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	req.Host = host
	req.Header.Set("Content-Type", "application/octet-stream")
	
	resp, err := t.Client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
	} else {
		// Log error
		// If upload fails, maybe retry? For TCP tunnel, packet loss is fatal?
		// SOCKS5 over TCP usually expects reliability. 
		// If we lose a chunk, the stream is corrupt.
		// For MVP, just log.
		fmt.Printf("Upload chunk failed: %v\n", err)
	}
}

func (t *Transport) downloadLoop(baseURL, host, id string, clientConn net.Conn, done chan struct{}) {
	url := fmt.Sprintf("%s/download?session_id=%s", baseURL, id)
	
	for {
		select {
		case <-done:
			return
		default:
		}

		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Host = host
		
		resp, err := t.Client.Do(req)
		if err != nil {
			time.Sleep(1 * time.Second) // Backoff
			continue
		}
		
		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			continue // No data, poll again
		}
		
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(1 * time.Second)
			continue
		}

		// Read Body: [IV][EncryptedData]
		iv := make([]byte, 16)
		if _, err := io.ReadFull(resp.Body, iv); err != nil {
			resp.Body.Close()
			continue
		}

		reader, _ := crypto.NewCryptoReader(resp.Body, t.Config.Secret, iv)
		io.Copy(clientConn, reader)
		
		resp.Body.Close()
	}
}
