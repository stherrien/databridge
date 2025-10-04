package core

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRetryPolicy(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       5,
		InitialDelay:      1 * time.Second,
		MaxDelay:          1 * time.Minute,
		BackoffMultiplier: 2.0,
		RetryableErrors:   []string{".*timeout.*", ".*connection.*"},
		PenaltyDuration:   30 * time.Second,
		UseJitter:         true,
	}

	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	assert.Equal(t, 5, policy.MaxAttempts)
	assert.Equal(t, 1*time.Second, policy.InitialDelay)
	assert.Equal(t, 1*time.Minute, policy.MaxDelay)
	assert.Equal(t, 2.0, policy.BackoffMultiplier)
	assert.Len(t, policy.RetryableErrorRegex, 2)
	assert.True(t, policy.UseJitter)
}

func TestNewRetryPolicy_InvalidRegex(t *testing.T) {
	config := RetryConfig{
		RetryableErrors: []string{"[invalid regex"},
	}

	_, err := NewRetryPolicy(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid error pattern")
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.False(t, config.Enabled) // Disabled by default
	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 5*time.Minute, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
	assert.Equal(t, 30*time.Second, config.PenaltyDuration)
	assert.True(t, config.UseJitter)
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       3,
		RetryableErrors:   []string{".*timeout.*", ".*connection.*refused.*"},
	}

	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	tests := []struct {
		name         string
		err          error
		attemptCount int
		expected     bool
	}{
		{
			name:         "nil error",
			err:          nil,
			attemptCount: 0,
			expected:     false,
		},
		{
			name:         "retryable error, first attempt",
			err:          errors.New("connection timeout"),
			attemptCount: 0,
			expected:     true,
		},
		{
			name:         "retryable error, max attempts reached",
			err:          errors.New("connection timeout"),
			attemptCount: 3,
			expected:     false,
		},
		{
			name:         "matching pattern - timeout",
			err:          errors.New("operation timeout"),
			attemptCount: 1,
			expected:     true,
		},
		{
			name:         "matching pattern - connection refused",
			err:          errors.New("connection refused by server"),
			attemptCount: 1,
			expected:     true,
		},
		{
			name:         "non-matching pattern",
			err:          errors.New("invalid input"),
			attemptCount: 1,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := policy.ShouldRetry(tt.err, tt.attemptCount)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRetryPolicy_ShouldRetry_NoPatterns(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		RetryableErrors: []string{}, // No patterns - retry all
	}

	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	// Should retry any error when no patterns specified
	assert.True(t, policy.ShouldRetry(errors.New("any error"), 0))
	assert.True(t, policy.ShouldRetry(errors.New("another error"), 1))
	assert.False(t, policy.ShouldRetry(errors.New("any error"), 3)) // Max attempts
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	config := RetryConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
		UseJitter:         false, // Disable jitter for predictable tests
	}

	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	tests := []struct {
		name         string
		attemptCount int
		expected     time.Duration
	}{
		{
			name:         "first attempt",
			attemptCount: 0,
			expected:     1 * time.Second,
		},
		{
			name:         "second attempt",
			attemptCount: 1,
			expected:     2 * time.Second,
		},
		{
			name:         "third attempt",
			attemptCount: 2,
			expected:     4 * time.Second,
		},
		{
			name:         "fourth attempt",
			attemptCount: 3,
			expected:     8 * time.Second,
		},
		{
			name:         "capped at max delay",
			attemptCount: 5,
			expected:     10 * time.Second, // Capped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := policy.CalculateDelay(tt.attemptCount)
			assert.Equal(t, tt.expected, delay)
		})
	}
}

func TestRetryPolicy_CalculateDelay_WithJitter(t *testing.T) {
	config := RetryConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
		UseJitter:         true,
	}

	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	// Calculate delay multiple times to test jitter variability
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = policy.CalculateDelay(2)
	}

	// With jitter, we should see variation (not all values identical)
	// Base delay for attempt 2 is 4s, jitter is ±25% (3s to 5s range)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	// With jitter, delays should vary
	// Note: There's a small chance this could fail if random values happen to be identical
	assert.False(t, allSame, "expected delays to vary with jitter")

	// All delays should be positive
	for _, delay := range delays {
		assert.True(t, delay > 0)
	}
}

