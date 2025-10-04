package core

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		MaxTimeout:       5 * time.Minute,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	assert.Equal(t, "test-breaker", cb.name)
	assert.Equal(t, CircuitClosed, cb.state)
	assert.Equal(t, 5, cb.failureThreshold)
	assert.Equal(t, 2, cb.successThreshold)
	assert.Equal(t, 30*time.Second, cb.timeout)
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.False(t, config.Enabled) // Disabled by default
	assert.Equal(t, 5, config.FailureThreshold)
	assert.Equal(t, 2, config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 5*time.Minute, config.MaxTimeout)
	assert.True(t, config.UseExponentialBackoff)
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	assert.Equal(t, CircuitClosed, cb.GetState())

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Should transition to open
	assert.Equal(t, CircuitOpen, cb.GetState())
	assert.Equal(t, 0, cb.GetFailureCount()) // Reset after opening

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(3), metrics.FailedRequests)
	assert.False(t, metrics.LastOpenTime.IsZero())
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Force circuit to open
	cb.RecordFailure()
	cb.RecordFailure()

	assert.Equal(t, CircuitOpen, cb.GetState())

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should allow request (transitioning to half-open)
	allowed := cb.AllowRequest()
	assert.True(t, allowed)
	assert.Equal(t, CircuitHalfOpen, cb.GetState())
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Force to half-open state
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.AllowRequest()

	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Record successes up to threshold
	cb.RecordSuccess()
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	cb.RecordSuccess()
	// Should transition to closed
	assert.Equal(t, CircuitClosed, cb.GetState())

	metrics := cb.GetMetrics()
	assert.False(t, metrics.LastCloseTime.IsZero())
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Force to half-open state
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.AllowRequest()

	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	// Any failure in half-open should reopen
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())
}

func TestCircuitBreaker_AllowRequest(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Closed state - allow requests
	assert.True(t, cb.AllowRequest())

	// Open circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Open state - reject requests
	assert.False(t, cb.AllowRequest())

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should allow request (transitioning to half-open)
	assert.True(t, cb.AllowRequest())
	assert.Equal(t, CircuitHalfOpen, cb.GetState())
}

func TestCircuitBreaker_Call(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Successful call
	err := cb.Call(func() error {
		return nil
	})
	require.NoError(t, err)

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.SuccessfulRequests)

	// Failed call
	err = cb.Call(func() error {
		return errors.New("test error")
	})
	require.Error(t, err)

	metrics = cb.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.FailedRequests)
}

func TestCircuitBreaker_CallWhenOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Force circuit open
	cb.RecordFailure()
	cb.RecordFailure()

	// Call should be rejected
	err := cb.Call(func() error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker is open")

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(1), metrics.RejectedRequests)
}

func TestCircuitBreaker_RecordSuccess(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Record failure then success in closed state
	cb.RecordFailure()
	assert.Equal(t, 1, cb.GetFailureCount())

	cb.RecordSuccess()
	assert.Equal(t, 0, cb.GetFailureCount()) // Success resets failure count

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(1), metrics.SuccessfulRequests)
}

func TestCircuitBreaker_GetMetrics(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Generate some activity
	cb.Call(func() error { return nil })
	cb.Call(func() error { return errors.New("error") })
	cb.Call(func() error { return errors.New("error") })

	// Circuit should be open now
	cb.Call(func() error { return nil }) // Rejected

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(4), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.SuccessfulRequests)
	assert.Equal(t, int64(2), metrics.FailedRequests)
	assert.Equal(t, int64(1), metrics.RejectedRequests)
	assert.True(t, metrics.StateTransitions > 0)
}

