package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareChain(t *testing.T) {
    tests := []struct {
        name           string
        middleware    []Middleware
        wantHeaders   map[string]string
        wantStatus    int
    }{
        {
            name: "auth middleware blocks",
            middleware: []Middleware{
                NewAuth(mockValidator{valid: false}),
            },
            wantStatus: http.StatusUnauthorized,
        },
        {
            name: "rate limit middleware",
            middleware: []Middleware{
                NewRateLimit(1, 1),
            },
            wantStatus: http.StatusOK,
        },
        {
            name: "multiple middleware chain",
            middleware: []Middleware{
                NewAuth(mockValidator{valid: true}),
                NewRateLimit(1, 1),
                NewLogging([]string{"Authorization"}),
            },
            wantHeaders: map[string]string{
                "X-Trace-ID": "test",
            },
            wantStatus: http.StatusOK,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                for k, v := range tt.wantHeaders {
                    w.Header().Set(k, v)
                }
                w.WriteHeader(http.StatusOK)
            })

            chain := Chain(handler, tt.middleware...)

            req := httptest.NewRequest("GET", "/test", nil)
            rec := httptest.NewRecorder()

            chain.ServeHTTP(rec, req)

            if rec.Code != tt.wantStatus {
                t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
            }

            for k, v := range tt.wantHeaders {
                if got := rec.Header().Get(k); got != v {
                    t.Errorf("header %s: got %s, want %s", k, got, v)
                }
            }
        })
    }
}