func TestRetryPolicy_RecordRetryAttempt(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	// Record successful retry
	policy.RecordRetryAttempt(true, 100*time.Millisecond)

	metrics := policy.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalRetries)
	assert.Equal(t, int64(1), metrics.SuccessfulRetries)
	assert.Equal(t, int64(0), metrics.FailedRetries)

	// Record failed retry
	policy.RecordRetryAttempt(false, 200*time.Millisecond)

	metrics = policy.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalRetries)
	assert.Equal(t, int64(1), metrics.SuccessfulRetries)
	assert.Equal(t, int64(1), metrics.FailedRetries)
	assert.True(t, metrics.TotalRetryTime > 0)
}

func TestRetryPolicy_GetMetrics(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	policy.RecordRetryAttempt(true, 100*time.Millisecond)
	policy.RecordRetryAttempt(false, 100*time.Millisecond)
	policy.RecordExhaustedRetry()

	metrics := policy.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalRetries)
	assert.Equal(t, int64(1), metrics.SuccessfulRetries)
	assert.Equal(t, int64(1), metrics.FailedRetries)
	assert.Equal(t, int64(1), metrics.ExhaustedRetries)
	assert.False(t, metrics.LastRetry.IsZero())
}

func TestRetryPolicy_ResetMetrics(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	policy.RecordRetryAttempt(true, 100*time.Millisecond)
	policy.RecordExhaustedRetry()

	// Reset
	policy.ResetMetrics()

	metrics := policy.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalRetries)
	assert.Equal(t, int64(0), metrics.SuccessfulRetries)
	assert.Equal(t, int64(0), metrics.FailedRetries)
	assert.Equal(t, int64(0), metrics.ExhaustedRetries)
	assert.True(t, metrics.LastRetry.IsZero())
}

func TestNewRetryQueue(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 1000)

	assert.Equal(t, 0, rq.Size())
	assert.Equal(t, 1000, rq.maxQueueSize)
}

func TestRetryQueue_Enqueue(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()
	err = rq.Enqueue(ff, errors.New("test error"))
	require.NoError(t, err)

	assert.Equal(t, 1, rq.Size())

	// Check FlowFile attributes were updated
	assert.Equal(t, "1", ff.Attributes["retry.count"])
	assert.Equal(t, "test error", ff.Attributes["retry.last.error"])
	assert.NotEmpty(t, ff.Attributes["retry.next.time"])
}

