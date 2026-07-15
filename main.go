package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := LoadFile(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logWriter, err := NewDailyLogWriter(cfg.LogDir, cfg.LogMaxDays)
	if err != nil {
		log.Fatalf("log error: %v", err)
	}
	defer logWriter.Close()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(os.Stdout, logWriter))

	handler := NewHandler(cfg)
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.ServerIdleTimeout,
	}

	log.Printf("langfuse-write-proxy listening on %s, projects=%d log_dir=%s log_max_days=%d", cfg.ListenAddr, len(cfg.Projects), cfg.LogDir, cfg.LogMaxDays)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
