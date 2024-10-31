package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"golang.org/x/time/rate"
)

// TracingMiddleware adds distributed tracing
type TracingMiddleware struct {
    tracer opentracing.Tracer
}

func NewTracing(tracer opentracing.Tracer) *TracingMiddleware {
    return &TracingMiddleware{tracer: tracer}
}

func (m *TracingMiddleware) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        spanCtx, _ := m.tracer.Extract(
            opentracing.HTTPHeaders,
            opentracing.HTTPHeadersCarrier(r.Header),
        )

        span := m.tracer.StartSpan(
            "http_request",
            ext.RPCServerOption(spanCtx),
        )
        defer span.Finish()

        // Add tags
        ext.HTTPMethod.Set(span, r.Method)
        ext.HTTPUrl.Set(span, r.URL.String())

        // Inject span into request context
        ctx := opentracing.ContextWithSpan(r.Context(), span)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// RateLimitMiddleware implements rate limiting
type RateLimitMiddleware struct {
    limiter *rate.Limiter
}

func NewRateLimit(r rate.Limit, b int) *RateLimitMiddleware {
    return &RateLimitMiddleware{
        limiter: rate.NewLimiter(r, b),
    }
}

func (m *RateLimitMiddleware) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !m.limiter.Allow() {
            http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// LoggingMiddleware adds request/response logging
type LoggingMiddleware struct {
    sensitiveHeaders []string
}

func NewLogging(sensitiveHeaders ...string) *LoggingMiddleware {
    return &LoggingMiddleware{
        sensitiveHeaders: sensitiveHeaders,
    }
}

func (m *LoggingMiddleware) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rw := &responseWriter{ResponseWriter: w}

        next.ServeHTTP(rw, r)

        // Create log entry
        entry := LogEntry{
            Method:     r.Method,
            Path:      r.URL.Path,
            Status:    rw.status,
            Duration:  time.Since(start),
            ClientIP:  r.RemoteAddr,
            Timestamp: start,
        }

        // Log as JSON
        json.NewEncoder(rw).Encode(entry)
    })
}

// AuthMiddleware handles authentication
type AuthMiddleware struct {
    validator TokenValidator
}

type TokenValidator interface {
    ValidateToken(string) bool
}

func NewAuth(validator TokenValidator) *AuthMiddleware {
    return &AuthMiddleware{
        validator: validator,
    }
}

func (m *AuthMiddleware) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if token == "" {
            http.Error(w, "No authorization token provided", http.StatusUnauthorized)
            return
        }

        if !m.validator.ValidateToken(token) {
            http.Error(w, "Invalid authorization token", http.StatusForbidden)
            return
        }

        next.ServeHTTP(w, r)
    })
}

// Helper types
type LogEntry struct {
    Method     string        `json:"method"`
    Path       string        `json:"path"`
    Status     int          `json:"status"`
    Duration   time.Duration `json:"duration"`
    ClientIP   string        `json:"clientIp"`
    Timestamp  time.Time     `json:"timestamp"`
}

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (rw *responseWriter) WriteHeader(status int) {
    rw.status = status
    rw.ResponseWriter.WriteHeader(status)
}
