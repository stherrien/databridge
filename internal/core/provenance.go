package core

import (
	"sync"

	"github.com/google/uuid"
)

// InMemoryProvenanceRepository is a simple in-memory implementation for testing
type InMemoryProvenanceRepository struct {
	events map[uuid.UUID]*ProvenanceEvent
	mu     sync.RWMutex
}

// NewInMemoryProvenanceRepository creates a new in-memory provenance repository
func NewInMemoryProvenanceRepository() *InMemoryProvenanceRepository {
	return &InMemoryProvenanceRepository{
		events: make(map[uuid.UUID]*ProvenanceEvent),
	}
}

// Store stores a provenance event
func (r *InMemoryProvenanceRepository) Store(event *ProvenanceEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[event.ID] = event
	return nil
}

// Query queries provenance events
func (r *InMemoryProvenanceRepository) Query(query ProvenanceQuery) ([]*ProvenanceEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*ProvenanceEvent
	for _, event := range r.events {
		// Simple filtering logic
		if query.FlowFileID != nil && event.FlowFileID != *query.FlowFileID {
			continue
		}
		if query.EventType != "" && event.EventType != query.EventType {
			continue
		}
		if query.ProcessorID != nil && event.ProcessorID != *query.ProcessorID {
			continue
		}
		if query.StartTime != nil && event.EventTime.Before(*query.StartTime) {
			continue
		}
		if query.EndTime != nil && event.EventTime.After(*query.EndTime) {
			continue
		}
		results = append(results, event)
	}

	// Apply pagination
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return results, nil
}

// GetLineage gets lineage for a FlowFile
func (r *InMemoryProvenanceRepository) GetLineage(flowFileId uuid.UUID) (*LineageGraph, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Simple implementation - collect all events for the FlowFile
	var nodes []LineageNode
	var links []LineageLink

	for _, event := range r.events {
		if event.FlowFileID == flowFileId {
			node := LineageNode{
				ID:          event.ID,
				FlowFileID:  event.FlowFileID,
				ProcessorID: event.ProcessorID,
				EventType:   event.EventType,
				Timestamp:   event.EventTime,
			}
			nodes = append(nodes, node)

			// Create links from parent events
			for _, parentID := range event.ParentIDs {
				link := LineageLink{
					Source: parentID,
					Target: event.ID,
				}
				links = append(links, link)
			}
		}
	}

	// Ensure we return initialized slices even if empty
	if nodes == nil {
		nodes = make([]LineageNode, 0)
	}
	if links == nil {
		links = make([]LineageLink, 0)
	}

	return &LineageGraph{
		Nodes: nodes,
		Links: links,
	}, nil
}

// GetEvents retrieves provenance events with pagination
func (r *InMemoryProvenanceRepository) GetEvents(offset, limit int) ([]*ProvenanceEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var events []*ProvenanceEvent
	for _, event := range r.events {
		events = append(events, event)
	}

	// Apply pagination
	if offset > 0 && offset < len(events) {
		events = events[offset:]
	} else if offset >= len(events) {
		return []*ProvenanceEvent{}, nil
	}

	if limit > 0 && limit < len(events) {
		events = events[:limit]
	}

	return events, nil
}

// AddEvent adds a provenance event (alias for Store)
func (r *InMemoryProvenanceRepository) AddEvent(event *ProvenanceEvent) error {
	return r.Store(event)
}

// Close closes the repository
func (r *InMemoryProvenanceRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = make(map[uuid.UUID]*ProvenanceEvent)
	return nil
}
