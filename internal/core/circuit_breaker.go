package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents circuit breaker state
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"    // Normal operation
	CircuitOpen     CircuitState = "open"      // Failing, reject requests
	CircuitHalfOpen CircuitState = "half_open" // Testing recovery
)

// CircuitBreaker prevents cascading failures
type CircuitBreaker struct {
	name                 string
	state                CircuitState
	failureThreshold     int           // Failures before opening
	successThreshold     int           // Successes to close from half-open
	timeout              time.Duration // Time before half-open attempt
	maxTimeout           time.Duration // Maximum timeout for exponential backoff
	failureCount         int32
	consecutiveSuccesses int32
	lastFailureTime      time.Time
	lastStateChange      time.Time
	attemptCount         int32 // Number of half-open attempts
	mu                   sync.RWMutex
	metrics              *CircuitBreakerMetrics
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	Enabled               bool
	FailureThreshold      int           // Failures before opening (default: 5)
	SuccessThreshold      int           // Successes to close from half-open (default: 2)
	Timeout               time.Duration // Time before half-open attempt (default: 30s)
	MaxTimeout            time.Duration // Max timeout for exponential backoff (default: 5m)
	UseExponentialBackoff bool          // Enable exponential backoff
}

// CircuitBreakerMetrics tracks circuit breaker statistics
type CircuitBreakerMetrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	RejectedRequests   int64
	StateTransitions   int64
	TimeInOpen         int64 // Nanoseconds spent in open state
	TimeInHalfOpen     int64 // Nanoseconds spent in half-open state
	LastOpenTime       time.Time
	LastCloseTime      time.Time
	mu                 sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 2
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxTimeout <= 0 {
		config.MaxTimeout = 5 * time.Minute
	}

	return &CircuitBreaker{
		name:             name,
		state:            CircuitClosed,
		failureThreshold: config.FailureThreshold,
		successThreshold: config.SuccessThreshold,
		timeout:          config.Timeout,
		maxTimeout:       config.MaxTimeout,
		lastStateChange:  time.Now(),
		metrics:          &CircuitBreakerMetrics{},
	}
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:               false, // Disabled by default
		FailureThreshold:      5,
		SuccessThreshold:      2,
		Timeout:               30 * time.Second,
		MaxTimeout:            5 * time.Minute,
		UseExponentialBackoff: true,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	atomic.AddInt64(&cb.metrics.TotalRequests, 1)

	// Check if we can proceed
	if !cb.AllowRequest() {
		atomic.AddInt64(&cb.metrics.RejectedRequests, 1)
		return fmt.Errorf("circuit breaker is open")
	}

	// Execute the function
	err := fn()

	// Record the result
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// AllowRequest checks if a request should be allowed
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if timeout has elapsed to transition to half-open
		if cb.shouldAttemptResetLocked() {
			cb.toHalfOpen()
			return true
		}
		return false

	case CircuitHalfOpen:
		// In half-open state, allow limited requests to test recovery
		return true

	default:
		return false
	}
}

// RecordSuccess records a successful execution
func (cb *CircuitBreaker) RecordSuccess() {
	atomic.AddInt64(&cb.metrics.SuccessfulRequests, 1)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		atomic.StoreInt32(&cb.failureCount, 0)

	case CircuitHalfOpen:
		// Increment consecutive successes
		successCount := atomic.AddInt32(&cb.consecutiveSuccesses, 1)

		if int(successCount) >= cb.successThreshold {
			cb.toClosed()
		}
	}
}

// RecordFailure records a failed execution
func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt64(&cb.metrics.FailedRequests, 1)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		failCount := atomic.AddInt32(&cb.failureCount, 1)
		if int(failCount) >= cb.failureThreshold {
			cb.toOpen()
		}

	case CircuitHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.toOpen()
	}
}

// State transitions (must be called with lock held)

func (cb *CircuitBreaker) toOpen() {
	cb.state = CircuitOpen
	cb.lastStateChange = time.Now()
	atomic.AddInt32(&cb.attemptCount, 1)
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.consecutiveSuccesses, 0)

	cb.recordStateTransition()
	cb.metrics.mu.Lock()
	cb.metrics.LastOpenTime = time.Now()
	cb.metrics.mu.Unlock()
}

func (cb *CircuitBreaker) toHalfOpen() {
	// Calculate time spent in open state
	if cb.state == CircuitOpen {
		timeInOpen := time.Since(cb.lastStateChange)
		atomic.AddInt64(&cb.metrics.TimeInOpen, timeInOpen.Nanoseconds())
	}

	cb.state = CircuitHalfOpen
	cb.lastStateChange = time.Now()
	atomic.StoreInt32(&cb.consecutiveSuccesses, 0)

	cb.recordStateTransition()
}

func (cb *CircuitBreaker) toClosed() {
	// Calculate time spent in half-open state
	if cb.state == CircuitHalfOpen {
		timeInHalfOpen := time.Since(cb.lastStateChange)
		atomic.AddInt64(&cb.metrics.TimeInHalfOpen, timeInHalfOpen.Nanoseconds())
	}

	cb.state = CircuitClosed
	cb.lastStateChange = time.Now()
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt32(&cb.attemptCount, 0)

	cb.recordStateTransition()
	cb.metrics.mu.Lock()
	cb.metrics.LastCloseTime = time.Now()
	cb.metrics.mu.Unlock()
}

