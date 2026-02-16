//go:build !darwin

package client

import "syscall"

func interfaceDialerControl(interfaceName string) func(network, address string, c syscall.RawConn) error {
	return nil
}
