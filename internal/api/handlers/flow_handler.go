package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/api/models"
	"github.com/shawntherrien/databridge/internal/core"
)

// FlowHandler handles flow-related HTTP requests
type FlowHandler struct {
	flowController *core.FlowController
}

// NewFlowHandler creates a new FlowHandler
func NewFlowHandler(flowController *core.FlowController) *FlowHandler {
	return &FlowHandler{
		flowController: flowController,
	}
}

// ListFlows handles GET /api/flows
func (h *FlowHandler) ListFlows(c *gin.Context) {
	flowsMap := h.flowController.GetProcessGroups()

	flowDTOs := make([]models.ProcessGroupDTO, 0, len(flowsMap))
	for _, flow := range flowsMap {
		var parentID *uuid.UUID
		if flow.Parent != nil {
			parentID = &flow.Parent.ID
		}

		flowDTOs = append(flowDTOs, models.ProcessGroupDTO{
			ID:       flow.ID,
			Name:     flow.Name,
			ParentID: parentID,
		})
	}

	response := models.ListResponse{
		Items: flowDTOs,
		Total: len(flowDTOs),
	}

	c.JSON(http.StatusOK, response)
}

// CreateFlow handles POST /api/flows
func (h *FlowHandler) CreateFlow(c *gin.Context) {
	var req models.CreateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	flow, err := h.flowController.CreateProcessGroup(req.Name, req.ParentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: err.Error(),
		})
		return
	}

	var parentID *uuid.UUID
	if flow.Parent != nil {
		parentID = &flow.Parent.ID
	}

	flowDTO := models.ProcessGroupDTO{
		ID:       flow.ID,
		Name:     flow.Name,
		ParentID: parentID,
	}

	c.JSON(http.StatusCreated, flowDTO)
}

// GetFlow handles GET /api/flows/:id
func (h *FlowHandler) GetFlow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid flow ID",
		})
		return
	}

	flow, exists := h.flowController.GetProcessGroup(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Flow not found",
		})
		return
	}

	var parentID *uuid.UUID
	if flow.Parent != nil {
		parentID = &flow.Parent.ID
	}

	flowDTO := models.ProcessGroupDTO{
		ID:       flow.ID,
		Name:     flow.Name,
		ParentID: parentID,
	}

	c.JSON(http.StatusOK, flowDTO)
}

// UpdateFlow handles PUT /api/flows/:id
func (h *FlowHandler) UpdateFlow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid flow ID",
		})
		return
	}

	var req models.UpdateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	flow, err := h.flowController.UpdateProcessGroup(id, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	var parentID *uuid.UUID
	if flow.Parent != nil {
		parentID = &flow.Parent.ID
	}

	flowDTO := models.ProcessGroupDTO{
		ID:       flow.ID,
		Name:     flow.Name,
		ParentID: parentID,
	}

	c.JSON(http.StatusOK, flowDTO)
}

// DeleteFlow handles DELETE /api/flows/:id
func (h *FlowHandler) DeleteFlow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid flow ID",
		})
		return
	}

	if err := h.flowController.DeleteProcessGroup(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "deletion_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Success: true,
		Message: "Flow deleted successfully",
	})
}

// GetFlowStatus handles GET /api/flows/:id/status
func (h *FlowHandler) GetFlowStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid flow ID",
		})
		return
	}

	flow, exists := h.flowController.GetProcessGroup(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Flow not found",
		})
		return
	}

	// Get flow status
	status := models.ProcessGroupStatusDTO{
		ID:   flow.ID,
		Name: flow.Name,
		// Other status fields would be populated from actual metrics
		ActiveThreads:      0,
		QueuedCount:        0,
		BytesQueued:        0,
		FlowFilesReceived:  0,
		BytesReceived:      0,
		FlowFilesSent:      0,
		BytesSent:          0,
		ActiveProcessors:   0,
		StoppedProcessors:  0,
		InvalidProcessors:  0,
		DisabledProcessors: 0,
	}

	c.JSON(http.StatusOK, status)
}
