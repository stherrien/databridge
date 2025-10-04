package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// ProcessGroupDTO represents a process group (flow)
type ProcessGroupDTO struct {
	ID          uuid.UUID                  `json:"id"`
	Name        string                     `json:"name"`
	ParentID    *uuid.UUID                 `json:"parentId,omitempty"`
	Children    []ProcessGroupDTO          `json:"children,omitempty"`
	Processors  []ProcessorDTO             `json:"processors,omitempty"`
	Connections []ConnectionDTO            `json:"connections,omitempty"`
	CreatedAt   time.Time                  `json:"createdAt,omitempty"`
	UpdatedAt   time.Time                  `json:"updatedAt,omitempty"`
}

// ProcessGroupStatusDTO represents the status of a process group
type ProcessGroupStatusDTO struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	ActiveThreads      int       `json:"activeThreads"`
	QueuedCount        int64     `json:"queuedCount"`
	BytesQueued        int64     `json:"bytesQueued"`
	FlowFilesReceived  int64     `json:"flowFilesReceived"`
	BytesReceived      int64     `json:"bytesReceived"`
	FlowFilesSent      int64     `json:"flowFilesSent"`
	BytesSent          int64     `json:"bytesSent"`
	ActiveProcessors   int       `json:"activeProcessors"`
	StoppedProcessors  int       `json:"stoppedProcessors"`
	InvalidProcessors  int       `json:"invalidProcessors"`
	DisabledProcessors int       `json:"disabledProcessors"`
}

// CreateProcessGroupRequest represents a request to create a process group
type CreateProcessGroupRequest struct {
	Name     string     `json:"name" binding:"required"`
	ParentID *uuid.UUID `json:"parentId,omitempty"`
}

// UpdateProcessGroupRequest represents a request to update a process group
type UpdateProcessGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

// ProcessorDTO represents a processor with its configuration
type ProcessorDTO struct {
	ID            uuid.UUID         `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	State         string            `json:"state"`
	Properties    map[string]string `json:"properties"`
	ScheduleType  string            `json:"scheduleType"`
	ScheduleValue string            `json:"scheduleValue"`
	Concurrency   int               `json:"concurrency"`
	AutoTerminate map[string]bool   `json:"autoTerminate"`
	Position      *PositionDTO      `json:"position,omitempty"`
	ProcessGroupID *uuid.UUID       `json:"processGroupId,omitempty"`
}

// PositionDTO represents the position of a component on the canvas
type PositionDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ProcessorStatusDTO represents the status of a processor
type ProcessorStatusDTO struct {
	ID                  uuid.UUID `json:"id"`
	Name                string    `json:"name"`
	Type                string    `json:"type"`
	State               string    `json:"state"`
	FlowFilesIn         int64     `json:"flowFilesIn"`
	FlowFilesOut        int64     `json:"flowFilesOut"`
	BytesIn             int64     `json:"bytesIn"`
	BytesOut            int64     `json:"bytesOut"`
	TasksCompleted      int64     `json:"tasksCompleted"`
	TasksRunning        int       `json:"tasksRunning"`
	AverageTaskTime     string    `json:"averageTaskTime"`
	LastRun             time.Time `json:"lastRun"`
	ValidationErrors    []string  `json:"validationErrors,omitempty"`
}

// CreateProcessorRequest represents a request to create a processor
type CreateProcessorRequest struct {
	Name           string            `json:"name" binding:"required"`
	Type           string            `json:"type" binding:"required"`
	Properties     map[string]string `json:"properties,omitempty"`
	ScheduleType   string            `json:"scheduleType,omitempty"`
	ScheduleValue  string            `json:"scheduleValue,omitempty"`
	Concurrency    int               `json:"concurrency,omitempty"`
	AutoTerminate  map[string]bool   `json:"autoTerminate,omitempty"`
	Position       *PositionDTO      `json:"position,omitempty"`
	ProcessGroupID *uuid.UUID        `json:"processGroupId,omitempty"`
}

// UpdateProcessorRequest represents a request to update a processor
type UpdateProcessorRequest struct {
	Name          *string            `json:"name,omitempty"`
	Properties    map[string]string  `json:"properties,omitempty"`
	ScheduleType  *string            `json:"scheduleType,omitempty"`
	ScheduleValue *string            `json:"scheduleValue,omitempty"`
	Concurrency   *int               `json:"concurrency,omitempty"`
	AutoTerminate map[string]bool    `json:"autoTerminate,omitempty"`
	Position      *PositionDTO       `json:"position,omitempty"`
}

// ProcessorInfoDTO represents detailed information about a processor type
type ProcessorInfoDTO struct {
	Name          string                  `json:"name"`
	Type          string                  `json:"type"`
	Description   string                  `json:"description"`
	Version       string                  `json:"version"`
	Author        string                  `json:"author"`
	Tags          []string                `json:"tags"`
	Properties    []PropertySpecDTO       `json:"properties"`
	Relationships []RelationshipDTO       `json:"relationships"`
}

// PropertySpecDTO represents a processor property specification
type PropertySpecDTO struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Required      bool     `json:"required"`
	Sensitive     bool     `json:"sensitive"`
	DefaultValue  string   `json:"defaultValue,omitempty"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
}

