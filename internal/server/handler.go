package server

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/paulGUZU/fsak/pkg/config"
	"github.com/paulGUZU/fsak/pkg/crypto"
)

type Session struct {
	id         string
	targetConn net.Conn
	lastActive time.Time
	mu         sync.Mutex
	closed     bool
}

func NewSession(id string) *Session {
	return &Session{
		id:         id,
		lastActive: time.Now(),
	}
}

type Handler struct {
	Config   *config.Config
	Sessions sync.Map // map[string]*Session
}

func NewHandler(cfg *config.Config) *Handler {
	h := &Handler{
		Config: cfg,
	}
	// Background cleanup of stale sessions
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
					s.targetConn.Close()
				}
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

	if r.Method == http.MethodPost {
		h.handleUpload(w, r, session)
	} else if r.Method == http.MethodGet {
		h.handleDownload(w, r, session)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request, s *Session) {
	defer r.Body.Close()

	// 1. Read IV (Per chunk)
	iv := make([]byte, 16)
	if _, err := io.ReadFull(r.Body, iv); err != nil {
		if err == io.EOF {
			return
		}
		http.Error(w, "failed to read iv", http.StatusBadRequest)
		return
	}

	// 2. Decrypt Body
	// Since we are doing per-request chunks, we can read the whole body or stream it.
	// Streaming is better for memory.
	reader, err := crypto.NewCryptoReader(r.Body, h.Config.Secret, iv)
	if err != nil {
		http.Error(w, "crypto error", http.StatusInternalServerError)
		return
	}

	// 3. Connect if needed
	s.mu.Lock()
	if s.targetConn == nil {
		if s.closed {
			s.mu.Unlock()
			http.Error(w, "session closed", http.StatusGone)
			return
		}
		
		// Protocol: First packet contains Target Parse Logic
		// [AddrLen(2)][Addr]
		addrLenBuf := make([]byte, 2)
		if _, err := io.ReadFull(reader, addrLenBuf); err != nil {
			s.mu.Unlock()
			return // Malformed or empty
		}
		addrLen := int(addrLenBuf[0])<<8 | int(addrLenBuf[1])
		
		addrBuf := make([]byte, addrLen)
		if _, err := io.ReadFull(reader, addrBuf); err != nil {
			s.mu.Unlock()
			return
		}
		targetAddr := string(addrBuf)
		
		fmt.Printf("Dialing target: %s\n", targetAddr)
		
		conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
		if err != nil {
			s.mu.Unlock()
			http.Error(w, fmt.Sprintf("dial failed: %v", err), http.StatusBadGateway)
			return
		}
		s.targetConn = conn
	}
	conn := s.targetConn
	s.mu.Unlock()

	// 4. Copy remaining data to target
	if _, err := io.Copy(conn, reader); err != nil {
		// Log error?
	}
	
	// Acknowledge success
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request, s *Session) {
	// Long Polling
	// Wait for data on s.targetConn
	
	s.mu.Lock()
	conn := s.targetConn
	if s.closed {
		s.mu.Unlock()
		http.Error(w, "session closed", http.StatusGone)
		return
	}
	s.mu.Unlock()

	// If connection not established yet, wait a bit then return empty (client retries)
	if conn == nil {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// We can't easily "Peek" net.Conn to see if data is ready without blocking.
	// But we can just try to Read.
	// If we block too long, the HTTP request might timeout on client/CDN side.
	// So we set a read deadline.
	
	// Setup Header
	w.Header().Set("Content-Type", "application/octet-stream")
	
	// Generate IV for this chunk
	iv := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		http.Error(w, "internal iv error", http.StatusInternalServerError)
		return
	}
	// Write IV
	if _, err := w.Write(iv); err != nil {
		return
	}
	
	writer, err := crypto.NewCryptoWriter(w, h.Config.Secret, iv)
	if err != nil {
		return
	}

	// Buffer for this chunk
	// Read larger chunks to improve downstream throughput
	buf := make([]byte, 256*1024)
	
	// Set Deadline for "Long Poll" behavior
	// e.g. Wait up to 15 seconds for first byte.
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	
	n, err := conn.Read(buf)
	if err != nil {
		// If timeout, we just return empty (well, IV only) so client keeps polling.
		// If EOF, connection closed.
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// Just return what we have (nothing)
			return
		}
		if err == io.EOF {
			// Signal EOF?
			// The protocol over HTTP is just "data". 
			// Client sees empty body (only IV) and knows nothing came. 
			// If real EOF, we might want to signal it explicitly?
			// For now, if server closes connection, next polling calls will fail or return empty?
			// Use StatusGone?
			return
		}
		return
	}
	
	// We got at least 1 byte.
	// Try to read more if available immediately?
	// conn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	// ... logic to fill buffer ...
	// Simple MVP: Just send what we got.
	
	writer.Write(buf[:n])
}
