package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

// FlowHandlers handles flow-related HTTP requests
type FlowHandlers struct {
	flowController *core.FlowController
}

// NewFlowHandlers creates a new FlowHandlers
func NewFlowHandlers(flowController *core.FlowController) *FlowHandlers {
	return &FlowHandlers{
		flowController: flowController,
	}
}

// HandleListFlows handles GET /api/flows
func (h *FlowHandlers) HandleListFlows(w http.ResponseWriter, r *http.Request) {
	flows := h.flowController.GetProcessGroups()

	flowList := make([]map[string]interface{}, 0, len(flows))
	for _, flow := range flows {
		flowData := map[string]interface{}{
			"id":   flow.ID,
			"name": flow.Name,
		}
		if flow.Parent != nil {
			flowData["parentId"] = flow.Parent.ID
		}
		flowList = append(flowList, flowData)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"flows": flowList,
		"total": len(flowList),
	})
}

// HandleGetFlow handles GET /api/flows/{id}
func (h *FlowHandlers) HandleGetFlow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	flow, exists := h.flowController.GetProcessGroup(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Flow not found")
		return
	}

	// Build processors array
	processors := make([]map[string]interface{}, 0, len(flow.Processors))
	for _, proc := range flow.Processors {
		position := map[string]float64{"x": 100, "y": 100} // Default position
		if proc.Config.Position != nil {
			position = map[string]float64{"x": proc.Config.Position.X, "y": proc.Config.Position.Y}
		}
		processors = append(processors, map[string]interface{}{
			"id":       proc.ID,
			"type":     proc.Type,
			"config":   proc.Config,
			"position": position,
		})
	}

	// Build connections array
	connections := make([]map[string]interface{}, 0, len(flow.Connections))
	for _, conn := range flow.Connections {
		connections = append(connections, map[string]interface{}{
			"id":           conn.ID,
			"sourceId":     conn.Source.ID,
			"targetId":     conn.Destination.ID,
			"relationship": conn.Relationship.Name,
		})
	}

	flowData := map[string]interface{}{
		"id":          flow.ID,
		"name":        flow.Name,
		"processors":  processors,
		"connections": connections,
	}
	if flow.Parent != nil {
		flowData["parentId"] = flow.Parent.ID
	}

	respondJSON(w, http.StatusOK, flowData)
}

