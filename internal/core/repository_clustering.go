package core

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// ClusterNode represents a node in the repository cluster
type ClusterNode struct {
	ID        string
	Address   string
	Status    string // "active", "inactive", "failed"
	Role      string // "leader", "follower"
	LastSeen  time.Time
	DataSize  int64
	FlowFiles int
}

// ReplicationConfig defines replication behavior
type ReplicationConfig struct {
	Enabled           bool
	ReplicationFactor int           // Number of replicas
	SyncMode          string        // "async" or "sync"
	SyncTimeout       time.Duration // Timeout for sync replication
	ConsistencyLevel  string        // "eventual", "strong"
}

// RepositoryCluster manages distributed repository operations
type RepositoryCluster struct {
	nodeID           string
	nodes            map[string]*ClusterNode
	leader           string
	config           ReplicationConfig
	flowFileRepo     FlowFileRepository
	contentRepo      ContentRepository
	provenanceRepo   ProvenanceRepository
	replicationQueue chan *ReplicationTask
	mu               sync.RWMutex
	stopCh           chan struct{}
	replicationLog   []*ReplicationLogEntry
}

// ReplicationTask represents a replication operation
type ReplicationTask struct {
	ID          uuid.UUID
	Operation   string // "store", "delete", "update"
	Type        string // "flowfile", "content", "provenance"
	Data        []byte
	TargetNodes []string
	Timestamp   time.Time
	Replicated  map[string]bool
	mu          sync.RWMutex
}

// ReplicationLogEntry tracks replication history
type ReplicationLogEntry struct {
	TaskID       uuid.UUID
	Operation    string
	Type         string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	ReplicaCount int
	Errors       []string
}

// ClusterStats provides cluster statistics
type ClusterStats struct {
	TotalNodes       int
	ActiveNodes      int
	LeaderNode       string
	TotalFlowFiles   int
	TotalDataSize    int64
	ReplicationTasks int
	FailedReplicas   int
}

// NewRepositoryCluster creates a new repository cluster
func NewRepositoryCluster(
	nodeID string,
	config ReplicationConfig,
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
) *RepositoryCluster {
	return &RepositoryCluster{
		nodeID:           nodeID,
		nodes:            make(map[string]*ClusterNode),
		config:           config,
		flowFileRepo:     flowFileRepo,
		contentRepo:      contentRepo,
		provenanceRepo:   provenanceRepo,
		replicationQueue: make(chan *ReplicationTask, 1000),
		stopCh:           make(chan struct{}),
		replicationLog:   make([]*ReplicationLogEntry, 0),
	}
}

// Start initializes the cluster and starts replication worker
func (c *RepositoryCluster) Start() error {
	// Register this node
	c.mu.Lock()
	c.nodes[c.nodeID] = &ClusterNode{
		ID:       c.nodeID,
		Status:   "active",
		Role:     "follower",
		LastSeen: time.Now(),
	}
	c.mu.Unlock()

	// Start replication worker
	go c.replicationWorker()

	// Start heartbeat
	go c.heartbeatWorker()

	return nil
}

// Stop shuts down the cluster
func (c *RepositoryCluster) Stop() error {
	close(c.stopCh)
	close(c.replicationQueue)
	return nil
}

// RegisterNode adds a new node to the cluster
func (c *RepositoryCluster) RegisterNode(node *ClusterNode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[node.ID]; exists {
		return fmt.Errorf("node %s already registered", node.ID)
	}

	node.LastSeen = time.Now()
	c.nodes[node.ID] = node

	return nil
}

// UnregisterNode removes a node from the cluster
func (c *RepositoryCluster) UnregisterNode(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if nodeID == c.nodeID {
		return fmt.Errorf("cannot unregister self")
	}

	delete(c.nodes, nodeID)
	return nil
}

