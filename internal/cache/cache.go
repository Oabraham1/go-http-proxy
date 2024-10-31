package cache

import (
	"net/http"
	"sync"
	"time"
)

type Cache struct {
    items sync.Map
    ttl   time.Duration
}

type cacheItem struct {
    response *http.Response
    body     []byte
    expires  time.Time
}

func New(ttl time.Duration) *Cache {
    cache := &Cache{ttl: ttl}
    go cache.startCleanup()
    return cache
}

func (c *Cache) Set(r *http.Request, resp *http.Response) error {
    // Check if response is cacheable
    if !isCacheable(r, resp) {
        return nil
    }

    // Read and store response body
    body, err := copyResponse(resp)
    if err != nil {
        return err
    }

    key := generateKey(r)
    c.items.Store(key, &cacheItem{
        response: resp,
        body:     body,
        expires:  time.Now().Add(c.ttl),
    })

    return nil
}

func (c *Cache) Get(r *http.Request) (*http.Response, bool) {
    if !isCacheable(r, nil) {
        return nil, false
    }

    key := generateKey(r)
    item, exists := c.items.Load(key)
    if !exists {
        return nil, false
    }

    ci := item.(*cacheItem)
    if time.Now().After(ci.expires) {
        c.items.Delete(key)
        return nil, false
    }

    // Create new response from cached item
    return copyResponseWithBody(ci.response, ci.body), true
}

func (c *Cache) startCleanup() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        now := time.Now()
        c.items.Range(func(key, value interface{}) bool {
            item := value.(*cacheItem)
            if now.After(item.expires) {
                c.items.Delete(key)
            }
            return true
        })
    }
}

// Helper functions for cache operations
func isCacheable(r *http.Request, resp *http.Response) bool {
    // Only cache GET requests
    if r.Method != http.MethodGet {
        return false
    }

    // Check cache control headers
    if resp != nil {
        if resp.Header.Get("Cache-Control") == "no-store" {
            return false
        }
    }

    return true
}

func generateKey(r *http.Request) string {
    return r.Method + r.URL.String()
}

func copyResponse(resp *http.Response) ([]byte, error) {
    // Implementation of response copying
    // This would read the response body and create a copy
    return nil, nil
}

func copyResponseWithBody(resp *http.Response, body []byte) *http.Response {
    // Implementation of response recreation with cached body
    return nil
}
