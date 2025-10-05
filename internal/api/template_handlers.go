package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Template represents a flow template
type Template struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Author      string                 `json:"author,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Variables   []TemplateVariable     `json:"variables,omitempty"`
	Processors  []TemplateProcessor    `json:"processors"`
	Connections []TemplateConnection   `json:"connections"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TemplateVariable represents a variable in a template
type TemplateVariable struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	Type         string `json:"type"` // string, number, boolean
	DefaultValue string `json:"defaultValue,omitempty"`
	Required     bool   `json:"required"`
}

// TemplateProcessor represents a processor in a template
type TemplateProcessor struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Position   *Position              `json:"position,omitempty"`
}

// TemplateConnection represents a connection in a template
type TemplateConnection struct {
	ID           string `json:"id"`
	SourceID     string `json:"sourceId"`
	TargetID     string `json:"targetId"`
	Relationship string `json:"relationship"`
}

// Position represents X/Y coordinates
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TemplateHandlers handles template-related HTTP requests
type TemplateHandlers struct {
	log       *logrus.Logger
	templates map[string]*Template
	mu        sync.RWMutex
}

// NewTemplateHandlers creates a new template handler
func NewTemplateHandlers(log *logrus.Logger) *TemplateHandlers {
	h := &TemplateHandlers{
		log:       log,
		templates: make(map[string]*Template),
	}

	// Add some example templates
	h.initializeExampleTemplates()

	return h
}

// initializeExampleTemplates adds example templates for demonstration
func (h *TemplateHandlers) initializeExampleTemplates() {
	now := time.Now()

	templates := []*Template{
		{
			ID:          uuid.New().String(),
			Name:        "File Ingestion Pipeline",
			Description: "Read files from a directory and process them",
			Author:      "DataBridge",
			Tags:        []string{"ingestion", "files", "basic"},
			Variables: []TemplateVariable{
				{Name: "inputDir", DisplayName: "Input Directory", Type: "string", DefaultValue: "/data/input", Required: true},
				{Name: "outputDir", DisplayName: "Output Directory", Type: "string", DefaultValue: "/data/output", Required: true},
			},
			Processors: []TemplateProcessor{
				{ID: "proc-1", Type: "GetFile", Name: "Read Files", Position: &Position{X: 100, Y: 100}},
				{ID: "proc-2", Type: "PutFile", Name: "Write Files", Position: &Position{X: 400, Y: 100}},
			},
			Connections: []TemplateConnection{
				{ID: "conn-1", SourceID: "proc-1", TargetID: "proc-2", Relationship: "success"},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          uuid.New().String(),
			Name:        "JSON Transform Pipeline",
			Description: "Transform JSON data with custom mappings",
			Author:      "DataBridge",
			Tags:        []string{"transform", "json", "etl"},
			Variables: []TemplateVariable{
				{Name: "transformPath", DisplayName: "JSONPath Expression", Type: "string", DefaultValue: "$.data", Required: true},
			},
			Processors: []TemplateProcessor{
				{ID: "proc-1", Type: "GenerateFlowFile", Name: "Generate Data", Position: &Position{X: 100, Y: 100}},
				{ID: "proc-2", Type: "TransformJSON", Name: "Transform", Position: &Position{X: 400, Y: 100}},
				{ID: "proc-3", Type: "PutFile", Name: "Output", Position: &Position{X: 700, Y: 100}},
			},
			Connections: []TemplateConnection{
				{ID: "conn-1", SourceID: "proc-1", TargetID: "proc-2", Relationship: "success"},
				{ID: "conn-2", SourceID: "proc-2", TargetID: "proc-3", Relationship: "success"},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          uuid.New().String(),
			Name:        "Database to File Export",
			Description: "Query database and export results to files",
			Author:      "DataBridge",
			Tags:        []string{"database", "export", "etl"},
			Variables: []TemplateVariable{
				{Name: "dbConnection", DisplayName: "Database Connection", Type: "string", Required: true},
				{Name: "query", DisplayName: "SQL Query", Type: "string", Required: true},
				{Name: "outputPath", DisplayName: "Output Path", Type: "string", DefaultValue: "/data/export", Required: true},
			},
			Processors: []TemplateProcessor{
				{ID: "proc-1", Type: "ExecuteSQL", Name: "Query Database", Position: &Position{X: 100, Y: 100}},
				{ID: "proc-2", Type: "TransformJSON", Name: "Format Results", Position: &Position{X: 400, Y: 100}},
				{ID: "proc-3", Type: "PutFile", Name: "Write File", Position: &Position{X: 700, Y: 100}},
			},
			Connections: []TemplateConnection{
				{ID: "conn-1", SourceID: "proc-1", TargetID: "proc-2", Relationship: "success"},
				{ID: "conn-2", SourceID: "proc-2", TargetID: "proc-3", Relationship: "success"},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, tmpl := range templates {
		h.templates[tmpl.ID] = tmpl
	}
}

// HandleListTemplates handles GET /api/templates
func (h *TemplateHandlers) HandleListTemplates(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	templates := make([]*Template, 0, len(h.templates))
	for _, tmpl := range h.templates {
		templates = append(templates, tmpl)
	}

	respondJSON(w, http.StatusOK, templates)
}

// HandleGetTemplate handles GET /api/templates/{id}
func (h *TemplateHandlers) HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.mu.RLock()
	tmpl, exists := h.templates[id]
	h.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, "Template not found")
		return
	}

	respondJSON(w, http.StatusOK, tmpl)
}

// HandleCreateTemplate handles POST /api/templates
func (h *TemplateHandlers) HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var req Template
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now()
	tmpl := &Template{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Author:      req.Author,
		Tags:        req.Tags,
		Variables:   req.Variables,
		Processors:  req.Processors,
		Connections: req.Connections,
		Metadata:    req.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	h.mu.Lock()
	h.templates[tmpl.ID] = tmpl
	h.mu.Unlock()

	h.log.WithField("templateId", tmpl.ID).Info("Template created")
	respondJSON(w, http.StatusCreated, tmpl)
}

// HandleUpdateTemplate handles PUT /api/templates/{id}
func (h *TemplateHandlers) HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req Template
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	tmpl, exists := h.templates[id]
	if !exists {
		respondError(w, http.StatusNotFound, "Template not found")
		return
	}

	// Update fields
	tmpl.Name = req.Name
	tmpl.Description = req.Description
	tmpl.Author = req.Author
	tmpl.Tags = req.Tags
	tmpl.Variables = req.Variables
	tmpl.Processors = req.Processors
	tmpl.Connections = req.Connections
	tmpl.Metadata = req.Metadata
	tmpl.UpdatedAt = time.Now()

	h.log.WithField("templateId", id).Info("Template updated")
	respondJSON(w, http.StatusOK, tmpl)
}

// HandleDeleteTemplate handles DELETE /api/templates/{id}
func (h *TemplateHandlers) HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.templates[id]; !exists {
		respondError(w, http.StatusNotFound, "Template not found")
		return
	}

	delete(h.templates, id)

	h.log.WithField("templateId", id).Info("Template deleted")
	w.WriteHeader(http.StatusNoContent)
}

