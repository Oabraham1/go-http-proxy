package cache

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
    cache := New(time.Second)

    tests := []struct {
        name       string
        method     string
        url        string
        cacheHit   bool
        setupCache bool
    }{
        {
            name:       "cache miss on first request",
            method:     "GET",
            url:        "/test",
            cacheHit:   false,
            setupCache: false,
        },
        {
            name:       "cache hit on second request",
            method:     "GET",
            url:        "/test",
            cacheHit:   true,
            setupCache: true,
        },
        {
            name:       "no cache for POST",
            method:     "POST",
            url:        "/test",
            cacheHit:   false,
            setupCache: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup test request
            req := httptest.NewRequest(tt.method, tt.url, nil)

            if tt.setupCache {
                // Store a response in cache
                resp := &http.Response{
                    StatusCode: http.StatusOK,
                    Header:     make(http.Header),
                }
                err := cache.Set(req, resp)
                if err != nil {
                    t.Fatalf("failed to setup cache: %v", err)
                }
            }

            // Try to get from cache
            resp, hit := cache.Get(req)
            if hit != tt.cacheHit {
                t.Errorf("got cache hit = %v, want %v", hit, tt.cacheHit)
            }

            if hit && resp == nil {
                t.Error("got nil response on cache hit")
            }
        })
    }
}

func TestCacheExpiration(t *testing.T) {
    cache := New(100 * time.Millisecond)
    req := httptest.NewRequest("GET", "/test", nil)

    // Store in cache
    resp := &http.Response{
        StatusCode: http.StatusOK,
        Header:     make(http.Header),
    }
    err := cache.Set(req, resp)
    if err != nil {
        t.Fatalf("failed to set cache: %v", err)
    }

    // Verify it's in cache
    if _, hit := cache.Get(req); !hit {
        t.Error("expected cache hit immediately after set")
    }

    // Wait for expiration
    time.Sleep(200 * time.Millisecond)

    // Verify it's expired
    if _, hit := cache.Get(req); hit {
        t.Error("expected cache miss after expiration")
    }
}
