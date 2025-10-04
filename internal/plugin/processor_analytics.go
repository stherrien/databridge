package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// ProcessorAnalytics tracks usage metrics and performance for processors
type ProcessorAnalytics struct {
	mu                sync.RWMutex
	processorMetrics  map[string]*ProcessorMetrics
	instanceMetrics   map[uuid.UUID]*InstanceMetrics
	aggregateMetrics  *AggregateMetrics
	retentionPeriod   time.Duration
	logger            types.Logger
}

// ProcessorMetrics tracks metrics for a processor type
type ProcessorMetrics struct {
	ProcessorType     string
	TotalInvocations  int64
	TotalFlowFiles    int64
	TotalBytes        int64
	TotalSuccesses    int64
	TotalFailures     int64
	TotalErrors       int64
	AverageExecTime   time.Duration
	MinExecTime       time.Duration
	MaxExecTime       time.Duration
	LastInvoked       time.Time
	FirstInvoked      time.Time
	ActiveInstances   int
	execTimes         []time.Duration // For calculating percentiles
}

// InstanceMetrics tracks metrics for a specific processor instance
type InstanceMetrics struct {
	InstanceID        uuid.UUID
	ProcessorType     string
	ProcessorName     string
	TotalInvocations  int64
	TotalFlowFiles    int64
	TotalBytes        int64
	TotalSuccesses    int64
	TotalFailures     int64
	TotalErrors       int64
	AverageExecTime   time.Duration
	MinExecTime       time.Duration
	MaxExecTime       time.Duration
	LastInvoked       time.Time
	CreatedAt         time.Time
	execTimes         []time.Duration
	errorMessages     []string
	recentInvocations []InvocationRecord
}

// InvocationRecord records a single invocation
type InvocationRecord struct {
	Timestamp      time.Time
	Duration       time.Duration
	FlowFilesIn    int
	FlowFilesOut   int
	BytesProcessed int64
	Success        bool
	Error          string
}

// AggregateMetrics tracks overall system metrics
type AggregateMetrics struct {
	TotalProcessors   int
	TotalInstances    int
	TotalInvocations  int64
	TotalFlowFiles    int64
	TotalBytes        int64
	AverageExecTime   time.Duration
	SystemStartTime   time.Time
	LastUpdateTime    time.Time
}

// PerformanceStats provides statistical analysis
type PerformanceStats struct {
	ProcessorType   string
	Mean            time.Duration
	Median          time.Duration
	P95             time.Duration
	P99             time.Duration
	StdDev          time.Duration
	ThroughputPerSec float64
	ErrorRate       float64
	SuccessRate     float64
}

// NewProcessorAnalytics creates a new analytics tracker
func NewProcessorAnalytics(retentionPeriod time.Duration, logger types.Logger) *ProcessorAnalytics {
	return &ProcessorAnalytics{
		processorMetrics: make(map[string]*ProcessorMetrics),
		instanceMetrics:  make(map[uuid.UUID]*InstanceMetrics),
		aggregateMetrics: &AggregateMetrics{
			SystemStartTime: time.Now(),
		},
		retentionPeriod: retentionPeriod,
		logger:          logger,
	}
}

// RegisterInstance registers a new processor instance
func (a *ProcessorAnalytics) RegisterInstance(instanceID uuid.UUID, processorType, processorName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Initialize processor metrics if not exists
	if _, exists := a.processorMetrics[processorType]; !exists {
		a.processorMetrics[processorType] = &ProcessorMetrics{
			ProcessorType: processorType,
			FirstInvoked:  time.Now(),
			MinExecTime:   time.Duration(1<<63 - 1), // Max duration
			execTimes:     make([]time.Duration, 0, 1000),
		}
		a.aggregateMetrics.TotalProcessors++
	}

	// Initialize instance metrics
	a.instanceMetrics[instanceID] = &InstanceMetrics{
		InstanceID:        instanceID,
		ProcessorType:     processorType,
		ProcessorName:     processorName,
		CreatedAt:         time.Now(),
		MinExecTime:       time.Duration(1<<63 - 1),
		execTimes:         make([]time.Duration, 0, 100),
		errorMessages:     make([]string, 0, 10),
		recentInvocations: make([]InvocationRecord, 0, 50),
	}

	a.processorMetrics[processorType].ActiveInstances++
	a.aggregateMetrics.TotalInstances++

	a.logger.Debug("Registered processor instance",
		"instanceId", instanceID,
		"type", processorType,
		"name", processorName)
}

// UnregisterInstance removes a processor instance
func (a *ProcessorAnalytics) UnregisterInstance(instanceID uuid.UUID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if instance, exists := a.instanceMetrics[instanceID]; exists {
		if proc, exists := a.processorMetrics[instance.ProcessorType]; exists {
			proc.ActiveInstances--
		}
		delete(a.instanceMetrics, instanceID)
		a.aggregateMetrics.TotalInstances--
	}
}

