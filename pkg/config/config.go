package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Addresses []string `json:"addressess"` // Note: keeping typo from user req for consistency with their json, or should I fix? I'll stick to user's "addressess" key but maybe clean field name.
	Host      string   `json:"host"`
	TLS       bool     `json:"tls"`
	SNI       string   `json:"sni"`
	Port      int      `json:"port"`
	ProxyPort int      `json:"proxy_port"`
	Secret    string   `json:"secret"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
