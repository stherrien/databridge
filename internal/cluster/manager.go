package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ClusterManager manages cluster membership and coordination
type ClusterManager struct {
	nodeID          string
	config          ClusterConfig
	nodes           map[string]*ClusterNode
	coordinator     *Coordinator
	healthChecker   *HealthChecker
	stateReplicator *StateReplicator
	loadBalancer    *LoadBalancer
	discovery       *Discovery
	logger          *logrus.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	eventChan       chan ClusterEvent
	running         bool
	mu              sync.RWMutex
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(config ClusterConfig, logger *logrus.Logger) (*ClusterManager, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("clustering is not enabled")
	}

	// Generate node ID if not provided
	if config.NodeID == "" {
		config.NodeID = uuid.New().String()
	}

	// Set defaults
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 10 * time.Second
	}
	if config.ElectionTimeout == 0 {
		config.ElectionTimeout = 30 * time.Second
	}
	if config.ReplicaCount == 0 {
		config.ReplicaCount = 3
	}

	ctx, cancel := context.WithCancel(context.Background())

	cm := &ClusterManager{
		nodeID:    config.NodeID,
		config:    config,
		nodes:     make(map[string]*ClusterNode),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan ClusterEvent, 100),
	}

	// Initialize health checker
	healthConfig := HealthCheckerConfig{
		Interval:      5 * time.Second,
		Timeout:       3 * time.Second,
		FailThreshold: 3,
	}
	cm.healthChecker = NewHealthChecker(healthConfig, logger)

	// Initialize load balancer
	lbConfig := LoadBalancerConfig{
		Strategy:        StrategyLeastLoaded,
		RebalanceWindow: 30 * time.Second,
	}
	cm.loadBalancer = NewLoadBalancer(lbConfig, logger)

	// Initialize state replicator
	replicationConfig := ReplicationConfig{
		Strategy:   ReplicationQuorum,
		AckTimeout: 5 * time.Second,
		RetryCount: 3,
	}
	cm.stateReplicator = NewStateReplicator(replicationConfig, logger)

	// Initialize coordinator (Raft-based)
	coordinatorConfig := CoordinatorConfig{
		NodeID:           config.NodeID,
		BindAddress:      config.BindAddress,
		BindPort:         config.BindPort,
		ElectionTimeout:  config.ElectionTimeout,
		HeartbeatTimeout: config.HeartbeatTimeout,
	}
	coordinator, err := NewCoordinator(coordinatorConfig, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create coordinator: %w", err)
	}
	cm.coordinator = coordinator

	// Initialize discovery
	discoveryConfig := DiscoveryConfig{
		Method:   DiscoveryMethod(config.Discovery),
		Seeds:    config.SeedNodes,
		Port:     config.BindPort,
		Interval: 10 * time.Second,
	}
	discovery, err := NewDiscovery(discoveryConfig, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create discovery: %w", err)
	}
	cm.discovery = discovery

	// Register local node
	localNode := &ClusterNode{
		ID:            config.NodeID,
		Address:       config.BindAddress,
		Port:          config.BindPort,
		Role:          RoleWorker, // Start as worker, election will determine primary
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
		Metadata: map[string]string{
			"version": "1.0.0",
		},
		Capacity: ResourceCapacity{
			CPUCores:       8,
			MemoryGB:       16,
			MaxProcessors:  100,
			MaxConnections: 1000,
		},
		Load: &NodeLoad{
			CPUUsage:         0.0,
			MemoryUsage:      0.0,
			ActiveProcessors: 0,
			QueueDepth:       0,
			Score:            0.0,
			LastUpdated:      time.Now(),
		},
	}
	cm.nodes[config.NodeID] = localNode

	logger.WithFields(logrus.Fields{
		"nodeId":      config.NodeID,
		"bindAddress": config.BindAddress,
		"bindPort":    config.BindPort,
	}).Info("Cluster manager created")

	return cm, nil
}

