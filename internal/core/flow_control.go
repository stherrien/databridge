package core

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
)

// BackPressureStrategy defines how to handle back pressure
type BackPressureStrategy string

const (
	BackPressureBlock   BackPressureStrategy = "block"   // Block until space available
	BackPressureDrop    BackPressureStrategy = "drop"    // Drop oldest FlowFiles
	BackPressureFail    BackPressureStrategy = "fail"    // Return error
	BackPressurePenalty BackPressureStrategy = "penalty" // Penalize source processor
)

// BackPressureConfig configures back pressure behavior
type BackPressureConfig struct {
	Enabled         bool
	Strategy        BackPressureStrategy
	ThresholdPct    int           // Percentage of queue full to trigger
	PenaltyDuration time.Duration // For penalty strategy
}

// BackPressureMetrics tracks back pressure statistics
type BackPressureMetrics struct {
	TriggeredCount    int64 // Number of times back pressure was triggered
	DroppedCount      int64 // Number of FlowFiles dropped
	BlockedDuration   int64 // Total time spent blocked (nanoseconds)
	PenaltyCount      int64 // Number of times penalty was applied
	LastTriggered     time.Time
	mu                sync.RWMutex
}

// DefaultBackPressureConfig returns a default back pressure configuration
func DefaultBackPressureConfig() BackPressureConfig {
	return BackPressureConfig{
		Enabled:         true,
		Strategy:        BackPressureBlock,
		ThresholdPct:    80,
		PenaltyDuration: 1 * time.Second,
	}
}

// ShouldTrigger checks if back pressure should be triggered
func (c *BackPressureConfig) ShouldTrigger(currentSize, maxSize int64) bool {
	if !c.Enabled || maxSize == 0 {
		return false
	}

	percentage := (currentSize * 100) / maxSize
	return percentage >= int64(c.ThresholdPct)
}

// EnqueueWithBackPressure attempts to enqueue a FlowFile with back pressure handling
func (q *FlowFileQueue) EnqueueWithBackPressure(flowFile *types.FlowFile, config BackPressureConfig) error {
	// Fast path - no back pressure
	if !config.ShouldTrigger(q.currentSize, q.maxSize) {
		q.mu.Lock()
		defer q.mu.Unlock()

		q.flowFiles = append(q.flowFiles, flowFile)
		q.currentSize++
		return nil
	}

	// Back pressure triggered
	q.recordBackPressureTriggered()

	switch config.Strategy {
	case BackPressureBlock:
		return q.enqueueBlocking(flowFile, config)

	case BackPressureDrop:
		return q.enqueueDropOldest(flowFile)

	case BackPressureFail:
		return fmt.Errorf("back pressure triggered: queue is at %d%% capacity",
			(q.currentSize*100)/q.maxSize)

	case BackPressurePenalty:
		return q.enqueueWithPenalty(flowFile, config)

	default:
		return fmt.Errorf("unknown back pressure strategy: %s", config.Strategy)
	}
}

// enqueueBlocking blocks until space is available or timeout
func (q *FlowFileQueue) enqueueBlocking(flowFile *types.FlowFile, config BackPressureConfig) error {
	startTime := time.Now()
	timeout := time.After(30 * time.Second) // 30 second timeout
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			duration := time.Since(startTime)
			q.recordBlockedDuration(duration)
			return fmt.Errorf("back pressure timeout: queue still full after %v", duration)

		case <-ticker.C:
			q.mu.RLock()
			hasSpace := !config.ShouldTrigger(q.currentSize, q.maxSize)
			q.mu.RUnlock()

			if hasSpace {
				q.mu.Lock()
				q.flowFiles = append(q.flowFiles, flowFile)
				q.currentSize++
				q.mu.Unlock()

				duration := time.Since(startTime)
				q.recordBlockedDuration(duration)
				return nil
			}
		}
	}
}

// enqueueDropOldest drops the oldest FlowFile and enqueues the new one
func (q *FlowFileQueue) enqueueDropOldest(flowFile *types.FlowFile) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.flowFiles) == 0 {
		return fmt.Errorf("queue is empty, cannot drop oldest")
	}

	// Drop oldest FlowFile
	q.flowFiles = q.flowFiles[1:]
	q.currentSize--
	q.recordDroppedFlowFile()

	// Enqueue new FlowFile
	q.flowFiles = append(q.flowFiles, flowFile)
	q.currentSize++

	return nil
}

// enqueueWithPenalty enqueues the FlowFile but records a penalty
func (q *FlowFileQueue) enqueueWithPenalty(flowFile *types.FlowFile, config BackPressureConfig) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Still enqueue, but record penalty for the source processor
	q.flowFiles = append(q.flowFiles, flowFile)
	q.currentSize++

	q.recordPenalty()

	// Set penalty timestamp on connection for scheduler to check
	if q.connection != nil {
		q.connection.mu.Lock()
		if q.connection.lastPenaltyTime == nil {
			q.connection.lastPenaltyTime = new(time.Time)
		}
		*q.connection.lastPenaltyTime = time.Now()
		q.connection.penaltyDuration = config.PenaltyDuration
		q.connection.mu.Unlock()
	}

	return nil
}

// Metrics recording methods

func (q *FlowFileQueue) recordBackPressureTriggered() {
	if q.metrics == nil {
		q.initMetrics()
	}

	atomic.AddInt64(&q.metrics.TriggeredCount, 1)

	q.metrics.mu.Lock()
	q.metrics.LastTriggered = time.Now()
	q.metrics.mu.Unlock()
}

