package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiter controls throughput using token bucket algorithm
type RateLimiter struct {
	rate       int           // Tokens per period
	period     time.Duration // Time period for rate
	burstSize  int           // Maximum burst size (tokens)
	tokens     int64         // Current available tokens (atomic)
	lastRefill time.Time
	mu         sync.Mutex
	enabled    bool
	metrics    *RateLimiterMetrics
}

// RateLimitConfig configures rate limiting behavior
type RateLimitConfig struct {
	Enabled       bool
	Rate          int           // FlowFiles per period
	Period        time.Duration // Time period
	BurstSize     int           // Max burst size (0 = same as rate)
	MaxConcurrent int           // Max concurrent executions
}

// ProcessorRateLimiter wraps rate limiting for processors
type ProcessorRateLimiter struct {
	limiter     *RateLimiter
	config      RateLimitConfig
	activeCount int32 // Current active executions
	mu          sync.RWMutex
}

// RateLimiterMetrics tracks rate limiting statistics
type RateLimiterMetrics struct {
	RequestCount   int64 // Total requests
	AllowedCount   int64 // Requests allowed
	ThrottledCount int64 // Requests throttled
	TotalWaitTime  int64 // Total wait time in nanoseconds
	LastThrottled  time.Time
	mu             sync.RWMutex
}

// NewRateLimiter creates a new rate limiter with token bucket algorithm
func NewRateLimiter(rate int, period time.Duration, burstSize int) *RateLimiter {
	if burstSize == 0 {
		burstSize = rate
	}

	return &RateLimiter{
		rate:       rate,
		period:     period,
		burstSize:  burstSize,
		tokens:     int64(burstSize),
		lastRefill: time.Now(),
		enabled:    true,
		metrics:    &RateLimiterMetrics{},
	}
}

// NewProcessorRateLimiter creates a rate limiter for a processor
func NewProcessorRateLimiter(config RateLimitConfig) *ProcessorRateLimiter {
	if !config.Enabled {
		return &ProcessorRateLimiter{
			config:  config,
			limiter: nil,
		}
	}

	if config.BurstSize == 0 {
		config.BurstSize = config.Rate
	}

	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 1
	}

	return &ProcessorRateLimiter{
		config:  config,
		limiter: NewRateLimiter(config.Rate, config.Period, config.BurstSize),
	}
}

// DefaultRateLimitConfig returns a default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:       false, // Disabled by default
		Rate:          100,
		Period:        1 * time.Second,
		BurstSize:     0, // Will be set to rate
		MaxConcurrent: 1,
	}
}

// Allow checks if a request should be allowed (non-blocking)
func (rl *RateLimiter) Allow() bool {
	if !rl.enabled {
		return true
	}

	rl.refillTokens()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	tokens := atomic.LoadInt64(&rl.tokens)

	atomic.AddInt64(&rl.metrics.RequestCount, 1)

	if tokens > 0 {
		atomic.AddInt64(&rl.tokens, -1)
		atomic.AddInt64(&rl.metrics.AllowedCount, 1)
		return true
	}

	atomic.AddInt64(&rl.metrics.ThrottledCount, 1)
	rl.metrics.mu.Lock()
	rl.metrics.LastThrottled = time.Now()
	rl.metrics.mu.Unlock()

	return false
}

// Wait blocks until a token is available or timeout
func (rl *RateLimiter) Wait(timeout time.Duration) error {
	if !rl.enabled {
		return nil
	}

	startTime := time.Now()
	deadline := startTime.Add(timeout)

	for {
		if rl.Allow() {
			waitTime := time.Since(startTime)
			atomic.AddInt64(&rl.metrics.TotalWaitTime, waitTime.Nanoseconds())
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("rate limit wait timeout after %v", timeout)
		}

		// Sleep for a fraction of the refill period
		sleepDuration := rl.period / time.Duration(rl.rate)
		if sleepDuration > 100*time.Millisecond {
			sleepDuration = 100 * time.Millisecond
		}
		time.Sleep(sleepDuration)
	}
}

// Reserve reserves a token for future use and returns wait duration
func (rl *RateLimiter) Reserve() time.Duration {
	if !rl.enabled {
		return 0
	}

	rl.refillTokens()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	tokens := atomic.LoadInt64(&rl.tokens)

	atomic.AddInt64(&rl.metrics.RequestCount, 1)

	if tokens > 0 {
		atomic.AddInt64(&rl.tokens, -1)
		atomic.AddInt64(&rl.metrics.AllowedCount, 1)
		return 0
	}

	// Calculate wait time based on rate
	atomic.AddInt64(&rl.metrics.ThrottledCount, 1)
	rl.metrics.mu.Lock()
	rl.metrics.LastThrottled = time.Now()
	rl.metrics.mu.Unlock()

	tokensPerNano := float64(rl.rate) / float64(rl.period.Nanoseconds())
	nanosToWait := int64(1.0 / tokensPerNano)

	return time.Duration(nanosToWait)
}