// RelationshipDTO represents a relationship definition
type RelationshipDTO struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	AutoTerminate bool   `json:"autoTerminate"`
}

// ConnectionDTO represents a connection between processors
type ConnectionDTO struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	SourceID         uuid.UUID `json:"sourceId"`
	DestinationID    uuid.UUID `json:"destinationId"`
	Relationship     string    `json:"relationship"`
	BackPressureSize int64     `json:"backPressureSize"`
	QueuedCount      int64     `json:"queuedCount"`
	ProcessGroupID   *uuid.UUID `json:"processGroupId,omitempty"`
}

// CreateConnectionRequest represents a request to create a connection
type CreateConnectionRequest struct {
	Name             string     `json:"name,omitempty"`
	SourceID         uuid.UUID  `json:"sourceId" binding:"required"`
	DestinationID    uuid.UUID  `json:"destinationId" binding:"required"`
	Relationship     string     `json:"relationship" binding:"required"`
	BackPressureSize *int64     `json:"backPressureSize,omitempty"`
	ProcessGroupID   *uuid.UUID `json:"processGroupId,omitempty"`
}

// UpdateConnectionRequest represents a request to update a connection
type UpdateConnectionRequest struct {
	Name             *string `json:"name,omitempty"`
	BackPressureSize *int64  `json:"backPressureSize,omitempty"`
}

// ListResponse represents a paginated list response
type ListResponse struct {
	Items      interface{} `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page,omitempty"`
	PageSize   int         `json:"pageSize,omitempty"`
	TotalPages int         `json:"totalPages,omitempty"`
}

// Helper functions to convert between domain models and DTOs

// ProcessorToDTO converts a ProcessorNode to ProcessorDTO
func ProcessorToDTO(processor interface{}) ProcessorDTO {
	// Type assertion would happen here in real code
	// For now, returning a basic structure
	return ProcessorDTO{}
}

// ProcessorStatusToDTO converts ProcessorStatus to ProcessorStatusDTO
func ProcessorStatusToDTO(status types.ProcessorStatus) ProcessorStatusDTO {
	return ProcessorStatusDTO{
		ID:               status.ID,
		Name:             status.Name,
		Type:             status.Type,
		State:            string(status.State),
		FlowFilesIn:      status.FlowFilesIn,
		FlowFilesOut:     status.FlowFilesOut,
		BytesIn:          status.BytesIn,
		BytesOut:         status.BytesOut,
		TasksCompleted:   status.TasksCompleted,
		TasksRunning:     status.TasksRunning,
		AverageTaskTime:  status.AverageTaskTime.String(),
		LastRun:          status.LastRun,
		ValidationErrors: status.ValidationErrors,
	}
}

// ProcessorInfoToDTO converts ProcessorInfo to ProcessorInfoDTO
func ProcessorInfoToDTO(info types.ProcessorInfo, processorType string) ProcessorInfoDTO {
	properties := make([]PropertySpecDTO, len(info.Properties))
	for i, prop := range info.Properties {
		properties[i] = PropertySpecDTO{
			Name:          prop.Name,
			Description:   prop.Description,
			Required:      prop.Required,
			Sensitive:     prop.Sensitive,
			DefaultValue:  prop.DefaultValue,
			AllowedValues: prop.AllowedValues,
			Pattern:       prop.Pattern,
		}
	}

	relationships := make([]RelationshipDTO, len(info.Relationships))
	for i, rel := range info.Relationships {
		relationships[i] = RelationshipDTO{
			Name:          rel.Name,
			Description:   rel.Description,
			AutoTerminate: rel.AutoTerminate,
		}
	}

	return ProcessorInfoDTO{
		Name:          info.Name,
		Type:          processorType,
		Description:   info.Description,
		Version:       info.Version,
		Author:        info.Author,
		Tags:          info.Tags,
		Properties:    properties,
		Relationships: relationships,
	}
}
