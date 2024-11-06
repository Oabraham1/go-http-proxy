package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthChecker(t *testing.T) {
	// Create test servers
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()

	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthy.Close()

	services := map[string]string{
		"healthy":   healthy.URL,
		"unhealthy": unhealthy.URL,
	}

	checker := NewChecker(services, time.Second)
	checker.Start()
	defer checker.Stop()

	// Wait for initial checks
	time.Sleep(2 * time.Second)

	// Test individual service status
	if status, ok := checker.GetStatus("healthy"); !ok || !status.Healthy {
		t.Error("expected healthy service to be healthy")
	}

	if status, ok := checker.GetStatus("unhealthy"); !ok || status.Healthy {
		t.Error("expected unhealthy service to be unhealthy")
	}

	// Test metrics
	metrics := checker.GetMetrics()
	if metrics.HealthyServices != 1 {
		t.Errorf("expected 1 healthy service, got %d", metrics.HealthyServices)
	}
	if metrics.UnhealthyServices != 1 {
		t.Errorf("expected 1 unhealthy service, got %d", metrics.UnhealthyServices)
	}
	if metrics.TotalChecks == 0 {
		t.Error("expected total checks to be greater than 0")
	}
}

func TestHealthCheckerTimeout(t *testing.T) {
	// Create a channel to control the slow handler
	done := make(chan struct{})
	defer close(done)

	// Create a slow server with controlled shutdown
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-done:
			return
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	}))

	// Create a client with a very short timeout
	checker := NewChecker(map[string]string{
		"slow": slow.URL,
	}, 100*time.Millisecond)

	// Override the default client with a shorter timeout
	checker.client.Timeout = 50 * time.Millisecond

	// Start checker
	checker.Start()

	// Wait for at least one check cycle
	time.Sleep(200 * time.Millisecond)

	// Get status
	status, ok := checker.GetStatus("slow")

	// Stop checker before closing server
	checker.Stop()
	slow.Close()

	// Verify results
	if !ok {
		t.Fatal("expected status for slow service")
	}
	if status.Healthy {
		t.Error("expected slow service to be marked unhealthy due to timeout")
	}
	if !strings.Contains(status.Message, "timeout") && !strings.Contains(status.Message, "deadline exceeded") {
		t.Errorf("expected timeout error message, got: %s", status.Message)
	}
}

func TestHealthCheckerInvalidURL(t *testing.T) {
	services := map[string]string{
		"invalid": "http://invalid.local.invalid:12345/health", // Use an invalid domain that won't resolve
	}

	checker := NewChecker(services, 100*time.Millisecond) // Use shorter interval for tests

	// Start the checker
	checker.Start()

	// Wait for at least one check cycle
	time.Sleep(200 * time.Millisecond)

	// Get the status
	status, ok := checker.GetStatus("invalid")

	// Stop the checker
	checker.Stop()

	// Verify results
	if !ok {
		t.Fatal("expected status for invalid service to be present")
	}

	if status.Healthy {
		t.Error("expected invalid service to be marked unhealthy")
	}

	// Verify the error message
	if status.Message == "" {
		t.Error("expected non-empty error message for invalid service")
	}

	// Log the actual error message for debugging
	t.Logf("Invalid service error message: %s", status.Message)
}

func TestHealthCheckerAllStatus(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	services := map[string]string{
		"service1": server.URL,
		"service2": server.URL,
	}

	checker := NewChecker(services, time.Second)
	checker.Start()
	defer checker.Stop()

	// Wait for initial checks
	time.Sleep(2 * time.Second)

	// Test GetAllStatus
	allStatus := checker.GetAllStatus()
	if len(allStatus) != 2 {
		t.Errorf("expected 2 services, got %d", len(allStatus))
	}

	for name, status := range allStatus {
		if !status.Healthy {
			t.Errorf("expected service %s to be healthy", name)
		}
	}
}

func TestHealthCheckerVariousInvalidURLs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantMsg string
	}{
		{
			name:    "invalid domain",
			url:     "http://invalid.local.invalid:12345/health",
			wantMsg: "Request failed",
		},
		{
			name:    "invalid port",
			url:     "http://localhost:99999",
			wantMsg: "Request failed",
		},
		{
			name:    "malformed URL",
			url:     "not-a-url",
			wantMsg: "Invalid URL",
		},
		{
			name:    "empty URL",
			url:     "",
			wantMsg: "Invalid URL",
		},
		{
			name:    "missing scheme",
			url:     "localhost:8080",
			wantMsg: "Invalid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services := map[string]string{
				"test": tt.url,
			}

			checker := NewChecker(services, 100*time.Millisecond)
			checker.Start()

			// Wait for at least one check
			err := checker.WaitForFirstCheck(time.Second)
			if err != nil {
				t.Fatalf("Failed to wait for first check: %v", err)
			}

			status, ok := checker.GetStatus("test")
			checker.Stop()

			if !ok {
				t.Fatal("expected status to be present")
			}

			if status.Healthy {
				t.Error("expected service to be marked unhealthy")
			}

			if !strings.Contains(status.Message, tt.wantMsg) {
				t.Errorf("expected message containing %q, got %q", tt.wantMsg, status.Message)
			}
		})
	}
}
