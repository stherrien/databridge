package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// SSEHandler manages Server-Sent Events for real-time updates
type SSEHandler struct {
	collector      *MetricsCollector
	logger         *logrus.Logger
	clients        map[chan MonitoringEvent]bool
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	updateInterval time.Duration
}

// NewSSEHandler creates a new SSEHandler
func NewSSEHandler(collector *MetricsCollector, logger *logrus.Logger, updateInterval time.Duration) *SSEHandler {
	ctx, cancel := context.WithCancel(context.Background())

	handler := &SSEHandler{
		collector:      collector,
		logger:         logger,
		clients:        make(map[chan MonitoringEvent]bool),
		ctx:            ctx,
		cancel:         cancel,
		updateInterval: updateInterval,
	}

	// Start broadcasting updates
	go handler.broadcast()

	return handler
}

// HandleStream handles GET /api/monitoring/stream
func (h *SSEHandler) HandleStream(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a new channel for this client
	clientChan := make(chan MonitoringEvent, 10)

	// Register client
	h.mu.Lock()
	h.clients[clientChan] = true
	h.mu.Unlock()

	// Remove client when connection closes
	defer func() {
		h.mu.Lock()
		if _, exists := h.clients[clientChan]; exists {
			delete(h.clients, clientChan)
			close(clientChan)
		}
		h.mu.Unlock()
	}()

	// Send initial data
	h.sendInitialData(w)

	// Create a flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("Streaming not supported")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send updates to client
	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case event, ok := <-clientChan:
			if !ok {
				// Channel closed
				return
			}

			// Send event
			if err := h.sendEvent(w, event); err != nil {
				h.logger.WithError(err).Error("Error sending SSE event")
				return
			}
			flusher.Flush()
		}
	}
}

// Stop stops the SSE handler
func (h *SSEHandler) Stop() {
	h.cancel()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Close all client channels
	for client := range h.clients {
		delete(h.clients, client)
		close(client)
	}
}

// Private methods

func (h *SSEHandler) broadcast() {
	ticker := time.NewTicker(h.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.sendUpdates()
		}
	}
}

func (h *SSEHandler) sendUpdates() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.clients) == 0 {
		return
	}

	// Collect current metrics
	systemStatus := h.collector.GetSystemStatus()
	processorMetrics := h.collector.GetProcessorMetrics()
	queueMetrics := h.collector.GetQueueMetrics()

	// Send system status update
	statusEvent := MonitoringEvent{
		EventType: "system_status",
		Data:      systemStatus,
		Timestamp: time.Now(),
	}
	h.broadcastEvent(statusEvent)

	// Send throughput update
	metrics := h.collector.GetSystemMetrics()
	throughputEvent := MonitoringEvent{
		EventType: "throughput",
		Data: ThroughputEvent{
			TotalFlowFiles:     metrics.Throughput.FlowFilesProcessed,
			FlowFilesPerSecond: metrics.Throughput.FlowFilesPerSecond,
			BytesPerSecond:     metrics.Throughput.BytesPerSecond,
		},
		Timestamp: time.Now(),
	}
	h.broadcastEvent(throughputEvent)

	// Send processor updates
	for _, proc := range processorMetrics {
		procEvent := MonitoringEvent{
			EventType: "processor_metrics",
			Data:      proc,
			Timestamp: time.Now(),
		}
		h.broadcastEvent(procEvent)
	}

	// Send queue depth update
	queueEvent := MonitoringEvent{
		EventType: "queue_metrics",
		Data:      queueMetrics,
		Timestamp: time.Now(),
	}
	h.broadcastEvent(queueEvent)
}

func (h *SSEHandler) broadcastEvent(event MonitoringEvent) {
	for client := range h.clients {
		select {
		case client <- event:
			// Event sent
		default:
			// Client buffer full, skip
		}
	}
}

func (h *SSEHandler) sendInitialData(w http.ResponseWriter) {
	// Send initial status
	systemStatus := h.collector.GetSystemStatus()
	event := MonitoringEvent{
		EventType: "system_status",
		Data:      systemStatus,
		Timestamp: time.Now(),
	}

	if err := h.sendEvent(w, event); err != nil {
		h.logger.WithError(err).Error("Error sending initial data")
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (h *SSEHandler) sendEvent(w http.ResponseWriter, event MonitoringEvent) error {
	// Serialize event data
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling event: %w", err)
	}

	// Format as SSE
	// Format: event: <eventType>\ndata: <jsonData>\n\n
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.EventType, string(data))
	return err
}
