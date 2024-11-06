package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/oabraham1/go-http-proxy/internal/cache"
	"github.com/oabraham1/go-http-proxy/internal/circuitbreaker"
	"github.com/oabraham1/go-http-proxy/internal/config"
	"github.com/oabraham1/go-http-proxy/internal/health"
	"github.com/oabraham1/go-http-proxy/internal/middleware"
	"github.com/oabraham1/go-http-proxy/pkg/filters"
)

type metrics struct {
	requests       atomic.Int64
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
	errors         atomic.Int64
	lastError      atomic.Int64
	activeRequests atomic.Int64
}

type Proxy struct {
	cfg         *config.Config
	server      *http.Server
	cache       *cache.Cache
	breakers    map[string]*circuitbreaker.CircuitBreaker
	healthCheck *health.Checker
	filters     []filters.Filter
	middlewares []middleware.Middleware
	metrics     *metrics
	client      *http.Client
	mu          sync.RWMutex
}

func New(cfg *config.Config) (*Proxy, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	p := &Proxy{
		cfg:      cfg,
		breakers: make(map[string]*circuitbreaker.CircuitBreaker),
		metrics:  &metrics{},
	}

	if err := p.initialize(); err != nil {
		return nil, fmt.Errorf("initialization failed: %w", err)
	}

	return p, nil
}

func (p *Proxy) initialize() error {
	// Initialize HTTP client
	p.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        p.cfg.Proxy.MaxIdleConns,
			MaxConnsPerHost:     p.cfg.Proxy.MaxConnsPerHost,
			IdleConnTimeout:     p.cfg.Proxy.IdleConnTimeout,
			DisableCompression:  true,
			DisableKeepAlives:   false,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: p.cfg.Proxy.TLSHandshakeTimeout,
		},
		Timeout: p.cfg.Proxy.ResponseTimeout,
	}

	// Initialize cache if enabled
	if p.cfg.Cache.Enabled {
		p.cache = cache.New(cache.Config{
			TTL: p.cfg.Cache.TTL,
		})
	}

	// Initialize circuit breakers
	for service, cfg := range p.cfg.Services {
		if cfg.CircuitBreaker != nil {
			p.breakers[service] = circuitbreaker.New(
				service,
				int64(cfg.CircuitBreaker.MaxFailures),
				cfg.CircuitBreaker.Timeout,
			)
		}
	}

	// Initialize health checker
	serviceURLs := make(map[string]string)
	for name, svc := range p.cfg.Services {
		serviceURLs[name] = svc.URL
	}
	p.healthCheck = health.NewChecker(serviceURLs, time.Minute)

	// Initialize middlewares
	if err := p.initMiddlewares(); err != nil {
		return fmt.Errorf("failed to initialize middlewares: %w", err)
	}

	// Initialize server
	serverHandler := p.handler()
	p.server = &http.Server{
		Addr:           fmt.Sprintf(":%d", p.cfg.Server.Port),
		Handler:        serverHandler,
		ReadTimeout:    p.cfg.Server.ReadTimeout,
		WriteTimeout:   p.cfg.Server.WriteTimeout,
		MaxHeaderBytes: p.cfg.Server.MaxHeaderBytes,
	}

	// Configure TLS if enabled
	if p.cfg.Server.TLS != nil && p.cfg.Server.TLS.Enabled {
		tlsConfig, err := configureTLS(p.cfg.Server.TLS)
		if err != nil {
			return fmt.Errorf("TLS configuration error: %w", err)
		}
		p.server.TLSConfig = tlsConfig
	}

	return nil
}

func configureTLS(cfg *config.TLSConfig) (*tls.Config, error) {
	var minVersion uint16
	switch cfg.MinVersion {
	case "1.2":
		minVersion = tls.VersionTLS12
	case "1.3":
		minVersion = tls.VersionTLS13
	default:
		minVersion = tls.VersionTLS12
	}

	return &tls.Config{
		MinVersion:   minVersion,
		CipherSuites: parseCipherSuites(cfg.CipherSuites),
	}, nil
}

func parseCipherSuites(ciphers []string) []uint16 {
	// Implementation to convert cipher suite strings to tls.uint16 values
	// This would need a mapping of string names to actual cipher suite values
	return nil // TODO: Implement cipher suite parsing
}

func (p *Proxy) initMiddlewares() error {
	if p.cfg.Tracing.Enabled {
		p.middlewares = append(p.middlewares,
			middleware.NewTracing(nil))
	}

	if p.cfg.RateLimit.Enabled {
		p.middlewares = append(p.middlewares,
			middleware.NewRateLimit(rate.Limit(p.cfg.RateLimit.Rate), p.cfg.RateLimit.Burst))
	}

	p.middlewares = append(p.middlewares, middleware.NewLogging())

	return nil
}

func (p *Proxy) Start() error {
	p.healthCheck.Start()
	go p.collectMetrics()

	if err := p.server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (p *Proxy) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p.healthCheck.Stop()

	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	return nil
}

func (p *Proxy) collectMetrics() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.logMetrics()
		p.metrics.activeRequests.Store(0)
	}
}

func (p *Proxy) logMetrics() {
	log.Printf("Proxy Metrics - Requests: %d, Cache Hits: %d, Cache Misses: %d, Errors: %d, Active Requests: %d",
		p.metrics.requests.Load(),
		p.metrics.cacheHits.Load(),
		p.metrics.cacheMisses.Load(),
		p.metrics.errors.Load(),
		p.metrics.activeRequests.Load(),
	)
}
