# Go HTTP Proxy Server

A high-performance, feature-rich HTTP proxy server implemented in Go.

## Features

- ✨ SSL Termination
- 🔄 Connection Management
- 🔒 Circuit Breaker Pattern
- 📝 Request/Response Logging
- 🚦 Rate Limiting
- 💾 Caching Support
- 🔍 Health Checking
- 📊 Metrics Collection
- 🔄 Protocol Upgrade/Downgrade
- 🛡️ Security Features

## Installation

```bash
go get github.com/oabraham1/go-http-proxy
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/yourusername/proxy/internal/config"
    "github.com/yourusername/proxy/internal/proxy"
)

func main() {
    cfg := &config.Config{
        Server: config.ServerConfig{
            Port: 8080,
        },
        Services: map[string]config.ServiceConfig{
            "api": {
                URL: "http://api.example.com",
            },
        },
    }

    proxy, err := proxy.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    log.Fatal(proxy.Start())
}
```

## Configuration

The proxy server can be configured using YAML:

```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

cache:
  enabled: true
  ttl: 5m

circuitBreaker:
  maxFailures: 5
  timeout: 10s

services:
  api:
    url: http://api.example.com
    timeout: 30s
    rateLimit:
      rate: 100
      burst: 10
```

## Features in Detail

### Circuit Breaker

Protects backend services from cascading failures:

```go
breaker := circuitbreaker.New("service", 5, time.Second)
handler := breaker.Wrap(backendHandler)
```

### Caching

Efficient caching of responses:

```go
cache := cache.New(5 * time.Minute)
cached, hit := cache.Get(request)
if hit {
    return cached
}
```

### Health Checking

Regular health checks of backend services:

```go
checker := health.NewChecker(map[string]string{
    "service1": "http://service1/health",
}, time.Minute)
checker.Start()
```

### Middleware

Easy to add custom middleware:

```go
proxy.Use(
    middleware.NewRateLimit(100, 10),
    middleware.NewAuth(authValidator),
    middleware.NewLogging(nil),
)
```

## Performance

Benchmark results:

```
BenchmarkProxy/DirectProxy-8         10000        112340 ns/op
BenchmarkProxy/WithCache-8          50000         31245 ns/op
BenchmarkProxy/WithCircuitBreaker-8 20000         89123 ns/op
BenchmarkProxy/FullStack-8          10000        156789 ns/op
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing`)
3. Commit your changes (`git commit -am 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing`)
5. Create a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [Gorilla Mux](https://github.com/gorilla/mux) for routing
- [OpenTracing](https://opentracing.io/) for distributed tracing
- [Prometheus](https://prometheus.io/) for metrics

## Contact

- GitHub: [@oabraham1](https://github.com/oabraham1)
- Twitter: [@ojima_abraham](https://x.com/ojima_abraham)
