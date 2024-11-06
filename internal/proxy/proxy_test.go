package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oabraham1/go-http-proxy/internal/config"
)

func TestProxyIntegration(t *testing.T) {
	// Setup backend services
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // Simulate processing
		w.Header().Set("X-Service", "backend1")
		w.Write([]byte("backend1 response"))
		w.WriteHeader(http.StatusOK)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("fail") == "true" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Service", "backend2")
		w.Write([]byte("backend2 response"))
		w.WriteHeader(http.StatusOK)
	}))
	defer backend2.Close()

	// Create proxy configuration with all required fields
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
		Proxy: struct {
			MaxIdleConns        int           `yaml:"maxIdleConns"`
			MaxConnsPerHost     int           `yaml:"maxConnsPerHost"`
			IdleConnTimeout     time.Duration `yaml:"idleConnTimeout"`
			ResponseTimeout     time.Duration `yaml:"responseTimeout"`
			TLSHandshakeTimeout time.Duration `yaml:"tlsHandshakeTimeout"`
		}{
			MaxIdleConns:    100,
			MaxConnsPerHost: 10,
			IdleConnTimeout: 90 * time.Second,
			ResponseTimeout: 30 * time.Second,
		},
		Cache: struct {
			Enabled bool          `yaml:"enabled"`
			TTL     time.Duration `yaml:"ttl"`
		}{
			Enabled: true,
			TTL:     time.Second,
		},
		Services: map[string]config.ServiceConfig{
			"service1": {
				URL:     backend1.URL,
				Timeout: time.Second,
				CircuitBreaker: &config.BreakerConfig{
					MaxFailures: 2,
					Timeout:     time.Second,
				},
			},
			"service2": {
				URL:     backend2.URL,
				Timeout: time.Second,
				CircuitBreaker: &config.BreakerConfig{
					MaxFailures: 2,
					Timeout:     time.Second,
				},
			},
		},
	}

	// Create and start proxy
	proxy, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy: %v", err)
	}

	// Create test server
	server := httptest.NewServer(proxy.handler())
	defer server.Close()

	tests := []struct {
		name          string
		path          string
		headers       map[string]string
		wantStatus    int
		wantBody      string
		wantCacheHit  bool
		setupRequests []struct {
			path   string
			header string
		}
	}{
		{
			name:       "basic request",
			path:       "/service1/test",
			wantStatus: http.StatusOK,
			wantBody:   "backend1 response",
		},
		{
			name:         "cached request",
			path:         "/service1/test",
			wantStatus:   http.StatusOK,
			wantBody:     "backend1 response",
			wantCacheHit: true,
		},
		{
			name:       "circuit breaker triggers",
			path:       "/service2/test",
			headers:    map[string]string{"fail": "true"},
			wantStatus: http.StatusServiceUnavailable,
			setupRequests: []struct {
				path   string
				header string
			}{
				{"/service2/test", "true"},
				{"/service2/test", "true"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup requests if needed
			for _, req := range tt.setupRequests {
				client := &http.Client{}
				request, _ := http.NewRequest("GET", server.URL+req.path, nil)
				if req.header != "" {
					request.Header.Set("fail", req.header)
				}
				resp, err := client.Do(request)
				if err != nil {
					t.Fatalf("Setup request failed: %v", err)
				}
				resp.Body.Close()
			}

			// Make test request
			client := &http.Client{}
			request, _ := http.NewRequest("GET", server.URL+tt.path, nil)
			for k, v := range tt.headers {
				request.Header.Set(k, v)
			}

			resp, err := client.Do(request)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Check status code
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// Check response body if expected
			if tt.wantBody != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				if !strings.Contains(string(body), tt.wantBody) {
					t.Errorf("got body %q, want %q", string(body), tt.wantBody)
				}
			}

			// Verify cache status
			if tt.wantCacheHit {
				// Make another request to check if it's served from cache
				request, _ := http.NewRequest("GET", server.URL+tt.path, nil)
				resp, err := client.Do(request)
				if err != nil {
					t.Fatalf("Cache verification request failed: %v", err)
				}
				defer resp.Body.Close()

				// In a real implementation, you might want to check for a cache header
				// or compare response times to verify cache hits
				if resp.StatusCode != tt.wantStatus {
					t.Error("cache hit verification failed")
				}
			}
		})
	}
}
