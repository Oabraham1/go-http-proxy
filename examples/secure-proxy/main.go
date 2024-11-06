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
			Port: 8443,
			TLS: config.TLSConfig{
				Enabled:    true,
				Cert:       "/certs/server.crt",
				Key:        "/certs/server.key",
				MinVersion: "1.2",
			},
		},
		Security: config.SecurityConfig{
			Headers: config.SecurityHeaders{
				Enabled: true,
				CSP:     "default-src 'self'; script-src 'self' 'unsafe-inline'",
				HSTS:    "max-age=31536000; includeSubDomains; preload",
			},
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				Rate:    10,
				Burst:   20,
				By:      "ip",
			},
			Auth: config.AuthConfig{
				Type: "jwt",
				JWT: config.JWTConfig{
					JWKSUrl:  "https://auth.example.com/.well-known/jwks.json",
					Audience: "https://api.example.com",
					Issuer:   "https://auth.example.com/",
				},
			},
			IPWhitelist: []string{
				"10.0.0.0/8",
				"172.16.0.0/12",
			},
		},
		Services: map[string]config.ServiceConfig{
			"api": {
				URL: "http://internal-api:8001",
				Security: config.ServiceSecurityConfig{
					RequiredScopes: []string{"read:users", "write:users"},
					IPWhitelist:    []string{"10.0.0.0/8"},
				},
				Routes: []config.RouteConfig{
					{
						Path:    "/api/users",
						Methods: []string{"GET", "POST"},
						Auth:    true,
						RateLimit: &config.RateLimitConfig{
							Rate:  5,
							Burst: 10,
						},
					},
				},
			},
		},
	}

	// Initialize secure proxy
	proxy, err := proxy.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// Start security audit logging
	go proxy.StartAuditLog()

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
