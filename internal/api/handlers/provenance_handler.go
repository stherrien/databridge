package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/core"
)

// ProvenanceHandler handles provenance API requests
type ProvenanceHandler struct {
	repository core.ProvenanceRepository
}

// NewProvenanceHandler creates a new provenance handler
func NewProvenanceHandler(repository core.ProvenanceRepository) *ProvenanceHandler {
	return &ProvenanceHandler{
		repository: repository,
	}
}

// QueryEventsRequest represents a provenance query request
type QueryEventsRequest struct {
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	FlowFileID  *string    `json:"flowFileId,omitempty"`
	ProcessorID *string    `json:"processorId,omitempty"`
	EventType   string     `json:"eventType,omitempty"`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
}

// EventsResponse represents the response for event queries
type EventsResponse struct {
	Events  []*core.ProvenanceEvent `json:"events"`
	Total   int                     `json:"total"`
	Limit   int                     `json:"limit"`
	Offset  int                     `json:"offset"`
	HasMore bool                    `json:"hasMore"`
}

// TimelineResponse represents timeline data
type TimelineResponse struct {
	Events []*TimelineEvent `json:"events"`
	Start  time.Time        `json:"start"`
	End    time.Time        `json:"end"`
}

type TimelineEvent struct {
	ID            uuid.UUID `json:"id"`
	EventType     string    `json:"eventType"`
	FlowFileID    uuid.UUID `json:"flowFileId"`
	ProcessorID   uuid.UUID `json:"processorId"`
	ProcessorName string    `json:"processorName"`
	Timestamp     time.Time `json:"timestamp"`
	Duration      int64     `json:"duration"` // milliseconds
}

// RegisterRoutes registers provenance API routes
func (h *ProvenanceHandler) RegisterRoutes(router *gin.RouterGroup) {
	provenance := router.Group("/provenance")
	{
		// Event queries
		provenance.GET("/events", h.GetEvents)
		provenance.GET("/events/:id", h.GetEvent)
		provenance.POST("/events/search", h.SearchEvents)

		// Lineage
		provenance.GET("/lineage/:flowFileId", h.GetLineage)

		// Timeline
		provenance.GET("/timeline", h.GetTimeline)

		// Statistics
		provenance.GET("/stats", h.GetStatistics)
		provenance.GET("/stats/processors", h.GetProcessorStatistics)
		provenance.GET("/stats/event-types", h.GetEventTypeStatistics)
	}
}

// GetEvents retrieves provenance events with optional filtering
// @Summary Get provenance events
// @Tags Provenance
// @Produce json
// @Param startTime query string false "Start time (RFC3339)"
// @Param endTime query string false "End time (RFC3339)"
// @Param flowFileId query string false "FlowFile ID"
// @Param processorId query string false "Processor ID"
// @Param eventType query string false "Event type"
// @Param limit query int false "Limit results" default(100)
// @Param offset query int false "Offset results" default(0)
// @Success 200 {object} EventsResponse
// @Router /provenance/events [get]
func (h *ProvenanceHandler) GetEvents(c *gin.Context) {
	query := core.ProvenanceQuery{
		Limit:  100,
		Offset: 0,
	}

	// Parse query parameters
	if startTimeStr := c.Query("startTime"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
			return
		}
		query.StartTime = &startTime
	}

	if endTimeStr := c.Query("endTime"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
			return
		}
		query.EndTime = &endTime
	}

	if flowFileIDStr := c.Query("flowFileId"); flowFileIDStr != "" {
		flowFileID, err := uuid.Parse(flowFileIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flow file ID"})
			return
		}
		query.FlowFileID = &flowFileID
	}

	if processorIDStr := c.Query("processorId"); processorIDStr != "" {
		processorID, err := uuid.Parse(processorIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processor ID"})
			return
		}
		query.ProcessorID = &processorID
	}

	query.EventType = c.Query("eventType")

	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		query.Limit = limit
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		query.Offset = offset
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := EventsResponse{
		Events:  events,
		Total:   len(events),
		Limit:   query.Limit,
		Offset:  query.Offset,
		HasMore: len(events) == query.Limit,
	}

	c.JSON(http.StatusOK, response)
}