// HandleInstantiateTemplate handles POST /api/templates/{id}/instantiate
func (h *TemplateHandlers) HandleInstantiateTemplate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Variables map[string]string `json:"variables"`
		FlowName  string            `json:"flowName,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.RLock()
	tmpl, exists := h.templates[id]
	h.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, "Template not found")
		return
	}

	// Create flow from template
	flowName := req.FlowName
	if flowName == "" {
		flowName = tmpl.Name + " - " + time.Now().Format("2006-01-02 15:04:05")
	}

	result := map[string]interface{}{
		"flowId":      uuid.New().String(),
		"flowName":    flowName,
		"templateId":  id,
		"processors":  len(tmpl.Processors),
		"connections": len(tmpl.Connections),
		"message":     "Template instantiated successfully",
	}

	h.log.WithFields(logrus.Fields{
		"templateId": id,
		"flowName":   flowName,
	}).Info("Template instantiated")

	respondJSON(w, http.StatusCreated, result)
}

// HandleSearchTemplates handles GET /api/templates/search?q=query
func (h *TemplateHandlers) HandleSearchTemplates(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	h.mu.RLock()
	defer h.mu.RUnlock()

	var results []*Template
	for _, tmpl := range h.templates {
		// Simple search in name, description, tags
		if contains(tmpl.Name, query) || contains(tmpl.Description, query) || containsTag(tmpl.Tags, query) {
			results = append(results, tmpl)
		}
	}

	respondJSON(w, http.StatusOK, results)
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if contains(tag, query) {
			return true
		}
	}
	return false
}
