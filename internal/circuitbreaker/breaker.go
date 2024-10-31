package circuitbreaker

import (
	"net/http"
	"sync"
	"time"

	"go.uber.org/atomic"
)

type State int

const (
    StateClosed State = iota
    StateHalfOpen
    StateOpen
)

type CircuitBreaker struct {
    name          string
    maxFailures   int64
    timeout       time.Duration
    failures      *atomic.Int64
    lastFailure   *atomic.Int64
    state         *atomic.Int32
    halfOpenLimit *atomic.Int32
    mu            sync.RWMutex
}

func New(name string, maxFailures int64, timeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        name:          name,
        maxFailures:   maxFailures,
        timeout:       timeout,
        failures:      atomic.NewInt64(0),
        lastFailure:   atomic.NewInt64(0),
        state:         atomic.NewInt32(int32(StateClosed)),
        halfOpenLimit: atomic.NewInt32(0),
    }
}

func (cb *CircuitBreaker) Wrap(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !cb.Allow() {
            http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
            return
        }

        rw := &responseWriter{ResponseWriter: w}
        next.ServeHTTP(rw, r)

        if rw.status >= 500 {
            cb.Failure()
        } else {
            cb.Success()
        }
    })
}

func (cb *CircuitBreaker) Allow() bool {
    state := State(cb.state.Load())
    switch state {
    case StateOpen:
        if time.Since(time.Unix(cb.lastFailure.Load(), 0)) > cb.timeout {
            cb.mu.Lock()
            if State(cb.state.Load()) == StateOpen {
                cb.state.Store(int32(StateHalfOpen))
                cb.halfOpenLimit.Store(0)
            }
            cb.mu.Unlock()
            return true
        }
        return false

    case StateHalfOpen:
        return cb.halfOpenLimit.Inc() <= 1

    default:
        return true
    }
}

func (cb *CircuitBreaker) Success() {
    if State(cb.state.Load()) == StateHalfOpen {
        cb.mu.Lock()
        cb.failures.Store(0)
        cb.state.Store(int32(StateClosed))
        cb.mu.Unlock()
    }
}

func (cb *CircuitBreaker) Failure() {
    failures := cb.failures.Inc()
    cb.lastFailure.Store(time.Now().Unix())

    if failures >= cb.maxFailures {
        cb.mu.Lock()
        if State(cb.state.Load()) == StateClosed {
            cb.state.Store(int32(StateOpen))
        }
        cb.mu.Unlock()
    }
}

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (rw *responseWriter) WriteHeader(status int) {
    rw.status = status
    rw.ResponseWriter.WriteHeader(status)
}
