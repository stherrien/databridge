package api

import (
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// SystemStatus represents the overall system status
type SystemStatus struct {
	Status           string               `json:"status"`
	Uptime           time.Duration        `json:"uptime"`
	UptimeSeconds    int64                `json:"uptimeSeconds"`
	FlowController   FlowControllerStatus `json:"flowController"`
	Scheduler        SchedulerStatus      `json:"scheduler"`
	ActiveProcessors int                  `json:"activeProcessors"`
	TotalProcessors  int                  `json:"totalProcessors"`
	TotalConnections int                  `json:"totalConnections"`
	Timestamp        time.Time            `json:"timestamp"`
}

// FlowControllerStatus represents FlowController status
type FlowControllerStatus struct {
	Running bool   `json:"running"`
	State   string `json:"state"`
}

// SchedulerStatus represents ProcessScheduler status
type SchedulerStatus struct {
	Running        bool `json:"running"`
	ScheduledTasks int  `json:"scheduledTasks"`
	ActiveWorkers  int  `json:"activeWorkers"`
	MaxWorkers     int  `json:"maxWorkers"`
}

// HealthStatus represents health check results
type HealthStatus struct {
	Status     string                     `json:"status"` // healthy, degraded, unhealthy
	Components map[string]ComponentHealth `json:"components"`
	Timestamp  time.Time                  `json:"timestamp"`
}

// ComponentHealth represents health of a component
type ComponentHealth struct {
	Status  string `json:"status"` // healthy, degraded, unhealthy
	Message string `json:"message,omitempty"`
}

// SystemMetrics represents system-wide metrics
type SystemMetrics struct {
	Throughput ThroughputMetrics `json:"throughput"`
	Memory     MemoryMetrics     `json:"memory"`
	CPU        CPUMetrics        `json:"cpu"`
	Repository RepositoryMetrics `json:"repository"`
	Timestamp  time.Time         `json:"timestamp"`
}

// ThroughputMetrics represents throughput statistics
type ThroughputMetrics struct {
	FlowFilesProcessed int64   `json:"flowFilesProcessed"`
	FlowFilesPerSecond float64 `json:"flowFilesPerSecond"`
	FlowFilesPerMinute float64 `json:"flowFilesPerMinute"`
	FlowFilesPerHour   float64 `json:"flowFilesPerHour"`
	BytesProcessed     int64   `json:"bytesProcessed"`
	BytesPerSecond     float64 `json:"bytesPerSecond"`
	TotalTransactions  int64   `json:"totalTransactions"`
}

// MemoryMetrics represents memory usage
type MemoryMetrics struct {
	AllocMB      float64 `json:"allocMB"`
	TotalAllocMB float64 `json:"totalAllocMB"`
	SysMB        float64 `json:"sysMB"`
	NumGC        uint32  `json:"numGC"`
	GCPauseMS    float64 `json:"gcPauseMS"`
}

// CPUMetrics represents CPU usage
type CPUMetrics struct {
	NumCPU       int `json:"numCPU"`
	NumGoroutine int `json:"numGoroutine"`
}

// RepositoryMetrics represents repository statistics
type RepositoryMetrics struct {
	FlowFileCount     int64 `json:"flowFileCount"`
	ContentClaimCount int64 `json:"contentClaimCount"`
	FlowFileRepoSize  int64 `json:"flowFileRepoSizeMB"`
	ContentRepoSize   int64 `json:"contentRepoSizeMB"`
}

// StatsSummary represents statistics summary
type StatsSummary struct {
	TotalFlowFilesProcessed int64                    `json:"totalFlowFilesProcessed"`
	TotalBytesProcessed     int64                    `json:"totalBytesProcessed"`
	ActiveProcessors        int                      `json:"activeProcessors"`
	TotalProcessors         int                      `json:"totalProcessors"`
	TotalConnections        int                      `json:"totalConnections"`
	AverageThroughput       float64                  `json:"averageThroughput"`
	Uptime                  time.Duration            `json:"uptime"`
	ProcessorStats          []ProcessorStatsSummary  `json:"processorStats"`
	ConnectionStats         []ConnectionStatsSummary `json:"connectionStats"`
	Timestamp               time.Time                `json:"timestamp"`
}

// ProcessorStatsSummary represents processor statistics summary
type ProcessorStatsSummary struct {
	ID             uuid.UUID            `json:"id"`
	Name           string               `json:"name"`
	Type           string               `json:"type"`
	State          types.ProcessorState `json:"state"`
	TasksCompleted int64                `json:"tasksCompleted"`
	TasksFailed    int64                `json:"tasksFailed"`
	FlowFilesIn    int64                `json:"flowFilesIn"`
	FlowFilesOut   int64                `json:"flowFilesOut"`
	BytesIn        int64                `json:"bytesIn"`
	BytesOut       int64                `json:"bytesOut"`
}

// ConnectionStatsSummary represents connection statistics summary
type ConnectionStatsSummary struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	QueueDepth      int64     `json:"queueDepth"`
	FlowFilesQueued int64     `json:"flowFilesQueued"`
}

