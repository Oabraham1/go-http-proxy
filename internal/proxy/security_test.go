package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/oabraham1/go-http-proxy/internal/config"
)

// Helper function to setup a secure proxy for testing
func setupSecureProxy(securityConfig config.SecurityConfig) *Proxy {
	cfg := &config.Config{
		Server: struct {
			Port           int           `yaml:"port"`
			ReadTimeout    time.Duration `yaml:"readTimeout"`
			WriteTimeout   time.Duration `yaml:"writeTimeout"`
			MaxHeaderBytes int           `yaml:"maxHeaderBytes"`
		}{
			Port:           8080,
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		Services: map[string]config.ServiceConfig{
			"test": {
				URL: "http://localhost:8081",
			},
		},
	}

	// Add security configuration
	cfg.Security = securityConfig

	proxy, err := New(cfg)
	if err != nil {
		panic(fmt.Sprintf("Failed to create proxy: %v", err))
	}

	return proxy
}

// Helper function to generate valid JWT token
func generateValidJWT(t *testing.T, secret string) string {
	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = jwt.MapClaims{
		"sub": "1234567890",
		"exp": time.Now().Add(time.Hour).Unix(),
	}

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	return tokenString
}

// Helper function to generate expired JWT token
func generateExpiredJWT(t *testing.T, secret string) string {
	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = jwt.MapClaims{
		"sub": "1234567890",
		"exp": time.Now().Add(-time.Hour).Unix(),
	}

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	return tokenString
}

func TestSecurityHeaders(t *testing.T) {
	tests := []struct {
		name        string
		config      config.SecurityConfig
		wantHeaders map[string]string
	}{
		{
			name: "default security headers",
			config: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					Enabled: true,
				},
			},
			wantHeaders: map[string]string{
				"X-Frame-Options":           "DENY",
				"X-Content-Type-Options":    "nosniff",
				"X-XSS-Protection":          "1; mode=block",
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
			},
		},
		{
			name: "custom CSP header",
			config: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					Enabled: true,
					CSP:     "default-src 'self'; script-src 'self' 'unsafe-inline'",
				},
			},
			wantHeaders: map[string]string{
				"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := setupSecureProxy(tt.config)
			server := httptest.NewServer(proxy.handler())
			defer server.Close()

			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			for header, want := range tt.wantHeaders {
				if got := resp.Header.Get(header); got != want {
					t.Errorf("header %s = %q; want %q", header, got, want)
				}
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	tests := []struct {
		name        string
		config      config.SecurityConfig
		wantHeaders map[string]string
	}{
		{
			name: "default security headers",
			config: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					Enabled: true,
				},
			},
			wantHeaders: map[string]string{
				"X-Frame-Options":           "DENY",
				"X-Content-Type-Options":    "nosniff",
				"X-XSS-Protection":          "1; mode=block",
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
			},
		},
		{
			name: "custom CSP header",
			config: config.SecurityConfig{
				Headers: config.SecurityHeaders{
					Enabled: true,
					CSP:     "default-src 'self'; script-src 'self' 'unsafe-inline'",
				},
			},
			wantHeaders: map[string]string{
				"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := setupSecureProxy(tt.config)
			server := httptest.NewServer(proxy.handler())
			defer server.Close()

			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			for header, want := range tt.wantHeaders {
				if got := resp.Header.Get(header); got != want {
					t.Errorf("header %s = %q; want %q", header, got, want)
				}
			}
		})
	}
}

func TestRateLimiting(t *testing.T) {
	config := config.SecurityConfig{
		RateLimit: config.RateLimitConfig{
			Enabled: true,
			Rate:    2,
			Burst:   1,
		},
	}

	proxy := setupSecureProxy(config)
	server := httptest.NewServer(proxy.handler())
	defer server.Close()

	client := &http.Client{}

	// Should succeed
	for i := 0; i < 2; i++ {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Request %d: got status %d; want %d", i, resp.StatusCode, http.StatusOK)
		}
	}

	// Should be rate limited
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Rate limited request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Rate limit: got status %d; want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
}

func TestJWTAuthentication(t *testing.T) {
	config := config.SecurityConfig{
		Auth: config.AuthConfig{
			Type: "jwt",
			JWT: config.JWTConfig{
				Secret: "test-secret",
			},
		},
	}

	proxy := setupSecureProxy(config)
	server := httptest.NewServer(proxy.handler())
	defer server.Close()

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{
			name:       "no token",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token",
			token:      "invalid.token.here",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token",
			token:      generateValidJWT(t, "test-secret"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "expired token",
			token:      generateExpiredJWT(t, "test-secret"),
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", server.URL, nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got status %d; want %d", resp.StatusCode, tt.wantStatus)
			}
		})
	}
}

func TestTLSConfiguration(t *testing.T) {
	config := config.SecurityConfig{
		TLS: config.TLSConfig{
			MinVersion: "1.2",
			CipherSuites: []string{
				"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
				"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
			},
		},
	}

	proxy := setupSecureProxy(config)
	server := httptest.NewTLSServer(proxy.handler())
	defer server.Close()

	tests := []struct {
		name      string
		tlsConfig *tls.Config
		wantError bool
	}{
		{
			name: "modern TLS config",
			tlsConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			wantError: false,
		},
		{
			name: "old TLS version",
			tlsConfig: &tls.Config{
				MinVersion: tls.VersionTLS10,
				MaxVersion: tls.VersionTLS10,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: tt.tlsConfig,
				},
			}

			_, err := client.Get(server.URL)
			if (err != nil) != tt.wantError {
				t.Errorf("got error %v; wantError %v", err, tt.wantError)
			}
		})
	}
}