// Start starts the cluster manager
func (cm *ClusterManager) Start() error {
	cm.mu.Lock()
	if cm.running {
		cm.mu.Unlock()
		return fmt.Errorf("cluster manager already running")
	}
	cm.running = true
	cm.mu.Unlock()

	cm.logger.Info("Starting cluster manager")

	// Start coordinator
	if err := cm.coordinator.Start(); err != nil {
		return fmt.Errorf("failed to start coordinator: %w", err)
	}

	// Start discovery
	if err := cm.discovery.Start(cm.ctx, cm.onNodeDiscovered); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}

	// Create a copy of nodes map for health checker to avoid data race
	cm.mu.Lock()
	nodesCopy := make(map[string]*ClusterNode, len(cm.nodes))
	for k, v := range cm.nodes {
		nodesCopy[k] = v
	}
	cm.mu.Unlock()

	// Start health checker
	if err := cm.healthChecker.Start(cm.ctx, nodesCopy, cm.onHealthCheckFailed); err != nil {
		return fmt.Errorf("failed to start health checker: %w", err)
	}

	// Start event processing
	go cm.processEvents()

	// Start load monitoring
	go cm.monitorLoad()

	cm.logger.Info("Cluster manager started successfully")
	return nil
}

// Stop stops the cluster manager
func (cm *ClusterManager) Stop() error {
	cm.mu.Lock()
	if !cm.running {
		cm.mu.Unlock()
		return nil
	}
	cm.running = false
	cm.mu.Unlock()

	cm.logger.Info("Stopping cluster manager")

	// Cancel context to stop all goroutines
	cm.cancel()

	// Stop coordinator
	if err := cm.coordinator.Stop(); err != nil {
		cm.logger.WithError(err).Error("Error stopping coordinator")
	}

	// Stop discovery
	if err := cm.discovery.Stop(); err != nil {
		cm.logger.WithError(err).Error("Error stopping discovery")
	}

	cm.logger.Info("Cluster manager stopped")
	return nil
}

// GetNodeID returns the ID of this node
func (cm *ClusterManager) GetNodeID() string {
	return cm.nodeID
}

// GetNodes returns all known nodes
func (cm *ClusterManager) GetNodes() []*ClusterNode {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	nodes := make([]*ClusterNode, 0, len(cm.nodes))
	for _, node := range cm.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetNode returns a specific node by ID
func (cm *ClusterManager) GetNode(nodeID string) (*ClusterNode, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	node, exists := cm.nodes[nodeID]
	return node, exists
}

// GetHealthyNodes returns all healthy nodes
func (cm *ClusterManager) GetHealthyNodes() []*ClusterNode {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var healthy []*ClusterNode
	for _, node := range cm.nodes {
		if node.IsHealthy() {
			healthy = append(healthy, node)
		}
	}
	return healthy
}

// IsLeader returns whether this node is the cluster leader
func (cm *ClusterManager) IsLeader() bool {
	return cm.coordinator.IsLeader()
}

// GetLeader returns the current cluster leader
func (cm *ClusterManager) GetLeader() string {
	return cm.coordinator.GetLeader()
}

// AssignProcessor assigns a processor to a node
func (cm *ClusterManager) AssignProcessor(processorID uuid.UUID) (string, error) {
	if !cm.IsLeader() {
		return "", fmt.Errorf("only leader can assign processors")
	}

	// Get healthy nodes
	healthyNodes := cm.GetHealthyNodes()
	if len(healthyNodes) == 0 {
		return "", fmt.Errorf("no healthy nodes available")
	}

	// Use load balancer to select node
	selectedNode := cm.loadBalancer.SelectNode(healthyNodes, processorID.String())
	if selectedNode == nil {
		return "", fmt.Errorf("failed to select node for processor")
	}

	// Assign processor via coordinator
	if err := cm.coordinator.AssignProcessor(processorID, selectedNode.ID); err != nil {
		return "", fmt.Errorf("failed to assign processor: %w", err)
	}

	cm.logger.WithFields(logrus.Fields{
		"processorId": processorID,
		"nodeId":      selectedNode.ID,
	}).Info("Assigned processor to node")

	// Emit event
	cm.emitEvent(ClusterEvent{
		Type:      EventWorkAssigned,
		NodeID:    selectedNode.ID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"processorId": processorID.String(),
		},
	})

	return selectedNode.ID, nil
}

// GetProcessorAssignment returns the node assigned to a processor
func (cm *ClusterManager) GetProcessorAssignment(processorID uuid.UUID) (string, bool) {
	return cm.coordinator.GetProcessorAssignment(processorID)
}

