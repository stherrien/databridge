package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ProcessSessionImpl implements the ProcessSession interface
type ProcessSessionImpl struct {
	id               uuid.UUID
	flowFileRepo     FlowFileRepository
	contentRepo      ContentRepository
	provenanceRepo   ProvenanceRepository
	logger           types.Logger
	ctx              context.Context

	// Queue integration
	inputQueues      []*FlowFileQueue
	outputConnections []*Connection
	currentQueueIndex int

	// Transaction state
	mu               sync.RWMutex
	flowFiles        map[uuid.UUID]*types.FlowFile
	transfers        map[uuid.UUID]types.Relationship
	removals         []uuid.UUID
	creations        []*types.FlowFile
	modifications    map[uuid.UUID]*types.FlowFile
	committed        bool
	rolledBack       bool
}

// NewProcessSession creates a new process session
func NewProcessSession(
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
	logger types.Logger,
	ctx context.Context,
	inputQueues []*FlowFileQueue,
	outputConnections []*Connection,
) *ProcessSessionImpl {
	return &ProcessSessionImpl{
		id:                uuid.New(),
		flowFileRepo:      flowFileRepo,
		contentRepo:       contentRepo,
		provenanceRepo:    provenanceRepo,
		logger:            logger,
		ctx:               ctx,
		inputQueues:       inputQueues,
		outputConnections: outputConnections,
		flowFiles:         make(map[uuid.UUID]*types.FlowFile),
		transfers:         make(map[uuid.UUID]types.Relationship),
		modifications:     make(map[uuid.UUID]*types.FlowFile),
	}
}

// Get retrieves a FlowFile from the input queue
func (s *ProcessSessionImpl) Get() *types.FlowFile {
	// If no input queues, return nil (e.g., source processors like GenerateFlowFile)
	if len(s.inputQueues) == 0 {
		return nil
	}

	// Try to dequeue from input queues using round-robin
	startIndex := s.currentQueueIndex
	for {
		queue := s.inputQueues[s.currentQueueIndex]

		// Try to dequeue from current queue
		flowFile := queue.Dequeue()

		// Move to next queue for round-robin
		s.currentQueueIndex = (s.currentQueueIndex + 1) % len(s.inputQueues)

		if flowFile != nil {
			// Track FlowFile in session
			s.mu.Lock()
			s.flowFiles[flowFile.ID] = flowFile
			s.mu.Unlock()

			s.logger.Debug("Retrieved FlowFile from queue",
				"id", flowFile.ID,
				"queueConnection", queue.connection.Name)
			return flowFile
		}

		// If we've checked all queues and found nothing, return nil
		if s.currentQueueIndex == startIndex {
			return nil
		}
	}
}

// GetBatch retrieves multiple FlowFiles up to maxResults
func (s *ProcessSessionImpl) GetBatch(maxResults int) []*types.FlowFile {
	var batch []*types.FlowFile

	for i := 0; i < maxResults; i++ {
		flowFile := s.Get()
		if flowFile == nil {
			break
		}
		batch = append(batch, flowFile)
	}

	s.logger.Debug("Retrieved FlowFile batch", "count", len(batch))
	return batch
}

// Create creates a new FlowFile
func (s *ProcessSessionImpl) Create() *types.FlowFile {
	flowFile := types.NewFlowFile()

	s.mu.Lock()
	s.flowFiles[flowFile.ID] = flowFile
	s.creations = append(s.creations, flowFile)
	s.mu.Unlock()

	s.logger.Debug("Created new FlowFile", "id", flowFile.ID)
	return flowFile
}

// CreateChild creates a child FlowFile from a parent
func (s *ProcessSessionImpl) CreateChild(parent *types.FlowFile) *types.FlowFile {
	child := types.NewFlowFileBuilder().
		WithParent(parent).
		WithAttributes(parent.Attributes).
		Build()

	s.mu.Lock()
	s.flowFiles[child.ID] = child
	s.creations = append(s.creations, child)
	s.mu.Unlock()

	s.logger.Debug("Created child FlowFile", "childId", child.ID, "parentId", parent.ID)
	return child
}

