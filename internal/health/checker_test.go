package health

import (
	"net/http"
	"net/http/httptest"
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

    tests := []struct {
        name        string
        service     string
        wantHealthy bool
    }{
        {
            name:        "healthy service",
            service:     "healthy",
            wantHealthy: true,
        },
        {
            name:        "unhealthy service",
            service:     "unhealthy",
            wantHealthy: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            status, ok := checker.GetStatus(tt.service)
            if !ok {
                t.Fatal("status not found")
            }
            if status.Healthy != tt.wantHealthy {
                t.Errorf("got healthy = %v, want %v", status.Healthy, tt.wantHealthy)
            }
        })
    }
}
