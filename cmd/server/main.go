package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/paulGUZU/fsak/internal/server"
	"github.com/paulGUZU/fsak/pkg/config"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override port if needed or just use logic
	addr := fmt.Sprintf(":%d", cfg.Port)
	if addr == ":0" {
		addr = ":8080"
	}

	handler := server.NewHandler(cfg)

	log.Printf("Server listening on %s", addr)
	
	// If TLS is enabled in config? 
	// The user requirement says "tls: true/false" in JSON.
	// Note: Standard library http server usually needs cert files.
	// But typically proxies might be behind Nginx or self-terminated.
	// The user prompt implies the server ITSELF might handle TLS if specified?
	// Or maybe that's for the Client connecting TO the server?
	// "tls : true/false , sni : if tls is true it must have the sni"
	// This usually refers to the Client Configuration (how client connects to server).
	// But the Server also needs to know if it should serve TLS.
	// Let's assume for Server, if we have certs we serves TLS. 
	// The prompt doesn't specify cert paths in config, just "tls: true".
	// Maybe it assumes auto-cert or files key.pem/cert.pem exist?
	// I'll implement standard HTTP for now, as TLS termination is often external or requires explicit cert paths which are missing from the spec.
	// Wait, the "config" is shared? "client and server must have this options in a json file".
	// If so, the server needs to know what port to listen on.
	// I'll stick to HTTP for the MVP unless user provides certs, 
	// OR I can use `ListenAndServeTLS` if I had paths.
	// Given "host" and "sni" are in config, that strongly implies Client-side settings.
	// For Server, I'll just listen generic HTTP.
	
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
