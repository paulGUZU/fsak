package main

import (
	"flag"
	"log"
	
	"github.com/paulGUZU/fsak/internal/client"
	"github.com/paulGUZU/fsak/pkg/config"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize Address Pool
	pool, err := client.NewAddressPool(cfg.Addresses)
	if err != nil {
		log.Fatalf("Failed to init address pool: %v", err)
	}

	// Initialize Transport
	transport := client.NewTransport(cfg, pool)

	// Initialize SOCKS5 Server
	socks := client.NewSOCKS5Server(cfg.ProxyPort, transport)

	// Start
	log.Printf("Starting SOCKS5 Client on port %d...", cfg.ProxyPort)
	if err := socks.ListenAndServe(); err != nil {
		log.Fatalf("SOCKS5 Server failed: %v", err)
	}
}
