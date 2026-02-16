package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Addresses []string `json:"addressess"`
	Host      string   `json:"host"`
	TLS       bool     `json:"tls"`
	SNI       string   `json:"sni"`
	Port      int      `json:"port"`
	ProxyPort int      `json:"proxy_port"`
	Secret    string   `json:"secret"`
}

func (c *Config) UnmarshalJSON(data []byte) error {
	aux := struct {
		AddressesLegacy []string `json:"addressess"`
		AddressesNew    []string `json:"addresses"`
		Host            string   `json:"host"`
		TLS             bool     `json:"tls"`
		SNI             string   `json:"sni"`
		Port            int      `json:"port"`
		ProxyPort       int      `json:"proxy_port"`
		Secret          string   `json:"secret"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Host = aux.Host
	c.TLS = aux.TLS
	c.SNI = aux.SNI
	c.Port = aux.Port
	c.ProxyPort = aux.ProxyPort
	c.Secret = aux.Secret
	if len(aux.AddressesLegacy) > 0 {
		c.Addresses = aux.AddressesLegacy
	} else if len(aux.AddressesNew) > 0 {
		c.Addresses = aux.AddressesNew
	} else {
		c.Addresses = nil
	}
	return nil
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