// GetEvent retrieves a specific provenance event by ID
// @Summary Get provenance event
// @Tags Provenance
// @Produce json
// @Param id path string true "Event ID"
// @Success 200 {object} core.ProvenanceEvent
// @Router /provenance/events/{id} [get]
func (h *ProvenanceHandler) GetEvent(c *gin.Context) {
	idStr := c.Param("id")
	eventID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	// Query for the specific event
	query := core.ProvenanceQuery{
		Limit: 1,
	}

	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Find the event by ID
	for _, event := range events {
		if event.ID == eventID {
			c.JSON(http.StatusOK, event)
			return
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
}

// SearchEvents searches for provenance events using POST body
// @Summary Search provenance events
// @Tags Provenance
// @Accept json
// @Produce json
// @Param request body QueryEventsRequest true "Search criteria"
// @Success 200 {object} EventsResponse
// @Router /provenance/events/search [post]
func (h *ProvenanceHandler) SearchEvents(c *gin.Context) {
	var req QueryEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	query := core.ProvenanceQuery{
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		EventType: req.EventType,
		Limit:     req.Limit,
		Offset:    req.Offset,
	}

	if req.Limit == 0 {
		query.Limit = 100
	}

	if req.FlowFileID != nil {
		flowFileID, err := uuid.Parse(*req.FlowFileID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flow file ID"})
			return
		}
		query.FlowFileID = &flowFileID
	}

	if req.ProcessorID != nil {
		processorID, err := uuid.Parse(*req.ProcessorID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processor ID"})
			return
		}
		query.ProcessorID = &processorID
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response
	response := EventsResponse{
		Events:  events,
		Total:   len(events),
		Limit:   query.Limit,
		Offset:  query.Offset,
		HasMore: len(events) == query.Limit,
	}

	c.JSON(http.StatusOK, response)
}

// GetLineage retrieves data lineage for a FlowFile
// @Summary Get FlowFile lineage
// @Tags Provenance
// @Produce json
// @Param flowFileId path string true "FlowFile ID"
// @Success 200 {object} core.LineageGraph
// @Router /provenance/lineage/{flowFileId} [get]
func (h *ProvenanceHandler) GetLineage(c *gin.Context) {
	idStr := c.Param("flowFileId")
	flowFileID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid flow file ID"})
		return
	}

	lineage, err := h.repository.GetLineage(flowFileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, lineage)
}

// GetTimeline retrieves a chronological timeline of events
// @Summary Get event timeline
// @Tags Provenance
// @Produce json
// @Param startTime query string false "Start time (RFC3339)"
// @Param endTime query string false "End time (RFC3339)"
// @Param processorId query string false "Processor ID"
// @Param limit query int false "Limit results" default(100)
// @Success 200 {object} TimelineResponse
// @Router /provenance/timeline [get]
func (h *ProvenanceHandler) GetTimeline(c *gin.Context) {
	query := core.ProvenanceQuery{
		Limit: 100,
	}

	// Parse query parameters
	if startTimeStr := c.Query("startTime"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
			return
		}
		query.StartTime = &startTime
	}

	if endTimeStr := c.Query("endTime"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
			return
		}
		query.EndTime = &endTime
	}

	if processorIDStr := c.Query("processorId"); processorIDStr != "" {
		processorID, err := uuid.Parse(processorIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processor ID"})
			return
		}
		query.ProcessorID = &processorID
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		query.Limit = limit
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build timeline response
	timelineEvents := make([]*TimelineEvent, len(events))
	var start, end time.Time

	for i, event := range events {
		timelineEvents[i] = &TimelineEvent{
			ID:            event.ID,
			EventType:     event.EventType,
			FlowFileID:    event.FlowFileID,
			ProcessorID:   event.ProcessorID,
			ProcessorName: event.ProcessorName,
			Timestamp:     event.EventTime,
			Duration:      event.Duration.Milliseconds(),
		}

		if i == 0 || event.EventTime.Before(start) {
			start = event.EventTime
		}
		if i == 0 || event.EventTime.After(end) {
			end = event.EventTime
		}
	}

	response := TimelineResponse{
		Events: timelineEvents,
		Start:  start,
		End:    end,
	}

	c.JSON(http.StatusOK, response)
}

// GetStatistics returns overall provenance statistics
// @Summary Get provenance statistics
// @Tags Provenance
// @Produce json
// @Param startTime query string false "Start time (RFC3339)"
// @Param endTime query string false "End time (RFC3339)"
// @Success 200 {object} map[string]interface{}
// @Router /provenance/stats [get]
func (h *ProvenanceHandler) GetStatistics(c *gin.Context) {
	query := core.ProvenanceQuery{
		Limit: 10000, // Get a large sample
	}

	// Parse time range if provided
	if startTimeStr := c.Query("startTime"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
			return
		}
		query.StartTime = &startTime
	}

	if endTimeStr := c.Query("endTime"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
			return
		}
		query.EndTime = &endTime
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate statistics
	stats := gin.H{
		"totalEvents": len(events),
		"eventTypes":  make(map[string]int),
		"processors":  make(map[string]int),
		"flowFiles":   make(map[string]bool),
	}

	eventTypes := stats["eventTypes"].(map[string]int)
	processors := stats["processors"].(map[string]int)
	flowFiles := stats["flowFiles"].(map[string]bool)

	var totalDuration time.Duration
	for _, event := range events {
		eventTypes[event.EventType]++
		processors[event.ProcessorID.String()]++
		flowFiles[event.FlowFileID.String()] = true
		totalDuration += event.Duration
	}

	stats["uniqueFlowFiles"] = len(flowFiles)
	stats["uniqueProcessors"] = len(processors)

	if len(events) > 0 {
		stats["averageDuration"] = totalDuration.Milliseconds() / int64(len(events))
	} else {
		stats["averageDuration"] = 0
	}

	delete(stats, "flowFiles") // Remove the map, keep only the count

	c.JSON(http.StatusOK, stats)
}

