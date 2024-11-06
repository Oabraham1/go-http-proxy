package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/oabraham1/go-http-proxy/internal/config"
)

type HTTPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

type HealthStatus struct {
	Status    string          `json:"status"`
	Services  map[string]bool `json:"services"`
	Timestamp time.Time       `json:"timestamp"`
}

type ProxyMetrics struct {
	Requests       int64     `json:"requests"`
	CacheHits      int64     `json:"cache_hits"`
	CacheMisses    int64     `json:"cache_misses"`
	Errors         int64     `json:"errors"`
	LastError      time.Time `json:"last_error,omitempty"`
	ActiveRequests int64     `json:"active_requests"`
}

func (p *Proxy) handler() http.Handler {
	router := mux.NewRouter().SkipClean(true)

	// Add middleware chain
	var handler http.Handler = router
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		handler = p.middlewares[i].Wrap(handler)
	}

	// Configure routes
	p.configureRoutes(router)

	return handler
}

func (p *Proxy) configureRoutes(router *mux.Router) {
	router.HandleFunc("/health", p.handleHealth).Methods("GET")
	router.HandleFunc("/metrics", p.handleMetrics).Methods("GET")

	for service, cfg := range p.cfg.Services {
		handler := p.serviceHandler(service, cfg)
		router.PathPrefix("/" + service).Handler(handler)
	}
}

func (p *Proxy) serviceHandler(service string, cfg config.ServiceConfig) http.Handler {
	var baseHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply service-specific filters
		for _, filter := range p.filters {
			if err := filter.Process(r); err != nil {
				p.handleError(w, r, err)
				return
			}
		}

		p.handleRequest(w, r, service, cfg)
	})

	if breaker, exists := p.breakers[service]; exists {
		baseHandler = breaker.Wrap(baseHandler)
	}

	return baseHandler
}

func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request, service string, cfg config.ServiceConfig) {
	start := time.Now()
	var cacheHit bool
	var err error

	// Wrap the response writer to capture status code and size
	lw := p.wrapResponseWriter(w)

	// Defer logging until the end of the request
	defer func() {
		p.logRequest(start, lw, r, service, cacheHit, err)
	}()

	p.metrics.activeRequests.Add(1)
	defer p.metrics.activeRequests.Add(-1)
	p.metrics.requests.Add(1)

	// Check cache
	if p.cache != nil {
		if cached, ok := p.cache.Get(r); ok {
			cacheHit = true
			p.metrics.cacheHits.Add(1)
			p.writeResponse(lw, cached)
			return
		}
		p.metrics.cacheMisses.Add(1)
	}

	// Forward request
	resp, err := p.forwardRequest(r, cfg)
	if err != nil {
		p.handleError(lw, r, err)
		return
	}
	defer resp.Body.Close()

	// Cache response if appropriate
	if p.cache != nil && resp.StatusCode == http.StatusOK {
		p.cache.Set(r, resp)
	}

	p.writeResponse(lw, resp)
}

func (p *Proxy) forwardRequest(r *http.Request, cfg config.ServiceConfig) (*http.Response, error) {
	// Clone the request
	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""

	// Set timeout if configured
	if cfg.Timeout > 0 {
		ctx, cancel := context.WithTimeout(outReq.Context(), cfg.Timeout)
		defer cancel()
		outReq = outReq.WithContext(ctx)
	}

	// Add configured headers
	for k, v := range cfg.Headers {
		outReq.Header.Set(k, v)
	}

	// Add X-Forwarded headers
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior := outReq.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		outReq.Header.Set("X-Forwarded-For", clientIP)
	}

	return p.client.Do(outReq)
}

func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := HealthStatus{
		Status:    "ok",
		Services:  make(map[string]bool),
		Timestamp: time.Now(),
	}

	for name, cfg := range p.cfg.Services {
		if breaker, exists := p.breakers[name]; exists {
			health.Services[name] = breaker.Allow()
		} else {
			_, err := http.Head(cfg.URL)
			health.Services[name] = err == nil
		}
	}

	for _, healthy := range health.Services {
		if !healthy {
			health.Status = "degraded"
			break
		}
	}

	p.writeJSON(w, health)
}

func (p *Proxy) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ProxyMetrics{
		Requests:       p.metrics.requests.Load(),
		CacheHits:      p.metrics.cacheHits.Load(),
		CacheMisses:    p.metrics.cacheMisses.Load(),
		Errors:         p.metrics.errors.Load(),
		LastError:      time.Unix(p.metrics.lastError.Load(), 0),
		ActiveRequests: p.metrics.activeRequests.Load(),
	}

	p.writeJSON(w, metrics)
}

func (p *Proxy) writeResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if resp.Body != nil {
		io.Copy(w, resp.Body)
	}
}

func (p *Proxy) writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

func (p *Proxy) handleError(w http.ResponseWriter, r *http.Request, err error) {
	p.metrics.errors.Add(1)
	p.metrics.lastError.Store(time.Now().Unix())

	log.Printf("Error handling request %s %s: %v", r.Method, r.URL.Path, err)

	code := http.StatusInternalServerError
	msg := "Internal Server Error"

	if httpErr, ok := err.(HTTPError); ok {
		code = httpErr.Code
		msg = httpErr.Message
	}

	http.Error(w, msg, code)
}
