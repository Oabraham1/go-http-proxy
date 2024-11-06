package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
    Server struct {
        Port           int           `yaml:"port"`
        ReadTimeout    time.Duration `yaml:"readTimeout"`
        WriteTimeout   time.Duration `yaml:"writeTimeout"`
        MaxHeaderBytes int           `yaml:"maxHeaderBytes"`
        TLS           *TLSConfig    `yaml:"tls,omitempty"`  // Add TLS config here
    } `yaml:"server"`

    Proxy struct {
        MaxIdleConns        int           `yaml:"maxIdleConns"`
        MaxConnsPerHost     int           `yaml:"maxConnsPerHost"`
        IdleConnTimeout     time.Duration `yaml:"idleConnTimeout"`
        ResponseTimeout     time.Duration `yaml:"responseTimeout"`
        TLSHandshakeTimeout time.Duration `yaml:"tlsHandshakeTimeout"`
    } `yaml:"proxy"`

    CircuitBreaker struct {
        MaxFailures int           `yaml:"maxFailures"`
        Timeout     time.Duration `yaml:"timeout"`
    } `yaml:"circuitBreaker"`

    Tracing struct {
        Enabled     bool    `yaml:"enabled"`
        ServiceName string  `yaml:"serviceName"`
        AgentHost   string  `yaml:"agentHost"`
        SampleRate  float64 `yaml:"sampleRate"`
    } `yaml:"tracing"`

    Cache struct {
        Enabled bool          `yaml:"enabled"`
        TTL     time.Duration `yaml:"ttl"`
    } `yaml:"cache"`

    RateLimit struct {
        Enabled bool    `yaml:"enabled"`
        Rate    float64 `yaml:"rate"`
        Burst   int     `yaml:"burst"`
    } `yaml:"rateLimit"`

    Security struct {
        Headers struct {
            Enabled bool   `yaml:"enabled"`
            CSP     string `yaml:"csp,omitempty"`
        } `yaml:"headers"`
        CORS struct {
            Enabled          bool     `yaml:"enabled"`
            AllowedOrigins   []string `yaml:"allowedOrigins"`
            AllowedMethods   []string `yaml:"allowedMethods"`
            AllowedHeaders   []string `yaml:"allowedHeaders"`
            ExposedHeaders   []string `yaml:"exposedHeaders"`
            AllowCredentials bool     `yaml:"allowCredentials"`
            MaxAge          int      `yaml:"maxAge"`
        } `yaml:"cors"`
    } `yaml:"security"`

    Services map[string]ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
    URL            string            `yaml:"url"`
    Timeout        time.Duration     `yaml:"timeout"`
    RateLimit      *RateLimitConfig  `yaml:"rateLimit,omitempty"`
    CircuitBreaker *BreakerConfig    `yaml:"circuitBreaker,omitempty"`
    Headers        map[string]string `yaml:"headers,omitempty"`
}

type RateLimitConfig struct {
    Rate  float64 `yaml:"rate"`
    Burst int     `yaml:"burst"`
}

type BreakerConfig struct {
    MaxFailures int           `yaml:"maxFailures"`
    Timeout     time.Duration `yaml:"timeout"`
}

type TLSConfig struct {
    Enabled      bool     `yaml:"enabled"`
    CertFile     string   `yaml:"certFile"`
    KeyFile      string   `yaml:"keyFile"`
    MinVersion   string   `yaml:"minVersion"`
    CipherSuites []string `yaml:"cipherSuites"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var config Config
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, err
    }

    return &config, nil
}
