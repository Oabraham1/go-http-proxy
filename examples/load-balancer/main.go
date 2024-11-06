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
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		LoadBalancer: config.LoadBalancerConfig{
			Algorithm: "round-robin",
			Sticky:    true,
		},
		HealthCheck: config.HealthCheckConfig{
			Interval:           10,
			Timeout:            2,
			UnhealthyThreshold: 3,
			HealthyThreshold:   2,
		},
		Services: map[string]config.ServiceConfig{
			"web": {
				Backends: []config.BackendConfig{
					{
						URL:    "http://web1:8001",
						Weight: 100,
					},
					{
						URL:    "http://web2:8001",
						Weight: 100,
					},
					{
						URL:    "http://web3:8001",
						Weight: 50,
					},
				},
				HealthCheck: &config.HealthCheckConfig{
					Path:     "/health",
					Interval: 5,
					Headers: map[string]string{
						"X-Health-Check": "true",
					},
				},
			},
		},
	}

	// Initialize proxy with load balancer
	proxy, err := proxy.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// Start metrics server for monitoring
	go func() {
		if err := proxy.StartMetricsServer(":9090"); err != nil {
			log.Printf("Metrics server failed: %v", err)
		}
	}()

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
