package models

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/paulGUZU/fsak/pkg/config"
)

// ClientProfile represents a named profile with configuration
type ClientProfile struct {
	Name   string       `json:"name"`
	Config ClientConfig `json:"config"`
}

// ClientConfig holds the client configuration settings
type ClientConfig struct {
	Addresses []string `json:"addresses"`
	Host      string   `json:"host"`
	TLS       bool     `json:"tls"`
	SNI       string   `json:"sni"`
	Port      int      `json:"port"`
	ProxyPort int      `json:"proxy_port"`
	Secret    string   `json:"secret"`
}

// ProfilesStore is the top-level JSON structure for persistence
type ProfilesStore struct {
	Selected string          `json:"selected"`
	Profiles []ClientProfile `json:"profiles"`
}

// Normalize validates and normalizes a ClientConfig
func (c ClientConfig) Normalize() (ClientConfig, error) {
	cfg := c
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.SNI = strings.TrimSpace(cfg.SNI)
	cfg.Secret = strings.TrimSpace(cfg.Secret)

	addrs := make([]string, 0, len(cfg.Addresses))
	for _, addr := range cfg.Addresses {
		trimmed := strings.TrimSpace(addr)
		if trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}
	cfg.Addresses = addrs

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks if the configuration is valid
func (c ClientConfig) Validate() error {
	if len(c.Addresses) == 0 {
		return errors.New("at least one address is required")
	}
	if c.Host == "" {
		return errors.New("host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if c.ProxyPort < 1 || c.ProxyPort > 65535 {
		return errors.New("proxy_port must be between 1 and 65535")
	}
	if c.Secret == "" {
		return errors.New("secret is required")
	}
	if c.TLS && c.SNI == "" {
		return errors.New("sni is required when tls is enabled")
	}
	return nil
}

// ToInternal converts to pkg/config.Config
func (c ClientConfig) ToInternal() config.Config {
	return config.Config{
		Addresses: c.Addresses,
		Host:      c.Host,
		TLS:       c.TLS,
		SNI:       c.SNI,
		Port:      c.Port,
		ProxyPort: c.ProxyPort,
		Secret:    c.Secret,
	}
}

// ClientConfigFromInternal creates ClientConfig from pkg/config.Config
func ClientConfigFromInternal(c config.Config) ClientConfig {
	return ClientConfig{
		Addresses: c.Addresses,
		Host:      c.Host,
		TLS:       c.TLS,
		SNI:       c.SNI,
		Port:      c.Port,
		ProxyPort: c.ProxyPort,
		Secret:    c.Secret,
	}
}

// ParseAddresses parses addresses from multi-line or comma-separated string
func ParseAddresses(input string) []string {
	raw := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	addrs := make([]string, 0, len(raw))
	for _, addr := range raw {
		trimmed := strings.TrimSpace(addr)
		if trimmed != "" {
			addrs = append(addrs, trimmed)
		}
	}
	return addrs
}

// FormatAddresses formats addresses for display (one per line)
func FormatAddresses(addrs []string) string {
	return strings.Join(addrs, "\n")
}

// ParsePort safely parses a port string
func ParsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("port is required")
	}
	port, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("port must be a number")
	}
	return port, nil
}

// SortedProfileNames returns sorted profile names from a map
func SortedProfileNames(profiles map[string]ClientConfig) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SanitizeString trims whitespace from a string
func SanitizeString(s string) string {
	return strings.TrimSpace(s)
}

// MarshalJSON implements custom JSON marshaling for persistence
func (c ClientConfig) MarshalJSON() ([]byte, error) {
	type Alias ClientConfig
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&c),
	})
}
