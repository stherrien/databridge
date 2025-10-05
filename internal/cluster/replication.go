package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// StateReplicator handles state replication across cluster nodes
type StateReplicator struct {
	config          ReplicationConfig
	pendingRequests map[string]*ReplicationRequest
	responses       map[string][]*ReplicationResponse
	logger          *logrus.Logger
	mu              sync.RWMutex
}

// ReplicationConfig holds replication configuration
type ReplicationConfig struct {
	Strategy   ReplicationStrategy
	AckTimeout time.Duration
	RetryCount int
}

// NewStateReplicator creates a new state replicator
func NewStateReplicator(config ReplicationConfig, logger *logrus.Logger) *StateReplicator {
	// Set defaults
	if config.AckTimeout == 0 {
		config.AckTimeout = 5 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}

	return &StateReplicator{
		config:          config,
		pendingRequests: make(map[string]*ReplicationRequest),
		responses:       make(map[string][]*ReplicationResponse),
		logger:          logger,
	}
}

// ReplicateState replicates state to cluster nodes
func (sr *StateReplicator) ReplicateState(ctx context.Context, stateType string, data interface{}, nodes []*ClusterNode) error {
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available for replication")
	}

	// Create replication request
	req := &ReplicationRequest{
		RequestID:   uuid.New().String(),
		StateType:   stateType,
		Data:        data,
		Version:     uint64(time.Now().UnixNano()),
		Timestamp:   time.Now(),
		AckRequired: sr.config.Strategy != ReplicationAsync,
	}

	sr.mu.Lock()
	sr.pendingRequests[req.RequestID] = req
	sr.responses[req.RequestID] = make([]*ReplicationResponse, 0)
	sr.mu.Unlock()

	sr.logger.WithFields(logrus.Fields{
		"requestId": req.RequestID,
		"stateType": stateType,
		"nodes":     len(nodes),
		"strategy":  sr.config.Strategy,
	}).Info("Starting state replication")

	// Replicate based on strategy
	switch sr.config.Strategy {
	case ReplicationSync:
		return sr.replicateSync(ctx, req, nodes)
	case ReplicationAsync:
		return sr.replicateAsync(ctx, req, nodes)
	case ReplicationQuorum:
		return sr.replicateQuorum(ctx, req, nodes)
	default:
		return sr.replicateQuorum(ctx, req, nodes)
	}
}