func (q *FlowFileQueue) recordDroppedFlowFile() {
	if q.metrics == nil {
		q.initMetrics()
	}
	atomic.AddInt64(&q.metrics.DroppedCount, 1)
}

func (q *FlowFileQueue) recordBlockedDuration(duration time.Duration) {
	if q.metrics == nil {
		q.initMetrics()
	}
	atomic.AddInt64(&q.metrics.BlockedDuration, duration.Nanoseconds())
}

func (q *FlowFileQueue) recordPenalty() {
	if q.metrics == nil {
		q.initMetrics()
	}
	atomic.AddInt64(&q.metrics.PenaltyCount, 1)
}

func (q *FlowFileQueue) initMetrics() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.metrics == nil {
		q.metrics = &BackPressureMetrics{}
	}
}

// GetBackPressureMetrics returns current back pressure metrics
func (q *FlowFileQueue) GetBackPressureMetrics() BackPressureMetrics {
	if q.metrics == nil {
		return BackPressureMetrics{}
	}

	q.metrics.mu.RLock()
	defer q.metrics.mu.RUnlock()

	return BackPressureMetrics{
		TriggeredCount:  atomic.LoadInt64(&q.metrics.TriggeredCount),
		DroppedCount:    atomic.LoadInt64(&q.metrics.DroppedCount),
		BlockedDuration: atomic.LoadInt64(&q.metrics.BlockedDuration),
		PenaltyCount:    atomic.LoadInt64(&q.metrics.PenaltyCount),
		LastTriggered:   q.metrics.LastTriggered,
	}
}

// ResetBackPressureMetrics resets the metrics
func (q *FlowFileQueue) ResetBackPressureMetrics() {
	if q.metrics == nil {
		return
	}

	atomic.StoreInt64(&q.metrics.TriggeredCount, 0)
	atomic.StoreInt64(&q.metrics.DroppedCount, 0)
	atomic.StoreInt64(&q.metrics.BlockedDuration, 0)
	atomic.StoreInt64(&q.metrics.PenaltyCount, 0)

	q.metrics.mu.Lock()
	q.metrics.LastTriggered = time.Time{}
	q.metrics.mu.Unlock()
}

// GetBackPressureStatus returns human-readable back pressure status
func (q *FlowFileQueue) GetBackPressureStatus(config BackPressureConfig) string {
	q.mu.RLock()
	currentSize := q.currentSize
	maxSize := q.maxSize
	q.mu.RUnlock()

	if maxSize == 0 {
		return "N/A"
	}

	percentage := (currentSize * 100) / maxSize

	if !config.Enabled {
		return fmt.Sprintf("Disabled (%d%%)", percentage)
	}

	if config.ShouldTrigger(currentSize, maxSize) {
		return fmt.Sprintf("Active (%d%%, strategy: %s)", percentage, config.Strategy)
	}

	return fmt.Sprintf("Normal (%d%%)", percentage)
}

// IsPenalized checks if a connection is currently penalized
func (c *Connection) IsPenalized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastPenaltyTime == nil {
		return false
	}

	penaltyEnd := c.lastPenaltyTime.Add(c.penaltyDuration)
	return time.Now().Before(penaltyEnd)
}

// GetPenaltyRemaining returns the remaining penalty duration
func (c *Connection) GetPenaltyRemaining() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastPenaltyTime == nil {
		return 0
	}

	penaltyEnd := c.lastPenaltyTime.Add(c.penaltyDuration)
	remaining := time.Until(penaltyEnd)

	if remaining < 0 {
		return 0
	}

	return remaining
}

// ClearPenalty clears any active penalty
func (c *Connection) ClearPenalty() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastPenaltyTime = nil
	c.penaltyDuration = 0
}

// ConnectionBackPressureInfo contains back pressure information for a connection
type ConnectionBackPressureInfo struct {
	ConnectionID      string
	ConnectionName    string
	CurrentSize       int64
	MaxSize           int64
	PercentageFull    int64
	BackPressureStatus string
	IsPenalized       bool
	PenaltyRemaining  time.Duration
	Metrics           BackPressureMetrics
}

// GetBackPressureInfo returns comprehensive back pressure information
func (c *Connection) GetBackPressureInfo(config BackPressureConfig) ConnectionBackPressureInfo {
	c.mu.RLock()
	queue := c.Queue
	c.mu.RUnlock()

	if queue == nil {
		return ConnectionBackPressureInfo{
			ConnectionID:   c.ID.String(),
			ConnectionName: c.Name,
		}
	}

	queue.mu.RLock()
	currentSize := queue.currentSize
	maxSize := queue.maxSize
	queue.mu.RUnlock()

	var percentageFull int64
	if maxSize > 0 {
		percentageFull = (currentSize * 100) / maxSize
	}

	return ConnectionBackPressureInfo{
		ConnectionID:       c.ID.String(),
		ConnectionName:     c.Name,
		CurrentSize:        currentSize,
		MaxSize:            maxSize,
		PercentageFull:     percentageFull,
		BackPressureStatus: queue.GetBackPressureStatus(config),
		IsPenalized:        c.IsPenalized(),
		PenaltyRemaining:   c.GetPenaltyRemaining(),
		Metrics:            queue.GetBackPressureMetrics(),
	}
}