// RecordInvocation records a processor invocation
func (a *ProcessorAnalytics) RecordInvocation(
	instanceID uuid.UUID,
	duration time.Duration,
	flowFilesIn, flowFilesOut int,
	bytesProcessed int64,
	success bool,
	err error,
) {
	a.mu.Lock()
	defer a.mu.Unlock()

	instance, exists := a.instanceMetrics[instanceID]
	if !exists {
		a.logger.Warn("Instance not registered", "instanceId", instanceID)
		return
	}

	proc := a.processorMetrics[instance.ProcessorType]

	// Create invocation record
	record := InvocationRecord{
		Timestamp:      time.Now(),
		Duration:       duration,
		FlowFilesIn:    flowFilesIn,
		FlowFilesOut:   flowFilesOut,
		BytesProcessed: bytesProcessed,
		Success:        success,
	}
	if err != nil {
		record.Error = err.Error()
	}

	// Update instance metrics
	instance.TotalInvocations++
	instance.TotalFlowFiles += int64(flowFilesIn)
	instance.TotalBytes += bytesProcessed
	instance.LastInvoked = record.Timestamp

	if success {
		instance.TotalSuccesses++
	} else {
		instance.TotalFailures++
	}

	if err != nil {
		instance.TotalErrors++
		if len(instance.errorMessages) < 10 {
			instance.errorMessages = append(instance.errorMessages, err.Error())
		}
	}

	// Update execution times
	instance.execTimes = append(instance.execTimes, duration)
	if len(instance.execTimes) > 100 {
		instance.execTimes = instance.execTimes[1:]
	}

	if duration < instance.MinExecTime {
		instance.MinExecTime = duration
	}
	if duration > instance.MaxExecTime {
		instance.MaxExecTime = duration
	}

	// Recalculate average
	instance.AverageExecTime = calculateAverage(instance.execTimes)

	// Store recent invocations
	instance.recentInvocations = append(instance.recentInvocations, record)
	if len(instance.recentInvocations) > 50 {
		instance.recentInvocations = instance.recentInvocations[1:]
	}

	// Update processor metrics
	proc.TotalInvocations++
	proc.TotalFlowFiles += int64(flowFilesIn)
	proc.TotalBytes += bytesProcessed
	proc.LastInvoked = record.Timestamp

	if success {
		proc.TotalSuccesses++
	} else {
		proc.TotalFailures++
	}

	if err != nil {
		proc.TotalErrors++
	}

	proc.execTimes = append(proc.execTimes, duration)
	if len(proc.execTimes) > 1000 {
		proc.execTimes = proc.execTimes[1:]
	}

	if duration < proc.MinExecTime {
		proc.MinExecTime = duration
	}
	if duration > proc.MaxExecTime {
		proc.MaxExecTime = duration
	}

	proc.AverageExecTime = calculateAverage(proc.execTimes)

	// Update aggregate metrics
	a.aggregateMetrics.TotalInvocations++
	a.aggregateMetrics.TotalFlowFiles += int64(flowFilesIn)
	a.aggregateMetrics.TotalBytes += bytesProcessed
	a.aggregateMetrics.LastUpdateTime = time.Now()

	// Recalculate aggregate average
	var totalDurations time.Duration
	var count int
	for _, p := range a.processorMetrics {
		if len(p.execTimes) > 0 {
			totalDurations += p.AverageExecTime
			count++
		}
	}
	if count > 0 {
		a.aggregateMetrics.AverageExecTime = totalDurations / time.Duration(count)
	}
}

// GetProcessorMetrics retrieves metrics for a processor type
func (a *ProcessorAnalytics) GetProcessorMetrics(processorType string) (*ProcessorMetrics, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics, exists := a.processorMetrics[processorType]
	if !exists {
		return nil, fmt.Errorf("no metrics found for processor type: %s", processorType)
	}

	// Return a copy
	copy := *metrics
	return &copy, nil
}

// GetInstanceMetrics retrieves metrics for a processor instance
func (a *ProcessorAnalytics) GetInstanceMetrics(instanceID uuid.UUID) (*InstanceMetrics, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics, exists := a.instanceMetrics[instanceID]
	if !exists {
		return nil, fmt.Errorf("no metrics found for instance: %s", instanceID)
	}

	// Return a copy
	copy := *metrics
	return &copy, nil
}

// GetAggregateMetrics retrieves overall system metrics
func (a *ProcessorAnalytics) GetAggregateMetrics() *AggregateMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	copy := *a.aggregateMetrics
	copy.TotalProcessors = len(a.processorMetrics)
	copy.TotalInstances = len(a.instanceMetrics)
	return &copy
}

