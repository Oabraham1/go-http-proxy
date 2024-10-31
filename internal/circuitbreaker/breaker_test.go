package circuitbreaker

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
    tests := []struct {
        name        string
        maxFailures int64
        timeout     time.Duration
        operations  []struct {
            status int
            want   bool
        }
    }{
        {
            name:        "opens after failures",
            maxFailures: 2,
            timeout:     time.Second,
            operations: []struct {
                status int
                want   bool
            }{
                {http.StatusOK, true},
                {http.StatusInternalServerError, true},
                {http.StatusInternalServerError, true},
                {http.StatusOK, false}, // circuit is now open
            },
        },
        {
            name:        "recovers after timeout",
            maxFailures: 1,
            timeout:     100 * time.Millisecond,
            operations: []struct {
                status int
                want   bool
            }{
                {http.StatusInternalServerError, true},
                {http.StatusOK, false}, // circuit opens
                {-1, false},           // wait for timeout
                {http.StatusOK, true},  // circuit is half-open
                {http.StatusOK, true},  // circuit closes
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cb := New("test", tt.maxFailures, tt.timeout)

            for i, op := range tt.operations {
                if op.status == -1 {
                    // Wait operation
                    time.Sleep(tt.timeout + 10*time.Millisecond)
                    continue
                }

                // Create test request
                req := httptest.NewRequest("GET", "/test", nil)
                rec := httptest.NewRecorder()

                // Create handler that returns the specified status
                handler := cb.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                    w.WriteHeader(op.status)
                }))

                // Execute request
                handler.ServeHTTP(rec, req)

                // Verify circuit breaker state
                if allowed := cb.Allow(); allowed != op.want {
                    t.Errorf("operation %d: got allowed = %v, want %v", i, allowed, op.want)
                }
            }
        })
    }
}