// replicateSync waits for all nodes to acknowledge
func (sr *StateReplicator) replicateSync(ctx context.Context, req *ReplicationRequest, nodes []*ClusterNode) error {
	sr.logger.WithField("requestId", req.RequestID).Debug("Sync replication started")

	// Send to all nodes
	errChan := make(chan error, len(nodes))
	for _, node := range nodes {
		go func(n *ClusterNode) {
			errChan <- sr.sendReplication(ctx, req, n)
		}(node)
	}

	// Wait for all responses
	timeout := time.NewTimer(sr.config.AckTimeout)
	defer timeout.Stop()

	successCount := 0
	var lastErr error

	for i := 0; i < len(nodes); i++ {
		select {
		case err := <-errChan:
			if err == nil {
				successCount++
			} else {
				lastErr = err
				sr.logger.WithError(err).Warn("Replication failed for node")
			}
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for sync replication")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if successCount < len(nodes) {
		return fmt.Errorf("sync replication failed: %d/%d nodes acknowledged, last error: %v",
			successCount, len(nodes), lastErr)
	}

	sr.logger.WithField("requestId", req.RequestID).Info("Sync replication completed successfully")
	return nil
}

// replicateAsync sends without waiting for acknowledgments
func (sr *StateReplicator) replicateAsync(ctx context.Context, req *ReplicationRequest, nodes []*ClusterNode) error {
	sr.logger.WithField("requestId", req.RequestID).Debug("Async replication started")

	// Send to all nodes without waiting
	for _, node := range nodes {
		go func(n *ClusterNode) {
			if err := sr.sendReplication(ctx, req, n); err != nil {
				sr.logger.WithError(err).WithField("nodeId", n.ID).Warn("Async replication failed")
			}
		}(node)
	}

	sr.logger.WithField("requestId", req.RequestID).Info("Async replication initiated")
	return nil
}

// replicateQuorum waits for majority to acknowledge
func (sr *StateReplicator) replicateQuorum(ctx context.Context, req *ReplicationRequest, nodes []*ClusterNode) error {
	sr.logger.WithField("requestId", req.RequestID).Debug("Quorum replication started")

	requiredAcks := (len(nodes) / 2) + 1

	// Send to all nodes
	errChan := make(chan error, len(nodes))
	for _, node := range nodes {
		go func(n *ClusterNode) {
			errChan <- sr.sendReplication(ctx, req, n)
		}(node)
	}

	// Wait for quorum
	timeout := time.NewTimer(sr.config.AckTimeout)
	defer timeout.Stop()

	successCount := 0
	failCount := 0

	for i := 0; i < len(nodes); i++ {
		select {
		case err := <-errChan:
			if err == nil {
				successCount++
				if successCount >= requiredAcks {
					sr.logger.WithFields(logrus.Fields{
						"requestId": req.RequestID,
						"acks":      successCount,
						"required":  requiredAcks,
					}).Info("Quorum replication completed successfully")
					return nil
				}
			} else {
				failCount++
				sr.logger.WithError(err).Warn("Replication failed for node")
				// Check if we can still achieve quorum
				remaining := len(nodes) - (successCount + failCount)
				if successCount+remaining < requiredAcks {
					return fmt.Errorf("quorum replication failed: cannot achieve required %d acks", requiredAcks)
				}
			}
		case <-timeout.C:
			return fmt.Errorf("timeout waiting for quorum: got %d/%d acks", successCount, requiredAcks)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if successCount >= requiredAcks {
		sr.logger.WithField("requestId", req.RequestID).Info("Quorum replication completed")
		return nil
	}

	return fmt.Errorf("quorum replication failed: %d/%d acks received", successCount, requiredAcks)
}

// sendReplication sends replication request to a single node
func (sr *StateReplicator) sendReplication(ctx context.Context, req *ReplicationRequest, node *ClusterNode) error {
	// In production, would send actual RPC/HTTP request to node
	// For now, simulate with delay and success

	// Simulate network delay
	time.Sleep(10 * time.Millisecond)

	// Check if node is healthy
	if !node.IsHealthy() {
		return fmt.Errorf("node %s is not healthy", node.ID)
	}

	// Create response
	response := &ReplicationResponse{
		RequestID: req.RequestID,
		NodeID:    node.ID,
		Success:   true,
		Timestamp: time.Now(),
	}

	// Record response
	sr.mu.Lock()
	if responses, exists := sr.responses[req.RequestID]; exists {
		sr.responses[req.RequestID] = append(responses, response)
	}
	sr.mu.Unlock()

	sr.logger.WithFields(logrus.Fields{
		"requestId": req.RequestID,
		"nodeId":    node.ID,
	}).Debug("Replication sent successfully")

	return nil
}

// GetReplicationStatus returns the status of a replication request
func (sr *StateReplicator) GetReplicationStatus(requestID string) (*ReplicationStatus, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	req, exists := sr.pendingRequests[requestID]
	if !exists {
		return nil, fmt.Errorf("replication request not found: %s", requestID)
	}

	responses := sr.responses[requestID]

	status := &ReplicationStatus{
		RequestID:         requestID,
		StateType:         req.StateType,
		TotalNodes:        0, // Would track from original request
		AcknowledgedNodes: len(responses),
		Pending:           req.AckRequired && len(responses) == 0,
		Completed:         !req.AckRequired || len(responses) > 0,
		Timestamp:         req.Timestamp,
	}

	return status, nil
}

// CleanupOldRequests removes old replication requests
func (sr *StateReplicator) CleanupOldRequests(maxAge time.Duration) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for requestID, req := range sr.pendingRequests {
		if req.Timestamp.Before(cutoff) {
			delete(sr.pendingRequests, requestID)
			delete(sr.responses, requestID)
			sr.logger.WithField("requestId", requestID).Debug("Cleaned up old replication request")
		}
	}
}

// GetPendingReplicationCount returns the number of pending replications
func (sr *StateReplicator) GetPendingReplicationCount() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return len(sr.pendingRequests)
}

// GetReplicationStatistics returns replication statistics
func (sr *StateReplicator) GetReplicationStatistics() *ReplicationStatistics {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	stats := &ReplicationStatistics{
		TotalRequests:     len(sr.pendingRequests),
		PendingRequests:   0,
		CompletedRequests: 0,
		FailedRequests:    0,
	}

	for requestID, req := range sr.pendingRequests {
		responses := sr.responses[requestID]

		if !req.AckRequired {
			stats.CompletedRequests++
		} else if len(responses) > 0 {
			stats.CompletedRequests++
		} else {
			stats.PendingRequests++
		}
	}

	return stats
}

// ReplicationStatus represents the status of a replication request
type ReplicationStatus struct {
	RequestID         string    `json:"requestId"`
	StateType         string    `json:"stateType"`
	TotalNodes        int       `json:"totalNodes"`
	AcknowledgedNodes int       `json:"acknowledgedNodes"`
	Pending           bool      `json:"pending"`
	Completed         bool      `json:"completed"`
	Timestamp         time.Time `json:"timestamp"`
}

// ReplicationStatistics holds replication statistics
type ReplicationStatistics struct {
	TotalRequests     int       `json:"totalRequests"`
	PendingRequests   int       `json:"pendingRequests"`
	CompletedRequests int       `json:"completedRequests"`
	FailedRequests    int       `json:"failedRequests"`
	LastUpdated       time.Time `json:"lastUpdated"`
}

// SetStrategy changes the replication strategy
func (sr *StateReplicator) SetStrategy(strategy ReplicationStrategy) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.config.Strategy = strategy
	sr.logger.WithField("strategy", strategy).Info("Changed replication strategy")
}

