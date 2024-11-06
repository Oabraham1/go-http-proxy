package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oabraham1/go-http-proxy/internal/config"
	"github.com/oabraham1/go-http-proxy/internal/proxy"
)

func main() {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         8080,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		Cache: config.CacheConfig{
			Enabled:         true,
			TTL:             5 * time.Minute,
			MaxSize:         1024 * 1024 * 1024, // 1GB
			CleanupInterval: time.Minute,
			Rules: []config.CacheRule{
				{
					Path:    "/images/*",
					TTL:     24 * time.Hour,
					Methods: []string{"GET"},
					MaxSize: 100 * 1024 * 1024, // 100MB
				},
				{
					Path:    "/api/products*",
					TTL:     5 * time.Minute,
					Methods: []string{"GET"},
					Headers: []string{"Accept", "Accept-Language"},
					VaryBy:  []string{"Authorization"},
				},
			},
		},
		Services: map[string]config.ServiceConfig{
			"static": {
				URL: "http://static-content:8001",
				Routes: []config.RouteConfig{
					{
						Path:    "/images/*",
						Methods: []string{"GET"},
						Cache:   true,
						Headers: map[string]string{
							"Cache-Control": "public, max-age=86400",
						},
					},
				},
			},
			"api": {
				URL: "http://api:8002",
				Routes: []config.RouteConfig{
					{
						Path:    "/api/products*",
						Methods: []string{"GET"},
						Cache:   true,
						Headers: map[string]string{
							"Cache-Control": "public, max-age=300",
						},
					},
				},
			},
		},
	}

	// Initialize proxy with caching
	proxy, err := proxy.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// Start cache metrics collection
	go proxy.CollectCacheMetrics()

	// Start proxy
	go func() {
		if err := proxy.Start(); err != nil {
			log.Fatalf("Proxy server failed: %v", err)
		}
	}()

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	proxy.Shutdown()
}
