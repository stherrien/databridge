package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Coordinator provides distributed coordination and consensus
// This is a simplified implementation that simulates Raft-like behavior
// In production, would use hashicorp/raft or etcd
type Coordinator struct {
	config        CoordinatorConfig
	state         *ClusterState
	isLeader      bool
	leader        string
	peers         map[string]*Peer
	logger     *logrus.Logger
	mu         sync.RWMutex
	leaderChan chan bool
	stopChan   chan struct{}
	running    bool
}

// CoordinatorConfig holds coordinator configuration
type CoordinatorConfig struct {
	NodeID           string
	BindAddress      string
	BindPort         int
	ElectionTimeout  time.Duration
	HeartbeatTimeout time.Duration
}

// Peer represents a cluster peer
type Peer struct {
	ID          string
	Address     string
	Port        int
	LastContact time.Time
	mu          sync.RWMutex
}

// NewCoordinator creates a new coordinator
func NewCoordinator(config CoordinatorConfig, logger *logrus.Logger) (*Coordinator, error) {
	c := &Coordinator{
		config:     config,
		state:      NewClusterState(),
		isLeader:   false,
		leader:     "",
		peers:      make(map[string]*Peer),
		logger:     logger,
		leaderChan: make(chan bool, 1),
		stopChan:   make(chan struct{}),
	}

	logger.WithField("nodeId", config.NodeID).Info("Coordinator created")
	return c, nil
}

// Start starts the coordinator
func (c *Coordinator) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("coordinator already running")
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Info("Starting coordinator")

	// Start election timer
	go c.runElection()

	// Start heartbeat (if leader)
	go c.runHeartbeat()

	return nil
}

// Stop stops the coordinator
func (c *Coordinator) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info("Stopping coordinator")
	close(c.stopChan)
	return nil
}

// IsLeader returns whether this node is the leader
func (c *Coordinator) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isLeader
}

// GetLeader returns the current leader
func (c *Coordinator) GetLeader() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.leader
}

// AssignProcessor assigns a processor to a node
func (c *Coordinator) AssignProcessor(processorID uuid.UUID, nodeID string) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader, cannot assign processor")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.state.AssignProcessor(processorID, nodeID)

	c.logger.WithFields(logrus.Fields{
		"processorId": processorID,
		"nodeId":      nodeID,
	}).Info("Assigned processor via coordinator")

	return nil
}

// GetProcessorAssignment returns the node assigned to a processor
func (c *Coordinator) GetProcessorAssignment(processorID uuid.UUID) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.GetProcessorAssignment(processorID)
}

// GetAllProcessorAssignments returns all processor assignments
func (c *Coordinator) GetAllProcessorAssignments() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	assignments := make(map[string]string)
	for k, v := range c.state.ProcessorLeases {
		assignments[k] = v
	}
	return assignments
}

// AddNode adds a node to the cluster
func (c *Coordinator) AddNode(node *ClusterNode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state.AddNode(node)

	// Add as peer
	peer := &Peer{
		ID:          node.ID,
		Address:     node.Address,
		Port:        node.Port,
		LastContact: time.Now(),
	}
	c.peers[node.ID] = peer

	c.logger.WithField("nodeId", node.ID).Info("Added node to coordinator")
	return nil
}

// RemoveNode removes a node from the cluster
func (c *Coordinator) RemoveNode(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state.RemoveNode(nodeID)
	delete(c.peers, nodeID)

	c.logger.WithField("nodeId", nodeID).Info("Removed node from coordinator")
	return nil
}

// JoinCluster joins an existing cluster
func (c *Coordinator) JoinCluster(seedNode string) error {
	c.logger.WithField("seedNode", seedNode).Info("Joining cluster via coordinator")

	// In a real implementation, would send join request to seed node
	// For now, simulate successful join
	c.mu.Lock()
	c.leader = seedNode
	c.isLeader = false
	c.mu.Unlock()

	return nil
}

// LeaveCluster gracefully leaves the cluster
func (c *Coordinator) LeaveCluster() error {
	c.logger.Info("Leaving cluster via coordinator")

	c.mu.Lock()
	defer c.mu.Unlock()

	// Step down if leader
	if c.isLeader {
		c.isLeader = false
		c.leader = ""
		c.logger.Info("Stepped down as leader")
	}

	return nil
}

// GetState returns the current cluster state
func (c *Coordinator) GetState() *ClusterState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// ApplyCommand applies a command to the state machine
func (c *Coordinator) ApplyCommand(command interface{}) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader, cannot apply command")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// In real Raft, would replicate log entry to followers
	// For now, apply directly
	c.state.Version++

	c.logger.WithField("version", c.state.Version).Debug("Applied command to state machine")
	return nil
}

// Private methods

func (c *Coordinator) runElection() {
	// Simplified election logic
	// In production, would implement full Raft election algorithm

	timeout := c.config.ElectionTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-timer.C:
			// If no leader and not already leader, start election
			c.mu.RLock()
			hasLeader := c.leader != ""
			alreadyLeader := c.isLeader
			c.mu.RUnlock()

			if !hasLeader && !alreadyLeader {
				c.startElection()
			}

			// Reset timer
			timer.Reset(timeout)
		}
	}
}

