package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := LoadFile(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	handler := NewHandler(cfg)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	log.Printf("langfuse-write-proxy listening on %s, projects=%d", cfg.ListenAddr, len(cfg.Projects))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
