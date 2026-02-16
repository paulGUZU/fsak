//go:build !darwin

package client

import "fmt"

func EnableSystemProxy(port int) (SystemProxySession, error) {
	return nil, fmt.Errorf("TUN mode currently supports macOS only")
}
