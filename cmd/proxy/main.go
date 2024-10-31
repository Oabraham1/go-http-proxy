package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/oabraham1/go-http-proxy/internal/config"
	"github.com/oabraham1/go-http-proxy/internal/proxy"
)

func main() {
    configPath := flag.String("config", "config.yaml", "path to config file")
    flag.Parse()

    // Load configuration
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Initialize proxy
    p, err := proxy.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create proxy: %v", err)
    }

    // Start proxy
    go func() {
        if err := p.Start(); err != nil {
            log.Fatalf("Proxy server failed: %v", err)
        }
    }()

    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    // Graceful shutdown
    p.Shutdown()
}