func TestRetryQueue_Enqueue_MaxQueueSize(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 2) // Very small queue

	// Fill queue
	ff1 := types.NewFlowFile()
	err = rq.Enqueue(ff1, errors.New("error 1"))
	require.NoError(t, err)

	ff2 := types.NewFlowFile()
	err = rq.Enqueue(ff2, errors.New("error 2"))
	require.NoError(t, err)

	// Should fail when full
	ff3 := types.NewFlowFile()
	err = rq.Enqueue(ff3, errors.New("error 3"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queue is full")
}

func TestRetryQueue_Enqueue_ExhaustedRetries(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     2,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()

	// First enqueue
	err = rq.Enqueue(ff, errors.New("error 1"))
	require.NoError(t, err)
	rq.Remove(ff.ID)

	// Second enqueue
	err = rq.Enqueue(ff, errors.New("error 2"))
	require.NoError(t, err)
	rq.Remove(ff.ID)

	// Third enqueue should fail (exhausted)
	err = rq.Enqueue(ff, errors.New("error 3"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry exhausted")
}

func TestRetryQueue_Dequeue(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    50 * time.Millisecond,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()
	err = rq.Enqueue(ff, errors.New("test error"))
	require.NoError(t, err)

	// Immediate dequeue should return empty (not ready yet)
	ready := rq.Dequeue(10)
	assert.Empty(t, ready)

	// Wait for retry delay
	time.Sleep(100 * time.Millisecond)

	// Should dequeue now
	ready = rq.Dequeue(10)
	require.Len(t, ready, 1)
	assert.Equal(t, ff.ID, ready[0].ID)

	// Queue should be empty
	assert.Equal(t, 0, rq.Size())
}

func TestRetryQueue_Dequeue_MaxItems(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    10 * time.Millisecond,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	// Enqueue multiple items
	for i := 0; i < 5; i++ {
		ff := types.NewFlowFile()
		rq.Enqueue(ff, errors.New(fmt.Sprintf("error %d", i)))
	}

	// Wait for all to be ready
	time.Sleep(50 * time.Millisecond)

	// Dequeue with limit
	ready := rq.Dequeue(3)
	assert.Len(t, ready, 3)

	// Should have 2 remaining
	assert.Equal(t, 2, rq.Size())
}

func TestRetryQueue_Remove(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()
	err = rq.Enqueue(ff, errors.New("test error"))
	require.NoError(t, err)

	assert.Equal(t, 1, rq.Size())

	rq.Remove(ff.ID)
	assert.Equal(t, 0, rq.Size())
}

func TestRetryQueue_Clear(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	// Add multiple items
	for i := 0; i < 5; i++ {
		ff := types.NewFlowFile()
		rq.Enqueue(ff, errors.New(fmt.Sprintf("error %d", i)))
	}

	assert.Equal(t, 5, rq.Size())

	rq.Clear()
	assert.Equal(t, 0, rq.Size())
}

func TestRetryQueue_GetState(t *testing.T) {
	config := DefaultRetryConfig()
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()
	err = rq.Enqueue(ff, errors.New("test error"))
	require.NoError(t, err)

	state, exists := rq.GetState(ff.ID)
	require.True(t, exists)
	assert.Equal(t, ff.ID, state.FlowFileID)
	assert.Equal(t, 1, state.AttemptCount)
	assert.Equal(t, "test error", state.LastError)
}

func TestRetryQueue_GetReadyCount(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    50 * time.Millisecond,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	// Add items
	for i := 0; i < 3; i++ {
		ff := types.NewFlowFile()
		rq.Enqueue(ff, errors.New(fmt.Sprintf("error %d", i)))
	}

	// None should be ready immediately
	assert.Equal(t, 0, rq.GetReadyCount())

	// Wait for retry delay
	time.Sleep(100 * time.Millisecond)

	// All should be ready now
	assert.Equal(t, 3, rq.GetReadyCount())
}

func TestRetryQueue_GetPenalizedCount(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    10 * time.Millisecond,
		PenaltyDuration: 100 * time.Millisecond,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	ff := types.NewFlowFile()

	// First attempt - not penalized
	err = rq.Enqueue(ff, errors.New("error 1"))
	require.NoError(t, err)
	assert.Equal(t, 0, rq.GetPenalizedCount())

	rq.Remove(ff.ID)

	// Second attempt - should be penalized
	err = rq.Enqueue(ff, errors.New("error 2"))
	require.NoError(t, err)
	assert.Equal(t, 1, rq.GetPenalizedCount())

	// Wait for penalty to expire
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 0, rq.GetPenalizedCount())
}

func TestRetryQueue_GetInfo(t *testing.T) {
	config := RetryConfig{
		InitialDelay:    10 * time.Millisecond,
		RetryableErrors: []string{".*"},
	}
	policy, err := NewRetryPolicy(config)
	require.NoError(t, err)

	rq := NewRetryQueue(policy, 100)

	// Add items
	for i := 0; i < 3; i++ {
		ff := types.NewFlowFile()
		rq.Enqueue(ff, errors.New(fmt.Sprintf("error %d", i)))
	}

	info := rq.GetInfo()
	assert.Equal(t, 3, info.TotalItems)
	assert.Equal(t, 100, info.MaxQueueSize)
	assert.True(t, info.OldestItemAge > 0)
}

func TestRetryableRelationship(t *testing.T) {
	assert.Equal(t, "retry", RetryableRelationship.Name)
	assert.Contains(t, RetryableRelationship.Description, "retried")
}