// Rebalance triggers a rebalancing of work across nodes
func (cm *ClusterManager) Rebalance() error {
	if !cm.IsLeader() {
		return fmt.Errorf("only leader can trigger rebalancing")
	}

	cm.logger.Info("Starting cluster rebalancing")

	// Get current assignments
	assignments := cm.coordinator.GetAllProcessorAssignments()

	// Get healthy nodes
	healthyNodes := cm.GetHealthyNodes()
	if len(healthyNodes) == 0 {
		return fmt.Errorf("no healthy nodes available for rebalancing")
	}

	// Calculate new assignments
	newAssignments := cm.loadBalancer.Rebalance(healthyNodes, assignments)

	// Apply new assignments
	for processorID, nodeID := range newAssignments {
		procUUID, err := uuid.Parse(processorID)
		if err != nil {
			cm.logger.WithError(err).Errorf("Invalid processor ID: %s", processorID)
			continue
		}

		if err := cm.coordinator.AssignProcessor(procUUID, nodeID); err != nil {
			cm.logger.WithError(err).Errorf("Failed to assign processor %s to node %s", processorID, nodeID)
		}
	}

	cm.logger.WithField("reassignments", len(newAssignments)).Info("Cluster rebalancing completed")

	// Emit event
	cm.emitEvent(ClusterEvent{
		Type:      EventWorkRebalanced,
		NodeID:    cm.nodeID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"reassignments": len(newAssignments),
		},
	})

	return nil
}

// JoinCluster joins an existing cluster
func (cm *ClusterManager) JoinCluster(seedNode string) error {
	cm.logger.WithField("seedNode", seedNode).Info("Joining cluster")

	// Use coordinator to join
	if err := cm.coordinator.JoinCluster(seedNode); err != nil {
		return fmt.Errorf("failed to join cluster: %w", err)
	}

	cm.logger.Info("Successfully joined cluster")
	return nil
}

// LeaveCluster gracefully leaves the cluster
func (cm *ClusterManager) LeaveCluster() error {
	cm.logger.Info("Leaving cluster")

	// Use coordinator to leave
	if err := cm.coordinator.LeaveCluster(); err != nil {
		return fmt.Errorf("failed to leave cluster: %w", err)
	}

	cm.logger.Info("Successfully left cluster")
	return nil
}

// GetStatistics returns cluster-wide statistics
func (cm *ClusterManager) GetStatistics() *ClusterStatistics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := &ClusterStatistics{
		TotalNodes:  len(cm.nodes),
		Leader:      cm.GetLeader(),
		NodeStats:   make(map[string]*NodeStats),
		LastUpdated: time.Now(),
	}

	// Count healthy nodes and calculate average load
	var totalLoad float64
	var totalProcessors int
	var activeProcessors int

	for _, node := range cm.nodes {
		if node.IsHealthy() {
			stats.HealthyNodes++
		}

		if node.Load != nil {
			totalLoad += node.Load.Score
			activeProcessors += node.Load.ActiveProcessors
		}

		// Create node stats
		nodeStats := &NodeStats{
			NodeID:          node.ID,
			Uptime:          time.Since(node.LastHeartbeat),
			ProcessorsCount: 0, // Would need to count from assignments
			QueueDepth:      0,
			ProcessedCount:  0,
			LastHeartbeat:   node.LastHeartbeat,
		}

		if node.Load != nil {
			nodeStats.QueueDepth = node.Load.QueueDepth
		}

		stats.NodeStats[node.ID] = nodeStats
		totalProcessors++
	}

	if len(cm.nodes) > 0 {
		stats.AverageLoad = totalLoad / float64(len(cm.nodes))
	}

	stats.TotalProcessors = totalProcessors
	stats.ActiveProcessors = activeProcessors

	return stats
}

// UpdateNodeLoad updates the load metrics for a node
func (cm *ClusterManager) UpdateNodeLoad(load *NodeLoad) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if node, exists := cm.nodes[cm.nodeID]; exists {
		node.Load = load
		node.Load.LastUpdated = time.Now()
	}

	// Update load balancer
	cm.loadBalancer.UpdateNodeLoad(cm.nodeID, load)
}

// Private methods

