package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/paulGUZU/fsak/internal/server"
	"github.com/paulGUZU/fsak/pkg/banner"
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

	// Banner
	banner.Print("SERVER")
	banner.PrintServerStatus(addr)
	
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
