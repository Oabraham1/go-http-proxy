package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

// TestTracingMiddleware tests the basic functionality of the tracing middleware
func TestTracingMiddleware(t *testing.T) {
	tracer := NewTracing(nil)

	handler := tracer.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status OK; got %v", rec.Code)
	}
}

// TestRateLimitMiddleware tests the rate limiting functionality
func TestRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		limit         rate.Limit
		burst         int
		requests      int
		expectedCodes []int
	}{
		{
			name:          "under limit",
			limit:         rate.Limit(10),
			burst:         1,
			requests:      1,
			expectedCodes: []int{200},
		},
		{
			name:          "at limit",
			limit:         rate.Limit(2),
			burst:         2,
			requests:      2,
			expectedCodes: []int{200, 200},
		},
		{
			name:          "exceed limit",
			limit:         rate.Limit(1),
			burst:         1,
			requests:      3,
			expectedCodes: []int{200, 429, 429},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimit(tt.limit, tt.burst)
			handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			for i := 0; i < tt.requests; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if rec.Code != tt.expectedCodes[i] {
					t.Errorf("request %d: expected status %d; got %d", i+1, tt.expectedCodes[i], rec.Code)
				}
			}
		})
	}
}

// TestLoggingMiddleware tests the logging functionality
func TestLoggingMiddleware(t *testing.T) {
	logging := NewLogging("Authorization", "Password")

	handler := logging.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"test"}`))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("X-Test", "visible-header")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200; got %d", rec.Code)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json; got %s", rec.Header().Get("Content-Type"))
	}
}

// TestAuthMiddleware tests the authentication middleware
func TestAuthMiddleware(t *testing.T) {
	validator := &mockTokenValidator{
		validTokens: map[string]bool{
			"valid-token": true,
		},
	}

	auth := NewAuth(validator)
	handler := auth.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{
			name:       "valid token",
			token:      "valid-token",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid token",
			token:      "invalid-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing token",
			token:      "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", tt.token)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d; got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

// TestMiddlewareChain tests the chaining of multiple middleware
func TestMiddlewareChain(t *testing.T) {
	var executionOrder []string

	// Create base handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, "final-handler")
		w.WriteHeader(http.StatusOK)
	})

	// Create middleware chain
	chain := Chain(finalHandler,
		createTestMiddleware("auth", &executionOrder),
		createTestMiddleware("rate-limit", &executionOrder),
		createTestMiddleware("logging", &executionOrder),
	)

	// Test the chain
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	chain.ServeHTTP(rec, req)

	// Verify execution order
	expected := []string{"auth", "rate-limit", "logging", "final-handler"}
	if len(executionOrder) != len(expected) {
		t.Errorf("expected %d middleware executions; got %d", len(expected), len(executionOrder))
	}
	for i, exp := range expected {
		if i >= len(executionOrder) || executionOrder[i] != exp {
			t.Errorf("execution order mismatch at position %d: expected %s; got %s",
				i, exp, executionOrder[i])
		}
	}
}

// Helper types and functions
type mockTokenValidator struct {
	validTokens map[string]bool
}

func (m *mockTokenValidator) ValidateToken(token string) bool {
	return m.validTokens[token]
}

type testMiddleware struct {
	name           string
	executionOrder *[]string
}

func createTestMiddleware(name string, order *[]string) Middleware {
	return &testMiddleware{
		name:           name,
		executionOrder: order,
	}
}

func (m *testMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*m.executionOrder = append(*m.executionOrder, m.name)
		next.ServeHTTP(w, r)
	})
}

// TestConcurrentRateLimit tests rate limiting under concurrent load
func TestConcurrentRateLimit(t *testing.T) {
	limiter := NewRateLimit(rate.Limit(50), 1)
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const concurrent = 100
	results := make(chan int, concurrent)

	// Make concurrent requests
	for i := 0; i < concurrent; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}

	// Collect results
	accepted := 0
	rejected := 0
	for i := 0; i < concurrent; i++ {
		code := <-results
		switch code {
		case http.StatusOK:
			accepted++
		case http.StatusTooManyRequests:
			rejected++
		default:
			t.Errorf("unexpected status code: %d", code)
		}
	}

	// Verify some requests were accepted and some were rejected
	if accepted == 0 {
		t.Error("expected some requests to be accepted")
	}
	if rejected == 0 {
		t.Error("expected some requests to be rejected")
	}
}
