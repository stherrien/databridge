package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/core"
)

// SchedulerHandler handles scheduler API requests
type SchedulerHandler struct {
	scheduler *core.ProcessScheduler
}

// NewSchedulerHandler creates a new scheduler handler
func NewSchedulerHandler(scheduler *core.ProcessScheduler) *SchedulerHandler {
	return &SchedulerHandler{
		scheduler: scheduler,
	}
}

// SetLoadThresholdsRequest represents load threshold configuration
type SetLoadThresholdsRequest struct {
	CPU        float64 `json:"cpu" binding:"required,min=0,max=1"`
	Memory     float64 `json:"memory" binding:"required,min=0,max=1"`
	Goroutines int     `json:"goroutines" binding:"required,min=0"`
}

// AddExecutionRuleRequest represents an execution rule creation request
type AddExecutionRuleRequest struct {
	ProcessorID  string                   `json:"processorId" binding:"required"`
	Name         string                   `json:"name" binding:"required"`
	Conditions   []core.Condition         `json:"conditions" binding:"required"`
	Action       core.RuleAction          `json:"action" binding:"required"`
	EvaluateMode core.EvaluateMode        `json:"evaluateMode" binding:"required"`
	Enabled      bool                     `json:"enabled"`
}

// RegisterRoutes registers scheduler API routes
func (h *SchedulerHandler) RegisterRoutes(router *gin.RouterGroup) {
	scheduler := router.Group("/scheduler")
	{
		// Load-based scheduling
		scheduler.GET("/load", h.GetSystemLoad)
		scheduler.GET("/load/average", h.GetAverageLoad)
		scheduler.POST("/load/thresholds", h.SetLoadThresholds)

		// Priority scheduling
		scheduler.GET("/priority/queues", h.GetPriorityQueueDepths)

		// Conditional scheduling
		scheduler.POST("/rules", h.AddExecutionRule)
		scheduler.GET("/rules/:processorId", h.GetExecutionRules)
		scheduler.DELETE("/rules/:ruleId", h.RemoveExecutionRule)
	}
}

// GetSystemLoad returns current system load metrics
// @Summary Get system load
// @Tags Scheduler
// @Produce json
// @Success 200 {object} core.SystemLoad
// @Router /scheduler/load [get]
func (h *SchedulerHandler) GetSystemLoad(c *gin.Context) {
	load := h.scheduler.GetSystemLoad()
	c.JSON(http.StatusOK, load)
}

// GetAverageLoad returns average system load
// @Summary Get average load
// @Tags Scheduler
// @Produce json
// @Success 200 {object} core.SystemLoad
// @Router /scheduler/load/average [get]
func (h *SchedulerHandler) GetAverageLoad(c *gin.Context) {
	load := h.scheduler.GetAverageLoad()
	c.JSON(http.StatusOK, load)
}

// SetLoadThresholds configures load thresholds
// @Summary Set load thresholds
// @Tags Scheduler
// @Accept json
// @Produce json
// @Param request body SetLoadThresholdsRequest true "Load thresholds"
// @Success 200 {object} map[string]interface{}
// @Router /scheduler/load/thresholds [post]
func (h *SchedulerHandler) SetLoadThresholds(c *gin.Context) {
	var req SetLoadThresholdsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.scheduler.SetLoadThresholds(req.CPU, req.Memory, req.Goroutines)

	c.JSON(http.StatusOK, gin.H{
		"message": "load thresholds updated successfully",
		"cpu":     req.CPU,
		"memory":  req.Memory,
		"goroutines": req.Goroutines,
	})
}

// GetPriorityQueueDepths returns priority queue depths
// @Summary Get priority queue depths
// @Tags Scheduler
// @Produce json
// @Success 200 {object} map[string]int
// @Router /scheduler/priority/queues [get]
func (h *SchedulerHandler) GetPriorityQueueDepths(c *gin.Context) {
	depths := h.scheduler.GetPriorityQueueDepths()

	// Convert to string keys for JSON
	result := make(map[string]int)
	for priority, depth := range depths {
		var priorityName string
		switch priority {
		case core.PriorityCritical:
			priorityName = "critical"
		case core.PriorityHigh:
			priorityName = "high"
		case core.PriorityNormal:
			priorityName = "normal"
		case core.PriorityLow:
			priorityName = "low"
		case core.PriorityIdle:
			priorityName = "idle"
		}
		result[priorityName] = depth
	}

	c.JSON(http.StatusOK, result)
}

// AddExecutionRule adds a conditional execution rule
// @Summary Add execution rule
// @Tags Scheduler
// @Accept json
// @Produce json
// @Param request body AddExecutionRuleRequest true "Execution rule"
// @Success 201 {object} core.ExecutionRule
// @Router /scheduler/rules [post]
func (h *SchedulerHandler) AddExecutionRule(c *gin.Context) {
	var req AddExecutionRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processorID, err := uuid.Parse(req.ProcessorID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processor ID"})
		return
	}

	rule := &core.ExecutionRule{
		ID:           uuid.New(),
		ProcessorID:  processorID,
		Name:         req.Name,
		Conditions:   req.Conditions,
		Action:       req.Action,
		Enabled:      req.Enabled,
		EvaluateMode: req.EvaluateMode,
	}

	if err := h.scheduler.AddExecutionRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// GetExecutionRules retrieves execution rules for a processor
// @Summary Get execution rules
// @Tags Scheduler
// @Produce json
// @Param processorId path string true "Processor ID"
// @Success 200 {array} core.ExecutionRule
// @Router /scheduler/rules/{processorId} [get]
func (h *SchedulerHandler) GetExecutionRules(c *gin.Context) {
	idStr := c.Param("processorId")
	processorID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processor ID"})
		return
	}

	rules := h.scheduler.GetExecutionRules(processorID)
	c.JSON(http.StatusOK, rules)
}

// RemoveExecutionRule removes an execution rule
// @Summary Remove execution rule
// @Tags Scheduler
// @Param ruleId path string true "Rule ID"
// @Success 204
// @Router /scheduler/rules/{ruleId} [delete]
func (h *SchedulerHandler) RemoveExecutionRule(c *gin.Context) {
	idStr := c.Param("ruleId")
	ruleID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule ID"})
		return
	}

	h.scheduler.RemoveExecutionRule(ruleID)
	c.Status(http.StatusNoContent)
}

// GetSchedulerStatus returns detailed scheduler status
// @Summary Get scheduler status
// @Tags Scheduler
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /scheduler/status [get]
func (h *SchedulerHandler) GetSchedulerStatus(c *gin.Context) {
	load := h.scheduler.GetSystemLoad()
	avgLoad := h.scheduler.GetAverageLoad()
	queueDepths := h.scheduler.GetPriorityQueueDepths()

	status := gin.H{
		"currentLoad":       load,
		"averageLoad":       avgLoad,
		"priorityQueues":    queueDepths,
		"loadScheduling":    gin.H{"enabled": true},
		"priorityScheduling": gin.H{"enabled": true},
		"conditionalScheduling": gin.H{"enabled": true},
	}

	c.JSON(http.StatusOK, status)
}
