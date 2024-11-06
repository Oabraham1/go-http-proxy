package circuitbreaker

import (
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	tests := []struct {
		name        string
		maxFailures int64
		timeout     time.Duration
		operations  []struct {
			action string // "allow", "success", "fail", "wait"
			want   bool   // expected result for "allow" actions
		}
	}{
		{
			name:        "opens after failures",
			maxFailures: 2,
			timeout:     time.Second,
			operations: []struct {
				action string
				want   bool
			}{
				{"allow", true},  // First request allowed
				{"fail", true},   // Record failure
				{"allow", true},  // Second request allowed
				{"fail", true},   // Record failure
				{"allow", false}, // Circuit is now open
			},
		},
		{
			name:        "recovers after timeout",
			maxFailures: 2,
			timeout:     100 * time.Millisecond,
			operations: []struct {
				action string
				want   bool
			}{
				{"allow", true},   // First request allowed
				{"fail", true},    // Record failure
				{"fail", true},    // Record failure
				{"allow", false},  // Circuit is open
				{"wait", true},    // Wait for timeout
				{"allow", true},   // Circuit is half-open
				{"success", true}, // Record success
				{"allow", true},   // Circuit is closed
			},
		},
		{
			name:        "stays closed on success",
			maxFailures: 2,
			timeout:     time.Second,
			operations: []struct {
				action string
				want   bool
			}{
				{"allow", true},   // Request allowed
				{"success", true}, // Record success
				{"allow", true},   // Still allowed
				{"success", true}, // Record success
				{"allow", true},   // Still allowed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := New("test", tt.maxFailures, tt.timeout)

			for i, op := range tt.operations {
				switch op.action {
				case "allow":
					if got := cb.Allow(); got != op.want {
						t.Errorf("operation %d: got allowed = %v, want %v", i, got, op.want)
						t.Logf("circuit breaker state: %v, failures: %d", cb.GetState(), cb.failures)
					}
				case "fail":
					cb.Failure()
				case "success":
					cb.Success()
				case "wait":
					time.Sleep(tt.timeout + 10*time.Millisecond)
				}
			}
		})
	}
}

func TestStateTransitions(t *testing.T) {
	cb := New("test", 2, 100*time.Millisecond)

	// Should start closed
	if state := cb.GetState(); state != StateClosed {
		t.Errorf("initial state = %v, want %v", state, StateClosed)
	}

	// Record failures to open the circuit
	cb.Failure()
	cb.Failure()

	// Should be open
	if state := cb.GetState(); state != StateOpen {
		t.Errorf("state after failures = %v, want %v", state, StateOpen)
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next Allow() should move to half-open
	if allowed := cb.Allow(); !allowed {
		t.Error("should be allowed after timeout")
	}
	if state := cb.GetState(); state != StateHalfOpen {
		t.Errorf("state after timeout = %v, want %v", state, StateHalfOpen)
	}

	// Success should close the circuit
	cb.Success()
	if state := cb.GetState(); state != StateClosed {
		t.Errorf("state after success = %v, want %v", state, StateClosed)
	}
}
