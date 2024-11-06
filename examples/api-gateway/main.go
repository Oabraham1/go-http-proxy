package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/oabraham1/go-http-proxy/internal/config"
	"github.com/oabraham1/go-http-proxy/internal/proxy"
)

func main() {
	// Load configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Cache: config.CacheConfig{
			Enabled: true,
			TTL:     300,
		},
		Security: config.SecurityConfig{
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				Rate:    100,
				Burst:   20,
			},
			Headers: config.SecurityHeaders{
				Enabled: true,
			},
		},
		Services: map[string]config.ServiceConfig{
			"users": {
				URL: "http://users-service:8001",
				Routes: []config.RouteConfig{
					{
						Path:    "/api/users",
						Methods: []string{"GET", "POST"},
						Auth:    true,
					},
					{
						Path:    "/api/users/{id}",
						Methods: []string{"GET", "PUT", "DELETE"},
						Auth:    true,
					},
				},
			},
			"products": {
				URL: "http://products-service:8002",
				Routes: []config.RouteConfig{
					{
						Path:    "/api/products",
						Methods: []string{"GET"},
						Cache:   true,
					},
					{
						Path:    "/api/products/{id}",
						Methods: []string{"GET"},
						Cache:   true,
					},
				},
			},
		},
	}

	// Initialize proxy
	proxy, err := proxy.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// Start proxy in background
	go func() {
		if err := proxy.Start(); err != nil {
			log.Fatalf("Proxy server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Graceful shutdown
	proxy.Shutdown()
}
