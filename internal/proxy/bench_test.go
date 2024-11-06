package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oabraham1/go-http-proxy/internal/cache"
	"github.com/oabraham1/go-http-proxy/internal/circuitbreaker"
	"github.com/oabraham1/go-http-proxy/internal/middleware"
)

func BenchmarkProxy(b *testing.B) {
	// Setup test backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	benchmarks := []struct {
		name  string
		setup func() http.Handler
	}{
		{
			name: "DirectProxy",
			setup: func() http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.DefaultTransport.RoundTrip(r)
				})
			},
		},
		{
			name: "WithCache",
			setup: func() http.Handler {
				c := cache.New(cache.Config{
					MaxSize: 1024 * 1024 * 10, // 10MB
					TTL:     time.Minute,
				})
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if resp, hit := c.Get(r); hit {
						w.WriteHeader(resp.StatusCode)
						return
					}
					resp, _ := http.DefaultTransport.RoundTrip(r)
					c.Set(r, resp)
					w.WriteHeader(resp.StatusCode)
				})
			},
		},
		{
			name: "WithCircuitBreaker",
			setup: func() http.Handler {
				cb := circuitbreaker.New("test", 5, time.Second)
				return cb.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.DefaultTransport.RoundTrip(r)
				}))
			},
		},
		{
			name: "FullStack",
			setup: func() http.Handler {
				c := cache.New(cache.Config{
					MaxSize: 1024 * 1024 * 10, // 10MB
					TTL:     time.Minute,
				})
				cb := circuitbreaker.New("test", 5, time.Second)
				return middleware.Chain(
					http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if resp, hit := c.Get(r); hit {
							w.WriteHeader(resp.StatusCode)
							return
						}
						resp, _ := http.DefaultTransport.RoundTrip(r)
						c.Set(r, resp)
						w.WriteHeader(resp.StatusCode)
					}),
					middleware.NewTracing(nil),
					middleware.NewRateLimit(100, 10),
					cb,
				)
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			handler := bm.setup()
			server := httptest.NewServer(handler)
			defer server.Close()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				client := &http.Client{}
				for pb.Next() {
					req, _ := http.NewRequest("GET", server.URL, nil)
					resp, err := client.Do(req)
					if err != nil {
						b.Fatal(err)
					}
					resp.Body.Close()
				}
			})
		})
	}
}
