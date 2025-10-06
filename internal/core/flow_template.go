package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// FlowTemplate represents a reusable flow configuration template
type FlowTemplate struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Version     string    `json:"version"`
	Author      string    `json:"author"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// Template content
	Processors    []ProcessorTemplate    `json:"processors"`
	Connections   []ConnectionTemplate   `json:"connections"`
	ProcessGroups []ProcessGroupTemplate `json:"processGroups"`
	Variables     map[string]string      `json:"variables"` // Parameterized values
}

// ProcessorTemplate represents a processor in a template
type ProcessorTemplate struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Properties    map[string]string `json:"properties"`
	Schedule      ProcessorSchedule `json:"schedule"`
	Position      Position          `json:"position"`
	AutoTerminate map[string]bool   `json:"autoTerminate"`
}

// ConnectionTemplate represents a connection in a template
type ConnectionTemplate struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	SourceID           string `json:"sourceId"`
	DestinationID      string `json:"destinationId"`
	SourceRelationship string `json:"sourceRelationship"`
	QueueSize          int    `json:"queueSize"`
}

// ProcessGroupTemplate represents a process group in a template
type ProcessGroupTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Position    Position `json:"position"`
}

// Position represents a 2D position for visual layout
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowTemplateManager manages flow templates
type FlowTemplateManager struct {
	mu             sync.RWMutex
	templates      map[uuid.UUID]*FlowTemplate
	templateDir    string
	flowController *FlowController
}

// FlowExportFormat represents different export formats
type FlowExportFormat string

const (
	ExportFormatJSON FlowExportFormat = "JSON"
	ExportFormatYAML FlowExportFormat = "YAML"
)

// FlowExport represents an exportable flow configuration
type FlowExport struct {
	Metadata      FlowMetadata         `json:"metadata"`
	Processors    []ProcessorExport    `json:"processors"`
	Connections   []ConnectionExport   `json:"connections"`
	ProcessGroups []ProcessGroupExport `json:"processGroups"`
	ExportedAt    time.Time            `json:"exportedAt"`
	Version       string               `json:"version"`
}

// FlowMetadata contains metadata about an exported flow
type FlowMetadata struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Tags        []string          `json:"tags"`
	Variables   map[string]string `json:"variables"`
}

// ProcessorExport represents an exported processor
type ProcessorExport struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	Properties    map[string]string     `json:"properties"`
	Schedule      types.ProcessorConfig `json:"schedule"`
	Position      Position              `json:"position"`
	AutoTerminate map[string]bool       `json:"autoTerminate"`
}

// ConnectionExport represents an exported connection
type ConnectionExport struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	SourceID           string `json:"sourceId"`
	DestinationID      string `json:"destinationId"`
	SourceRelationship string `json:"sourceRelationship"`
	QueueSize          int    `json:"queueSize"`
}

// ProcessGroupExport represents an exported process group
type ProcessGroupExport struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Position    Position `json:"position"`
}

// NewFlowTemplateManager creates a new template manager
func NewFlowTemplateManager(templateDir string, flowController *FlowController) *FlowTemplateManager {
	return &FlowTemplateManager{
		templates:      make(map[uuid.UUID]*FlowTemplate),
		templateDir:    templateDir,
		flowController: flowController,
	}
}

// CreateTemplate creates a new flow template from an existing flow
func (ftm *FlowTemplateManager) CreateTemplate(name, description, author string, processorIDs []uuid.UUID) (*FlowTemplate, error) {
	ftm.mu.Lock()
	defer ftm.mu.Unlock()

	template := &FlowTemplate{
		ID:          uuid.New(),
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Author:      author,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Variables:   make(map[string]string),
		Processors:  make([]ProcessorTemplate, 0),
		Connections: make([]ConnectionTemplate, 0),
	}

	// Extract processor configurations
	processors := ftm.flowController.GetProcessors()
	processorMap := make(map[uuid.UUID]*ProcessorNode)

	for _, proc := range processors {
		processorMap[proc.ID] = proc
	}

	// Add processors to template
	for _, procID := range processorIDs {
		proc, exists := processorMap[procID]
		if !exists {
			continue
		}

		proc.RLock()
		procTemplate := ProcessorTemplate{
			ID:            proc.ID.String(),
			Name:          proc.Name,
			Type:          proc.Type,
			Properties:    proc.Config.Properties,
			AutoTerminate: proc.Config.AutoTerminate,
			Position:      Position{X: 0, Y: 0}, // Would need to store positions
		}
		proc.RUnlock()

		template.Processors = append(template.Processors, procTemplate)
	}

	// Extract connections between template processors
	connections := ftm.flowController.GetConnections()
	templateProcSet := make(map[uuid.UUID]bool)
	for _, procID := range processorIDs {
		templateProcSet[procID] = true
	}

	for _, conn := range connections {
		conn.RLock()
		sourceID := conn.Source.ID
		destID := conn.Destination.ID
		sourceInTemplate := templateProcSet[sourceID]
		destInTemplate := templateProcSet[destID]
		conn.RUnlock()

		if sourceInTemplate && destInTemplate {
			conn.RLock()
			connTemplate := ConnectionTemplate{
				ID:                 conn.ID.String(),
				Name:               conn.Name,
				SourceID:           sourceID.String(),
				DestinationID:      destID.String(),
				SourceRelationship: conn.Relationship.Name,
				QueueSize:          int(conn.Queue.maxSize),
			}
			conn.RUnlock()

			template.Connections = append(template.Connections, connTemplate)
		}
	}

	ftm.templates[template.ID] = template
	return template, nil
}

// InstantiateTemplate creates a new flow from a template
func (ftm *FlowTemplateManager) InstantiateTemplate(templateID uuid.UUID, variables map[string]string) ([]uuid.UUID, error) {
	ftm.mu.RLock()
	template, exists := ftm.templates[templateID]
	ftm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("template %s not found", templateID)
	}

	// Merge variables
	mergedVars := make(map[string]string)
	for k, v := range template.Variables {
		mergedVars[k] = v
	}
	for k, v := range variables {
		mergedVars[k] = v
	}

	// Create processors
	processorIDMap := make(map[string]uuid.UUID)
	createdProcessors := make([]uuid.UUID, 0)

	for _, procTemplate := range template.Processors {
		// Substitute variables in properties
		properties := make(map[string]string)
		for k, v := range procTemplate.Properties {
			properties[k] = substituteVariables(v, mergedVars)
		}

		config := types.ProcessorConfig{
			ID:            uuid.New(),
			Name:          procTemplate.Name,
			Type:          procTemplate.Type,
			Properties:    properties,
			AutoTerminate: procTemplate.AutoTerminate,
		}

		processor, err := ftm.flowController.CreateProcessorByType(procTemplate.Type, config)
		if err != nil {
			// Cleanup created processors on error
			for _, procID := range createdProcessors {
				_ = ftm.flowController.RemoveProcessor(procID)
			}
			return nil, fmt.Errorf("failed to create processor %s: %w", procTemplate.Name, err)
		}

		processorIDMap[procTemplate.ID] = processor.ID
		createdProcessors = append(createdProcessors, processor.ID)
	}

	// Create connections
	for _, connTemplate := range template.Connections {
		sourceID, sourceExists := processorIDMap[connTemplate.SourceID]
		destID, destExists := processorIDMap[connTemplate.DestinationID]

		if !sourceExists || !destExists {
			continue
		}

		relationship := types.Relationship{
			Name:        connTemplate.SourceRelationship,
			Description: "",
		}

		_, err := ftm.flowController.AddConnection(sourceID, destID, relationship)
		if err != nil {
			return createdProcessors, fmt.Errorf("failed to create connection: %w", err)
		}
	}

	return createdProcessors, nil
}

// SaveTemplate saves a template to disk
func (ftm *FlowTemplateManager) SaveTemplate(templateID uuid.UUID) error {
	ftm.mu.RLock()
	template, exists := ftm.templates[templateID]
	ftm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("template %s not found", templateID)
	}

	// Ensure template directory exists
	if err := os.MkdirAll(ftm.templateDir, 0750); err != nil {
		return fmt.Errorf("failed to create template directory: %w", err)
	}

	// Marshal template to JSON
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	// Write to file
	filename := filepath.Join(ftm.templateDir, fmt.Sprintf("%s.json", template.ID))
	if err := os.WriteFile(filename, data, 0600); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	return nil
}

// LoadTemplate loads a template from disk
func (ftm *FlowTemplateManager) LoadTemplate(filename string) (*FlowTemplate, error) {
	// #nosec G304 - filename is controlled by template manager and points to template directory
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	var template FlowTemplate
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	ftm.mu.Lock()
	ftm.templates[template.ID] = &template
	ftm.mu.Unlock()

	return &template, nil
}

// LoadAllTemplates loads all templates from the template directory
func (ftm *FlowTemplateManager) LoadAllTemplates() error {
	if _, err := os.Stat(ftm.templateDir); os.IsNotExist(err) {
		return nil // Directory doesn't exist yet, nothing to load
	}

	files, err := filepath.Glob(filepath.Join(ftm.templateDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list template files: %w", err)
	}

	for _, file := range files {
		if _, err := ftm.LoadTemplate(file); err != nil {
			return fmt.Errorf("failed to load template %s: %w", file, err)
		}
	}

	return nil
}

// GetTemplate retrieves a template by ID
func (ftm *FlowTemplateManager) GetTemplate(templateID uuid.UUID) (*FlowTemplate, error) {
	ftm.mu.RLock()
	defer ftm.mu.RUnlock()

	template, exists := ftm.templates[templateID]
	if !exists {
		return nil, fmt.Errorf("template %s not found", templateID)
	}

	return template, nil
}

// ListTemplates returns all available templates
func (ftm *FlowTemplateManager) ListTemplates() []*FlowTemplate {
	ftm.mu.RLock()
	defer ftm.mu.RUnlock()

	templates := make([]*FlowTemplate, 0, len(ftm.templates))
	for _, template := range ftm.templates {
		templates = append(templates, template)
	}

	return templates
}

// DeleteTemplate deletes a template
func (ftm *FlowTemplateManager) DeleteTemplate(templateID uuid.UUID) error {
	ftm.mu.Lock()
	delete(ftm.templates, templateID)
	ftm.mu.Unlock()

	// Delete file from disk
	filename := filepath.Join(ftm.templateDir, fmt.Sprintf("%s.json", templateID))
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete template file: %w", err)
	}

	return nil
}

// UpdateTemplate updates an existing template
func (ftm *FlowTemplateManager) UpdateTemplate(templateID uuid.UUID, updates *FlowTemplate) error {
	ftm.mu.Lock()
	defer ftm.mu.Unlock()

	template, exists := ftm.templates[templateID]
	if !exists {
		return fmt.Errorf("template %s not found", templateID)
	}

	// Update fields
	template.Name = updates.Name
	template.Description = updates.Description
	template.Tags = updates.Tags
	template.UpdatedAt = time.Now()

	return nil
}

// ExportFlow exports the entire flow configuration
func (ftm *FlowTemplateManager) ExportFlow(name, description, author string, format FlowExportFormat) (*FlowExport, error) {
	export := &FlowExport{
		Metadata: FlowMetadata{
			Name:        name,
			Description: description,
			Author:      author,
			Tags:        []string{},
			Variables:   make(map[string]string),
		},
		Processors:    make([]ProcessorExport, 0),
		Connections:   make([]ConnectionExport, 0),
		ProcessGroups: make([]ProcessGroupExport, 0),
		ExportedAt:    time.Now(),
		Version:       "1.0.0",
	}

	// Export all processors
	processors := ftm.flowController.GetProcessors()
	for _, proc := range processors {
		proc.RLock()
		procExport := ProcessorExport{
			ID:            proc.ID.String(),
			Name:          proc.Name,
			Type:          proc.Type,
			Properties:    proc.Config.Properties,
			Schedule:      proc.Config,
			AutoTerminate: proc.Config.AutoTerminate,
			Position:      Position{X: 0, Y: 0},
		}
		proc.RUnlock()

		export.Processors = append(export.Processors, procExport)
	}

	// Export all connections
	connections := ftm.flowController.GetConnections()
	for _, conn := range connections {
		conn.RLock()
		connExport := ConnectionExport{
			ID:                 conn.ID.String(),
			Name:               conn.Name,
			SourceID:           conn.Source.ID.String(),
			DestinationID:      conn.Destination.ID.String(),
			SourceRelationship: conn.Relationship.Name,
			QueueSize:          int(conn.Queue.maxSize),
		}
		conn.RUnlock()

		export.Connections = append(export.Connections, connExport)
	}

	return export, nil
}

// ImportFlow imports a flow configuration
func (ftm *FlowTemplateManager) ImportFlow(export *FlowExport) ([]uuid.UUID, error) {
	processorIDMap := make(map[string]uuid.UUID)
	createdProcessors := make([]uuid.UUID, 0)

	// Import processors
	for _, procExport := range export.Processors {
		config := types.ProcessorConfig{
			ID:            uuid.New(),
			Name:          procExport.Name,
			Type:          procExport.Type,
			Properties:    procExport.Properties,
			ScheduleType:  procExport.Schedule.ScheduleType,
			ScheduleValue: procExport.Schedule.ScheduleValue,
			Concurrency:   procExport.Schedule.Concurrency,
			AutoTerminate: procExport.AutoTerminate,
		}

		processor, err := ftm.flowController.CreateProcessorByType(procExport.Type, config)
		if err != nil {
			// Cleanup on error
			for _, procID := range createdProcessors {
				_ = ftm.flowController.RemoveProcessor(procID)
			}
			return nil, fmt.Errorf("failed to import processor %s: %w", procExport.Name, err)
		}

		processorIDMap[procExport.ID] = processor.ID
		createdProcessors = append(createdProcessors, processor.ID)
	}

	// Import connections
	for _, connExport := range export.Connections {
		sourceID, sourceExists := processorIDMap[connExport.SourceID]
		destID, destExists := processorIDMap[connExport.DestinationID]

		if !sourceExists || !destExists {
			continue
		}

		relationship := types.Relationship{
			Name:        connExport.SourceRelationship,
			Description: "",
		}

		_, err := ftm.flowController.AddConnection(sourceID, destID, relationship)
		if err != nil {
			return createdProcessors, fmt.Errorf("failed to import connection: %w", err)
		}
	}

	return createdProcessors, nil
}

// Helper function to substitute variables in strings
func substituteVariables(input string, variables map[string]string) string {
	result := input
	for key, value := range variables {
		placeholder := fmt.Sprintf("${%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// SearchTemplates searches templates by name, description, or tags
func (ftm *FlowTemplateManager) SearchTemplates(query string) []*FlowTemplate {
	ftm.mu.RLock()
	defer ftm.mu.RUnlock()

	results := make([]*FlowTemplate, 0)
	queryLower := strings.ToLower(query)

	for _, template := range ftm.templates {
		// Search in name
		if strings.Contains(strings.ToLower(template.Name), queryLower) {
			results = append(results, template)
			continue
		}

		// Search in description
		if strings.Contains(strings.ToLower(template.Description), queryLower) {
			results = append(results, template)
			continue
		}

		// Search in tags
		for _, tag := range template.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				results = append(results, template)
				break
			}
		}
	}

	return results
}