// Clone creates a copy of a FlowFile
func (s *ProcessSessionImpl) Clone(original *types.FlowFile) *types.FlowFile {
	clone := original.Clone()

	s.mu.Lock()
	s.flowFiles[clone.ID] = clone
	s.creations = append(s.creations, clone)
	s.mu.Unlock()

	// Increment reference count on content claim
	if clone.ContentClaim != nil {
		s.contentRepo.IncrementRef(clone.ContentClaim)
	}

	s.logger.Debug("Cloned FlowFile", "cloneId", clone.ID, "originalId", original.ID)
	return clone
}

// Transfer routes a FlowFile to a relationship
func (s *ProcessSessionImpl) Transfer(flowFile *types.FlowFile, relationship types.Relationship) {
	s.mu.Lock()
	s.transfers[flowFile.ID] = relationship
	s.mu.Unlock()

	s.logger.Debug("Transferred FlowFile", "id", flowFile.ID, "relationship", relationship.Name)
}

// Remove removes a FlowFile from the session
func (s *ProcessSessionImpl) Remove(flowFile *types.FlowFile) {
	s.mu.Lock()
	s.removals = append(s.removals, flowFile.ID)
	delete(s.flowFiles, flowFile.ID)
	s.mu.Unlock()

	s.logger.Debug("Removed FlowFile", "id", flowFile.ID)
}

// PutAttribute adds or updates an attribute
func (s *ProcessSessionImpl) PutAttribute(flowFile *types.FlowFile, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Track modification
	if _, exists := s.modifications[flowFile.ID]; !exists {
		// Create a copy for modification tracking
		modified := *flowFile
		modified.Attributes = make(map[string]string)
		for k, v := range flowFile.Attributes {
			modified.Attributes[k] = v
		}
		s.modifications[flowFile.ID] = &modified
	}

	flowFile.UpdateAttribute(key, value)
	s.modifications[flowFile.ID].UpdateAttribute(key, value)

	s.logger.Debug("Updated FlowFile attribute", "id", flowFile.ID, "key", key, "value", value)
}

// PutAllAttributes updates multiple attributes
func (s *ProcessSessionImpl) PutAllAttributes(flowFile *types.FlowFile, attributes map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Track modification
	if _, exists := s.modifications[flowFile.ID]; !exists {
		modified := *flowFile
		modified.Attributes = make(map[string]string)
		for k, v := range flowFile.Attributes {
			modified.Attributes[k] = v
		}
		s.modifications[flowFile.ID] = &modified
	}

	for key, value := range attributes {
		flowFile.UpdateAttribute(key, value)
		s.modifications[flowFile.ID].UpdateAttribute(key, value)
	}

	s.logger.Debug("Updated FlowFile attributes", "id", flowFile.ID, "count", len(attributes))
}

// RemoveAttribute removes an attribute
func (s *ProcessSessionImpl) RemoveAttribute(flowFile *types.FlowFile, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Track modification
	if _, exists := s.modifications[flowFile.ID]; !exists {
		modified := *flowFile
		modified.Attributes = make(map[string]string)
		for k, v := range flowFile.Attributes {
			modified.Attributes[k] = v
		}
		s.modifications[flowFile.ID] = &modified
	}

	flowFile.RemoveAttribute(key)
	s.modifications[flowFile.ID].RemoveAttribute(key)

	s.logger.Debug("Removed FlowFile attribute", "id", flowFile.ID, "key", key)
}

// Write writes content to a FlowFile
func (s *ProcessSessionImpl) Write(flowFile *types.FlowFile, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store content and get claim
	claim, err := s.contentRepo.Store(content)
	if err != nil {
		return fmt.Errorf("failed to store content: %w", err)
	}

	// Decrement reference count on old content claim
	if flowFile.ContentClaim != nil {
		s.contentRepo.DecrementRef(flowFile.ContentClaim)
	}

	// Update FlowFile with new content claim
	flowFile.ContentClaim = claim
	flowFile.Size = claim.Length
	flowFile.UpdatedAt = time.Now()

	// Track modification
	if _, exists := s.modifications[flowFile.ID]; !exists {
		modified := *flowFile
		s.modifications[flowFile.ID] = &modified
	} else {
		s.modifications[flowFile.ID].ContentClaim = claim
		s.modifications[flowFile.ID].Size = claim.Length
		s.modifications[flowFile.ID].UpdatedAt = flowFile.UpdatedAt
	}

	s.logger.Debug("Wrote content to FlowFile", "id", flowFile.ID, "size", len(content))
	return nil
}