func (cm *ClusterManager) onNodeDiscovered(node *ClusterNode) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.nodes[node.ID]; !exists {
		cm.nodes[node.ID] = node
		cm.logger.WithField("nodeId", node.ID).Info("Discovered new node")

		// Start health checking for this node
		cm.healthChecker.AddNode(node)

		// Emit event
		cm.emitEvent(ClusterEvent{
			Type:      EventNodeJoined,
			NodeID:    node.ID,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"address": node.Address,
				"port":    node.Port,
			},
		})
	}
}

func (cm *ClusterManager) onHealthCheckFailed(nodeID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if node, exists := cm.nodes[nodeID]; exists {
		previousState := node.State
		node.UpdateState(StateUnhealthy)

		cm.logger.WithField("nodeId", nodeID).Warn("Node health check failed")

		// If leader, trigger rebalancing for failed node's work
		if cm.IsLeader() && previousState != StateUnhealthy {
			go func() {
				cm.logger.WithField("nodeId", nodeID).Info("Rebalancing work from unhealthy node")
				if err := cm.Rebalance(); err != nil {
					cm.logger.WithError(err).Error("Failed to rebalance after node failure")
				}
			}()
		}

		// Emit event
		cm.emitEvent(ClusterEvent{
			Type:      EventNodeFailed,
			NodeID:    nodeID,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{},
		})
	}
}

func (cm *ClusterManager) processEvents() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		case event := <-cm.eventChan:
			cm.handleEvent(event)
		}
	}
}

func (cm *ClusterManager) handleEvent(event ClusterEvent) {
	cm.logger.WithFields(logrus.Fields{
		"type":   event.Type,
		"nodeId": event.NodeID,
	}).Debug("Processing cluster event")

	// Additional event handling logic can be added here
	// For example, triggering notifications, metrics, etc.
}

func (cm *ClusterManager) emitEvent(event ClusterEvent) {
	select {
	case cm.eventChan <- event:
	default:
		cm.logger.Warn("Event channel full, dropping event")
	}
}

func (cm *ClusterManager) monitorLoad() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			// Calculate current load
			load := cm.calculateLoad()
			cm.UpdateNodeLoad(load)
		}
	}
}

func (cm *ClusterManager) calculateLoad() *NodeLoad {
	// This is a simplified load calculation
	// In production, would integrate with actual system metrics
	return &NodeLoad{
		CPUUsage:         0.0, // Would get from runtime
		MemoryUsage:      0.0, // Would get from runtime
		ActiveProcessors: 0,   // Would get from flow controller
		QueueDepth:       0,   // Would get from queues
		Score:            0.0, // Composite score
		LastUpdated:      time.Now(),
	}
}

// RemoveNode removes a node from the cluster
func (cm *ClusterManager) RemoveNode(nodeID string) error {
	if !cm.IsLeader() {
		return fmt.Errorf("only leader can remove nodes")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if nodeID == cm.nodeID {
		return fmt.Errorf("cannot remove local node")
	}

	if _, exists := cm.nodes[nodeID]; !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	// Remove from health checker
	cm.healthChecker.RemoveNode(nodeID)

	// Remove node
	delete(cm.nodes, nodeID)

	cm.logger.WithField("nodeId", nodeID).Info("Removed node from cluster")

	// Emit event
	cm.emitEvent(ClusterEvent{
		Type:      EventNodeLeft,
		NodeID:    nodeID,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{},
	})

	// Trigger rebalancing
	go func() {
		if err := cm.Rebalance(); err != nil {
			cm.logger.WithError(err).Error("Failed to rebalance after node removal")
		}
	}()

	return nil
}

// GetCoordinator returns the coordinator instance
func (cm *ClusterManager) GetCoordinator() *Coordinator {
	return cm.coordinator
}

// GetLoadBalancer returns the load balancer instance
func (cm *ClusterManager) GetLoadBalancer() *LoadBalancer {
	return cm.loadBalancer
}

// GetHealthChecker returns the health checker instance
func (cm *ClusterManager) GetHealthChecker() *HealthChecker {
	return cm.healthChecker
}

// SubscribeToEvents returns a channel for cluster events
func (cm *ClusterManager) SubscribeToEvents() <-chan ClusterEvent {
	return cm.eventChan
}
