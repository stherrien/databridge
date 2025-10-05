package core

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// RetryPolicy defines retry behavior for failed FlowFiles
type RetryPolicy struct {
	MaxAttempts         int
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	BackoffMultiplier   float64
	RetryableErrors     []string       // Error patterns that should retry
	RetryableErrorRegex []*regexp.Regexp
	PenaltyDuration     time.Duration  // Penalization for repeated failures
	UseJitter           bool           // Add random jitter to delays
	mu                  sync.RWMutex
	metrics             *RetryMetrics
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	Enabled             bool
	MaxAttempts         int
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	BackoffMultiplier   float64
	RetryableErrors     []string
	PenaltyDuration     time.Duration
	UseJitter           bool
}

// RetryMetrics tracks retry statistics
// RetryMetricsSnapshot is a point-in-time snapshot of retry metrics
type RetryMetricsSnapshot struct {
	TotalRetries      int64
	SuccessfulRetries int64
	FailedRetries     int64
	ExhaustedRetries  int64 // Retries that hit max attempts
	TotalRetryTime    int64 // Total time spent retrying (nanoseconds)
	LastRetry         time.Time
}

type RetryMetrics struct {
	TotalRetries      int64
	SuccessfulRetries int64
	FailedRetries     int64
	ExhaustedRetries  int64 // Retries that hit max attempts
	TotalRetryTime    int64 // Total time spent retrying (nanoseconds)
	LastRetry         time.Time
	mu                sync.RWMutex
}

// RetryableRelationship indicates a failure that can be retried
var RetryableRelationship = types.Relationship{
	Name:        "retry",
	Description: "FlowFiles that failed but can be retried",
}

// NewRetryPolicy creates a new retry policy
func NewRetryPolicy(config RetryConfig) (*RetryPolicy, error) {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 1 * time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 5 * time.Minute
	}
	if config.BackoffMultiplier <= 0 {
		config.BackoffMultiplier = 2.0
	}

	policy := &RetryPolicy{
		MaxAttempts:       config.MaxAttempts,
		InitialDelay:      config.InitialDelay,
		MaxDelay:          config.MaxDelay,
		BackoffMultiplier: config.BackoffMultiplier,
		RetryableErrors:   config.RetryableErrors,
		PenaltyDuration:   config.PenaltyDuration,
		UseJitter:         config.UseJitter,
		metrics:           &RetryMetrics{},
	}

	// Compile error patterns
	if len(config.RetryableErrors) > 0 {
		policy.RetryableErrorRegex = make([]*regexp.Regexp, 0, len(config.RetryableErrors))
		for _, pattern := range config.RetryableErrors {
			regex, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid error pattern '%s': %w", pattern, err)
			}
			policy.RetryableErrorRegex = append(policy.RetryableErrorRegex, regex)
		}
	}

	return policy, nil
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		Enabled:           false, // Disabled by default
		MaxAttempts:       3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          5 * time.Minute,
		BackoffMultiplier: 2.0,
		RetryableErrors: []string{
			".*timeout.*",
			".*connection.*refused.*",
			".*temporary.*failure.*",
		},
		PenaltyDuration: 30 * time.Second,
		UseJitter:       true,
	}
}

// ShouldRetry determines if an error should trigger a retry
func (rp *RetryPolicy) ShouldRetry(err error, attemptCount int) bool {
	if err == nil {
		return false
	}

	if attemptCount >= rp.MaxAttempts {
		return false
	}

	// If no patterns specified, retry all errors
	if len(rp.RetryableErrorRegex) == 0 {
		return true
	}

	// Check if error matches any retryable pattern
	errMsg := strings.ToLower(err.Error())
	for _, regex := range rp.RetryableErrorRegex {
		if regex.MatchString(errMsg) {
			return true
		}
	}

	return false
}

// CalculateDelay calculates the delay before the next retry attempt
func (rp *RetryPolicy) CalculateDelay(attemptCount int) time.Duration {
	if attemptCount <= 0 {
		return rp.InitialDelay
	}

	// Exponential backoff: initialDelay * (multiplier ^ attempt)
	delay := float64(rp.InitialDelay)
	for i := 0; i < attemptCount; i++ {
		delay *= rp.BackoffMultiplier
	}

	duration := time.Duration(delay)

	// Cap at max delay
	if duration > rp.MaxDelay {
		duration = rp.MaxDelay
	}

	// Add jitter if enabled (±25% random variance)
	if rp.UseJitter {
		jitterRange := float64(duration) * 0.25
		jitter := time.Duration(float64(time.Now().UnixNano()%int64(jitterRange*2)) - jitterRange)
		duration += jitter

		// Ensure non-negative
		if duration < 0 {
			duration = rp.InitialDelay
		}
	}

	return duration
}

