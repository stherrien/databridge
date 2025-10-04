package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/api/models"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ConnectionHandler handles connection-related HTTP requests
type ConnectionHandler struct {
	flowController *core.FlowController
}

// NewConnectionHandler creates a new ConnectionHandler
func NewConnectionHandler(flowController *core.FlowController) *ConnectionHandler {
	return &ConnectionHandler{
		flowController: flowController,
	}
}

// ListConnections handles GET /api/connections
func (h *ConnectionHandler) ListConnections(c *gin.Context) {
	connectionsMap := h.flowController.GetConnections()

	connectionDTOs := make([]models.ConnectionDTO, 0, len(connectionsMap))
	for _, conn := range connectionsMap {
		connectionDTOs = append(connectionDTOs, models.ConnectionDTO{
			ID:               conn.ID,
			Name:             conn.Name,
			SourceID:         conn.Source.ID,
			DestinationID:    conn.Destination.ID,
			Relationship:     conn.Relationship.Name,
			BackPressureSize: conn.BackPressureSize,
			QueuedCount:      conn.Queue.Size(),
			ProcessGroupID:   nil, // Connection doesn't have ProcessGroupID field
		})
	}

	response := models.ListResponse{
		Items: connectionDTOs,
		Total: len(connectionDTOs),
	}

	c.JSON(http.StatusOK, response)
}

// CreateConnection handles POST /api/connections
func (h *ConnectionHandler) CreateConnection(c *gin.Context) {
	var req models.CreateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	// Parse relationship
	relationship := types.Relationship{
		Name:        req.Relationship,
		Description: "",
	}

	connection, err := h.flowController.AddConnection(req.SourceID, req.DestinationID, relationship)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "creation_failed",
			Message: err.Error(),
		})
		return
	}

	// Update name if provided
	if req.Name != "" {
		connection.Name = req.Name
	}

	// Update back pressure size if provided
	if req.BackPressureSize != nil {
		connection.BackPressureSize = *req.BackPressureSize
	}

	connectionDTO := models.ConnectionDTO{
		ID:               connection.ID,
		Name:             connection.Name,
		SourceID:         connection.Source.ID,
		DestinationID:    connection.Destination.ID,
		Relationship:     connection.Relationship.Name,
		BackPressureSize: connection.BackPressureSize,
		QueuedCount:      connection.Queue.Size(),
		ProcessGroupID:   nil,
	}

	c.JSON(http.StatusCreated, connectionDTO)
}

// GetConnection handles GET /api/connections/:id
func (h *ConnectionHandler) GetConnection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid connection ID",
		})
		return
	}

	connection, exists := h.flowController.GetConnection(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Connection not found",
		})
		return
	}

	connectionDTO := models.ConnectionDTO{
		ID:               connection.ID,
		Name:             connection.Name,
		SourceID:         connection.Source.ID,
		DestinationID:    connection.Destination.ID,
		Relationship:     connection.Relationship.Name,
		BackPressureSize: connection.BackPressureSize,
		QueuedCount:      connection.Queue.Size(),
		ProcessGroupID:   nil,
	}

	c.JSON(http.StatusOK, connectionDTO)
}

// UpdateConnection handles PUT /api/connections/:id
func (h *ConnectionHandler) UpdateConnection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid connection ID",
		})
		return
	}

	var req models.UpdateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
		return
	}

	// Get current connection
	connection, exists := h.flowController.GetConnection(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Connection not found",
		})
		return
	}

	// Apply updates
	if req.Name != nil {
		connection.Name = *req.Name
	}
	if req.BackPressureSize != nil {
		connection.BackPressureSize = *req.BackPressureSize
	}

	connectionDTO := models.ConnectionDTO{
		ID:               connection.ID,
		Name:             connection.Name,
		SourceID:         connection.Source.ID,
		DestinationID:    connection.Destination.ID,
		Relationship:     connection.Relationship.Name,
		BackPressureSize: connection.BackPressureSize,
		QueuedCount:      connection.Queue.Size(),
		ProcessGroupID:   nil,
	}

	c.JSON(http.StatusOK, connectionDTO)
}

// DeleteConnection handles DELETE /api/connections/:id
func (h *ConnectionHandler) DeleteConnection(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid connection ID",
		})
		return
	}

	if err := h.flowController.RemoveConnection(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "deletion_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Success: true,
		Message: "Connection deleted successfully",
	})
}