func TestCircuitBreaker_ResetMetrics(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Generate some activity
	cb.Call(func() error { return nil })
	cb.Call(func() error { return errors.New("error") })

	// Reset metrics
	cb.ResetMetrics()

	metrics := cb.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalRequests)
	assert.Equal(t, int64(0), metrics.SuccessfulRequests)
	assert.Equal(t, int64(0), metrics.FailedRequests)
	assert.Equal(t, int64(0), metrics.RejectedRequests)
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Open circuit
	cb.RecordFailure()
	cb.RecordFailure()

	assert.Equal(t, CircuitOpen, cb.GetState())

	// Manual reset
	cb.Reset()
	assert.Equal(t, CircuitClosed, cb.GetState())
	assert.Equal(t, 0, cb.GetFailureCount())
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	assert.Equal(t, CircuitClosed, cb.GetState())

	// Force open
	cb.ForceOpen()
	assert.Equal(t, CircuitOpen, cb.GetState())
}

func TestCircuitBreaker_GetTimeUntilReset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          200 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Closed state - no reset time
	assert.Equal(t, time.Duration(0), cb.GetTimeUntilReset())

	// Open circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Should have time until reset
	remaining := cb.GetTimeUntilReset()
	assert.True(t, remaining > 0)
	assert.True(t, remaining <= 200*time.Millisecond)

	// Wait and check again
	time.Sleep(100 * time.Millisecond)
	remaining = cb.GetTimeUntilReset()
	assert.True(t, remaining < 100*time.Millisecond)
}

func TestCircuitBreaker_ExponentialBackoff(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:      2,
		SuccessThreshold:      2,
		Timeout:               100 * time.Millisecond,
		MaxTimeout:            1 * time.Second,
		UseExponentialBackoff: true,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// First failure cycle
	cb.RecordFailure()
	cb.RecordFailure()
	time1 := cb.GetTimeUntilReset()

	// Trigger half-open and fail again
	time.Sleep(150 * time.Millisecond)
	cb.AllowRequest()
	cb.RecordFailure()

	// Second failure cycle - timeout should be longer
	time2 := cb.GetTimeUntilReset()

	// Due to exponential backoff, second timeout should be longer
	// Note: This is approximate due to timing variations
	assert.True(t, time2 > time1*3/2, "expected exponential backoff, time1=%v, time2=%v", time1, time2)
}

func TestCircuitBreaker_GetInfo(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	info := cb.GetInfo()

	assert.Equal(t, "test-breaker", info.Name)
	assert.Equal(t, CircuitClosed, info.State)
	assert.Equal(t, 2, info.FailureCount)
	assert.Equal(t, 3, info.FailureThreshold)
	assert.Equal(t, 2, info.SuccessThreshold)

	// Open the circuit
	cb.RecordFailure()

	info = cb.GetInfo()
	assert.Equal(t, CircuitOpen, info.State)
	assert.False(t, info.LastStateChange.IsZero())
}

func TestCircuitBreaker_GetState(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	assert.Equal(t, CircuitClosed, cb.GetState())

	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, CircuitOpen, cb.GetState())

	time.Sleep(150 * time.Millisecond)
	cb.AllowRequest()
	assert.Equal(t, CircuitHalfOpen, cb.GetState())

	cb.RecordSuccess()
	cb.RecordSuccess()
	assert.Equal(t, CircuitClosed, cb.GetState())
}

func TestCircuitBreaker_GetName(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("my-test-breaker", config)

	assert.Equal(t, "my-test-breaker", cb.GetName())
}

func TestCircuitBreaker_GetLastFailureTime(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       1 * time.Second,
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// No failures yet
	assert.True(t, cb.GetLastFailureTime().IsZero())

	// Record a failure
	cb.RecordFailure()

	// Should have a timestamp
	assert.False(t, cb.GetLastFailureTime().IsZero())
}

func TestCircuitBreaker_MaxTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		MaxTimeout:       200 * time.Millisecond, // Very low max
	}

	cb := NewCircuitBreaker("test-breaker", config)

	// Force multiple open cycles to test max timeout
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
		cb.RecordFailure()

		if i > 0 {
			time.Sleep(250 * time.Millisecond) // Wait longer than max timeout
			cb.AllowRequest()
		}
	}

	// Timeout should be capped at max timeout
	remaining := cb.GetTimeUntilReset()
	assert.True(t, remaining <= 200*time.Millisecond)
}
