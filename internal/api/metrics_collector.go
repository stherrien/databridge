package api

import (
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

// MetricsCollector collects and aggregates metrics from the system
type MetricsCollector struct {
	mu              sync.RWMutex
	flowController  *core.FlowController
	scheduler       *core.ProcessScheduler
	startTime       time.Time

	// Cached metrics
	lastUpdate      time.Time
	cacheInterval   time.Duration
	cachedMetrics   *SystemMetrics

	// Counters for throughput calculations
	totalFlowFiles  int64
	totalBytes      int64
	lastFlowFiles   int64
	lastBytes       int64
	lastCheckTime   time.Time
}

// NewMetricsCollector creates a new MetricsCollector
func NewMetricsCollector(flowController *core.FlowController, scheduler *core.ProcessScheduler, cacheInterval time.Duration) *MetricsCollector {
	now := time.Now()
	return &MetricsCollector{
		flowController: flowController,
		scheduler:      scheduler,
		startTime:      now,
		cacheInterval:  cacheInterval,
		lastCheckTime:  now,
		lastUpdate:     time.Time{},
	}
}

// GetSystemStatus returns overall system status
func (mc *MetricsCollector) GetSystemStatus() SystemStatus {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	processors := mc.flowController.GetProcessors()
	connections := mc.flowController.GetConnections()

	activeCount := 0
	for _, proc := range processors {
		proc.RLock()
		if proc.Status.State == types.ProcessorStateRunning {
			activeCount++
		}
		proc.RUnlock()
	}

	uptime := time.Since(mc.startTime)

	return SystemStatus{
		Status:           "running",
		Uptime:           uptime,
		UptimeSeconds:    int64(uptime.Seconds()),
		FlowController:   FlowControllerStatus{Running: true, State: "RUNNING"},
		Scheduler:        mc.getSchedulerStatus(),
		ActiveProcessors: activeCount,
		TotalProcessors:  len(processors),
		TotalConnections: len(connections),
		Timestamp:        time.Now(),
	}
}

// GetHealthStatus returns health check results
func (mc *MetricsCollector) GetHealthStatus() HealthStatus {
	components := make(map[string]ComponentHealth)

	// Check FlowController
	components["flowController"] = ComponentHealth{
		Status:  "healthy",
		Message: "FlowController is running",
	}

	// Check Scheduler
	schedulerStatus := mc.getSchedulerStatus()
	if schedulerStatus.Running {
		components["scheduler"] = ComponentHealth{
			Status:  "healthy",
			Message: "ProcessScheduler is running",
		}
	} else {
		components["scheduler"] = ComponentHealth{
			Status:  "unhealthy",
			Message: "ProcessScheduler is not running",
		}
	}

	// Check processors
	processors := mc.flowController.GetProcessors()
	invalidCount := 0
	for _, proc := range processors {
		proc.RLock()
		if proc.Status.State == types.ProcessorStateInvalid {
			invalidCount++
		}
		proc.RUnlock()
	}

	if invalidCount > 0 {
		components["processors"] = ComponentHealth{
			Status:  "degraded",
			Message: "Some processors have invalid configuration",
		}
	} else {
		components["processors"] = ComponentHealth{
			Status:  "healthy",
			Message: "All processors are valid",
		}
	}

	// Determine overall status
	overallStatus := "healthy"
	for _, comp := range components {
		if comp.Status == "unhealthy" {
			overallStatus = "unhealthy"
			break
		} else if comp.Status == "degraded" && overallStatus == "healthy" {
			overallStatus = "degraded"
		}
	}

	return HealthStatus{
		Status:     overallStatus,
		Components: components,
		Timestamp:  time.Now(),
	}
}

// GetSystemMetrics returns system-wide metrics
func (mc *MetricsCollector) GetSystemMetrics() SystemMetrics {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check if cache is still valid
	if mc.cachedMetrics != nil && time.Since(mc.lastUpdate) < mc.cacheInterval {
		return *mc.cachedMetrics
	}

	// Collect fresh metrics
	metrics := mc.collectMetrics()
	mc.cachedMetrics = &metrics
	mc.lastUpdate = time.Now()

	return metrics
}

// GetStatsSummary returns statistics summary
func (mc *MetricsCollector) GetStatsSummary() StatsSummary {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	processors := mc.flowController.GetProcessors()
	connections := mc.flowController.GetConnections()

	// Aggregate processor stats
	var processorStats []ProcessorStatsSummary
	var totalFlowFiles, totalBytes int64
	activeCount := 0

	for _, proc := range processors {
		proc.RLock()

		if proc.Status.State == types.ProcessorStateRunning {
			activeCount++
		}

		totalFlowFiles += proc.Status.FlowFilesOut
		totalBytes += proc.Status.BytesOut

		// Calculate failed tasks (if we track them)
		tasksFailed := int64(0)

		processorStats = append(processorStats, ProcessorStatsSummary{
			ID:             proc.ID,
			Name:           proc.Name,
			Type:           proc.Type,
			State:          proc.Status.State,
			TasksCompleted: proc.Status.TasksCompleted,
			TasksFailed:    tasksFailed,
			FlowFilesIn:    proc.Status.FlowFilesIn,
			FlowFilesOut:   proc.Status.FlowFilesOut,
			BytesIn:        proc.Status.BytesIn,
			BytesOut:       proc.Status.BytesOut,
		})
		proc.RUnlock()
	}

	// Aggregate connection stats
	var connectionStats []ConnectionStatsSummary
	for _, conn := range connections {
		conn.RLock()
		queueDepth := conn.Queue.Size()
		connectionStats = append(connectionStats, ConnectionStatsSummary{
			ID:              conn.ID,
			Name:            conn.Name,
			QueueDepth:      queueDepth,
			FlowFilesQueued: queueDepth,
		})
		conn.RUnlock()
	}

	// Calculate average throughput
	uptime := time.Since(mc.startTime).Seconds()
	avgThroughput := 0.0
	if uptime > 0 {
		avgThroughput = float64(totalFlowFiles) / uptime
	}

	return StatsSummary{
		TotalFlowFilesProcessed: totalFlowFiles,
		TotalBytesProcessed:     totalBytes,
		ActiveProcessors:        activeCount,
		TotalProcessors:         len(processors),
		TotalConnections:        len(connections),
		AverageThroughput:       avgThroughput,
		Uptime:                  time.Since(mc.startTime),
		ProcessorStats:          processorStats,
		ConnectionStats:         connectionStats,
		Timestamp:               time.Now(),
	}
}

// GetProcessorMetrics returns metrics for all processors
func (mc *MetricsCollector) GetProcessorMetrics() []ProcessorMetrics {
	processors := mc.flowController.GetProcessors()
	metrics := make([]ProcessorMetrics, 0, len(processors))

	now := time.Now()

	for _, proc := range processors {
		metrics = append(metrics, mc.getProcessorMetrics(proc, now))
	}

	return metrics
}

// GetProcessorMetricsByID returns metrics for a specific processor
func (mc *MetricsCollector) GetProcessorMetricsByID(id uuid.UUID) (*ProcessorMetrics, bool) {
	proc, exists := mc.flowController.GetProcessor(id)
	if !exists {
		return nil, false
	}

	metrics := mc.getProcessorMetrics(proc, time.Now())
	return &metrics, true
}

// GetConnectionMetrics returns metrics for all connections
func (mc *MetricsCollector) GetConnectionMetrics() []ConnectionMetrics {
	connections := mc.flowController.GetConnections()
	metrics := make([]ConnectionMetrics, 0, len(connections))

	now := time.Now()

	for _, conn := range connections {
		metrics = append(metrics, mc.getConnectionMetrics(conn, now))
	}

	return metrics
}

// GetConnectionMetricsByID returns metrics for a specific connection
func (mc *MetricsCollector) GetConnectionMetricsByID(id uuid.UUID) (*ConnectionMetrics, bool) {
	conn, exists := mc.flowController.GetConnection(id)
	if !exists {
		return nil, false
	}

	metrics := mc.getConnectionMetrics(conn, time.Now())
	return &metrics, true
}

// GetQueueMetrics returns queue depth and statistics
func (mc *MetricsCollector) GetQueueMetrics() QueueMetrics {
	connections := mc.flowController.GetConnections()

	var totalQueued, totalCapacity int64
	queueMetrics := make([]ConnectionMetrics, 0, len(connections))

	now := time.Now()

	for _, conn := range connections {
		connMetrics := mc.getConnectionMetrics(conn, now)
		queueMetrics = append(queueMetrics, connMetrics)

		totalQueued += connMetrics.QueueDepth
		totalCapacity += connMetrics.MaxQueueSize
	}

	percentFull := 0.0
	if totalCapacity > 0 {
		percentFull = float64(totalQueued) / float64(totalCapacity) * 100
	}

	return QueueMetrics{
		TotalQueued:       totalQueued,
		TotalCapacity:     totalCapacity,
		PercentFull:       percentFull,
		BackPressureCount: 0, // Would need to track this
		Queues:            queueMetrics,
		Timestamp:         now,
	}
}

// Private helper methods

func (mc *MetricsCollector) getSchedulerStatus() SchedulerStatus {
	// Access scheduler status through public methods
	// Note: We'll need to add methods to ProcessScheduler to expose this
	return SchedulerStatus{
		Running:        true,  // Assume running if FlowController is running
		ScheduledTasks: 0,     // Would need scheduler access
		ActiveWorkers:  0,     // Would need worker pool access
		MaxWorkers:     10,    // Default from scheduler
	}
}

func (mc *MetricsCollector) collectMetrics() SystemMetrics {
	now := time.Now()

	// Calculate throughput
	throughput := mc.calculateThroughput()

	// Collect memory metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memory := MemoryMetrics{
		AllocMB:      float64(memStats.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(memStats.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(memStats.Sys) / 1024 / 1024,
		NumGC:        memStats.NumGC,
		GCPauseMS:    float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000,
	}

	// Collect CPU metrics
	cpu := CPUMetrics{
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
	}

	// Collect repository metrics (stub - would need actual implementation)
	repository := RepositoryMetrics{
		FlowFileCount:     0, // Would query FlowFileRepository
		ContentClaimCount: 0, // Would query ContentRepository
		FlowFileRepoSize:  0, // Would calculate from disk
		ContentRepoSize:   0, // Would calculate from disk
	}

	return SystemMetrics{
		Throughput: throughput,
		Memory:     memory,
		CPU:        cpu,
		Repository: repository,
		Timestamp:  now,
	}
}

func (mc *MetricsCollector) calculateThroughput() ThroughputMetrics {
	now := time.Now()
	elapsed := now.Sub(mc.lastCheckTime).Seconds()

	// Aggregate current totals
	processors := mc.flowController.GetProcessors()
	var currentFlowFiles, currentBytes int64

	for _, proc := range processors {
		proc.RLock()
		currentFlowFiles += proc.Status.FlowFilesOut
		currentBytes += proc.Status.BytesOut
		proc.RUnlock()
	}

	// Calculate rates
	flowFilesDelta := currentFlowFiles - mc.lastFlowFiles
	bytesDelta := currentBytes - mc.lastBytes

	var flowFilesPerSec, bytesPerSec float64
	if elapsed > 0 {
		flowFilesPerSec = float64(flowFilesDelta) / elapsed
		bytesPerSec = float64(bytesDelta) / elapsed
	}

	// Update for next calculation
	mc.lastFlowFiles = currentFlowFiles
	mc.lastBytes = currentBytes
	mc.lastCheckTime = now

	return ThroughputMetrics{
		FlowFilesProcessed: currentFlowFiles,
		FlowFilesPerSecond: flowFilesPerSec,
		FlowFilesPerMinute: flowFilesPerSec * 60,
		FlowFilesPerHour:   flowFilesPerSec * 3600,
		BytesProcessed:     currentBytes,
		BytesPerSecond:     bytesPerSec,
		TotalTransactions:  currentFlowFiles,
	}
}

func (mc *MetricsCollector) getProcessorMetrics(proc *core.ProcessorNode, now time.Time) ProcessorMetrics {
	proc.RLock()
	defer proc.RUnlock()

	// Calculate throughput
	uptime := now.Sub(proc.Status.LastRun).Seconds()
	var flowFilesPerSec, bytesPerSec float64
	if uptime > 0 && uptime < 3600 { // Only calculate if recently active
		flowFilesPerSec = float64(proc.Status.FlowFilesOut) / uptime
		bytesPerSec = float64(proc.Status.BytesOut) / uptime
	}

	return ProcessorMetrics{
		ID:                     proc.ID,
		Name:                   proc.Name,
		Type:                   proc.Type,
		State:                  proc.Status.State,
		TasksCompleted:         proc.Status.TasksCompleted,
		TasksFailed:            0, // Would need to track failures
		TasksRunning:           proc.Status.TasksRunning,
		FlowFilesIn:            proc.Status.FlowFilesIn,
		FlowFilesOut:           proc.Status.FlowFilesOut,
		BytesIn:                proc.Status.BytesIn,
		BytesOut:               proc.Status.BytesOut,
		AverageExecutionTime:   proc.Status.AverageTaskTime,
		AverageExecutionTimeMS: float64(proc.Status.AverageTaskTime.Milliseconds()),
		LastRunTime:            proc.Status.LastRun,
		RunCount:               proc.Status.TasksCompleted,
		Throughput: ProcessorThroughput{
			FlowFilesPerSecond: flowFilesPerSec,
			FlowFilesPerMinute: flowFilesPerSec * 60,
			BytesPerSecond:     bytesPerSec,
		},
		Timestamp: now,
	}
}

func (mc *MetricsCollector) getConnectionMetrics(conn *core.Connection, now time.Time) ConnectionMetrics {
	conn.RLock()
	defer conn.RUnlock()

	queueDepth := conn.Queue.Size()
	maxSize := conn.BackPressureSize

	percentFull := 0.0
	if maxSize > 0 {
		percentFull = float64(queueDepth) / float64(maxSize) * 100
	}

	return ConnectionMetrics{
		ID:                    conn.ID,
		Name:                  conn.Name,
		SourceID:              conn.Source.ID,
		SourceName:            conn.Source.Name,
		DestinationID:         conn.Destination.ID,
		DestinationName:       conn.Destination.Name,
		Relationship:          conn.Relationship.Name,
		QueueDepth:            queueDepth,
		MaxQueueSize:          maxSize,
		FlowFilesQueued:       queueDepth,
		BackPressureTriggered: 0, // Would need to track this
		PercentFull:           percentFull,
		Throughput: ConnectionThroughput{
			FlowFilesPerSecond: 0, // Would need to track enqueue/dequeue rates
			EnqueueRate:        0,
			DequeueRate:        0,
		},
		Timestamp: now,
	}
}
