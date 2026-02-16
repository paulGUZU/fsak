//go:build darwin

package client

import (
	"net"
	"strings"
	"syscall"
)

func interfaceDialerControl(interfaceName string) func(network, address string, c syscall.RawConn) error {
	interfaceName = strings.TrimSpace(interfaceName)
	if interfaceName == "" {
		return nil
	}

	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return func(string, string, syscall.RawConn) error {
			return err
		}
	}
	index := iface.Index

	return func(network, address string, c syscall.RawConn) error {
		var controlErr error
		if err := c.Control(func(fd uintptr) {
			err4 := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_BOUND_IF, index)
			err6 := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_BOUND_IF, index)
			if err4 != nil && err6 != nil {
				controlErr = err4
			}
		}); err != nil {
			return err
		}
		return controlErr
	}
}