// shouldAttemptResetLocked checks if enough time has passed (must be called with lock held)
func (cb *CircuitBreaker) shouldAttemptResetLocked() bool {
	if cb.state != CircuitOpen {
		return false
	}

	// Calculate timeout with exponential backoff
	timeout := cb.timeout
	attempts := atomic.LoadInt32(&cb.attemptCount)

	if attempts > 1 {
		// Exponential backoff: timeout * 2^(attempts-1)
		// Safe conversion: we cap attempts to prevent overflow
		attemptsUint := uint(attempts - 1)
		if attemptsUint > 30 {
			attemptsUint = 30 // Cap to prevent overflow (2^30 is already very large)
		}
		backoffMultiplier := time.Duration(1 << attemptsUint) // #nosec G115 - capped at 30
		timeout = cb.timeout * backoffMultiplier

		if timeout > cb.maxTimeout {
			timeout = cb.maxTimeout
		}
	}

	elapsed := time.Since(cb.lastStateChange)
	return elapsed >= timeout
}

// GetState returns the current circuit state
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	return int(atomic.LoadInt32(&cb.failureCount))
}

// GetSuccessCount returns the consecutive success count
func (cb *CircuitBreaker) GetSuccessCount() int {
	return int(atomic.LoadInt32(&cb.consecutiveSuccesses))
}

// GetMetrics returns current circuit breaker metrics
func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.metrics.mu.RLock()
	defer cb.metrics.mu.RUnlock()

	return CircuitBreakerMetrics{
		TotalRequests:      atomic.LoadInt64(&cb.metrics.TotalRequests),
		SuccessfulRequests: atomic.LoadInt64(&cb.metrics.SuccessfulRequests),
		FailedRequests:     atomic.LoadInt64(&cb.metrics.FailedRequests),
		RejectedRequests:   atomic.LoadInt64(&cb.metrics.RejectedRequests),
		StateTransitions:   atomic.LoadInt64(&cb.metrics.StateTransitions),
		TimeInOpen:         atomic.LoadInt64(&cb.metrics.TimeInOpen),
		TimeInHalfOpen:     atomic.LoadInt64(&cb.metrics.TimeInHalfOpen),
		LastOpenTime:       cb.metrics.LastOpenTime,
		LastCloseTime:      cb.metrics.LastCloseTime,
	}
}

// ResetMetrics resets all circuit breaker metrics
func (cb *CircuitBreaker) ResetMetrics() {
	atomic.StoreInt64(&cb.metrics.TotalRequests, 0)
	atomic.StoreInt64(&cb.metrics.SuccessfulRequests, 0)
	atomic.StoreInt64(&cb.metrics.FailedRequests, 0)
	atomic.StoreInt64(&cb.metrics.RejectedRequests, 0)
	atomic.StoreInt64(&cb.metrics.StateTransitions, 0)
	atomic.StoreInt64(&cb.metrics.TimeInOpen, 0)
	atomic.StoreInt64(&cb.metrics.TimeInHalfOpen, 0)

	cb.metrics.mu.Lock()
	cb.metrics.LastOpenTime = time.Time{}
	cb.metrics.LastCloseTime = time.Time{}
	cb.metrics.mu.Unlock()
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.toClosed()
}

// ForceOpen manually forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.toOpen()
}

// GetName returns the circuit breaker name
func (cb *CircuitBreaker) GetName() string {
	return cb.name
}

// GetLastFailureTime returns the time of the last failure
func (cb *CircuitBreaker) GetLastFailureTime() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastFailureTime
}

// GetLastStateChange returns the time of the last state change
func (cb *CircuitBreaker) GetLastStateChange() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastStateChange
}

// recordStateTransition records a state transition
func (cb *CircuitBreaker) recordStateTransition() {
	atomic.AddInt64(&cb.metrics.StateTransitions, 1)
}

// GetTimeUntilReset returns the time remaining until the circuit breaker attempts to reset
func (cb *CircuitBreaker) GetTimeUntilReset() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state != CircuitOpen {
		return 0
	}

	timeout := cb.timeout
	attempts := atomic.LoadInt32(&cb.attemptCount)

	if attempts > 1 {
		// Safe conversion: we cap attempts to prevent overflow
		attemptsUint := uint(attempts - 1)
		if attemptsUint > 30 {
			attemptsUint = 30 // Cap to prevent overflow (2^30 is already very large)
		}
		backoffMultiplier := time.Duration(1 << attemptsUint) // #nosec G115 - capped at 30
		timeout = cb.timeout * backoffMultiplier
		if timeout > cb.maxTimeout {
			timeout = cb.maxTimeout
		}
	}

	elapsed := time.Since(cb.lastStateChange)
	remaining := timeout - elapsed

	if remaining < 0 {
		return 0
	}

	return remaining
}

// CircuitBreakerInfo contains comprehensive circuit breaker information
type CircuitBreakerInfo struct {
	Name             string
	State            CircuitState
	FailureCount     int
	SuccessCount     int
	FailureThreshold int
	SuccessThreshold int
	TimeUntilReset   time.Duration
	LastFailureTime  time.Time
	LastStateChange  time.Time
	AttemptCount     int
	Metrics          CircuitBreakerMetrics
}

// GetInfo returns comprehensive circuit breaker information
func (cb *CircuitBreaker) GetInfo() CircuitBreakerInfo {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerInfo{
		Name:             cb.name,
		State:            cb.state,
		FailureCount:     int(atomic.LoadInt32(&cb.failureCount)),
		SuccessCount:     int(atomic.LoadInt32(&cb.consecutiveSuccesses)),
		FailureThreshold: cb.failureThreshold,
		SuccessThreshold: cb.successThreshold,
		TimeUntilReset:   cb.GetTimeUntilReset(),
		LastFailureTime:  cb.lastFailureTime,
		LastStateChange:  cb.lastStateChange,
		AttemptCount:     int(atomic.LoadInt32(&cb.attemptCount)),
		Metrics:          cb.GetMetrics(),
	}
}
