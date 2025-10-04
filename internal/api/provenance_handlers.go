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

// ProvenanceEvent represents a provenance event
type ProvenanceEvent struct {
	ID              string                 `json:"id"`
	EventType       string                 `json:"eventType"` // CREATE, RECEIVE, SEND, DROP, MODIFY, etc.
	FlowFileID      string                 `json:"flowFileId"`
	ProcessorID     string                 `json:"processorId"`
	ProcessorName   string                 `json:"processorName"`
	ProcessorType   string                 `json:"processorType"`
	Timestamp       time.Time              `json:"timestamp"`
	Duration        int64                  `json:"duration"` // milliseconds
	FileSize        int64                  `json:"fileSize"`
	Attributes      map[string]string      `json:"attributes,omitempty"`
	ParentIDs       []string               `json:"parentIds,omitempty"`
	ChildIDs        []string               `json:"childIds,omitempty"`
	Details         string                 `json:"details,omitempty"`
	ComponentID     string                 `json:"componentId,omitempty"`
	ComponentType   string                 `json:"componentType,omitempty"`
	TransitURI      string                 `json:"transitUri,omitempty"`
	Relationship    string                 `json:"relationship,omitempty"`
	AlternateID     string                 `json:"alternateId,omitempty"`
	ContentClaim    string                 `json:"contentClaim,omitempty"`
	PreviousFileSize int64                 `json:"previousFileSize,omitempty"`
	UpdatedAttributes map[string]string    `json:"updatedAttributes,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// ProvenanceLineage represents the lineage of a FlowFile
type ProvenanceLineage struct {
	FlowFileID string             `json:"flowFileId"`
	Events     []ProvenanceEvent  `json:"events"`
	Nodes      []LineageNode      `json:"nodes"`
	Links      []LineageLink      `json:"links"`
}

// LineageNode represents a node in the lineage graph
type LineageNode struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // processor, flowfile
	Label     string    `json:"label"`
	Timestamp time.Time `json:"timestamp"`
}

// LineageLink represents a link in the lineage graph
type LineageLink struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Relationship string `json:"relationship"`
}

// ProvenanceStatistics represents aggregated provenance statistics
type ProvenanceStatistics struct {
	TotalEvents       int64              `json:"totalEvents"`
	EventsByType      map[string]int64   `json:"eventsByType"`
	EventsByProcessor map[string]int64   `json:"eventsByProcessor"`
	TotalBytes        int64              `json:"totalBytes"`
	AvgDuration       float64            `json:"avgDuration"`
	TimeRange         TimeRange          `json:"timeRange"`
}

// TimeRange represents a time range
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ProvenanceHandlers handles provenance-related HTTP requests
type ProvenanceHandlers struct {
	log    *logrus.Logger
	events map[string]*ProvenanceEvent
	mu     sync.RWMutex
}

// NewProvenanceHandlers creates a new provenance handler
func NewProvenanceHandlers(log *logrus.Logger) *ProvenanceHandlers {
	h := &ProvenanceHandlers{
		log:    log,
		events: make(map[string]*ProvenanceEvent),
	}

	// Initialize with example events
	h.initializeExampleEvents()

	return h
}

// initializeExampleEvents adds example provenance events
func (h *ProvenanceHandlers) initializeExampleEvents() {
	now := time.Now()
	processorID := uuid.New().String()
	flowFileID := uuid.New().String()

	events := []*ProvenanceEvent{
		{
			ID:            uuid.New().String(),
			EventType:     "CREATE",
			FlowFileID:    flowFileID,
			ProcessorID:   processorID,
			ProcessorName: "GenerateFlowFile",
			ProcessorType: "GenerateFlowFile",
			Timestamp:     now.Add(-5 * time.Minute),
			Duration:      15,
			FileSize:      1024,
			Attributes:    map[string]string{"filename": "data.txt"},
			Details:       "FlowFile created with sample data",
		},
		{
			ID:            uuid.New().String(),
			EventType:     "ATTRIBUTES_MODIFIED",
			FlowFileID:    flowFileID,
			ProcessorID:   uuid.New().String(),
			ProcessorName: "UpdateAttribute",
			ProcessorType: "UpdateAttribute",
			Timestamp:     now.Add(-4 * time.Minute),
			Duration:      5,
			FileSize:      1024,
			UpdatedAttributes: map[string]string{"processed": "true"},
			Details:       "Attributes updated",
		},
		{
			ID:            uuid.New().String(),
			EventType:     "CONTENT_MODIFIED",
			FlowFileID:    flowFileID,
			ProcessorID:   uuid.New().String(),
			ProcessorName: "TransformJSON",
			ProcessorType: "TransformJSON",
			Timestamp:     now.Add(-3 * time.Minute),
			Duration:      250,
			FileSize:      2048,
			PreviousFileSize: 1024,
			Details:       "Content transformed",
		},
		{
			ID:            uuid.New().String(),
			EventType:     "SEND",
			FlowFileID:    flowFileID,
			ProcessorID:   uuid.New().String(),
			ProcessorName: "PutFile",
			ProcessorType: "PutFile",
			Timestamp:     now.Add(-2 * time.Minute),
			Duration:      100,
			FileSize:      2048,
			TransitURI:    "file:///data/output/data.txt",
			Relationship:  "success",
			Details:       "FlowFile sent to destination",
		},
	}

	for _, event := range events {
		h.events[event.ID] = event
	}
}

// HandleGetEvents handles GET /api/provenance/events
func (h *ProvenanceHandlers) HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Parse query parameters
	_ = 100 // limit - reserved for future pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		// Parse limit (simplified)
	}

	events := make([]*ProvenanceEvent, 0, len(h.events))
	for _, event := range h.events {
		events = append(events, event)
	}

	// Sort by timestamp descending (most recent first)
	// Simplified - in production use proper sorting

	result := map[string]interface{}{
		"events": events,
		"total":  len(events),
	}

	respondJSON(w, http.StatusOK, result)
}

// HandleGetEvent handles GET /api/provenance/events/{id}
func (h *ProvenanceHandlers) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	h.mu.RLock()
	event, exists := h.events[id]
	h.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, "Event not found")
		return
	}

	respondJSON(w, http.StatusOK, event)
}

// HandleSearchEvents handles POST /api/provenance/events/search
func (h *ProvenanceHandlers) HandleSearchEvents(w http.ResponseWriter, r *http.Request) {
	var searchReq struct {
		FlowFileID    string    `json:"flowFileId,omitempty"`
		ProcessorID   string    `json:"processorId,omitempty"`
		EventType     string    `json:"eventType,omitempty"`
		StartTime     time.Time `json:"startTime,omitempty"`
		EndTime       time.Time `json:"endTime,omitempty"`
		Limit         int       `json:"limit,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	var results []*ProvenanceEvent
	for _, event := range h.events {
		// Filter by criteria
		if searchReq.FlowFileID != "" && event.FlowFileID != searchReq.FlowFileID {
			continue
		}
		if searchReq.ProcessorID != "" && event.ProcessorID != searchReq.ProcessorID {
			continue
		}
		if searchReq.EventType != "" && event.EventType != searchReq.EventType {
			continue
		}
		if !searchReq.StartTime.IsZero() && event.Timestamp.Before(searchReq.StartTime) {
			continue
		}
		if !searchReq.EndTime.IsZero() && event.Timestamp.After(searchReq.EndTime) {
			continue
		}

		results = append(results, event)
	}

	result := map[string]interface{}{
		"events": results,
		"total":  len(results),
	}

	respondJSON(w, http.StatusOK, result)
}

