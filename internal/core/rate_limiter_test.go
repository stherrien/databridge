package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(100, 1*time.Second, 0)

	assert.Equal(t, 100, rl.rate)
	assert.Equal(t, 1*time.Second, rl.period)
	assert.Equal(t, 100, rl.burstSize) // Should default to rate
	assert.Equal(t, int64(100), rl.tokens)
	assert.True(t, rl.enabled)
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second, 10)

	// Should allow up to burst size
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow(), "request %d should be allowed", i)
	}

	// Next request should be denied
	assert.False(t, rl.Allow())

	metrics := rl.GetMetrics()
	assert.Equal(t, int64(11), metrics.RequestCount)
	assert.Equal(t, int64(10), metrics.AllowedCount)
	assert.Equal(t, int64(1), metrics.ThrottledCount)
}

func TestRateLimiter_AllowWithRefill(t *testing.T) {
	rl := NewRateLimiter(10, 100*time.Millisecond, 10)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow())
	}

	// Should be denied
	assert.False(t, rl.Allow())

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should allow again
	assert.True(t, rl.Allow())
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(10, 100*time.Millisecond, 10)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow())
	}

	// Wait should succeed after refill
	err := rl.Wait(200 * time.Millisecond)
	require.NoError(t, err)

	metrics := rl.GetMetrics()
	assert.True(t, metrics.TotalWaitTime > 0)
}

func TestRateLimiter_WaitTimeout(t *testing.T) {
	// Create limiter with very slow refill rate
	rl := NewRateLimiter(10, 10*time.Second, 10)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow())
	}

	// Wait with short timeout should fail since refill is slow
	err := rl.Wait(50 * time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestRateLimiter_Reserve(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second, 10)

	// First reserve should have no wait
	wait := rl.Reserve()
	assert.Equal(t, time.Duration(0), wait)

	// Consume remaining tokens
	for i := 0; i < 9; i++ {
		rl.Reserve()
	}

	// Next reserve should return wait time
	wait = rl.Reserve()
	assert.True(t, wait > 0)
}

func TestRateLimiter_GetTokens(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second, 10)

	assert.Equal(t, 10, rl.GetTokens())

	rl.Allow()
	assert.Equal(t, 9, rl.GetTokens())
}

func TestRateLimiter_SetEnabled(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second, 10)

	assert.True(t, rl.IsEnabled())

	rl.SetEnabled(false)
	assert.False(t, rl.IsEnabled())

	// When disabled, all requests should be allowed
	for i := 0; i < 20; i++ {
		assert.True(t, rl.Allow())
	}
}

func TestRateLimiter_ResetMetrics(t *testing.T) {
	rl := NewRateLimiter(10, 1*time.Second, 10)

	// Generate some metrics
	for i := 0; i < 15; i++ {
		rl.Allow()
	}

	metrics := rl.GetMetrics()
	assert.True(t, metrics.RequestCount > 0)
	assert.True(t, metrics.AllowedCount > 0)
	assert.True(t, metrics.ThrottledCount > 0)

	// Reset
	rl.ResetMetrics()

	metrics = rl.GetMetrics()
	assert.Equal(t, int64(0), metrics.RequestCount)
	assert.Equal(t, int64(0), metrics.AllowedCount)
	assert.Equal(t, int64(0), metrics.ThrottledCount)
}

func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	assert.False(t, config.Enabled) // Disabled by default
	assert.Equal(t, 100, config.Rate)
	assert.Equal(t, 1*time.Second, config.Period)
	assert.Equal(t, 1, config.MaxConcurrent)
}

func TestNewProcessorRateLimiter(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          50,
		Period:        1 * time.Second,
		BurstSize:     0, // Should default to rate
		MaxConcurrent: 2,
	}

	prl := NewProcessorRateLimiter(config)

	assert.True(t, prl.IsEnabled())
	assert.Equal(t, 50, prl.config.BurstSize)
	assert.Equal(t, 2, prl.config.MaxConcurrent)
	assert.NotNil(t, prl.limiter)
}

func TestNewProcessorRateLimiter_Disabled(t *testing.T) {
	config := RateLimitConfig{
		Enabled: false,
	}

	prl := NewProcessorRateLimiter(config)

	assert.False(t, prl.IsEnabled())
	assert.Nil(t, prl.limiter)
}

func TestProcessorRateLimiter_CanExecute(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          10,
		Period:        1 * time.Second,
		BurstSize:     10,
		MaxConcurrent: 2,
	}

	prl := NewProcessorRateLimiter(config)

	// Should allow execution
	assert.True(t, prl.CanExecute())

	// Begin executions up to max concurrent
	prl.BeginExecution()
	assert.True(t, prl.CanExecute())

	prl.BeginExecution()
	assert.False(t, prl.CanExecute()) // Max concurrent reached

	// End one execution
	prl.EndExecution()
	assert.True(t, prl.CanExecute())
}

