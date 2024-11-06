# Proxy Configuration Examples

## Basic API Gateway
```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

services:
  users:
    url: "http://users-service:8001"
    timeout: 10s
    headers:
      X-Service: "users"
    rateLimit:
      rate: 100
      burst: 20

  orders:
    url: "http://orders-service:8002"
    timeout: 15s
    headers:
      X-Service: "orders"
    rateLimit:
      rate: 50
      burst: 10

logging:
  level: "info"
  format: "json"
```

## High-Performance Cache Layer
```yaml
server:
  port: 8080
  readTimeout: 5s
  writeTimeout: 5s

cache:
  enabled: true
  ttl: 5m
  maxSize: "1GB"
  cleanupInterval: 1m

services:
  static-content:
    url: "http://cdn:8001"
    cacheRules:
      - path: "/images/*"
        ttl: 1h
      - path: "/css/*"
        ttl: 24h
      - path: "/js/*"
        ttl: 24h

  api:
    url: "http://api:8002"
    cacheRules:
      - path: "/products"
        ttl: 5m
        methods: ["GET"]
      - path: "/categories"
        ttl: 1h
        methods: ["GET"]
```

## Microservices Gateway with Circuit Breaker
```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

circuitBreaker:
  default:
    maxFailures: 5
    timeout: 10s
    halfOpenLimit: 2

services:
  auth:
    url: "http://auth:8001"
    circuitBreaker:
      maxFailures: 3
      timeout: 5s
    retries:
      max: 2
      backoff: 100ms

  payments:
    url: "http://payments:8002"
    circuitBreaker:
      maxFailures: 2
      timeout: 30s
    timeout: 20s

  inventory:
    url: "http://inventory:8003"
    circuitBreaker:
      maxFailures: 5
      timeout: 10s
```

## Load Balancer with Health Checks
```yaml
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s

loadBalancer:
  algorithm: "round-robin"  # or "least-connections", "ip-hash"
  sticky: true
  cookieName: "SERVERID"

healthCheck:
  interval: 10s
  timeout: 2s
  unhealthyThreshold: 3
  healthyThreshold: 2

services:
  web:
    backends:
      - url: "http://web1:8001"
        weight: 100
      - url: "http://web2:8001"
        weight: 100
      - url: "http://web3:8001"
        weight: 50
    healthCheck:
      path: "/health"
      interval: 5s
```

## Security-Focused Configuration
```yaml
server:
  port: 8443
  readTimeout: 30s
  writeTimeout: 30s
  tls:
    enabled: true
    cert: "/certs/server.crt"
    key: "/certs/server.key"
    minVersion: "1.2"

security:
  rateLimit:
    enabled: true
    rate: 10
    burst: 20
    by: "ip"  # or "token", "user"

  cors:
    enabled: true
    allowOrigins: ["https://app.example.com"]
    allowMethods: ["GET", "POST", "PUT", "DELETE"]
    allowHeaders: ["Authorization", "Content-Type"]
    exposeHeaders: ["X-Request-ID"]
    maxAge: 3600

  auth:
    type: "jwt"
    jwksUrl: "https://auth.example.com/.well-known/jwks.json"
    audience: "https://api.example.com"
    issuer: "https://auth.example.com/"

services:
  api:
    url: "http://internal-api:8001"
    security:
      ipWhitelist: ["10.0.0.0/8"]
      requiredScopes: ["read:users", "write:users"]
```
