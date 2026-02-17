//go:build !darwin && !linux && !windows

package client

import "fmt"

// EnableSystemProxy returns an error on unsupported platforms
func EnableSystemProxy(port int) (SystemProxySession, error) {
	return nil, fmt.Errorf("system proxy is not supported on this platform")
}
