package server

import (
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"errors"
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
	uploadFlagFirst      byte = 1
	uploadFrameMinHeader      = 5
	downloadChunkSize         = 256 * 1024
)

type Session struct {
	id         string
	targetConn net.Conn
	lastActive time.Time
	mu         sync.Mutex
	closed     bool

	nextUploadSeq uint32
	pendingUpload map[uint32][]byte
}

func NewSession(id string) *Session {
	return &Session{
		id:            id,
		lastActive:    time.Now(),
		pendingUpload: make(map[uint32][]byte),
	}
}

type Handler struct {
	Config   *config.Config
	Sessions sync.Map

	secretKey [32]byte
	bufPool   sync.Pool
}

func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{
		Config:    cfg,
		secretKey: crypto.DeriveKey(cfg.Secret),
		bufPool: sync.Pool{
			New: func() any { return make([]byte, downloadChunkSize) },
		},
	}
	go h.cleanupLoop()
	return h
}

func (h *Handler) cleanupLoop() {
	for {
		time.Sleep(1 * time.Minute)
		h.Sessions.Range(func(key, value interface{}) bool {
			s := value.(*Session)
			s.mu.Lock()
			if time.Since(s.lastActive) > 2*time.Minute {
				if s.targetConn != nil {
					_ = s.targetConn.Close()
					s.targetConn = nil
				}
				s.closed = true
				s.pendingUpload = nil
				h.Sessions.Delete(key)
			}
			s.mu.Unlock()
			return true
		})
	}
}

func (h *Handler) GetSession(id string) *Session {
	v, _ := h.Sessions.LoadOrStore(id, NewSession(id))
	return v.(*Session)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "missing session_id", http.StatusBadRequest)
		return
	}

	session := h.GetSession(sessionID)
	session.mu.Lock()
	session.lastActive = time.Now()
	session.mu.Unlock()

	switch r.Method {
	case http.MethodPost:
		h.handleUpload(w, r, session)
	case http.MethodGet:
		h.handleDownload(w, r, session)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request, s *Session) {
	defer r.Body.Close()

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(r.Body, iv); err != nil {
		http.Error(w, "failed to read iv", http.StatusBadRequest)
		return
	}

	encryptedPayload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read payload", http.StatusBadRequest)
		return
	}
	if len(encryptedPayload) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := crypto.XORCTRInPlace(h.secretKey, iv, encryptedPayload); err != nil {
		http.Error(w, "crypto error", http.StatusInternalServerError)
		return
	}

	seq, isFirst, targetAddr, payload, err := parseUploadFrame(encryptedPayload)
	if err != nil {
		http.Error(w, "invalid upload frame", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "session closed", http.StatusGone)
		return
	}

	if seq < s.nextUploadSeq {
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return
	}
	if _, exists := s.pendingUpload[seq]; !exists {
		// Keep a compact copy in pending map.
		s.pendingUpload[seq] = append([]byte(nil), payload...)
	}
	needDial := s.targetConn == nil && isFirst
	s.mu.Unlock()

	if needDial {
		conn, dialErr := net.DialTimeout("tcp", targetAddr, 10*time.Second)
		if dialErr != nil {
			http.Error(w, fmt.Sprintf("dial failed: %v", dialErr), http.StatusBadGateway)
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			http.Error(w, "session closed", http.StatusGone)
			return
		}
		if s.targetConn == nil {
			s.targetConn = conn
		} else {
			_ = conn.Close()
		}
		s.mu.Unlock()
	}

	for {
		s.mu.Lock()
		conn := s.targetConn
		data, ok := s.pendingUpload[s.nextUploadSeq]
		if !ok || conn == nil {
			s.mu.Unlock()
			break
		}
		delete(s.pendingUpload, s.nextUploadSeq)
		s.nextUploadSeq++
		s.mu.Unlock()

		if len(data) == 0 {
			continue
		}
		if _, writeErr := conn.Write(data); writeErr != nil {
			s.mu.Lock()
			if s.targetConn != nil {
				_ = s.targetConn.Close()
				s.targetConn = nil
			}
			s.closed = true
			s.mu.Unlock()
			http.Error(w, "target connection closed", http.StatusBadGateway)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func parseUploadFrame(frame []byte) (seq uint32, isFirst bool, target string, payload []byte, err error) {
	if len(frame) < uploadFrameMinHeader {
		return 0, false, "", nil, errors.New("frame too short")
	}

	seq = binary.BigEndian.Uint32(frame[0:4])
	flags := frame[4]
	isFirst = flags&uploadFlagFirst != 0
	offset := uploadFrameMinHeader

	if isFirst {
		if len(frame) < offset+2 {
			return 0, false, "", nil, errors.New("missing target len")
		}
		targetLen := int(binary.BigEndian.Uint16(frame[offset : offset+2]))
		offset += 2
		if targetLen < 0 || len(frame) < offset+targetLen {
			return 0, false, "", nil, errors.New("invalid target len")
		}
		target = string(frame[offset : offset+targetLen])
		offset += targetLen
		if strings.TrimSpace(target) == "" {
			return 0, false, "", nil, errors.New("empty target")
		}
	}

	if offset > len(frame) {
		return 0, false, "", nil, errors.New("invalid frame")
	}
	return seq, isFirst, target, frame[offset:], nil
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, s *Session) {
	s.mu.Lock()
	conn := s.targetConn
	closed := s.closed
	s.mu.Unlock()

	if closed {
		http.Error(w, "session closed", http.StatusGone)
		return
	}
	if conn == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	buf := h.bufPool.Get().([]byte)
	defer h.bufPool.Put(buf)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err == io.EOF {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	total := n
	for total < len(buf) {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Millisecond))
		m, readErr := conn.Read(buf[total:])
		if m > 0 {
			total += m
		}
		if readErr != nil {
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				break
			}
			if readErr == io.EOF {
				break
			}
			break
		}
		if m == 0 {
			break
		}
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		http.Error(w, "internal iv error", http.StatusInternalServerError)
		return
	}
	payload := buf[:total]
	if err := crypto.XORCTRInPlace(h.secretKey, iv, payload); err != nil {
		http.Error(w, "crypto error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write(iv); err != nil {
		return
	}
	_, _ = w.Write(payload)
}
