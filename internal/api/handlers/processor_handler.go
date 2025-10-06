package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/api/models"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ProcessorHandler handles processor-related HTTP requests
type ProcessorHandler struct {
	flowController *core.FlowController
}

// NewProcessorHandler creates a new ProcessorHandler
func NewProcessorHandler(flowController *core.FlowController) *ProcessorHandler {
	return &ProcessorHandler{
		flowController: flowController,
	}
}

// ListProcessors handles GET /api/processors
func (h *ProcessorHandler) ListProcessors(c *gin.Context) {
	processorsMap := h.flowController.GetProcessors()

	processorDTOs := make([]models.ProcessorDTO, 0, len(processorsMap))
	for _, processor := range processorsMap {
		processorDTOs = append(processorDTOs, models.ProcessorDTO{
			ID:             processor.ID,
			Name:           processor.Name,
			Type:           processor.Type,
			State:          string(processor.Status.State),
			Properties:     processor.Config.Properties,
			ScheduleType:   string(processor.Config.ScheduleType),
			ScheduleValue:  processor.Config.ScheduleValue,
			Concurrency:    processor.Config.Concurrency,
			AutoTerminate:  processor.Config.AutoTerminate,
			ProcessGroupID: nil, // ProcessorNode doesn't have ProcessGroupID field
		})
	}

	response := models.ListResponse{
		Items: processorDTOs,
		Total: len(processorDTOs),
	}

	c.JSON(http.StatusOK, response)
}

// GetProcessor handles GET /api/processors/:id
func (h *ProcessorHandler) GetProcessor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	processor, exists := h.flowController.GetProcessor(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Processor not found",
		})
		return
	}

	processorDTO := models.ProcessorDTO{
		ID:             processor.ID,
		Name:           processor.Name,
		Type:           processor.Type,
		State:          string(processor.Status.State),
		Properties:     processor.Config.Properties,
		ScheduleType:   string(processor.Config.ScheduleType),
		ScheduleValue:  processor.Config.ScheduleValue,
		Concurrency:    processor.Config.Concurrency,
		AutoTerminate:  processor.Config.AutoTerminate,
		ProcessGroupID: nil,
	}

	c.JSON(http.StatusOK, processorDTO)
}

// UpdateProcessor handles PUT /api/processors/:id
func (h *ProcessorHandler) UpdateProcessor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	var req models.UpdateProcessorRequest
	if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: bindErr.Error(),
		})
		return
	}

	// Get current processor
	processor, exists := h.flowController.GetProcessor(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Processor not found",
		})
		return
	}

	// Apply updates
	config := processor.Config
	if req.Name != nil {
		config.Name = *req.Name
	}
	if req.Properties != nil {
		config.Properties = req.Properties
	}
	if req.ScheduleType != nil {
		config.ScheduleType = types.ScheduleType(*req.ScheduleType)
	}
	if req.ScheduleValue != nil {
		config.ScheduleValue = *req.ScheduleValue
	}
	if req.Concurrency != nil {
		config.Concurrency = *req.Concurrency
	}
	if req.AutoTerminate != nil {
		config.AutoTerminate = req.AutoTerminate
	}

	// Update processor
	processor, err = h.flowController.UpdateProcessor(id, config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "update_failed",
			Message: err.Error(),
		})
		return
	}

	processorDTO := models.ProcessorDTO{
		ID:             processor.ID,
		Name:           processor.Name,
		Type:           processor.Type,
		State:          string(processor.Status.State),
		Properties:     processor.Config.Properties,
		ScheduleType:   string(processor.Config.ScheduleType),
		ScheduleValue:  processor.Config.ScheduleValue,
		Concurrency:    processor.Config.Concurrency,
		AutoTerminate:  processor.Config.AutoTerminate,
		ProcessGroupID: nil,
	}

	c.JSON(http.StatusOK, processorDTO)
}

// DeleteProcessor handles DELETE /api/processors/:id
func (h *ProcessorHandler) DeleteProcessor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	if err := h.flowController.RemoveProcessor(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "deletion_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Success: true,
		Message: "Processor deleted successfully",
	})
}

// StartProcessor handles PUT /api/processors/:id/start
func (h *ProcessorHandler) StartProcessor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	if err := h.flowController.StartProcessor(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "start_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Success: true,
		Message: "Processor started successfully",
	})
}

// StopProcessor handles PUT /api/processors/:id/stop
func (h *ProcessorHandler) StopProcessor(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	if err := h.flowController.StopProcessor(id); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "stop_failed",
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse{
		Success: true,
		Message: "Processor stopped successfully",
	})
}

// GetProcessorStatus handles GET /api/processors/:id/status
func (h *ProcessorHandler) GetProcessorStatus(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid processor ID",
		})
		return
	}

	processor, exists := h.flowController.GetProcessor(id)
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Processor not found",
		})
		return
	}

	statusDTO := models.ProcessorStatusToDTO(processor.Status)

	c.JSON(http.StatusOK, statusDTO)
}