func TestProcessorRateLimiter_WaitForExecution(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          10,
		Period:        100 * time.Millisecond,
		BurstSize:     10,
		MaxConcurrent: 1,
	}

	prl := NewProcessorRateLimiter(config)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		prl.limiter.Allow()
	}

	// Should wait and succeed
	err := prl.WaitForExecution(200 * time.Millisecond)
	require.NoError(t, err)
}

func TestProcessorRateLimiter_WaitForExecution_ConcurrencyLimit(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          100,
		Period:        1 * time.Second,
		BurstSize:     100,
		MaxConcurrent: 1,
	}

	prl := NewProcessorRateLimiter(config)

	// Begin execution to hit concurrency limit
	prl.BeginExecution()

	// Should timeout waiting for concurrency slot
	err := prl.WaitForExecution(100 * time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent executions")
}

func TestProcessorRateLimiter_BeginEndExecution(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          10,
		Period:        1 * time.Second,
		BurstSize:     10,
		MaxConcurrent: 3,
	}

	prl := NewProcessorRateLimiter(config)

	assert.Equal(t, 0, prl.GetActiveCount())

	prl.BeginExecution()
	assert.Equal(t, 1, prl.GetActiveCount())

	prl.BeginExecution()
	assert.Equal(t, 2, prl.GetActiveCount())

	prl.EndExecution()
	assert.Equal(t, 1, prl.GetActiveCount())

	prl.EndExecution()
	assert.Equal(t, 0, prl.GetActiveCount())
}

func TestProcessorRateLimiter_GetMetrics(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          10,
		Period:        1 * time.Second,
		BurstSize:     10,
		MaxConcurrent: 1,
	}

	prl := NewProcessorRateLimiter(config)

	// Generate some activity
	for i := 0; i < 15; i++ {
		prl.CanExecute()
	}

	metrics := prl.GetMetrics()
	assert.True(t, metrics.RequestCount > 0)
}

func TestProcessorRateLimiter_UpdateConfig(t *testing.T) {
	config := RateLimitConfig{
		Enabled:       true,
		Rate:          10,
		Period:        1 * time.Second,
		BurstSize:     10,
		MaxConcurrent: 1,
	}

	prl := NewProcessorRateLimiter(config)

	newConfig := RateLimitConfig{
		Enabled:       true,
		Rate:          20,
		Period:        1 * time.Second,
		BurstSize:     20,
		MaxConcurrent: 2,
	}

	prl.UpdateConfig(newConfig)

	updatedConfig := prl.GetConfig()
	assert.Equal(t, 20, updatedConfig.Rate)
	assert.Equal(t, 2, updatedConfig.MaxConcurrent)
}

func TestProcessorRateLimiter_Disabled(t *testing.T) {
	config := RateLimitConfig{
		Enabled: false,
	}

	prl := NewProcessorRateLimiter(config)

	// All operations should succeed immediately when disabled
	assert.True(t, prl.CanExecute())

	err := prl.WaitForExecution(10 * time.Millisecond)
	require.NoError(t, err)

	// Begin/End execution should be no-ops
	prl.BeginExecution()
	assert.Equal(t, 0, prl.GetActiveCount())
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(10, 100*time.Millisecond, 10)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		assert.True(t, rl.Allow())
	}

	assert.Equal(t, 0, rl.GetTokens())

	// Wait for partial refill
	time.Sleep(50 * time.Millisecond)
	rl.refillTokens()

	tokens := rl.GetTokens()
	assert.True(t, tokens > 0 && tokens < 10, "expected partial refill, got %d", tokens)

	// Wait for full refill
	time.Sleep(100 * time.Millisecond)
	rl.refillTokens()

	assert.Equal(t, 10, rl.GetTokens())
}

func TestRateLimiter_BurstSize(t *testing.T) {
	// Create rate limiter with burst size larger than rate
	rl := NewRateLimiter(10, 1*time.Second, 20)

	// Should allow up to burst size
	for i := 0; i < 20; i++ {
		assert.True(t, rl.Allow(), "request %d should be allowed", i)
	}

	// Next request should be denied
	assert.False(t, rl.Allow())

	// Wait for refill - refills at rate per period (10 tokens/second)
	// After 1.1 seconds, should have refilled 11 tokens (10 + 1)
	time.Sleep(1100 * time.Millisecond)
	rl.refillTokens()

	// Should have refilled 11 tokens (10 full seconds + 0.1 second = 11)
	assert.True(t, rl.GetTokens() >= 10, "should have at least 10 tokens after 1.1s")
	assert.True(t, rl.GetTokens() <= 12, "should have at most 12 tokens after 1.1s")
}
