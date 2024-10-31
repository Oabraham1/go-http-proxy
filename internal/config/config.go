package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
    Server struct {
        Port            int           `yaml:"port"`
        ReadTimeout     time.Duration `yaml:"readTimeout"`
        WriteTimeout    time.Duration `yaml:"writeTimeout"`
        MaxHeaderBytes  int           `yaml:"maxHeaderBytes"`
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

    Services map[string]ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
    URL             string            `yaml:"url"`
    Timeout         time.Duration     `yaml:"timeout"`
    RateLimit       *RateLimitConfig  `yaml:"rateLimit,omitempty"`
    CircuitBreaker  *BreakerConfig   `yaml:"circuitBreaker,omitempty"`
    Headers         map[string]string `yaml:"headers,omitempty"`
}

type RateLimitConfig struct {
    Rate  float64 `yaml:"rate"`
    Burst int     `yaml:"burst"`
}

type BreakerConfig struct {
    MaxFailures int           `yaml:"maxFailures"`
    Timeout     time.Duration `yaml:"timeout"`
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
