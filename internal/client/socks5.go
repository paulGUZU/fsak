package client

import (
	"fmt"
	"io"
	"log"
	"net"
	"encoding/binary"
)

// SOCKS5 Constants
const (
	verSocks5 = 0x05
	cmdConnect = 0x01
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

type SOCKS5Server struct {
	addr string
	transport *Transport
}

func NewSOCKS5Server(port int, t *Transport) *SOCKS5Server {
	return &SOCKS5Server{
		addr: fmt.Sprintf(":%d", port),
		transport: t,
	}
}

func (s *SOCKS5Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	log.Printf("SOCKS5 Proxy listening on %s", s.addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Accept failed: %v", err)
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *SOCKS5Server) handleConnection(conn net.Conn) {
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
