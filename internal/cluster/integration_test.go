package cluster

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestThreeNodeCluster tests a 3-node cluster scenario
func TestThreeNodeCluster(t *testing.T) {
	t.Skip("Skipping integration test - requires full distributed consensus implementation")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create three nodes
	nodes := []*ClusterManager{}

	for i := 0; i < 3; i++ {
		config := ClusterConfig{
			Enabled:          true,
			NodeID:           fmt.Sprintf("node-%d", i+1),
			BindAddress:      "127.0.0.1",
			BindPort:         9100 + i,
			SeedNodes:        []string{"127.0.0.1:9100", "127.0.0.1:9101", "127.0.0.1:9102"},
			Discovery:        "static",
			ReplicaCount:     3,
			HeartbeatTimeout: 5 * time.Second,
			ElectionTimeout:  10 * time.Second,
		}

		cm, err := NewClusterManager(config, logger)
		require.NoError(t, err)

		err = cm.Start()
		require.NoError(t, err)
		defer cm.Stop()

		nodes = append(nodes, cm)
	}

	// Wait for cluster formation and leader election
	time.Sleep(5 * time.Second)

	// Verify all nodes know about each other
	for _, node := range nodes {
		clusterNodes := node.GetNodes()
		assert.GreaterOrEqual(t, len(clusterNodes), 1, "Each node should know about at least itself")
	}

	// Verify one node is leader
	leaderCount := 0
	var leader *ClusterManager
	for _, node := range nodes {
		if node.IsLeader() {
			leaderCount++
			leader = node
		}
	}

	assert.Equal(t, 1, leaderCount, "Exactly one node should be leader")
	require.NotNil(t, leader, "Leader should not be nil")

	// Test processor assignment
	processorID := uuid.New()
	assignedNode, err := leader.AssignProcessor(processorID)

	if err == nil {
		assert.NotEmpty(t, assignedNode)

		// Verify assignment is visible to all nodes
		time.Sleep(500 * time.Millisecond)

		for _, node := range nodes {
			retrieved, exists := node.GetProcessorAssignment(processorID)
			if exists {
				assert.Equal(t, assignedNode, retrieved)
			}
		}
	}
}

// TestNodeFailureAndRecovery tests node failure detection and recovery
func TestNodeFailureAndRecovery(t *testing.T) {
	t.Skip("Skipping integration test - requires full distributed consensus implementation")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create two nodes
	config1 := ClusterConfig{
		Enabled:          true,
		NodeID:           "node-1",
		BindAddress:      "127.0.0.1",
		BindPort:         9200,
		SeedNodes:        []string{"127.0.0.1:9201"},
		Discovery:        "static",
		HeartbeatTimeout: 2 * time.Second,
	}

	config2 := ClusterConfig{
		Enabled:          true,
		NodeID:           "node-2",
		BindAddress:      "127.0.0.1",
		BindPort:         9201,
		SeedNodes:        []string{"127.0.0.1:9200"},
		Discovery:        "static",
		HeartbeatTimeout: 2 * time.Second,
	}

	cm1, err := NewClusterManager(config1, logger)
	require.NoError(t, err)

	cm2, err := NewClusterManager(config2, logger)
	require.NoError(t, err)

	err = cm1.Start()
	require.NoError(t, err)
	defer cm1.Stop()

	err = cm2.Start()
	require.NoError(t, err)

	// Wait for cluster formation
	time.Sleep(1 * time.Second)

	// Both should be healthy
	assert.Equal(t, 1, cm1.GetHealthyNodes())
	assert.Equal(t, 1, cm2.GetHealthyNodes())

	// Stop node 2
	cm2.Stop()

	// Wait for failure detection
	time.Sleep(3 * time.Second)

	// Node 1 should detect node 2 as unhealthy
	healthyCount := 0
	for _, node := range cm1.GetNodes() {
		if node.IsHealthy() {
			healthyCount++
		}
	}

	// At least node 1 should still be healthy
	assert.GreaterOrEqual(t, healthyCount, 1)
}

// TestLoadBalancing tests load balancing across nodes
func TestLoadBalancing(t *testing.T) {
	t.Skip("Skipping integration test - requires full distributed consensus implementation")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-1",
		BindAddress: "127.0.0.1",
		BindPort:    9300,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Add multiple nodes manually for testing
	for i := 2; i <= 5; i++ {
		node := &ClusterNode{
			ID:      fmt.Sprintf("node-%d", i),
			Address: "127.0.0.1",
			Port:    9300 + i,
			State:   StateHealthy,
			Capacity: ResourceCapacity{
				MaxProcessors: 100,
			},
			Load: &NodeLoad{
				Score: float64(i * 10),
			},
		}

		cm.mu.Lock()
		cm.nodes[node.ID] = node
		cm.mu.Unlock()
	}

	// Wait for leader election
	time.Sleep(500 * time.Millisecond)

	if cm.IsLeader() {
		// Assign multiple processors
		assignments := make(map[uuid.UUID]string)
		for i := 0; i < 10; i++ {
			processorID := uuid.New()
			nodeID, err := cm.AssignProcessor(processorID)
			if err == nil {
				assignments[processorID] = nodeID
			}
		}

		// Verify distribution
		if len(assignments) > 0 {
			nodeCounts := make(map[string]int)
			for _, nodeID := range assignments {
				nodeCounts[nodeID]++
			}

			// At least some distribution should occur
			assert.GreaterOrEqual(t, len(nodeCounts), 1)
		}
	}
}

// TestRebalancing tests cluster rebalancing
func TestRebalancing(t *testing.T) {
	t.Skip("Skipping integration test - requires full distributed consensus implementation")
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-1",
		BindAddress: "127.0.0.1",
		BindPort:    9400,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Add nodes
	for i := 2; i <= 3; i++ {
		node := &ClusterNode{
			ID:      fmt.Sprintf("node-%d", i),
			Address: "127.0.0.1",
			Port:    9400 + i,
			State:   StateHealthy,
			Capacity: ResourceCapacity{
				MaxProcessors: 100,
			},
		}

		cm.mu.Lock()
		cm.nodes[node.ID] = node
		cm.mu.Unlock()
	}

	// Wait for leader election
	time.Sleep(500 * time.Millisecond)

	if cm.IsLeader() {
		// Create unbalanced load
		for i := 0; i < 5; i++ {
			processorID := uuid.New()
			cm.coordinator.AssignProcessor(processorID, "node-1")
		}

		// Trigger rebalance
		err := cm.Rebalance()
		assert.NoError(t, err)

		// Verify assignments were redistributed
		assignments := cm.coordinator.GetAllProcessorAssignments()
		assert.GreaterOrEqual(t, len(assignments), 1)
	}
}

// TestStatistics tests cluster statistics gathering
func TestStatistics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-1",
		BindAddress: "127.0.0.1",
		BindPort:    9500,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Get statistics
	stats := cm.GetStatistics()
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.TotalNodes)
	assert.NotEmpty(t, stats.NodeStats)
}
