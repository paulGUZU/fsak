package client

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// SOCKS5 Constants
const (
	verSocks5  = 0x05
	cmdConnect = 0x01
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

type SOCKS5Server struct {
	addr      string
	transport *Transport
	mu        sync.Mutex
	listener  net.Listener
	conns     map[net.Conn]struct{}
	done      chan struct{}
	serveErr  chan error
	wg        sync.WaitGroup
}

func NewSOCKS5Server(port int, t *Transport) *SOCKS5Server {
	return &SOCKS5Server{
		addr:      fmt.Sprintf(":%d", port),
		transport: t,
		conns:     make(map[net.Conn]struct{}),
	}
}

func (s *SOCKS5Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return fmt.Errorf("SOCKS5 server already running")
	}

	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = l
	s.done = make(chan struct{})
	s.serveErr = make(chan error, 1)

	log.Printf("SOCKS5 Proxy listening on %s", s.addr)
	go s.acceptLoop(l, s.done, s.serveErr)
	return nil
}

func (s *SOCKS5Server) ListenAndServe() error {
	if err := s.Start(); err != nil {
		return err
	}
	s.mu.Lock()
	done := s.done
	errCh := s.serveErr
	s.mu.Unlock()

	<-done
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *SOCKS5Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	l := s.listener
	done := s.done
	s.listener = nil
	activeConns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		activeConns = append(activeConns, conn)
	}
	if l == nil && len(activeConns) == 0 {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if l != nil {
		if err := l.Close(); err != nil {
			return err
		}
	}
	for _, conn := range activeConns {
		_ = conn.Close()
	}

	if done == nil {
		return nil
	}

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	waitCh := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SOCKS5Server) acceptLoop(l net.Listener, done chan struct{}, errCh chan error) {
	defer close(done)
	for {
		conn, err := l.Accept()
		if err != nil {
			s.mu.Lock()
			currentListener := s.listener
			s.mu.Unlock()

			if currentListener == nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Printf("Accept temporary failure: %v", err)
				continue
			}
			select {
			case errCh <- err:
			default:
			}
			return
		}
		if !s.trackConn(conn) {
			_ = conn.Close()
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

func (s *SOCKS5Server) trackConn(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return false
	}
	s.conns[conn] = struct{}{}
	return true
}

func (s *SOCKS5Server) untrackConn(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
}

func (s *SOCKS5Server) handleConnection(conn net.Conn) {
	defer s.untrackConn(conn)
	defer conn.Close()

	// 1. Negotiation
	// Client sends: [VER, NMETHODS, METHODS...]
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	if header[0] != verSocks5 {
		return
	}
	numMethods := int(header[1])
	methods := make([]byte, numMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}

	// We only support NO AUTH (0x00)
	// Server responds: [VER, METHOD]
	if _, err := conn.Write([]byte{verSocks5, 0x00}); err != nil {
		return
	}

	// 2. Request
	// Client sends: [VER, CMD, RSV, ATYP, DST.ADDR, DST.PORT]
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return
	}
	// buf[1] is CMD
	if buf[1] != cmdConnect {
		// Reply Command Not Supported
		// ...
		return
	}

	var targetAddr string
	switch buf[3] {
	case atypIPv4:
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		targetAddr = net.IP(ip).String()
	case atypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		l := int(lenBuf[0])
		domain := make([]byte, l)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return
		}
		targetAddr = string(domain)
	case atypIPv6:
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		targetAddr = fmt.Sprintf("[%s]", net.IP(ip).String())
	default:
		return
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	target := fmt.Sprintf("%s:%d", targetAddr, port)

	// 3. Connect to Remote via HTTP Tunnel
	// log.Printf("Connecting to %s", target)

	// Delegate to Transport
	// Transport needs to return an error or nil.
	// We need to send SOCKS reply *before* piping data?
	// Standard SOCKS5:
	// - Client sends Request
	// - Server attempts connection
	// - Server sends Reply (Success/Fail)
	// - Then transfer begins.

	// BUT, our Transport establishes the tunnel asynchronously?
	// Actually, the server side dials the target.
	// If server side fails to dial, it writes nothing and closes?
	// OR we assume success and start streaming?
	// If server fails, the stream closes.

	// Let's send Success reply immediately then start tunnel.
	// This makes "connection refused" handling harder (user sees connection closed instead of SOCKS error),
	// but is faster/simpler for this tunnel Architecture.

	// Send Reply: Success
	// [VER, REP(0x00), RSV, ATYP, BND.ADDR, BND.PORT]
	// We just verify success.
	conn.Write([]byte{verSocks5, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	if err := s.transport.Tunnel(target, conn); err != nil {
		log.Printf("Tunnel error: %v", err)
	}
}