// HandleGetLineage handles GET /api/provenance/lineage/{flowFileId}
func (h *ProvenanceHandlers) HandleGetLineage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	flowFileID := vars["flowFileId"]

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Find all events for this FlowFile
	var events []ProvenanceEvent
	for _, event := range h.events {
		if event.FlowFileID == flowFileID {
			events = append(events, *event)
		}
	}

	// Build lineage graph
	nodes := make([]LineageNode, 0)
	links := make([]LineageLink, 0)

	for _, event := range events {
		// Add processor node
		nodes = append(nodes, LineageNode{
			ID:        event.ProcessorID,
			Type:      "processor",
			Label:     event.ProcessorName,
			Timestamp: event.Timestamp,
		})

		// Add links based on event type
		if len(nodes) > 1 {
			links = append(links, LineageLink{
				Source:       nodes[len(nodes)-2].ID,
				Target:       event.ProcessorID,
				Relationship: "processed",
			})
		}
	}

	lineage := ProvenanceLineage{
		FlowFileID: flowFileID,
		Events:     events,
		Nodes:      nodes,
		Links:      links,
	}

	respondJSON(w, http.StatusOK, lineage)
}

// HandleGetTimeline handles GET /api/provenance/timeline
func (h *ProvenanceHandlers) HandleGetTimeline(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Group events by time buckets
	timeline := make(map[string]int)
	for _, event := range h.events {
		bucket := event.Timestamp.Format("2006-01-02 15:04")
		timeline[bucket]++
	}

	respondJSON(w, http.StatusOK, timeline)
}