// ProcessorMetrics represents detailed processor metrics
type ProcessorMetrics struct {
	ID                     uuid.UUID            `json:"id"`
	Name                   string               `json:"name"`
	Type                   string               `json:"type"`
	State                  types.ProcessorState `json:"state"`
	TasksCompleted         int64                `json:"tasksCompleted"`
	TasksFailed            int64                `json:"tasksFailed"`
	TasksRunning           int                  `json:"tasksRunning"`
	FlowFilesIn            int64                `json:"flowFilesIn"`
	FlowFilesOut           int64                `json:"flowFilesOut"`
	BytesIn                int64                `json:"bytesIn"`
	BytesOut               int64                `json:"bytesOut"`
	AverageExecutionTime   time.Duration        `json:"averageExecutionTime"`
	AverageExecutionTimeMS float64              `json:"averageExecutionTimeMS"`
	LastRunTime            time.Time            `json:"lastRunTime"`
	RunCount               int64                `json:"runCount"`
	Throughput             ProcessorThroughput  `json:"throughput"`
	Timestamp              time.Time            `json:"timestamp"`
}

// ProcessorThroughput represents processor throughput metrics
type ProcessorThroughput struct {
	FlowFilesPerSecond float64 `json:"flowFilesPerSecond"`
	FlowFilesPerMinute float64 `json:"flowFilesPerMinute"`
	BytesPerSecond     float64 `json:"bytesPerSecond"`
}

// ConnectionMetrics represents detailed connection metrics
type ConnectionMetrics struct {
	ID                    uuid.UUID            `json:"id"`
	Name                  string               `json:"name"`
	SourceID              uuid.UUID            `json:"sourceId"`
	SourceName            string               `json:"sourceName"`
	DestinationID         uuid.UUID            `json:"destinationId"`
	DestinationName       string               `json:"destinationName"`
	Relationship          string               `json:"relationship"`
	QueueDepth            int64                `json:"queueDepth"`
	MaxQueueSize          int64                `json:"maxQueueSize"`
	FlowFilesQueued       int64                `json:"flowFilesQueued"`
	BackPressureTriggered int64                `json:"backPressureTriggered"`
	PercentFull           float64              `json:"percentFull"`
	Throughput            ConnectionThroughput `json:"throughput"`
	Timestamp             time.Time            `json:"timestamp"`
}

// ConnectionThroughput represents connection throughput metrics
type ConnectionThroughput struct {
	FlowFilesPerSecond float64 `json:"flowFilesPerSecond"`
	EnqueueRate        float64 `json:"enqueueRate"`
	DequeueRate        float64 `json:"dequeueRate"`
}

// QueueMetrics represents queue depth and statistics
type QueueMetrics struct {
	TotalQueued       int64               `json:"totalQueued"`
	TotalCapacity     int64               `json:"totalCapacity"`
	PercentFull       float64             `json:"percentFull"`
	BackPressureCount int64               `json:"backPressureCount"`
	Queues            []ConnectionMetrics `json:"queues"`
	Timestamp         time.Time           `json:"timestamp"`
}

// MonitoringEvent represents a real-time monitoring event
type MonitoringEvent struct {
	EventType string      `json:"eventType"` // processor_status, throughput, queue_depth
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// ProcessorStatusEvent represents processor status change event
type ProcessorStatusEvent struct {
	ProcessorID   uuid.UUID            `json:"processorId"`
	ProcessorName string               `json:"processorName"`
	OldState      types.ProcessorState `json:"oldState"`
	NewState      types.ProcessorState `json:"newState"`
}

// ThroughputEvent represents throughput update event
type ThroughputEvent struct {
	TotalFlowFiles     int64   `json:"totalFlowFiles"`
	FlowFilesPerSecond float64 `json:"flowFilesPerSecond"`
	BytesPerSecond     float64 `json:"bytesPerSecond"`
}

// QueueDepthEvent represents queue depth update event
type QueueDepthEvent struct {
	ConnectionID   uuid.UUID `json:"connectionId"`
	ConnectionName string    `json:"connectionName"`
	QueueDepth     int64     `json:"queueDepth"`
	PercentFull    float64   `json:"percentFull"`
}