func (c *Coordinator) startElection() {
	c.logger.Info("Starting leader election")

	// Simplified election: if no peers or majority available, become leader
	c.mu.Lock()
	defer c.mu.Unlock()

	peerCount := len(c.peers)
	if peerCount == 0 {
		// Single node cluster, become leader
		c.becomeLeader()
		return
	}

	// In real implementation, would request votes from peers
	// For now, simulate election based on node ID (lowest ID wins)
	lowestID := c.config.NodeID
	for _, peer := range c.peers {
		if peer.ID < lowestID {
			lowestID = peer.ID
		}
	}

	if lowestID == c.config.NodeID {
		c.becomeLeader()
	} else {
		c.leader = lowestID
		c.logger.WithField("leader", lowestID).Info("Recognized new leader")
	}
}

func (c *Coordinator) becomeLeader() {
	c.isLeader = true
	c.leader = c.config.NodeID

	c.logger.Info("Became cluster leader")

	// Notify via channel
	select {
	case c.leaderChan <- true:
	default:
	}
}

func (c *Coordinator) runHeartbeat() {
	timeout := c.config.HeartbeatTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ticker := time.NewTicker(timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if c.IsLeader() {
				c.sendHeartbeats()
			}
		}
	}
}

func (c *Coordinator) sendHeartbeats() {
	c.mu.RLock()
	peers := make([]*Peer, 0, len(c.peers))
	for _, peer := range c.peers {
		peers = append(peers, peer)
	}
	c.mu.RUnlock()

	// In real implementation, would send heartbeat RPCs to peers
	// For now, just update last contact time
	for _, peer := range peers {
		peer.mu.Lock()
		peer.LastContact = time.Now()
		peer.mu.Unlock()
	}
}

// ClusterFSM implements a finite state machine for cluster state
type ClusterFSM struct {
	state *ClusterState
	mu    sync.RWMutex
}

// NewClusterFSM creates a new cluster FSM
func NewClusterFSM() *ClusterFSM {
	return &ClusterFSM{
		state: NewClusterState(),
	}
}

// Apply applies a log entry to the FSM
func (fsm *ClusterFSM) Apply(logEntry interface{}) interface{} {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// Parse and apply command
	switch cmd := logEntry.(type) {
	case *AddNodeCommand:
		fsm.state.AddNode(cmd.Node)
	case *RemoveNodeCommand:
		fsm.state.RemoveNode(cmd.NodeID)
	case *AssignProcessorCommand:
		fsm.state.AssignProcessor(cmd.ProcessorID, cmd.NodeID)
	default:
		return fmt.Errorf("unknown command type")
	}

	return nil
}

// Snapshot creates a snapshot of the current state
func (fsm *ClusterFSM) Snapshot() ([]byte, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	return json.Marshal(fsm.state)
}

// Restore restores state from a snapshot
func (fsm *ClusterFSM) Restore(snapshot io.ReadCloser) error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	decoder := json.NewDecoder(snapshot)
	return decoder.Decode(&fsm.state)
}

// GetState returns the current state
func (fsm *ClusterFSM) GetState() *ClusterState {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.state
}

// Command types for FSM

// AddNodeCommand adds a node to the cluster
type AddNodeCommand struct {
	Node *ClusterNode
}

// RemoveNodeCommand removes a node from the cluster
type RemoveNodeCommand struct {
	NodeID string
}

// AssignProcessorCommand assigns a processor to a node
type AssignProcessorCommand struct {
	ProcessorID uuid.UUID
	NodeID      string
}

// UpdateConfigCommand updates shared configuration
type UpdateConfigCommand struct {
	Key   string
	Value interface{}
}

// WaitForLeader waits until a leader is elected
func (c *Coordinator) WaitForLeader(timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for leader election")
		case <-ticker.C:
			if c.GetLeader() != "" {
				return nil
			}
		}
	}
}

// GetPeers returns all known peers
func (c *Coordinator) GetPeers() []*Peer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	peers := make([]*Peer, 0, len(c.peers))
	for _, peer := range c.peers {
		peers = append(peers, peer)
	}
	return peers
}

// UpdateSharedConfig updates a shared configuration value
func (c *Coordinator) UpdateSharedConfig(key string, value interface{}) error {
	if !c.IsLeader() {
		return fmt.Errorf("not leader, cannot update config")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.state.SharedConfig[key] = value
	c.state.Version++

	c.logger.WithFields(logrus.Fields{
		"key":     key,
		"version": c.state.Version,
	}).Info("Updated shared config")

	return nil
}

// GetSharedConfig retrieves a shared configuration value
func (c *Coordinator) GetSharedConfig(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, exists := c.state.SharedConfig[key]
	return value, exists
}

// GetQuorumSize returns the required quorum size
func (c *Coordinator) GetQuorumSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalNodes := len(c.peers) + 1 // +1 for self
	return (totalNodes / 2) + 1
}

// HasQuorum checks if we have quorum
func (c *Coordinator) HasQuorum() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	requiredQuorum := c.GetQuorumSize()

	// Count healthy peers
	healthyPeers := 1 // Start with self
	for _, peer := range c.peers {
		peer.mu.RLock()
		isRecent := time.Since(peer.LastContact) < c.config.HeartbeatTimeout*2
		peer.mu.RUnlock()

		if isRecent {
			healthyPeers++
		}
	}

	return healthyPeers >= requiredQuorum
}
