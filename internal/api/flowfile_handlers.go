package api

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/shawntherrien/databridge/internal/core"
)

// FlowFileHandlers handles FlowFile inspection and queue operations
type FlowFileHandlers struct {
	flowController *core.FlowController
}

// NewFlowFileHandlers creates a new FlowFile handlers instance
func NewFlowFileHandlers(flowController *core.FlowController) *FlowFileHandlers {
	return &FlowFileHandlers{
		flowController: flowController,
	}
}

// FlowFileDetails represents detailed FlowFile information for inspection
type FlowFileDetails struct {
	ID             uuid.UUID                 `json:"id"`
	Size           int64                     `json:"size"`
	Attributes     map[string]string         `json:"attributes"`
	ContentPreview string                    `json:"contentPreview"`
	Provenance     []FlowFileProvenanceEvent `json:"provenance"`
}

// FlowFileProvenanceEvent represents a simple provenance event for the FlowFile
type FlowFileProvenanceEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Details   string `json:"details"`
}

// QueueInfo represents information about a connection queue
type QueueInfo struct {
	ConnectionID  uuid.UUID         `json:"connectionId"`
	SourceID      uuid.UUID         `json:"sourceId"`
	DestinationID uuid.UUID         `json:"destinationId"`
	Relationship  string            `json:"relationship"`
	FlowFileCount int               `json:"flowFileCount"`
	TotalSize     int64             `json:"totalSize"`
	FlowFiles     []FlowFileSummary `json:"flowFiles"`
}

// FlowFileSummary represents a summary of a FlowFile in a queue
type FlowFileSummary struct {
	ID         uuid.UUID         `json:"id"`
	Size       int64             `json:"size"`
	Attributes map[string]string `json:"attributes"`
	QueuedTime string            `json:"queuedTime"`
}

// HandleGetFlowFile handles GET /api/flowfiles/{id}
func (h *FlowFileHandlers) HandleGetFlowFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid FlowFile ID")
		return
	}

	// Get FlowFile from repository
	flowFile, err := h.flowController.GetFlowFileRepository().Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "FlowFile not found")
		return
	}

	// Read content preview (first 1KB)
	var contentPreview string
	if flowFile.ContentClaim != nil {
		reader, err := h.flowController.GetContentRepository().Read(flowFile.ContentClaim)
		if err == nil {
			defer func() {
				if err := reader.Close(); err != nil {
					// Log close error but don't override response
				}
			}()
			previewBytes := make([]byte, 1024)
			n, _ := reader.Read(previewBytes)
			contentPreview = string(previewBytes[:n])
		}
	}

	// Get provenance events (simplified for now)
	provenance := []FlowFileProvenanceEvent{}

	details := FlowFileDetails{
		ID:             flowFile.ID,
		Size:           flowFile.Size,
		Attributes:     flowFile.Attributes,
		ContentPreview: contentPreview,
		Provenance:     provenance,
	}

	respondJSON(w, http.StatusOK, details)
}

// HandleGetConnectionQueue handles GET /api/connections/{id}/queue
func (h *FlowFileHandlers) HandleGetConnectionQueue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connection ID")
		return
	}

	// Get connection
	connection, exists := h.flowController.GetConnection(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	// Get queue info
	queue := connection.Queue
	if queue == nil {
		queueInfo := QueueInfo{
			ConnectionID:  connection.ID,
			SourceID:      connection.Source.ID,
			DestinationID: connection.Destination.ID,
			Relationship:  connection.Relationship.Name,
			FlowFileCount: 0,
			TotalSize:     0,
			FlowFiles:     []FlowFileSummary{},
		}
		respondJSON(w, http.StatusOK, queueInfo)
		return
	}

	// Get FlowFiles in queue
	flowFiles := queue.List()
	summaries := make([]FlowFileSummary, 0, len(flowFiles))
	var totalSize int64

	for _, ff := range flowFiles {
		summaries = append(summaries, FlowFileSummary{
			ID:         ff.ID,
			Size:       ff.Size,
			Attributes: ff.Attributes,
			QueuedTime: ff.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
		totalSize += ff.Size
	}

	queueInfo := QueueInfo{
		ConnectionID:  connection.ID,
		SourceID:      connection.Source.ID,
		DestinationID: connection.Destination.ID,
		Relationship:  connection.Relationship.Name,
		FlowFileCount: len(flowFiles),
		TotalSize:     totalSize,
		FlowFiles:     summaries,
	}

	respondJSON(w, http.StatusOK, queueInfo)
}

// HandleClearConnectionQueue handles DELETE /api/connections/{id}/queue
func (h *FlowFileHandlers) HandleClearConnectionQueue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid connection ID")
		return
	}

	// Get connection
	connection, exists := h.flowController.GetConnection(id)
	if !exists {
		respondError(w, http.StatusNotFound, "Connection not found")
		return
	}

	// Clear queue
	if connection.Queue != nil {
		connection.Queue.Clear()
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Queue cleared successfully",
	})
}

// HandleGetFlowFileContent handles GET /api/flowfiles/{id}/content
func (h *FlowFileHandlers) HandleGetFlowFileContent(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid FlowFile ID")
		return
	}

	// Get FlowFile from repository
	flowFile, err := h.flowController.GetFlowFileRepository().Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "FlowFile not found")
		return
	}

	// Read full content
	if flowFile.ContentClaim == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	reader, err := h.flowController.GetContentRepository().Read(flowFile.ContentClaim)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read content")
		return
	}
	defer func() {
		if err := reader.Close(); err != nil {
			// Log close error but don't override response
		}
	}()

	// Set content type if available in attributes
	if mimeType, ok := flowFile.Attributes["mime.type"]; ok {
		w.Header().Set("Content-Type", mimeType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	w.WriteHeader(http.StatusOK)
	// Copy content from reader to response writer
	contentBytes := make([]byte, flowFile.Size)
	_, err = reader.Read(contentBytes)
	if err != nil && err.Error() != "EOF" {
		respondError(w, http.StatusInternalServerError, "Failed to read content")
		return
	}
	_, _ = w.Write(contentBytes) // Best effort write
}