// RecordRetryAttempt records a retry attempt
func (rp *RetryPolicy) RecordRetryAttempt(success bool, duration time.Duration) {
	atomic.AddInt64(&rp.metrics.TotalRetries, 1)
	atomic.AddInt64(&rp.metrics.TotalRetryTime, duration.Nanoseconds())

	if success {
		atomic.AddInt64(&rp.metrics.SuccessfulRetries, 1)
	} else {
		atomic.AddInt64(&rp.metrics.FailedRetries, 1)
	}

	rp.metrics.mu.Lock()
	rp.metrics.LastRetry = time.Now()
	rp.metrics.mu.Unlock()
}

// RecordExhaustedRetry records a retry that hit max attempts
func (rp *RetryPolicy) RecordExhaustedRetry() {
	atomic.AddInt64(&rp.metrics.ExhaustedRetries, 1)
}

// GetMetrics returns current retry metrics
func (rp *RetryPolicy) GetMetrics() RetryMetricsSnapshot {
	rp.metrics.mu.RLock()
	defer rp.metrics.mu.RUnlock()

	return RetryMetricsSnapshot{
		TotalRetries:      atomic.LoadInt64(&rp.metrics.TotalRetries),
		SuccessfulRetries: atomic.LoadInt64(&rp.metrics.SuccessfulRetries),
		FailedRetries:     atomic.LoadInt64(&rp.metrics.FailedRetries),
		ExhaustedRetries:  atomic.LoadInt64(&rp.metrics.ExhaustedRetries),
		TotalRetryTime:    atomic.LoadInt64(&rp.metrics.TotalRetryTime),
		LastRetry:         rp.metrics.LastRetry,
	}
}

// ResetMetrics resets all retry metrics
func (rp *RetryPolicy) ResetMetrics() {
	atomic.StoreInt64(&rp.metrics.TotalRetries, 0)
	atomic.StoreInt64(&rp.metrics.SuccessfulRetries, 0)
	atomic.StoreInt64(&rp.metrics.FailedRetries, 0)
	atomic.StoreInt64(&rp.metrics.ExhaustedRetries, 0)
	atomic.StoreInt64(&rp.metrics.TotalRetryTime, 0)

	rp.metrics.mu.Lock()
	rp.metrics.LastRetry = time.Time{}
	rp.metrics.mu.Unlock()
}

// FlowFileRetryState tracks retry state for a FlowFile
type FlowFileRetryState struct {
	FlowFileID    uuid.UUID
	AttemptCount  int
	FirstAttempt  time.Time
	LastAttempt   time.Time
	LastError     string
	NextRetryTime time.Time
	IsPenalized   bool
	PenaltyUntil  time.Time
}

// RetryQueue manages FlowFiles awaiting retry
type RetryQueue struct {
	mu           sync.RWMutex
	items        map[uuid.UUID]*RetryQueueItem
	policy       *RetryPolicy
	maxQueueSize int
}

// RetryQueueItem represents a FlowFile in the retry queue
type RetryQueueItem struct {
	FlowFile  *types.FlowFile
	State     *FlowFileRetryState
	EnqueuedAt time.Time
}

// NewRetryQueue creates a new retry queue
func NewRetryQueue(policy *RetryPolicy, maxQueueSize int) *RetryQueue {
	if maxQueueSize <= 0 {
		maxQueueSize = 1000
	}

	return &RetryQueue{
		items:        make(map[uuid.UUID]*RetryQueueItem),
		policy:       policy,
		maxQueueSize: maxQueueSize,
	}
}

