package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Status struct {
	Healthy   bool      `json:"healthy"`
	LastCheck time.Time `json:"lastCheck"`
	Message   string    `json:"message,omitempty"`
}

type Metrics struct {
	healthyServices   int64
	unhealthyServices int64
	totalChecks       int64
	lastCheckDuration time.Duration
	mu                sync.RWMutex
}

type Checker struct {
	services map[string]string
	status   sync.Map
	client   *http.Client
	interval time.Duration
	metrics  *Metrics
	stopCh   chan struct{}
}

func NewChecker(services map[string]string, interval time.Duration) *Checker {
	return &Checker{
		services: services,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				DisableKeepAlives: true,
				// Add timeouts to the transport level
				ResponseHeaderTimeout: 2 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				// Limit max idle connections
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
			},
		},
		interval: interval,
		metrics:  &Metrics{},
		stopCh:   make(chan struct{}),
	}
}

func (c *Checker) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.checkServices()
			case <-c.stopCh:
				return
			}
		}
	}()
}

func (c *Checker) Stop() {
	select {
	case <-c.stopCh:
		// Already stopped
		return
	default:
		close(c.stopCh)
	}
}

func (c *Checker) checkServices() {
	healthy := int64(0)
	unhealthy := int64(0)
	start := time.Now()

	var wg sync.WaitGroup
	for name, url := range c.services {
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			status := c.checkService(url)
			c.status.Store(name, status)

			if status.Healthy {
				atomic.AddInt64(&healthy, 1)
			} else {
				atomic.AddInt64(&unhealthy, 1)
			}
		}(name, url)
	}

	wg.Wait()

	// Update metrics
	c.metrics.mu.Lock()
	c.metrics.healthyServices = healthy
	c.metrics.unhealthyServices = unhealthy
	c.metrics.totalChecks++
	c.metrics.lastCheckDuration = time.Since(start)
	c.metrics.mu.Unlock()
}

func (c *Checker) checkService(url string) Status {
	// Initialize status with current time
	status := Status{
		LastCheck: time.Now(),
		Healthy:   false,
	}

	// Validate URL format first
	_, err := validateURL(url)
	if err != nil {
		status.Message = fmt.Sprintf("Invalid URL: %v", err)
		return status
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url+"/health", nil)
	if err != nil {
		status.Message = fmt.Sprintf("Failed to create request: %v", err)
		return status
	}

	req.Header.Set("User-Agent", "ProxyHealthCheck/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		status.Message = fmt.Sprintf("Request failed: %v", err)
		return status
	}
	defer resp.Body.Close()

	// Read and discard body with timeout
	bodyReader := io.LimitReader(resp.Body, 1024) // Limit body read to 1KB
	_, err = io.Copy(io.Discard, bodyReader)
	if err != nil {
		status.Message = fmt.Sprintf("Failed to read response body: %v", err)
		return status
	}

	if resp.StatusCode != http.StatusOK {
		status.Message = fmt.Sprintf("Unexpected status code: %s", resp.Status)
		return status
	}

	status.Healthy = true
	return status
}

// validateURL checks if the URL is valid
func validateURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	if parsedURL.Scheme == "" {
		return nil, fmt.Errorf("missing scheme")
	}

	if parsedURL.Host == "" {
		return nil, fmt.Errorf("missing host")
	}

	return parsedURL, nil
}

// Add a method to check if the checker is running
func (c *Checker) IsRunning() bool {
	select {
	case <-c.stopCh:
		return false
	default:
		return true
	}
}

// Add a method to wait for first check completion
func (c *Checker) WaitForFirstCheck(timeout time.Duration) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timeout waiting for first health check")
		case <-ticker.C:
			if c.metrics.totalChecks > 0 {
				return nil
			}
		case <-c.stopCh:
			return fmt.Errorf("health checker stopped")
		}
	}
}

func (c *Checker) GetStatus(service string) (Status, bool) {
	if status, ok := c.status.Load(service); ok {
		return status.(Status), true
	}
	return Status{}, false
}

func (c *Checker) GetAllStatus() map[string]Status {
	result := make(map[string]Status)
	c.status.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(Status)
		return true
	})
	return result
}

// GetMetrics returns the current health check metrics
func (c *Checker) GetMetrics() HealthMetrics {
	c.metrics.mu.RLock()
	defer c.metrics.mu.RUnlock()

	return HealthMetrics{
		HealthyServices:   c.metrics.healthyServices,
		UnhealthyServices: c.metrics.unhealthyServices,
		TotalChecks:       c.metrics.totalChecks,
		LastCheckDuration: c.metrics.lastCheckDuration,
	}
}

// HealthMetrics represents the health checker metrics
type HealthMetrics struct {
	HealthyServices   int64
	UnhealthyServices int64
	TotalChecks       int64
	LastCheckDuration time.Duration
}

// Add a function to cleanup resources
func (c *Checker) cleanup() {
	c.client.CloseIdleConnections()
}