// HandleGetStatistics handles GET /api/provenance/stats
func (h *ProvenanceHandlers) HandleGetStatistics(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := ProvenanceStatistics{
		TotalEvents:       int64(len(h.events)),
		EventsByType:      make(map[string]int64),
		EventsByProcessor: make(map[string]int64),
		TotalBytes:        0,
		AvgDuration:       0,
	}

	var totalDuration int64
	var minTime, maxTime time.Time

	for _, event := range h.events {
		stats.EventsByType[event.EventType]++
		stats.EventsByProcessor[event.ProcessorName]++
		stats.TotalBytes += event.FileSize
		totalDuration += event.Duration

		if minTime.IsZero() || event.Timestamp.Before(minTime) {
			minTime = event.Timestamp
		}
		if maxTime.IsZero() || event.Timestamp.After(maxTime) {
			maxTime = event.Timestamp
		}
	}

	if len(h.events) > 0 {
		stats.AvgDuration = float64(totalDuration) / float64(len(h.events))
	}

	stats.TimeRange = TimeRange{Start: minTime, End: maxTime}

	respondJSON(w, http.StatusOK, stats)
}

// HandleGetProcessorStatistics handles GET /api/provenance/stats/processors
func (h *ProvenanceHandlers) HandleGetProcessorStatistics(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	processorStats := make(map[string]map[string]interface{})

	for _, event := range h.events {
		if _, exists := processorStats[event.ProcessorID]; !exists {
			processorStats[event.ProcessorID] = map[string]interface{}{
				"id":           event.ProcessorID,
				"name":         event.ProcessorName,
				"type":         event.ProcessorType,
				"eventCount":   0,
				"totalBytes":   int64(0),
				"avgDuration":  0.0,
				"totalDuration": int64(0),
			}
		}

		stats := processorStats[event.ProcessorID]
		stats["eventCount"] = stats["eventCount"].(int) + 1
		stats["totalBytes"] = stats["totalBytes"].(int64) + event.FileSize
		stats["totalDuration"] = stats["totalDuration"].(int64) + event.Duration

		count := stats["eventCount"].(int)
		totalDur := stats["totalDuration"].(int64)
		stats["avgDuration"] = float64(totalDur) / float64(count)
	}

	result := make([]map[string]interface{}, 0, len(processorStats))
	for _, stats := range processorStats {
		result = append(result, stats)
	}

	respondJSON(w, http.StatusOK, result)
}

// HandleGetEventTypeStatistics handles GET /api/provenance/stats/event-types
func (h *ProvenanceHandlers) HandleGetEventTypeStatistics(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	eventTypeStats := make(map[string]map[string]interface{})

	for _, event := range h.events {
		if _, exists := eventTypeStats[event.EventType]; !exists {
			eventTypeStats[event.EventType] = map[string]interface{}{
				"eventType":   event.EventType,
				"count":       0,
				"totalBytes":  int64(0),
				"avgDuration": 0.0,
				"totalDuration": int64(0),
			}
		}

		stats := eventTypeStats[event.EventType]
		stats["count"] = stats["count"].(int) + 1
		stats["totalBytes"] = stats["totalBytes"].(int64) + event.FileSize
		stats["totalDuration"] = stats["totalDuration"].(int64) + event.Duration

		count := stats["count"].(int)
		totalDur := stats["totalDuration"].(int64)
		stats["avgDuration"] = float64(totalDur) / float64(count)
	}

	result := make([]map[string]interface{}, 0, len(eventTypeStats))
	for _, stats := range eventTypeStats {
		result = append(result, stats)
	}

	respondJSON(w, http.StatusOK, result)
}