// Enqueue adds a FlowFile to the retry queue
func (rq *RetryQueue) Enqueue(flowFile *types.FlowFile, err error) error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if len(rq.items) >= rq.maxQueueSize {
		return fmt.Errorf("retry queue is full")
	}

	// Get or create retry state
	state := rq.getOrCreateState(flowFile)

	// Check if should retry
	if !rq.policy.ShouldRetry(err, state.AttemptCount) {
		rq.policy.RecordExhaustedRetry()
		return fmt.Errorf("retry exhausted for FlowFile %s", flowFile.ID)
	}

	state.AttemptCount++
	state.LastAttempt = time.Now()
	state.LastError = err.Error()

	// Calculate next retry time
	delay := rq.policy.CalculateDelay(state.AttemptCount - 1)
	state.NextRetryTime = time.Now().Add(delay)

	// Check if should penalize
	if state.AttemptCount > 1 && rq.policy.PenaltyDuration > 0 {
		state.IsPenalized = true
		state.PenaltyUntil = time.Now().Add(rq.policy.PenaltyDuration)
	}

	item := &RetryQueueItem{
		FlowFile:   flowFile,
		State:      state,
		EnqueuedAt: time.Now(),
	}

	rq.items[flowFile.ID] = item

	// Update FlowFile attributes to track retry
	if flowFile.Attributes == nil {
		flowFile.Attributes = make(map[string]string)
	}
	flowFile.Attributes["retry.count"] = fmt.Sprintf("%d", state.AttemptCount)
	flowFile.Attributes["retry.last.error"] = state.LastError
	flowFile.Attributes["retry.next.time"] = state.NextRetryTime.Format(time.RFC3339)

	return nil
}

// Dequeue retrieves FlowFiles ready for retry
func (rq *RetryQueue) Dequeue(maxItems int) []*types.FlowFile {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	now := time.Now()
	ready := make([]*types.FlowFile, 0, maxItems)

	for id, item := range rq.items {
		if len(ready) >= maxItems {
			break
		}

		// Check if ready for retry
		if now.After(item.State.NextRetryTime) {
			// Check if still penalized
			if item.State.IsPenalized && now.Before(item.State.PenaltyUntil) {
				continue
			}

			ready = append(ready, item.FlowFile)
			delete(rq.items, id)
		}
	}

	return ready
}

// Remove removes a FlowFile from the retry queue
func (rq *RetryQueue) Remove(flowFileID uuid.UUID) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	delete(rq.items, flowFileID)
}

// Size returns the current queue size
func (rq *RetryQueue) Size() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	return len(rq.items)
}

// Clear removes all items from the queue
func (rq *RetryQueue) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	rq.items = make(map[uuid.UUID]*RetryQueueItem)
}

// GetState retrieves retry state for a FlowFile
func (rq *RetryQueue) GetState(flowFileID uuid.UUID) (*FlowFileRetryState, bool) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	item, exists := rq.items[flowFileID]
	if !exists {
		return nil, false
	}

	return item.State, true
}

// getOrCreateState gets existing state or creates new one (must be called with lock held)
func (rq *RetryQueue) getOrCreateState(flowFile *types.FlowFile) *FlowFileRetryState {
	// Check if already in queue
	if item, exists := rq.items[flowFile.ID]; exists {
		return item.State
	}

	// Create new state
	return &FlowFileRetryState{
		FlowFileID:   flowFile.ID,
		AttemptCount: 0,
		FirstAttempt: time.Now(),
	}
}

// GetReadyCount returns the number of FlowFiles ready for retry
func (rq *RetryQueue) GetReadyCount() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	now := time.Now()
	count := 0

	for _, item := range rq.items {
		if now.After(item.State.NextRetryTime) {
			if !item.State.IsPenalized || now.After(item.State.PenaltyUntil) {
				count++
			}
		}
	}

	return count
}

// GetPenalizedCount returns the number of penalized FlowFiles
func (rq *RetryQueue) GetPenalizedCount() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	now := time.Now()
	count := 0

	for _, item := range rq.items {
		if item.State.IsPenalized && now.Before(item.State.PenaltyUntil) {
			count++
		}
	}

	return count
}

// RetryQueueInfo contains information about the retry queue
type RetryQueueInfo struct {
	TotalItems      int
	ReadyItems      int
	PenalizedItems  int
	MaxQueueSize    int
	OldestItemAge   time.Duration
	Policy          RetryConfig
	Metrics         RetryMetricsSnapshot
}

// GetInfo returns comprehensive retry queue information
func (rq *RetryQueue) GetInfo() RetryQueueInfo {
	rq.mu.RLock()
	defer rq.mu.RUnlock()

	info := RetryQueueInfo{
		TotalItems:     len(rq.items),
		ReadyItems:     rq.GetReadyCount(),
		PenalizedItems: rq.GetPenalizedCount(),
		MaxQueueSize:   rq.maxQueueSize,
		Metrics:        rq.policy.GetMetrics(),
	}

	// Find oldest item
	var oldestTime time.Time
	for _, item := range rq.items {
		if oldestTime.IsZero() || item.EnqueuedAt.Before(oldestTime) {
			oldestTime = item.EnqueuedAt
		}
	}

	if !oldestTime.IsZero() {
		info.OldestItemAge = time.Since(oldestTime)
	}

	return info
}