// GetStrategy returns the current replication strategy
func (sr *StateReplicator) GetStrategy() ReplicationStrategy {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.config.Strategy
}

// RetryFailedReplications retries failed replication requests
func (sr *StateReplicator) RetryFailedReplications(ctx context.Context, nodes []*ClusterNode) error {
	sr.mu.RLock()
	failedRequests := make([]*ReplicationRequest, 0)

	for requestID, req := range sr.pendingRequests {
		responses := sr.responses[requestID]
		if req.AckRequired && len(responses) == 0 {
			// This request has no responses yet, consider it failed
			if time.Since(req.Timestamp) > sr.config.AckTimeout {
				failedRequests = append(failedRequests, req)
			}
		}
	}
	sr.mu.RUnlock()

	if len(failedRequests) == 0 {
		return nil
	}

	sr.logger.WithField("count", len(failedRequests)).Info("Retrying failed replications")

	// Retry each failed request
	for _, req := range failedRequests {
		// Create new request ID for retry
		originalID := req.RequestID
		req.RequestID = uuid.New().String()
		req.Timestamp = time.Now()

		// Remove old request
		sr.mu.Lock()
		delete(sr.pendingRequests, originalID)
		delete(sr.responses, originalID)
		sr.mu.Unlock()

		// Retry replication
		if err := sr.ReplicateState(ctx, req.StateType, req.Data, nodes); err != nil {
			sr.logger.WithError(err).WithField("requestId", req.RequestID).Error("Retry failed")
		}
	}

	return nil
}

// WaitForReplication waits for a replication to complete
func (sr *StateReplicator) WaitForReplication(ctx context.Context, requestID string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for replication")
		case <-ticker.C:
			status, err := sr.GetReplicationStatus(requestID)
			if err != nil {
				return err
			}
			if status.Completed {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Start starts background cleanup of old requests
func (sr *StateReplicator) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sr.CleanupOldRequests(30 * time.Minute)
			}
		}
	}()
}