// Read reads content from a FlowFile
func (s *ProcessSessionImpl) Read(flowFile *types.FlowFile) ([]byte, error) {
	if flowFile.ContentClaim == nil {
		return nil, fmt.Errorf("FlowFile has no content claim")
	}

	content, err := s.contentRepo.Get(flowFile.ContentClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	s.logger.Debug("Read content from FlowFile", "id", flowFile.ID, "size", len(content))
	return content, nil
}

// Commit commits all changes in the session
func (s *ProcessSessionImpl) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.committed || s.rolledBack {
		return fmt.Errorf("session already finalized")
	}

	// Store all created and modified FlowFiles
	for _, flowFile := range s.creations {
		if err := s.flowFileRepo.Store(flowFile); err != nil {
			return fmt.Errorf("failed to store created FlowFile: %w", err)
		}
	}

	for id, modified := range s.modifications {
		if err := s.flowFileRepo.Store(modified); err != nil {
			return fmt.Errorf("failed to store modified FlowFile %s: %w", id, err)
		}
	}

	// Remove FlowFiles marked for removal
	for _, id := range s.removals {
		if err := s.flowFileRepo.Delete(id); err != nil {
			s.logger.Warn("Failed to delete FlowFile", "id", id, "error", err)
		}
	}

	// Enqueue transferred FlowFiles to output connections
	for id, relationship := range s.transfers {
		flowFile, exists := s.flowFiles[id]
		if !exists {
			s.logger.Warn("FlowFile not found in session for transfer", "id", id)
			continue
		}

		// Find matching output connection for this relationship
		enqueued := false
		for _, conn := range s.outputConnections {
			if conn.Relationship.Name == relationship.Name {
				if err := conn.Queue.Enqueue(flowFile); err != nil {
					s.logger.Warn("Failed to enqueue FlowFile to connection",
						"flowFileId", id,
						"connection", conn.Name,
						"relationship", relationship.Name,
						"error", err)
					// Continue trying other connections
				} else {
					enqueued = true
					s.logger.Debug("Enqueued FlowFile to connection",
						"flowFileId", id,
						"connection", conn.Name,
						"relationship", relationship.Name)
					break
				}
			}
		}

		if !enqueued {
			s.logger.Debug("No matching output connection for relationship",
				"flowFileId", id,
				"relationship", relationship.Name)
		}

		// Create provenance event
		event := &ProvenanceEvent{
			ID:            uuid.New(),
			EventType:     "ROUTE",
			FlowFileID:    id,
			ProcessorID:   uuid.New(), // TODO: Get actual processor ID
			ProcessorName: "Unknown",  // TODO: Get actual processor name
			EventTime:     time.Now(),
			Details:       fmt.Sprintf("Routed to %s", relationship.Name),
		}

		if err := s.provenanceRepo.Store(event); err != nil {
			s.logger.Warn("Failed to store provenance event", "error", err)
		}
	}

	s.committed = true
	s.logger.Debug("Session committed successfully", "sessionId", s.id)
	return nil
}

// Rollback rolls back all changes in the session
func (s *ProcessSessionImpl) Rollback() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.committed || s.rolledBack {
		return
	}

	// Decrement reference counts on any content claims that were incremented
	for _, flowFile := range s.creations {
		if flowFile.ContentClaim != nil {
			s.contentRepo.DecrementRef(flowFile.ContentClaim)
		}
	}

	// Clear all transaction state
	s.flowFiles = make(map[uuid.UUID]*types.FlowFile)
	s.transfers = make(map[uuid.UUID]types.Relationship)
	s.removals = nil
	s.creations = nil
	s.modifications = make(map[uuid.UUID]*types.FlowFile)
	s.rolledBack = true

	s.logger.Debug("Session rolled back", "sessionId", s.id)
}

// GetLogger returns a logger for the session
func (s *ProcessSessionImpl) GetLogger() types.Logger {
	return s.logger
}

// IsCommitted returns whether the session has been committed
func (s *ProcessSessionImpl) IsCommitted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.committed
}

// IsRolledBack returns whether the session has been rolled back
func (s *ProcessSessionImpl) IsRolledBack() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rolledBack
}