// ElectLeader performs leader election
func (c *RepositoryCluster) ElectLeader() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple leader election: choose first active node alphabetically
	var candidates []string
	for id, node := range c.nodes {
		if node.Status == "active" {
			candidates = append(candidates, id)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no active nodes for leader election")
	}

	// Sort candidates (simplified - in real impl use Raft)
	leaderID := candidates[0]
	for _, id := range candidates {
		if id < leaderID {
			leaderID = id
		}
	}

	// Update roles
	for id, node := range c.nodes {
		if id == leaderID {
			node.Role = "leader"
		} else {
			node.Role = "follower"
		}
	}

	c.leader = leaderID
	return leaderID, nil
}

// IsLeader checks if this node is the leader
func (c *RepositoryCluster) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeID == c.leader
}

// GetLeader returns the current leader node ID
func (c *RepositoryCluster) GetLeader() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leader
}

// ReplicateFlowFile replicates a FlowFile to other nodes
func (c *RepositoryCluster) ReplicateFlowFile(ff *types.FlowFile, operation string) error {
	if !c.config.Enabled {
		return nil
	}

	// Serialize FlowFile
	data, err := json.Marshal(ff)
	if err != nil {
		return fmt.Errorf("failed to serialize FlowFile: %w", err)
	}

	// Create replication task
	task := &ReplicationTask{
		ID:         uuid.New(),
		Operation:  operation,
		Type:       "flowfile",
		Data:       data,
		Timestamp:  time.Now(),
		Replicated: make(map[string]bool),
	}

	// Select target nodes for replication
	task.TargetNodes = c.selectReplicationTargets()

	// Queue replication task
	select {
	case c.replicationQueue <- task:
		// Queued successfully
	case <-time.After(5 * time.Second):
		return fmt.Errorf("replication queue full")
	}

	// Wait for sync replication if configured
	if c.config.SyncMode == "sync" {
		return c.waitForReplication(task)
	}

	return nil
}

// ReplicateContent replicates content to other nodes
func (c *RepositoryCluster) ReplicateContent(claim *types.ContentClaim, content []byte) error {
	if !c.config.Enabled {
		return nil
	}

	// Create payload with claim and content
	payload := map[string]interface{}{
		"claim":   claim,
		"content": content,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize content: %w", err)
	}

	// Create replication task
	task := &ReplicationTask{
		ID:         uuid.New(),
		Operation:  "store",
		Type:       "content",
		Data:       data,
		Timestamp:  time.Now(),
		Replicated: make(map[string]bool),
	}

	task.TargetNodes = c.selectReplicationTargets()

	// Queue replication
	select {
	case c.replicationQueue <- task:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("replication queue full")
	}

	if c.config.SyncMode == "sync" {
		return c.waitForReplication(task)
	}

	return nil
}

// selectReplicationTargets selects nodes for replication
func (c *RepositoryCluster) selectReplicationTargets() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targets := make([]string, 0)
	count := 0

	for id, node := range c.nodes {
		if id == c.nodeID {
			continue // Don't replicate to self
		}
		if node.Status != "active" {
			continue
		}
		targets = append(targets, id)
		count++
		if count >= c.config.ReplicationFactor {
			break
		}
	}

	return targets
}

// replicationWorker processes replication tasks
func (c *RepositoryCluster) replicationWorker() {
	for {
		select {
		case task := <-c.replicationQueue:
			c.processReplicationTask(task)
		case <-c.stopCh:
			return
		}
	}
}

// processReplicationTask performs replication to target nodes
func (c *RepositoryCluster) processReplicationTask(task *ReplicationTask) {
	logEntry := &ReplicationLogEntry{
		TaskID:    task.ID,
		Operation: task.Operation,
		Type:      task.Type,
		Status:    "in_progress",
		StartTime: time.Now(),
		Errors:    make([]string, 0),
	}

	successCount := 0

	for _, nodeID := range task.TargetNodes {
		err := c.replicateToNode(nodeID, task)
		if err != nil {
			logEntry.Errors = append(logEntry.Errors, fmt.Sprintf("Node %s: %v", nodeID, err))
		} else {
			task.Replicated[nodeID] = true
			successCount++
		}
	}

	logEntry.EndTime = time.Now()
	logEntry.ReplicaCount = successCount

	if successCount >= len(task.TargetNodes)/2+1 {
		logEntry.Status = "success"
	} else {
		logEntry.Status = "failed"
	}

	c.mu.Lock()
	c.replicationLog = append(c.replicationLog, logEntry)
	c.mu.Unlock()
}

