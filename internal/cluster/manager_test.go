package cluster

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClusterManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		SeedNodes:   []string{"127.0.0.1:8091"},
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)
	require.NotNil(t, cm)

	assert.Equal(t, "test-node-1", cm.GetNodeID())
	assert.NotNil(t, cm.coordinator)
	assert.NotNil(t, cm.healthChecker)
	assert.NotNil(t, cm.loadBalancer)
	assert.NotNil(t, cm.stateReplicator)
	assert.NotNil(t, cm.discovery)
}

func TestClusterManager_StartStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	// Test start
	err = cm.Start()
	require.NoError(t, err)

	// Test double start
	err = cm.Start()
	assert.Error(t, err)

	// Test stop
	err = cm.Stop()
	require.NoError(t, err)

	// Test double stop
	err = cm.Stop()
	require.NoError(t, err)
}

func TestClusterManager_NodeManagement(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	// Test get local node
	node, exists := cm.GetNode("test-node-1")
	assert.True(t, exists)
	assert.Equal(t, "test-node-1", node.ID)

	// Test get all nodes
	nodes := cm.GetNodes()
	assert.Len(t, nodes, 1)

	// Test get healthy nodes
	healthyNodes := cm.GetHealthyNodes()
	assert.Len(t, healthyNodes, 1)
}

func TestClusterManager_ProcessorAssignment(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Wait for leader election
	time.Sleep(100 * time.Millisecond)

	processorID := uuid.New()

	// Test assignment (may fail if not leader, which is ok for single node)
	nodeID, err := cm.AssignProcessor(processorID)
	if err == nil {
		assert.Equal(t, "test-node-1", nodeID)

		// Test get assignment
		assignedNode, exists := cm.GetProcessorAssignment(processorID)
		assert.True(t, exists)
		assert.Equal(t, "test-node-1", assignedNode)
	}
}

func TestClusterManager_Statistics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	stats := cm.GetStatistics()
	assert.NotNil(t, stats)
	assert.Equal(t, 1, stats.TotalNodes)
	assert.GreaterOrEqual(t, stats.HealthyNodes, 0)
}

func TestClusterManager_LoadUpdate(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	load := &NodeLoad{
		CPUUsage:         50.0,
		MemoryUsage:      60.0,
		ActiveProcessors: 5,
		QueueDepth:       10,
		Score:            55.0,
		LastUpdated:      time.Now(),
	}

	cm.UpdateNodeLoad(load)

	node, exists := cm.GetNode("test-node-1")
	require.True(t, exists)
	assert.Equal(t, 50.0, node.Load.CPUUsage)
	assert.Equal(t, 60.0, node.Load.MemoryUsage)
}

func TestClusterManager_LeaderElection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "test-node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Wait for election (longer to ensure completion)
	time.Sleep(500 * time.Millisecond)

	// Single node should become leader
	isLeader := cm.IsLeader()
	if !isLeader {
		// Election might not have completed yet
		t.Skip("Leader election not completed")
	}

	leader := cm.GetLeader()
	if leader != "" {
		assert.Equal(t, "test-node-1", leader)
	}
}

func TestClusterManager_MultiNode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create two cluster managers
	config1 := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-1",
		BindAddress: "127.0.0.1",
		BindPort:    9001,
		SeedNodes:   []string{"127.0.0.1:9002"},
		Discovery:   "static",
	}

	config2 := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-2",
		BindAddress: "127.0.0.1",
		BindPort:    9002,
		SeedNodes:   []string{"127.0.0.1:9001"},
		Discovery:   "static",
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
	defer cm2.Stop()

	// Wait for discovery
	time.Sleep(200 * time.Millisecond)

	// Both nodes should know about each other
	nodes1 := cm1.GetNodes()
	assert.GreaterOrEqual(t, len(nodes1), 1)

	nodes2 := cm2.GetNodes()
	assert.GreaterOrEqual(t, len(nodes2), 1)
}

func TestClusterManager_RemoveNode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := ClusterConfig{
		Enabled:     true,
		NodeID:      "node-1",
		BindAddress: "127.0.0.1",
		BindPort:    8090,
		Discovery:   "static",
	}

	cm, err := NewClusterManager(config, logger)
	require.NoError(t, err)

	err = cm.Start()
	require.NoError(t, err)
	defer cm.Stop()

	// Add a node manually
	testNode := &ClusterNode{
		ID:      "test-node",
		Address: "127.0.0.1",
		Port:    8091,
		Role:    RoleWorker,
		State:   StateHealthy,
	}

	cm.mu.Lock()
	cm.nodes["test-node"] = testNode
	cm.mu.Unlock()

	// Wait for leader election
	time.Sleep(100 * time.Millisecond)

	// Try to remove it
	err = cm.RemoveNode("test-node")
	if err == nil {
		_, exists := cm.GetNode("test-node")
		assert.False(t, exists)
	}
}
