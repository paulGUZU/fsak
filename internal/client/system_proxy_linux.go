//go:build linux

package client

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// linuxSystemProxySession manages Linux system proxy settings
type linuxSystemProxySession struct {
	enabled  bool
	previous map[string]string // stores previous proxy settings
	mode     string            // "gnome" or "kde"
}

// EnableSystemProxy enables SOCKS proxy on Linux
func EnableSystemProxy(port int) (SystemProxySession, error) {
	// Try GNOME/gsettings first, then KDE
	if isGNOMEDesktop() {
		return enableGNOMEProxy(port)
	}
	if isKDEDesktop() {
		return enableKDEProxy(port)
	}
	
	// Try gsettings anyway as fallback (many desktops support it)
	if hasGSettings() {
		return enableGNOMEProxy(port)
	}
	
	return nil, fmt.Errorf("no supported desktop environment found for system proxy (tried GNOME/gsettings and KDE)")
}

func isGNOMEDesktop() bool {
	de := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	return strings.Contains(de, "gnome") || 
	       strings.Contains(de, "unity") || 
	       strings.Contains(de, "cinnamon") || 
	       strings.Contains(de, "budgie") ||
	       strings.Contains(de, "pantheon")
}

func isKDEDesktop() bool {
	de := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	return strings.Contains(de, "kde") || strings.Contains(de, "plasma")
}

func hasGSettings() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func enableGNOMEProxy(port int) (SystemProxySession, error) {
	session := &linuxSystemProxySession{
		previous: make(map[string]string),
		mode:     "gnome",
	}

	// Save current settings
	settings := []string{
		"org.gnome.system.proxy",
		"org.gnome.system.proxy.socks",
	}
	
	for _, schema := range settings {
		keys := getGSettingsKeys(schema)
		for _, key := range keys {
			val, err := getGSetting(schema, key)
			if err == nil {
				session.previous[schema+"."+key] = val
			}
		}
	}

	// Enable SOCKS proxy
	if err := setGSetting("org.gnome.system.proxy.socks", "host", "127.0.0.1"); err != nil {
		session.Disable()
		return nil, fmt.Errorf("failed to set SOCKS host: %w", err)
	}
	if err := setGSetting("org.gnome.system.proxy.socks", "port", fmt.Sprintf("%d", port)); err != nil {
		session.Disable()
		return nil, fmt.Errorf("failed to set SOCKS port: %w", err)
	}
	if err := setGSetting("org.gnome.system.proxy", "mode", "manual"); err != nil {
		session.Disable()
		return nil, fmt.Errorf("failed to enable manual proxy: %w", err)
	}

	session.enabled = true
	return session, nil
}

func enableKDEProxy(port int) (SystemProxySession, error) {
	session := &linuxSystemProxySession{
		previous: make(map[string]string),
		mode:     "kde",
	}

	// KDE uses kwriteconfig5 or kwriteconfig6
	// Save current config (read from kreadconfig)
	host, _ := getKSetting("Proxy/SOCKS/Proxy", "")
	if host != "" {
		session.previous["socks_proxy"] = host
	}
	
	mode, _ := getKSetting("Proxy/Mode", "")
	session.previous["proxy_mode"] = mode

	// Set SOCKS proxy
	configCmd := "kwriteconfig5"
	if _, err := exec.LookPath("kwriteconfig6"); err == nil {
		configCmd = "kwriteconfig6"
	}

	if err := setKSetting(configCmd, "Proxy/SOCKS/Proxy", fmt.Sprintf("127.0.0.1 %d", port)); err != nil {
		session.Disable()
		return nil, fmt.Errorf("failed to set SOCKS proxy: %w", err)
	}
	if err := setKSetting(configCmd, "Proxy/Mode", "1"); err != nil {
		session.Disable()
		return nil, fmt.Errorf("failed to enable proxy mode: %w", err)
	}

	// Reload KDE settings
	exec.Command("dbus-send", "--type=signal", "/KDE", "org.kde.KSettings", "notifyChange").Run()

	session.enabled = true
	return session, nil
}

func (s *linuxSystemProxySession) Disable() error {
	if !s.enabled {
		return nil
	}

	switch s.mode {
	case "gnome":
		return s.disableGNOME()
	case "kde":
		return s.disableKDE()
	}
	return nil
}

func (s *linuxSystemProxySession) disableGNOME() error {
	var errs []string

	// Restore previous mode
	if mode, ok := s.previous["org.gnome.system.proxy.mode"]; ok {
		if err := setGSetting("org.gnome.system.proxy", "mode", mode); err != nil {
			errs = append(errs, fmt.Sprintf("restore mode: %v", err))
		}
	} else {
		// Default to 'none'
		if err := setGSetting("org.gnome.system.proxy", "mode", "none"); err != nil {
			errs = append(errs, fmt.Sprintf("disable proxy: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to restore GNOME proxy: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *linuxSystemProxySession) disableKDE() error {
	configCmd := "kwriteconfig5"
	if _, err := exec.LookPath("kwriteconfig6"); err == nil {
		configCmd = "kwriteconfig6"
	}

	// Restore previous mode or disable
	mode := "0" // disabled
	if prevMode, ok := s.previous["proxy_mode"]; ok && prevMode != "" {
		mode = prevMode
	}

	if err := setKSetting(configCmd, "Proxy/Mode", mode); err != nil {
		return fmt.Errorf("failed to restore KDE proxy mode: %w", err)
	}

	// Reload KDE settings
	exec.Command("dbus-send", "--type=signal", "/KDE", "org.kde.KSettings", "notifyChange").Run()
	return nil
}

// Helper functions for gsettings
func getGSettingsKeys(schema string) []string {
	out, err := runGSettings("list-keys", schema)
	if err != nil {
		return []string{}
	}
	return strings.Fields(out)
}

func getGSetting(schema, key string) (string, error) {
	return runGSettings("get", schema, key)
}

func setGSetting(schema, key, value string) error {
	_, err := runGSettings("set", schema, key, value)
	return err
}

func runGSettings(args ...string) (string, error) {
	cmd := exec.Command("gsettings", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("gsettings %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Helper functions for KDE
func getKSetting(key, defaultVal string) (string, error) {
	readCmd := "kreadconfig5"
	if _, err := exec.LookPath("kreadconfig6"); err == nil {
		readCmd = "kreadconfig6"
	}

	cmd := exec.Command(readCmd, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", key, "--default", defaultVal)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func setKSetting(configCmd, key, value string) error {
	cmd := exec.Command(configCmd, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", key, value)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