// GetProcessorStatistics returns per-processor statistics
// @Summary Get processor statistics
// @Tags Provenance
// @Produce json
// @Param startTime query string false "Start time (RFC3339)"
// @Param endTime query string false "End time (RFC3339)"
// @Success 200 {object} map[string]interface{}
// @Router /provenance/stats/processors [get]
func (h *ProvenanceHandler) GetProcessorStatistics(c *gin.Context) {
	query := core.ProvenanceQuery{
		Limit: 10000,
	}

	// Parse time range if provided
	if startTimeStr := c.Query("startTime"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
			return
		}
		query.StartTime = &startTime
	}

	if endTimeStr := c.Query("endTime"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
			return
		}
		query.EndTime = &endTime
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate per-processor statistics
	processorStats := make(map[string]map[string]interface{})

	for _, event := range events {
		procID := event.ProcessorID.String()
		if _, exists := processorStats[procID]; !exists {
			processorStats[procID] = map[string]interface{}{
				"processorId":   event.ProcessorID,
				"processorName": event.ProcessorName,
				"eventCount":    0,
				"eventTypes":    make(map[string]int),
				"totalDuration": int64(0),
			}
		}

		stats := processorStats[procID]
		stats["eventCount"] = stats["eventCount"].(int) + 1
		stats["eventTypes"].(map[string]int)[event.EventType]++
		stats["totalDuration"] = stats["totalDuration"].(int64) + event.Duration.Milliseconds()
	}

	// Calculate averages
	for _, stats := range processorStats {
		eventCount := stats["eventCount"].(int)
		if eventCount > 0 {
			stats["averageDuration"] = stats["totalDuration"].(int64) / int64(eventCount)
		} else {
			stats["averageDuration"] = int64(0)
		}
	}

	c.JSON(http.StatusOK, processorStats)
}

// GetEventTypeStatistics returns statistics grouped by event type
// @Summary Get event type statistics
// @Tags Provenance
// @Produce json
// @Param startTime query string false "Start time (RFC3339)"
// @Param endTime query string false "End time (RFC3339)"
// @Success 200 {object} map[string]interface{}
// @Router /provenance/stats/event-types [get]
func (h *ProvenanceHandler) GetEventTypeStatistics(c *gin.Context) {
	query := core.ProvenanceQuery{
		Limit: 10000,
	}

	// Parse time range if provided
	if startTimeStr := c.Query("startTime"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time format"})
			return
		}
		query.StartTime = &startTime
	}

	if endTimeStr := c.Query("endTime"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time format"})
			return
		}
		query.EndTime = &endTime
	}

	// Query events
	events, err := h.repository.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate per-event-type statistics
	eventTypeStats := make(map[string]map[string]interface{})

	for _, event := range events {
		eventType := event.EventType
		if _, exists := eventTypeStats[eventType]; !exists {
			eventTypeStats[eventType] = map[string]interface{}{
				"eventType":     eventType,
				"count":         0,
				"totalDuration": int64(0),
				"processors":    make(map[string]bool),
			}
		}

		stats := eventTypeStats[eventType]
		stats["count"] = stats["count"].(int) + 1
		stats["totalDuration"] = stats["totalDuration"].(int64) + event.Duration.Milliseconds()
		stats["processors"].(map[string]bool)[event.ProcessorID.String()] = true
	}

	// Calculate averages and clean up
	for _, stats := range eventTypeStats {
		count := stats["count"].(int)
		if count > 0 {
			stats["averageDuration"] = stats["totalDuration"].(int64) / int64(count)
		} else {
			stats["averageDuration"] = int64(0)
		}
		stats["uniqueProcessors"] = len(stats["processors"].(map[string]bool))
		delete(stats, "processors") // Remove the map, keep only the count
	}

	c.JSON(http.StatusOK, eventTypeStats)
}
