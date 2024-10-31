package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/oabraham1/go-http-proxy/internal/cache"
	"github.com/oabraham1/go-http-proxy/internal/circuitbreaker"
	"github.com/oabraham1/go-http-proxy/internal/config"
	"github.com/oabraham1/go-http-proxy/internal/health"
	"github.com/oabraham1/go-http-proxy/internal/middleware"
	"github.com/oabraham1/go-http-proxy/pkg/filters"
)

type Proxy struct {
	cfg         *config.Config
	server      *http.Server
	cache       *cache.Cache
	breakers    map[string]*circuitbreaker.CircuitBreaker
	healthCheck *health.Checker
	filters     []filters.Filter
	middlewares []middleware.Middleware
	mu          sync.RWMutex
}

func New(cfg *config.Config) (*Proxy, error) {
	p := &Proxy{
		cfg:      cfg,
		breakers: make(map[string]*circuitbreaker.CircuitBreaker),
	}

	// Initialize components
	if err := p.initialize(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Proxy) initialize() error {
	// Initialize cache if enabled
	if p.cfg.Cache.Enabled {
		p.cache = cache.New(p.cfg.Cache.TTL)
	}

	// Initialize circuit breakers
	for service, cfg := range p.cfg.Services {
		if cfg.CircuitBreaker != nil {
			p.breakers[service] = circuitbreaker.New(
				service,
				cfg.CircuitBreaker.MaxFailures,
				cfg.CircuitBreaker.Timeout,
			)
		}
	}

	// Initialize health checker
	p.healthCheck = health.NewChecker(p.cfg.Services)

	// Initialize middlewares
	p.initMiddlewares()

	// Initialize server
	p.server = &http.Server{
		Addr:           fmt.Sprintf(":%d", p.cfg.Server.Port),
		Handler:        p.handler(),
		ReadTimeout:    p.cfg.Server.ReadTimeout,
		WriteTimeout:   p.cfg.Server.WriteTimeout,
		MaxHeaderBytes: p.cfg.Server.MaxHeaderBytes,
	}

	return nil
}

func (p *Proxy) initMiddlewares() {
	// Add tracing middleware if enabled
	if p.cfg.Tracing.Enabled {
		p.middlewares = append(p.middlewares,
			middleware.NewTracing(p.cfg.Tracing))
	}

	// Add rate limiting middleware if enabled
	if p.cfg.RateLimit.Enabled {
		p.middlewares = append(p.middlewares,
			middleware.NewRateLimit(p.cfg.RateLimit))
	}

	// Add authentication middleware
	p.middlewares = append(p.middlewares, middleware.NewAuth())

	// Add logging middleware
	p.middlewares = append(p.middlewares, middleware.NewLogging())
}

func (p *Proxy) Start() error {
	// Start health checker
	p.healthCheck.Start()

	// Start server
	return p.server.ListenAndServe()
}

func (p *Proxy) Shutdown() error {
	// Stop health checker
	p.healthCheck.Stop()

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return p.server.Shutdown(ctx)
}
