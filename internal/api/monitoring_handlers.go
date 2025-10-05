package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// MonitoringHandlers handles monitoring-related HTTP requests
type MonitoringHandlers struct {
	collector *MetricsCollector
}

// NewMonitoringHandlers creates a new MonitoringHandlers
func NewMonitoringHandlers(collector *MetricsCollector) *MonitoringHandlers {
	return &MonitoringHandlers{
		collector: collector,
	}
}

// HandleSystemStatus handles GET /api/system/status
func (h *MonitoringHandlers) HandleSystemStatus(w http.ResponseWriter, r *http.Request) {
	status := h.collector.GetSystemStatus()
	respondJSON(w, http.StatusOK, status)
}

// HandleHealth handles GET /api/system/health
func (h *MonitoringHandlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := h.collector.GetHealthStatus()

	// Set appropriate status code based on health
	statusCode := http.StatusOK
	switch health.Status {
	case "unhealthy":
		statusCode = http.StatusServiceUnavailable
	case "degraded":
		statusCode = http.StatusOK // Still return 200 for degraded
	}

	respondJSON(w, statusCode, health)
}

// HandleSystemMetrics handles GET /api/system/metrics
func (h *MonitoringHandlers) HandleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := h.collector.GetSystemMetrics()
	respondJSON(w, http.StatusOK, metrics)
}

// HandleSystemStats handles GET /api/system/stats
func (h *MonitoringHandlers) HandleSystemStats(w http.ResponseWriter, r *http.Request) {
	stats := h.collector.GetStatsSummary()
	respondJSON(w, http.StatusOK, stats)
}

// HandleProcessors handles GET /api/monitoring/processors
func (h *MonitoringHandlers) HandleProcessors(w http.ResponseWriter, r *http.Request) {
	metrics := h.collector.GetProcessorMetrics()
	respondJSON(w, http.StatusOK, metrics)
}

// HandleProcessor handles GET /api/monitoring/processors/{id}
func (h *MonitoringHandlers) HandleProcessor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid processor ID")
		return
	}

	metrics, exists := h.collector.GetProcessorMetricsByID(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Processor not found")
		return
	}

	respondJSON(w, http.StatusOK, metrics)
}

// HandleConnections handles GET /api/monitoring/connections
func (h *MonitoringHandlers) HandleConnections(w http.ResponseWriter, r *http.Request) {
	metrics := h.collector.GetConnectionMetrics()
	respondJSON(w, http.StatusOK, metrics)
}

// HandleConnection handles GET /api/monitoring/connections/{id}
func (h *MonitoringHandlers) HandleConnection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connection ID")
		return
	}

	metrics, exists := h.collector.GetConnectionMetricsByID(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	respondJSON(w, http.StatusOK, metrics)
}

// HandleQueues handles GET /api/monitoring/queues
func (h *MonitoringHandlers) HandleQueues(w http.ResponseWriter, r *http.Request) {
	metrics := h.collector.GetQueueMetrics()
	respondJSON(w, http.StatusOK, metrics)
}