// refillTokens adds tokens based on elapsed time
func (rl *RateLimiter) refillTokens() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	if elapsed < rl.period/time.Duration(rl.rate*10) {
		// Too soon to refill
		return
	}

	// Calculate tokens to add based on elapsed time
	tokensToAdd := int64(float64(elapsed.Nanoseconds()) * float64(rl.rate) / float64(rl.period.Nanoseconds()))

	if tokensToAdd > 0 {
		currentTokens := atomic.LoadInt64(&rl.tokens)
		newTokens := currentTokens + tokensToAdd

		if newTokens > int64(rl.burstSize) {
			newTokens = int64(rl.burstSize)
		}

		atomic.StoreInt64(&rl.tokens, newTokens)
		rl.lastRefill = now
	}
}

// GetTokens returns the current token count
func (rl *RateLimiter) GetTokens() int {
	rl.refillTokens()
	return int(atomic.LoadInt64(&rl.tokens))
}

// SetEnabled enables or disables the rate limiter
func (rl *RateLimiter) SetEnabled(enabled bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.enabled = enabled
}

// IsEnabled returns whether the rate limiter is enabled
func (rl *RateLimiter) IsEnabled() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.enabled
}

// GetMetrics returns current rate limiter metrics
func (rl *RateLimiter) GetMetrics() RateLimiterMetrics {
	rl.metrics.mu.RLock()
	defer rl.metrics.mu.RUnlock()

	return RateLimiterMetrics{
		RequestCount:   atomic.LoadInt64(&rl.metrics.RequestCount),
		AllowedCount:   atomic.LoadInt64(&rl.metrics.AllowedCount),
		ThrottledCount: atomic.LoadInt64(&rl.metrics.ThrottledCount),
		TotalWaitTime:  atomic.LoadInt64(&rl.metrics.TotalWaitTime),
		LastThrottled:  rl.metrics.LastThrottled,
	}
}

// ResetMetrics resets the rate limiter metrics
func (rl *RateLimiter) ResetMetrics() {
	atomic.StoreInt64(&rl.metrics.RequestCount, 0)
	atomic.StoreInt64(&rl.metrics.AllowedCount, 0)
	atomic.StoreInt64(&rl.metrics.ThrottledCount, 0)
	atomic.StoreInt64(&rl.metrics.TotalWaitTime, 0)

	rl.metrics.mu.Lock()
	rl.metrics.LastThrottled = time.Time{}
	rl.metrics.mu.Unlock()
}

// ProcessorRateLimiter methods

// CanExecute checks if processor can execute (considering rate limit and concurrency)
func (prl *ProcessorRateLimiter) CanExecute() bool {
	if !prl.config.Enabled {
		return true
	}

	// Check concurrency limit
	if prl.config.MaxConcurrent > 0 {
		activeCount := atomic.LoadInt32(&prl.activeCount)
		if int(activeCount) >= prl.config.MaxConcurrent {
			return false
		}
	}

	// Check rate limit
	if prl.limiter != nil {
		return prl.limiter.Allow()
	}

	return true
}

// WaitForExecution waits until execution is allowed
func (prl *ProcessorRateLimiter) WaitForExecution(timeout time.Duration) error {
	if !prl.config.Enabled {
		return nil
	}

	startTime := time.Now()
	deadline := startTime.Add(timeout)

	for {
		// Check concurrency limit
		if prl.config.MaxConcurrent > 0 {
			activeCount := atomic.LoadInt32(&prl.activeCount)
			if int(activeCount) >= prl.config.MaxConcurrent {
				if time.Now().After(deadline) {
					return fmt.Errorf("rate limit timeout: max concurrent executions reached")
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		// Check rate limit
		if prl.limiter != nil {
			remaining := time.Until(deadline)
			if err := prl.limiter.Wait(remaining); err != nil {
				return err
			}
		}

		return nil
	}
}

// BeginExecution marks the beginning of an execution
func (prl *ProcessorRateLimiter) BeginExecution() {
	if prl.config.Enabled && prl.config.MaxConcurrent > 0 {
		atomic.AddInt32(&prl.activeCount, 1)
	}
}

// EndExecution marks the end of an execution
func (prl *ProcessorRateLimiter) EndExecution() {
	if prl.config.Enabled && prl.config.MaxConcurrent > 0 {
		atomic.AddInt32(&prl.activeCount, -1)
	}
}

// GetActiveCount returns the current number of active executions
func (prl *ProcessorRateLimiter) GetActiveCount() int {
	return int(atomic.LoadInt32(&prl.activeCount))
}

// GetMetrics returns rate limiter metrics
func (prl *ProcessorRateLimiter) GetMetrics() RateLimiterMetrics {
	if prl.limiter == nil {
		return RateLimiterMetrics{}
	}
	return prl.limiter.GetMetrics()
}

// IsEnabled returns whether rate limiting is enabled
func (prl *ProcessorRateLimiter) IsEnabled() bool {
	return prl.config.Enabled
}

// GetConfig returns the rate limit configuration
func (prl *ProcessorRateLimiter) GetConfig() RateLimitConfig {
	prl.mu.RLock()
	defer prl.mu.RUnlock()
	return prl.config
}

// UpdateConfig updates the rate limit configuration
func (prl *ProcessorRateLimiter) UpdateConfig(config RateLimitConfig) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	prl.config = config

	if config.Enabled {
		if config.BurstSize == 0 {
			config.BurstSize = config.Rate
		}
		prl.limiter = NewRateLimiter(config.Rate, config.Period, config.BurstSize)
	} else {
		prl.limiter = nil
	}
}