// GetPerformanceStats calculates performance statistics
func (a *ProcessorAnalytics) GetPerformanceStats(processorType string) (*PerformanceStats, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics, exists := a.processorMetrics[processorType]
	if !exists {
		return nil, fmt.Errorf("no metrics found for processor type: %s", processorType)
	}

	if len(metrics.execTimes) == 0 {
		return &PerformanceStats{ProcessorType: processorType}, nil
	}

	stats := &PerformanceStats{
		ProcessorType: processorType,
		Mean:          metrics.AverageExecTime,
	}

	// Calculate median, p95, p99
	sortedTimes := make([]time.Duration, len(metrics.execTimes))
	copy(sortedTimes, metrics.execTimes)
	sortDurations(sortedTimes)

	stats.Median = sortedTimes[len(sortedTimes)/2]
	stats.P95 = sortedTimes[int(float64(len(sortedTimes))*0.95)]
	stats.P99 = sortedTimes[int(float64(len(sortedTimes))*0.99)]

	// Calculate standard deviation
	stats.StdDev = calculateStdDev(metrics.execTimes, metrics.AverageExecTime)

	// Calculate throughput (invocations per second)
	if !metrics.LastInvoked.IsZero() && !metrics.FirstInvoked.IsZero() {
		elapsed := metrics.LastInvoked.Sub(metrics.FirstInvoked).Seconds()
		if elapsed > 0 {
			stats.ThroughputPerSec = float64(metrics.TotalInvocations) / elapsed
		}
	}

	// Calculate error and success rates
	total := float64(metrics.TotalInvocations)
	if total > 0 {
		stats.ErrorRate = float64(metrics.TotalErrors) / total * 100
		stats.SuccessRate = float64(metrics.TotalSuccesses) / total * 100
	}

	return stats, nil
}

// GetTopProcessors returns the top N processors by invocation count
func (a *ProcessorAnalytics) GetTopProcessors(n int) []*ProcessorMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics := make([]*ProcessorMetrics, 0, len(a.processorMetrics))
	for _, m := range a.processorMetrics {
		copy := *m
		metrics = append(metrics, &copy)
	}

	// Sort by total invocations
	sortProcessorMetrics(metrics)

	if n > len(metrics) {
		n = len(metrics)
	}

	return metrics[:n]
}

// GetRecentErrors returns recent errors for an instance
func (a *ProcessorAnalytics) GetRecentErrors(instanceID uuid.UUID) ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	instance, exists := a.instanceMetrics[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	errors := make([]string, len(instance.errorMessages))
	copy(errors, instance.errorMessages)
	return errors, nil
}

// GetRecentInvocations returns recent invocations for an instance
func (a *ProcessorAnalytics) GetRecentInvocations(instanceID uuid.UUID, limit int) ([]InvocationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	instance, exists := a.instanceMetrics[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	start := len(instance.recentInvocations) - limit
	if start < 0 {
		start = 0
	}

	records := make([]InvocationRecord, len(instance.recentInvocations[start:]))
	copy(records, instance.recentInvocations[start:])
	return records, nil
}

// Reset clears all metrics
func (a *ProcessorAnalytics) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.processorMetrics = make(map[string]*ProcessorMetrics)
	a.instanceMetrics = make(map[uuid.UUID]*InstanceMetrics)
	a.aggregateMetrics = &AggregateMetrics{
		SystemStartTime: time.Now(),
	}

	a.logger.Info("Analytics reset")
}

// PurgeOldData removes data older than retention period
func (a *ProcessorAnalytics) PurgeOldData() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.retentionPeriod)
	purged := 0

	// Purge old invocation records
	for _, instance := range a.instanceMetrics {
		originalLen := len(instance.recentInvocations)
		filtered := make([]InvocationRecord, 0, originalLen)
		for _, record := range instance.recentInvocations {
			if record.Timestamp.After(cutoff) {
				filtered = append(filtered, record)
			}
		}
		instance.recentInvocations = filtered
		purged += originalLen - len(filtered)
	}

	a.logger.Info("Purged old analytics data", "records", purged)
	return purged
}

// Helper functions

func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	return total / time.Duration(len(durations))
}

func calculateStdDev(durations []time.Duration, mean time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var sumSquares float64
	meanFloat := float64(mean)

	for _, d := range durations {
		diff := float64(d) - meanFloat
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(durations))
	return time.Duration(variance)
}

func sortDurations(durations []time.Duration) {
	// Simple bubble sort for small arrays
	n := len(durations)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if durations[j] > durations[j+1] {
				durations[j], durations[j+1] = durations[j+1], durations[j]
			}
		}
	}
}

func sortProcessorMetrics(metrics []*ProcessorMetrics) {
	// Sort by total invocations descending
	n := len(metrics)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if metrics[j].TotalInvocations < metrics[j+1].TotalInvocations {
				metrics[j], metrics[j+1] = metrics[j+1], metrics[j]
			}
		}
	}
}
