package cache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		setup    func(*testing.T, *Cache)
		validate func(*testing.T, *Cache)
	}{
		{
			name: "basic set and get",
			config: Config{
				MaxSize: 1024 * 1024,
				TTL:     time.Minute,
			},
			setup: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("GET", "/test", nil)
				resp := createTestResponse(200, "test data")

				err := c.Set(req, resp)
				if err != nil {
					t.Fatalf("failed to set cache: %v", err)
				}
			},
			validate: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("GET", "/test", nil)
				resp, ok := c.Get(req)
				if !ok {
					t.Error("expected cache hit")
				}

				body, _ := io.ReadAll(resp.Body)
				if string(body) != "test data" {
					t.Errorf("got body %q, want %q", string(body), "test data")
				}
			},
		},
		{
			name: "respect max size",
			config: Config{
				MaxSize: 50, // Very small size to force eviction
				TTL:     time.Minute,
			},
			setup: func(t *testing.T, c *Cache) {
				// Add item larger than cache size
				req := httptest.NewRequest("GET", "/large", nil)
				resp := createTestResponse(200, strings.Repeat("x", 100))

				err := c.Set(req, resp)
				if err == nil {
					t.Error("expected error when setting item larger than cache size")
				}
			},
			validate: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("GET", "/large", nil)
				if _, ok := c.Get(req); ok {
					t.Error("expected cache miss for large item")
				}
			},
		},
		{
			name: "TTL expiration",
			config: Config{
				MaxSize: 1024,
				TTL:     100 * time.Millisecond,
			},
			setup: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("GET", "/test", nil)
				resp := createTestResponse(200, "test data")

				err := c.Set(req, resp)
				if err != nil {
					t.Fatalf("failed to set cache: %v", err)
				}

				time.Sleep(200 * time.Millisecond) // Wait for expiration
			},
			validate: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("GET", "/test", nil)
				if _, ok := c.Get(req); ok {
					t.Error("expected cache miss after TTL expiration")
				}
			},
		},
		{
			name: "non-cacheable methods",
			config: Config{
				MaxSize: 1024,
				TTL:     time.Minute,
			},
			setup: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("POST", "/test", nil)
				resp := createTestResponse(200, "test data")

				err := c.Set(req, resp)
				if err != nil {
					t.Fatalf("failed to set cache: %v", err)
				}
			},
			validate: func(t *testing.T, c *Cache) {
				req := httptest.NewRequest("POST", "/test", nil)
				if _, ok := c.Get(req); ok {
					t.Error("expected cache miss for POST request")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := New(tt.config)

			if tt.setup != nil {
				tt.setup(t, cache)
			}

			if tt.validate != nil {
				tt.validate(t, cache)
			}
		})
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := New(Config{
		MaxSize: 1024 * 1024,
		TTL:     time.Minute,
	})

	const goroutines = 10
	const requestsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < requestsPerGoroutine; j++ {
				key := fmt.Sprintf("/test-%d-%d", id, j)
				req := httptest.NewRequest("GET", key, nil)
				resp := createTestResponse(200, fmt.Sprintf("data-%d-%d", id, j))

				// Test Set
				err := cache.Set(req, resp)
				if err != nil {
					t.Errorf("failed to set cache: %v", err)
					return
				}

				// Test Get
				cached, ok := cache.Get(req)
				if !ok {
					t.Errorf("cache miss for key %s", key)
					return
				}

				body, _ := io.ReadAll(cached.Body)
				expected := fmt.Sprintf("data-%d-%d", id, j)
				if string(body) != expected {
					t.Errorf("got body %q, want %q", string(body), expected)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheEviction(t *testing.T) {
	cache := New(Config{
		MaxSize: 100, // Small size to force eviction
		TTL:     time.Minute,
	})

	// Fill cache with items
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", fmt.Sprintf("/test-%d", i), nil)
		resp := createTestResponse(200, "test data")
		cache.Set(req, resp)
	}

	// Verify size management
	if cache.size.Load() > cache.maxSize {
		t.Errorf("cache size %d exceeds max size %d", cache.size.Load(), cache.maxSize)
	}
}

// Helper function to create test responses
func createTestResponse(status int, body string) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode: status,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestResponseCopy(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
		want string
	}{
		{
			name: "simple response",
			resp: createTestResponse(200, "test data"),
			want: "test data",
		},
		{
			name: "empty body",
			resp: createTestResponse(200, ""),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := copyResponse(tt.resp)
			if err != nil {
				t.Fatalf("copyResponse failed: %v", err)
			}

			if string(body) != tt.want {
				t.Errorf("got body %q, want %q", string(body), tt.want)
			}

			// Check that original response body is still readable
			origBody, err := io.ReadAll(tt.resp.Body)
			if err != nil {
				t.Fatalf("failed to read original body: %v", err)
			}
			if string(origBody) != tt.want {
				t.Errorf("original body %q, want %q", string(origBody), tt.want)
			}
		})
	}
}

// Mock closer for testing response body closure
type mockCloser struct {
	*bytes.Buffer
	onClose func() error
}

func (m *mockCloser) Close() error {
	return m.onClose()
}