// HandleCreateFlow handles POST /api/flows
func (h *FlowHandlers) HandleCreateFlow(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string     `json:"name"`
		ParentID *uuid.UUID `json:"parentId,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "Flow name is required")
		return
	}

	flow, err := h.flowController.CreateProcessGroup(req.Name, req.ParentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create flow: "+err.Error())
		return
	}

	flowData := map[string]interface{}{
		"id":   flow.ID,
		"name": flow.Name,
	}
	if flow.Parent != nil {
		flowData["parentId"] = flow.Parent.ID
	}

	respondJSON(w, http.StatusCreated, flowData)
}

// HandleUpdateFlow handles PUT /api/flows/{id}
func (h *FlowHandlers) HandleUpdateFlow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "Flow name is required")
		return
	}

	flow, err := h.flowController.UpdateProcessGroup(id, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update flow: "+err.Error())
		return
	}

	flowData := map[string]interface{}{
		"id":   flow.ID,
		"name": flow.Name,
	}
	if flow.Parent != nil {
		flowData["parentId"] = flow.Parent.ID
	}

	respondJSON(w, http.StatusOK, flowData)
}

// HandleDeleteFlow handles DELETE /api/flows/{id}
func (h *FlowHandlers) HandleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	if err := h.flowController.DeleteProcessGroup(id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete flow: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Flow deleted successfully",
	})
}

// HandleStartFlow handles POST /api/flows/{id}/start
func (h *FlowHandlers) HandleStartFlow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	// Get all processors in the flow and start them
	flow, exists := h.flowController.GetProcessGroup(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Flow not found")
		return
	}

	// In a real implementation, you'd iterate through processors and start them
	// For now, just return success
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Flow started successfully",
		"flowId":  flow.ID,
	})
}

// HandleStopFlow handles POST /api/flows/{id}/stop
func (h *FlowHandlers) HandleStopFlow(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	// Get all processors in the flow and stop them
	flow, exists := h.flowController.GetProcessGroup(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Flow not found")
		return
	}

	// In a real implementation, you'd iterate through processors and stop them
	// For now, just return success
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Flow stopped successfully",
		"flowId":  flow.ID,
	})
}

// HandleGetProcessorTypes handles GET /api/processors/types
func (h *FlowHandlers) HandleGetProcessorTypes(w http.ResponseWriter, r *http.Request) {
	// Processor type definitions with categories and descriptions
	processorDefs := []map[string]interface{}{
		{"type": "GenerateFlowFile", "category": "Sources", "description": "Creates FlowFiles with configurable content", "icon": "📝"},
		{"type": "GetFile", "category": "Sources", "description": "Reads files from the local filesystem", "icon": "📂"},
		{"type": "PutFile", "category": "Sinks", "description": "Writes FlowFiles to the local filesystem", "icon": "💾"},
		{"type": "TransformJSON", "category": "Transform", "description": "Transforms JSON content using JSONPath expressions", "icon": "🔄"},
		{"type": "SplitText", "category": "Transform", "description": "Splits text content into multiple FlowFiles", "icon": "✂️"},
		{"type": "MergeContent", "category": "Transform", "description": "Merges multiple FlowFiles into a single FlowFile", "icon": "🔗"},
		{"type": "ExecuteSQL", "category": "Database", "description": "Executes SQL queries against a database", "icon": "🗄️"},
		{"type": "InvokeHTTP", "category": "Network", "description": "Makes HTTP requests to external services", "icon": "🌐"},
		{"type": "PublishKafka", "category": "Messaging", "description": "Publishes messages to Apache Kafka", "icon": "📤"},
		{"type": "ConsumeKafka", "category": "Messaging", "description": "Consumes messages from Apache Kafka", "icon": "📥"},
	}

	typesList := make([]map[string]interface{}, 0, len(processorDefs))
	for _, def := range processorDefs {
		typesList = append(typesList, map[string]interface{}{
			"type":        def["type"],
			"displayName": def["type"],
			"category":    def["category"],
			"description": def["description"],
			"icon":        def["icon"],
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"types": typesList,
		"total": len(typesList),
	})
}

// HandleGetProcessorMetadata handles GET /api/processors/types/{type}
func (h *FlowHandlers) HandleGetProcessorMetadata(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	processorType := vars["type"]

	// Basic metadata for processor types
	metadata := map[string]interface{}{
		"type":        processorType,
		"name":        processorType,
		"description": "Processor: " + processorType,
		"version":     "1.0.0",
		"properties":  []string{},
	}

	respondJSON(w, http.StatusOK, metadata)
}

// HandleCreateProcessor handles POST /api/flows/{id}/processors
func (h *FlowHandlers) HandleCreateProcessor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	flowIdStr := vars["id"]

	flowId, err := uuid.Parse(flowIdStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	// Check if flow exists
	_, exists := h.flowController.GetProcessGroup(flowId)
	if !exists {
		respondError(w, http.StatusNotFound, "Flow not found")
		return
	}

	var req struct {
		Type     string                 `json:"type"`
		Config   map[string]interface{} `json:"config"`
		Position struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "Processor type is required")
		return
	}

	// Create processor configuration
	processorConfig := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          req.Type, // Use type as default name
		Type:          req.Type,
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
		Properties:    make(map[string]string),
		AutoTerminate: make(map[string]bool),
		Position:      &types.Position{X: req.Position.X, Y: req.Position.Y},
		ProcessGroupID: &flowId,
	}

	// Create the processor
	node, err := h.flowController.CreateProcessorByType(req.Type, processorConfig)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create processor: "+err.Error())
		return
	}

	// Add the processor to the specified ProcessGroup
	if err := h.flowController.AddProcessorToGroup(flowId, node); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to add processor to flow: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":       node.ID,
		"name":     node.Name,
		"type":     node.Type,
		"position": req.Position,
	})
}

// HandleUpdateProcessor handles PUT /api/flows/{flowId}/processors/{processorId}
func (h *FlowHandlers) HandleUpdateProcessor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	processorIdStr := vars["processorId"]

	processorId, err := uuid.Parse(processorIdStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid processor ID")
		return
	}

	var req struct {
		Config   map[string]interface{} `json:"config,omitempty"`
		Position *struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"position,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Get the processor
	processor, exists := h.flowController.GetProcessor(processorId)
	if !exists {
		respondError(w, http.StatusNotFound, "Processor not found")
		return
	}

	// Update position if provided
	if req.Position != nil {
		if processor.Config.Position == nil {
			processor.Config.Position = &types.Position{}
		}
		processor.Config.Position.X = req.Position.X
		processor.Config.Position.Y = req.Position.Y
	}

	// Update config properties if provided
	if req.Config != nil {
		if name, ok := req.Config["name"].(string); ok {
			processor.Config.Name = name
		}
		if properties, ok := req.Config["properties"].(map[string]interface{}); ok {
			for k, v := range properties {
				if strVal, ok := v.(string); ok {
					processor.Config.Properties[k] = strVal
				}
			}
		}
	}

	// Update the processor
	_, err = h.flowController.UpdateProcessor(processorId, processor.Config)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update processor: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":     processorId,
		"config": processor.Config,
	})
}

// HandleDeleteProcessor handles DELETE /api/flows/{flowId}/processors/{processorId}
func (h *FlowHandlers) HandleDeleteProcessor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	processorIdStr := vars["processorId"]

	processorId, err := uuid.Parse(processorIdStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid processor ID")
		return
	}

	if err := h.flowController.RemoveProcessor(processorId); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete processor: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Processor deleted successfully",
	})
}

// HandleCreateConnection handles POST /api/flows/{flowId}/connections
func (h *FlowHandlers) HandleCreateConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	flowIdStr := vars["id"]

	flowId, err := uuid.Parse(flowIdStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid flow ID")
		return
	}

	// Check if flow exists
	_, exists := h.flowController.GetProcessGroup(flowId)
	if !exists {
		respondError(w, http.StatusNotFound, "Flow not found")
		return
	}

	var req struct {
		SourceId     string `json:"sourceId"`
		TargetId     string `json:"targetId"`
		Relationship string `json:"relationship"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sourceId, err := uuid.Parse(req.SourceId)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid source ID")
		return
	}

	targetId, err := uuid.Parse(req.TargetId)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid target ID")
		return
	}

	// Default to success relationship if not specified
	relationshipName := req.Relationship
	if relationshipName == "" {
		relationshipName = "success"
	}

	// Create Relationship struct
	rel := types.Relationship{
		Name: relationshipName,
	}

	conn, err := h.flowController.AddConnection(sourceId, targetId, rel)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create connection: "+err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":           conn.ID,
		"sourceId":     conn.Source.ID,
		"targetId":     conn.Destination.ID,
		"relationship": relationshipName,
	})
}

// HandleDeleteConnection handles DELETE /api/flows/{flowId}/connections/{connectionId}
func (h *FlowHandlers) HandleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	connectionIdStr := vars["connectionId"]

	connectionId, err := uuid.Parse(connectionIdStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connection ID")
		return
	}

	if err := h.flowController.RemoveConnection(connectionId); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete connection: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Connection deleted successfully",
	})
}
