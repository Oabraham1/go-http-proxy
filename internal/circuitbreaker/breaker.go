package circuitbreaker

import (
	"net/http"
	"sync"
	"time"
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
	failures      int64
	lastFailure   time.Time
	state         State
	mutex         sync.RWMutex
	onStateChange func(from, to State)
}

func New(name string, maxFailures int64, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:        name,
		maxFailures: maxFailures,
		timeout:     timeout,
		state:       StateClosed,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case StateOpen:
		if time.Since(cb.lastFailure) > cb.timeout {
			// Try to move to half-open
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			// Recheck state after getting write lock
			if cb.state == StateOpen {
				cb.setState(StateHalfOpen)
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return true
		}
		return false

	case StateHalfOpen:
		return true

	default: // StateClosed
		return true
	}
}

func (cb *CircuitBreaker) Success() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == StateHalfOpen {
		cb.setState(StateClosed)
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) Failure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == StateClosed && cb.failures >= cb.maxFailures {
		cb.setState(StateOpen)
	} else if cb.state == StateHalfOpen {
		cb.setState(StateOpen)
	}
}

func (cb *CircuitBreaker) setState(newState State) {
	if cb.state != newState {
		oldState := cb.state
		cb.state = newState
		if cb.onStateChange != nil {
			cb.onStateChange(oldState, newState)
		}
	}
}

func (cb *CircuitBreaker) GetState() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.failures = 0
	cb.setState(StateClosed)
}

// Wrap wraps an http.Handler with the circuit breaker
func (cb *CircuitBreaker) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cb.Allow() {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)

		if sw.status >= 500 {
			cb.Failure()
		} else {
			cb.Success()
		}
	})
}

// statusWriter wraps http.ResponseWriter to capture the status code
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}
