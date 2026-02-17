//go:build windows

package client

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const (
	// Internet Settings registry path
	internetSettingsPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	
	// Registry value names
	proxyEnableKey  = "ProxyEnable"
	proxyServerKey  = "ProxyServer"
	proxyOverrideKey = "ProxyOverride"
)

// windowsSystemProxySession manages Windows system proxy settings
type windowsSystemProxySession struct {
	enabled           bool
	previousEnable    uint32
	previousServer    string
	previousOverride  string
}

// EnableSystemProxy enables SOCKS proxy on Windows
func EnableSystemProxy(port int) (SystemProxySession, error) {
	session := &windowsSystemProxySession{}

	// Open Internet Settings registry key
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	// Save current settings
	if val, _, err := key.GetIntegerValue(proxyEnableKey); err == nil {
		session.previousEnable = uint32(val)
	}
	if val, _, err := key.GetStringValue(proxyServerKey); err == nil {
		session.previousServer = val
	}
	if val, _, err := key.GetStringValue(proxyOverrideKey); err == nil {
		session.previousOverride = val
	}

	// Set SOCKS proxy (format: socks=host:port)
	proxyServer := fmt.Sprintf("socks=127.0.0.1:%d", port)
	
	if err := key.SetDWordValue(proxyEnableKey, 1); err != nil {
		return nil, fmt.Errorf("failed to enable proxy: %w", err)
	}
	if err := key.SetStringValue(proxyServerKey, proxyServer); err != nil {
		// Try to rollback
		key.SetDWordValue(proxyEnableKey, uint32(session.previousEnable))
		return nil, fmt.Errorf("failed to set proxy server: %w", err)
	}
	// Set bypass list (optional - bypass proxy for local addresses)
	if err := key.SetStringValue(proxyOverrideKey, "<local>"); err != nil {
		// Non-critical, continue
	}

	session.enabled = true

	// Notify Windows that proxy settings have changed
	refreshInternetSettings()

	return session, nil
}

func (s *windowsSystemProxySession) Disable() error {
	if !s.enabled {
		return nil
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	var errs []string

	// Restore previous settings
	if s.previousEnable == 0 {
		if err := key.DeleteValue(proxyEnableKey); err != nil {
			errs = append(errs, fmt.Sprintf("disable proxy: %v", err))
		}
	} else {
		if err := key.SetDWordValue(proxyEnableKey, uint32(s.previousEnable)); err != nil {
			errs = append(errs, fmt.Sprintf("restore proxy enable: %v", err))
		}
	}

	if s.previousServer == "" {
		if err := key.DeleteValue(proxyServerKey); err != nil {
			errs = append(errs, fmt.Sprintf("delete proxy server: %v", err))
		}
	} else {
		if err := key.SetStringValue(proxyServerKey, s.previousServer); err != nil {
			errs = append(errs, fmt.Sprintf("restore proxy server: %v", err))
		}
	}

	if s.previousOverride == "" {
		key.DeleteValue(proxyOverrideKey)
	} else {
		key.SetStringValue(proxyOverrideKey, s.previousOverride)
	}

	// Notify Windows
	refreshInternetSettings()

	if len(errs) > 0 {
		return fmt.Errorf("failed to restore some proxy settings: %s", errs[0])
	}
	return nil
}

// refreshInternetSettings notifies Windows that internet settings have changed
func refreshInternetSettings() {
	// Load wininet.dll
	wininet := syscall.NewLazyDLL("wininet.dll")
	internetSetOption := wininet.NewProc("InternetSetOptionW")

	// Constants from wininet.h
	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)

	// Notify settings changed
	internetSetOption.Call(
		0,
		uintptr(INTERNET_OPTION_SETTINGS_CHANGED),
		0,
		0,
	)

	// Refresh
	internetSetOption.Call(
		0,
		uintptr(INTERNET_OPTION_REFRESH),
		0,
		0,
	)
}


