package types

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Processor represents a processing component in the flow
type Processor interface {
	// Initialize is called once when the processor is created
	Initialize(ctx ProcessorContext) error

	// OnTrigger is called to process FlowFiles
	OnTrigger(ctx context.Context, session ProcessSession) error

	// GetInfo returns processor metadata
	GetInfo() ProcessorInfo

	// Validate validates the processor configuration
	Validate(config ProcessorConfig) []ValidationResult

	// OnStopped is called when the processor is stopped
	OnStopped(ctx context.Context)
}

// ProcessorInfo contains metadata about a processor
type ProcessorInfo struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Author       string            `json:"author"`
	Tags         []string          `json:"tags"`
	Properties   []PropertySpec    `json:"properties"`
	Relationships []Relationship   `json:"relationships"`
}

// PropertySpec defines a processor property specification
type PropertySpec struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Required     bool   `json:"required"`
	Sensitive    bool   `json:"sensitive"`
	DefaultValue string `json:"defaultValue"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	Pattern      string `json:"pattern,omitempty"`
}

// Relationship defines how FlowFiles are routed after processing
type Relationship struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AutoTerminate bool `json:"autoTerminate"`
}

// Common relationships
var (
	RelationshipSuccess = Relationship{
		Name:        "success",
		Description: "FlowFiles that are successfully processed",
	}
	RelationshipFailure = Relationship{
		Name:        "failure",
		Description: "FlowFiles that failed processing",
	}
	RelationshipRetry = Relationship{
		Name:        "retry",
		Description: "FlowFiles that should be retried",
	}
	RelationshipOriginal = Relationship{
		Name:        "original",
		Description: "The original FlowFile",
	}
)

// Position represents a processor's position on the canvas
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ProcessorConfig contains processor configuration
type ProcessorConfig struct {
	ID            uuid.UUID         `json:"id"`
	Name          string           `json:"name"`
	Type          string           `json:"type"`
	Properties    map[string]string `json:"properties"`
	ScheduleType  ScheduleType     `json:"scheduleType"`
	ScheduleValue string           `json:"scheduleValue"`
	Concurrency   int              `json:"concurrency"`
	AutoTerminate map[string]bool  `json:"autoTerminate"`
	Position      *Position        `json:"position,omitempty"` // Canvas position for UI
	ProcessGroupID *uuid.UUID      `json:"processGroupId,omitempty"` // Parent process group
}

// ScheduleType defines how a processor is scheduled
type ScheduleType string

const (
	ScheduleTypeTimer       ScheduleType = "TIMER_DRIVEN"
	ScheduleTypeEvent       ScheduleType = "EVENT_DRIVEN"
	ScheduleTypeCron        ScheduleType = "CRON_DRIVEN"
	ScheduleTypePrimaryNode ScheduleType = "PRIMARY_NODE_ONLY"
)

// ValidationResult represents a configuration validation result
type ValidationResult struct {
	Property string `json:"property"`
	Valid    bool   `json:"valid"`
	Message  string `json:"message"`
}

// ProcessorContext provides access to processor services and configuration
type ProcessorContext interface {
	GetProperty(name string) (string, bool)
	GetPropertyValue(name string) string
	HasProperty(name string) bool
	GetProcessorConfig() ProcessorConfig
	GetLogger() Logger
}

// ProcessSession provides transactional access to FlowFiles
type ProcessSession interface {
	// Get retrieves a FlowFile from the input queue
	Get() *FlowFile

	// GetBatch retrieves multiple FlowFiles up to maxResults
	GetBatch(maxResults int) []*FlowFile

	// Create creates a new FlowFile
	Create() *FlowFile

	// CreateChild creates a child FlowFile from a parent
	CreateChild(parent *FlowFile) *FlowFile

	// Clone creates a copy of a FlowFile
	Clone(original *FlowFile) *FlowFile

	// Transfer routes a FlowFile to a relationship
	Transfer(flowFile *FlowFile, relationship Relationship)

	// Remove removes a FlowFile from the session
	Remove(flowFile *FlowFile)

	// PutAttribute adds or updates an attribute
	PutAttribute(flowFile *FlowFile, key, value string)

	// PutAllAttributes updates multiple attributes
	PutAllAttributes(flowFile *FlowFile, attributes map[string]string)

	// RemoveAttribute removes an attribute
	RemoveAttribute(flowFile *FlowFile, key string)

	// Write writes content to a FlowFile
	Write(flowFile *FlowFile, content []byte) error

	// Read reads content from a FlowFile
	Read(flowFile *FlowFile) ([]byte, error)

	// Commit commits all changes in the session
	Commit() error

	// Rollback rolls back all changes in the session
	Rollback()

	// GetLogger returns a logger for the session
	GetLogger() Logger
}

// ProcessorStatus represents the current status of a processor
type ProcessorStatus struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	State             ProcessorState `json:"state"`
	FlowFilesIn       int64     `json:"flowFilesIn"`
	FlowFilesOut      int64     `json:"flowFilesOut"`
	BytesIn           int64     `json:"bytesIn"`
	BytesOut          int64     `json:"bytesOut"`
	TasksCompleted    int64     `json:"tasksCompleted"`
	TasksRunning      int       `json:"tasksRunning"`
	AverageTaskTime   time.Duration `json:"averageTaskTime"`
	LastRun           time.Time `json:"lastRun"`
	ValidationErrors  []string  `json:"validationErrors,omitempty"`
}

// ProcessorState represents the processor lifecycle state
type ProcessorState string

const (
	ProcessorStateStopped  ProcessorState = "STOPPED"
	ProcessorStateRunning  ProcessorState = "RUNNING"
	ProcessorStateDisabled ProcessorState = "DISABLED"
	ProcessorStateInvalid  ProcessorState = "INVALID"
)

// Logger interface for processor logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// BaseProcessor provides common processor functionality
type BaseProcessor struct {
	info ProcessorInfo
}

// NewBaseProcessor creates a new BaseProcessor
func NewBaseProcessor(info ProcessorInfo) *BaseProcessor {
	return &BaseProcessor{info: info}
}

// GetInfo returns processor information
func (p *BaseProcessor) GetInfo() ProcessorInfo {
	return p.info
}

// Validate provides default validation (override in specific processors)
func (p *BaseProcessor) Validate(config ProcessorConfig) []ValidationResult {
	var results []ValidationResult

	// Validate required properties
	for _, prop := range p.info.Properties {
		if prop.Required {
			if value, exists := config.Properties[prop.Name]; !exists || value == "" {
				results = append(results, ValidationResult{
					Property: prop.Name,
					Valid:    false,
					Message:  "Required property is missing or empty",
				})
			}
		}
	}

	return results
}

// OnStopped provides default cleanup (override if needed)
func (p *BaseProcessor) OnStopped(ctx context.Context) {
	// Default implementation does nothing
}