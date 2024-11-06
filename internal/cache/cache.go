package cache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Cache struct {
	items   sync.Map
	size    atomic.Int64
	maxSize int64
	ttl     time.Duration
}

type cacheItem struct {
	response *http.Response
	body     []byte
	size     int64
	expires  time.Time
	lastUsed time.Time
	hits     atomic.Int64
	mu       sync.RWMutex
}

type Config struct {
	MaxSize int64         // Maximum size in bytes
	TTL     time.Duration // Time to live for cache entries
}

func New(config Config) *Cache {
	cache := &Cache{
		maxSize: config.MaxSize,
		ttl:     config.TTL,
	}

	// Start maintenance routine
	go cache.maintenance()

	return cache
}

func (c *Cache) Set(r *http.Request, resp *http.Response) error {
	// Skip caching if response shouldn't be cached
	if !isCacheable(r, resp) {
		return nil
	}

	// Copy the response
	body, err := copyResponse(resp)
	if err != nil {
		return fmt.Errorf("failed to copy response: %w", err)
	}

	item := &cacheItem{
		response: resp,
		body:     body,
		size:     int64(len(body)),
		expires:  time.Now().Add(c.ttl),
		lastUsed: time.Now(),
	}

	// Check if adding this item would exceed max size
	newSize := c.size.Load() + item.size
	if c.maxSize > 0 && newSize > c.maxSize {
		// Try to free up space
		c.evict(item.size)

		// Check again after eviction
		if c.size.Load()+item.size > c.maxSize {
			return fmt.Errorf("cache full: cannot store item of size %d", item.size)
		}
	}

	key := generateKey(r)
	c.items.Store(key, item)
	c.size.Add(item.size)

	return nil
}

func (c *Cache) Get(r *http.Request) (*http.Response, bool) {
	key := generateKey(r)
	value, ok := c.items.Load(key)
	if !ok {
		return nil, false
	}

	item := value.(*cacheItem)

	item.mu.RLock()
	defer item.mu.RUnlock()

	// Check if expired
	if time.Now().After(item.expires) {
		c.items.Delete(key)
		c.size.Add(-item.size)
		return nil, false
	}

	// Update stats
	item.hits.Add(1)
	item.lastUsed = time.Now()

	// Return a copy of the response
	return copyResponseWithBody(item.response, item.body), true
}

func (c *Cache) maintenance() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.evictExpired()
	}
}

func (c *Cache) evictExpired() {
	now := time.Now()
	var keysToEvict []string

	c.items.Range(func(key, value interface{}) bool {
		item := value.(*cacheItem)
		item.mu.RLock()
		expired := now.After(item.expires)
		item.mu.RUnlock()

		if expired {
			keysToEvict = append(keysToEvict, key.(string))
		}
		return true
	})

	for _, key := range keysToEvict {
		if item, loaded := c.items.LoadAndDelete(key); loaded {
			c.size.Add(-item.(*cacheItem).size)
		}
	}
}

func (c *Cache) evict(needed int64) {
	type evictionCandidate struct {
		key   string
		item  *cacheItem
		score float64
	}

	var candidates []evictionCandidate

	// Collect candidates
	c.items.Range(func(key, value interface{}) bool {
		item := value.(*cacheItem)
		score := float64(time.Since(item.lastUsed).Seconds()) / float64(item.hits.Load()+1)
		candidates = append(candidates, evictionCandidate{
			key:   key.(string),
			item:  item,
			score: score,
		})
		return true
	})

	// Sort by score (higher score = better eviction candidate)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Evict until we have enough space
	spaceFreed := int64(0)
	for _, candidate := range candidates {
		if spaceFreed >= needed {
			break
		}
		if _, loaded := c.items.LoadAndDelete(candidate.key); loaded {
			spaceFreed += candidate.item.size
			c.size.Add(-candidate.item.size)
		}
	}
}

func copyResponse(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("invalid response or body")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	// Create new body reader for original response
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	return body, nil
}

func copyResponseWithBody(resp *http.Response, body []byte) *http.Response {
	newResp := &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        make(http.Header),
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewBuffer(body)),
		Request:       resp.Request,
	}

	// Copy headers
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}

	return newResp
}

func generateKey(r *http.Request) string {
	return r.Method + r.URL.String()
}

func isCacheable(r *http.Request, resp *http.Response) bool {
	// Only cache GET requests
	if r.Method != http.MethodGet {
		return false
	}

	// Check response code
	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Check cache control headers
	if resp.Header.Get("Cache-Control") == "no-store" {
		return false
	}

	return true
}
