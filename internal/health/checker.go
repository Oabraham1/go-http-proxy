package health

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Status struct {
    Healthy   bool      `json:"healthy"`
    LastCheck time.Time `json:"lastCheck"`
    Message   string    `json:"message,omitempty"`
}

type Checker struct {
    services map[string]string
    status   sync.Map
    client   *http.Client
    interval time.Duration
    stopCh   chan struct{}
}

func NewChecker(services map[string]string, interval time.Duration) *Checker {
    return &Checker{
        services: services,
        client: &http.Client{
            Timeout: 5 * time.Second,
            Transport: &http.Transport{
                DisableKeepAlives: true,
            },
        },
        interval: interval,
        stopCh:   make(chan struct{}),
    }
}

func (c *Checker) Start() {
    ticker := time.NewTicker(c.interval)
    go func() {
        for {
            select {
            case <-ticker.C:
                c.checkServices()
            case <-c.stopCh:
                ticker.Stop()
                return
            }
        }
    }()
}

func (c *Checker) Stop() {
    close(c.stopCh)
}

func (c *Checker) checkServices() {
    for name, url := range c.services {
        go func(name, url string) {
            status := c.checkService(url)
            c.status.Store(name, status)
        }(name, url)
    }
}

func (c *Checker) checkService(url string) Status {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", url+"/health", nil)
    if err != nil {
        return Status{
            Healthy:   false,
            LastCheck: time.Now(),
            Message:   "Failed to create request: " + err.Error(),
        }
    }

    req.Header.Set("User-Agent", "ProxyHealthCheck/1.0")

    resp, err := c.client.Do(req)
    if err != nil {
        return Status{
            Healthy:   false,
            LastCheck: time.Now(),
            Message:   "Request failed: " + err.Error(),
        }
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return Status{
            Healthy:   false,
            LastCheck: time.Now(),
            Message:   "Unexpected status code: " + resp.Status,
        }
    }

    return Status{
        Healthy:   true,
        LastCheck: time.Now(),
    }
}

func (c *Checker) GetStatus(service string) (Status, bool) {
    if status, ok := c.status.Load(service); ok {
        return status.(Status), true
    }
    return Status{}, false
}

func (c *Checker) GetAllStatus() map[string]Status {
    result := make(map[string]Status)
    c.status.Range(func(key, value interface{}) bool {
        result[key.(string)] = value.(Status)
        return true
    })
    return result
}

// MetricsCollector handles health check metrics
type MetricsCollector struct {
    healthyServices   prometheus.Gauge
    unhealthyServices prometheus.Gauge
    checkDuration     prometheus.Histogram
}

func NewMetricsCollector(registry prometheus.Registerer) *MetricsCollector {
    m := &MetricsCollector{
        healthyServices: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "proxy_healthy_services",
            Help: "Number of healthy services",
        }),
        unhealthyServices: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "proxy_unhealthy_services",
            Help: "Number of unhealthy services",
        }),
        checkDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name:    "proxy_health_check_duration_seconds",
            Help:    "Duration of health checks",
            Buckets: prometheus.DefBuckets,
        }),
    }

    registry.MustRegister(m.healthyServices)
    registry.MustRegister(m.unhealthyServices)
    registry.MustRegister(m.checkDuration)

    return m
}