// replicateToNode sends replication data to a specific node
func (c *RepositoryCluster) replicateToNode(nodeID string, task *ReplicationTask) error {
	c.mu.RLock()
	node, exists := c.nodes[nodeID]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if node.Status != "active" {
		return fmt.Errorf("node %s is not active", nodeID)
	}

	// In a real implementation, this would use RPC/HTTP to send data
	// For now, we'll simulate successful replication
	// TODO: Implement actual network replication using gRPC or HTTP

	time.Sleep(10 * time.Millisecond) // Simulate network latency

	return nil
}

// waitForReplication waits for synchronous replication to complete
func (c *RepositoryCluster) waitForReplication(task *ReplicationTask) error {
	timeout := time.After(c.config.SyncTimeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	requiredReplicas := len(task.TargetNodes)/2 + 1

	for {
		select {
		case <-timeout:
			return fmt.Errorf("sync replication timeout")
		case <-ticker.C:
			task.mu.RLock()
			replicated := len(task.Replicated)
			task.mu.RUnlock()

			if replicated >= requiredReplicas {
				return nil
			}
		}
	}
}

// heartbeatWorker sends periodic heartbeats
func (c *RepositoryCluster) heartbeatWorker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sendHeartbeat()
		case <-c.stopCh:
			return
		}
	}
}

// sendHeartbeat updates node status
func (c *RepositoryCluster) sendHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, exists := c.nodes[c.nodeID]; exists {
		node.LastSeen = time.Now()

		// Update node stats
		count, _ := c.flowFileRepo.Count()
		node.FlowFiles = count

		// In real implementation, calculate actual data size
		node.DataSize = int64(count * 1024) // Placeholder
	}

	// Check for failed nodes
	cutoff := time.Now().Add(-30 * time.Second)
	for id, node := range c.nodes {
		if id == c.nodeID {
			continue
		}
		if node.LastSeen.Before(cutoff) && node.Status == "active" {
			node.Status = "failed"
		}
	}
}

// GetNodes returns all cluster nodes
func (c *RepositoryCluster) GetNodes() []*ClusterNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]*ClusterNode, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

// GetNode returns a specific node
func (c *RepositoryCluster) GetNode(nodeID string) (*ClusterNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node, nil
}

// GetStats returns cluster statistics
func (c *RepositoryCluster) GetStats() ClusterStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := ClusterStats{
		TotalNodes:       len(c.nodes),
		LeaderNode:       c.leader,
		ReplicationTasks: len(c.replicationQueue),
	}

	for _, node := range c.nodes {
		if node.Status == "active" {
			stats.ActiveNodes++
		}
		stats.TotalFlowFiles += node.FlowFiles
		stats.TotalDataSize += node.DataSize
	}

	// Count failed replicas
	for _, entry := range c.replicationLog {
		if entry.Status == "failed" {
			stats.FailedReplicas++
		}
	}

	return stats
}

// GetReplicationLog returns recent replication log entries
func (c *RepositoryCluster) GetReplicationLog(limit int) []*ReplicationLogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit == 0 || limit > len(c.replicationLog) {
		limit = len(c.replicationLog)
	}

	start := len(c.replicationLog) - limit
	if start < 0 {
		start = 0
	}

	entries := make([]*ReplicationLogEntry, limit)
	copy(entries, c.replicationLog[start:])

	return entries
}

// SetConfig updates replication configuration
func (c *RepositoryCluster) SetConfig(config ReplicationConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = config
}

// GetConfig returns current replication configuration
func (c *RepositoryCluster) GetConfig() ReplicationConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}
