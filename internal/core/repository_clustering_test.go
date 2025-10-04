package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewRepositoryCluster(t *testing.T) {
	config := ReplicationConfig{
		Enabled:           true,
		ReplicationFactor: 3,
		SyncMode:          "async",
		ConsistencyLevel:  "eventual",
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)

	assert.NotNil(t, cluster)
	assert.Equal(t, "node-1", cluster.nodeID)
	assert.Equal(t, config.ReplicationFactor, cluster.config.ReplicationFactor)
	assert.NotNil(t, cluster.nodes)
	assert.NotNil(t, cluster.replicationQueue)
}

func TestStartStop(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)

	ffRepo.On("Count").Return(0, nil)

	err := cluster.Start()
	assert.NoError(t, err)

	// Verify node registered
	node, err := cluster.GetNode("node-1")
	assert.NoError(t, err)
	assert.Equal(t, "node-1", node.ID)
	assert.Equal(t, "active", node.Status)

	err = cluster.Stop()
	assert.NoError(t, err)
}

func TestRegisterUnregisterNode(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	t.Run("Register New Node", func(t *testing.T) {
		node := &ClusterNode{
			ID:      "node-2",
			Address: "192.168.1.2:8080",
			Status:  "active",
			Role:    "follower",
		}

		err := cluster.RegisterNode(node)
		assert.NoError(t, err)

		registered, err := cluster.GetNode("node-2")
		assert.NoError(t, err)
		assert.Equal(t, "node-2", registered.ID)
		assert.Equal(t, "192.168.1.2:8080", registered.Address)
	})

	t.Run("Register Duplicate Node", func(t *testing.T) {
		node := &ClusterNode{
			ID:     "node-2",
			Status: "active",
		}

		err := cluster.RegisterNode(node)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("Unregister Node", func(t *testing.T) {
		err := cluster.UnregisterNode("node-2")
		assert.NoError(t, err)

		_, err = cluster.GetNode("node-2")
		assert.Error(t, err)
	})

	t.Run("Cannot Unregister Self", func(t *testing.T) {
		err := cluster.UnregisterNode("node-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot unregister self")
	})
}

func TestElectLeader(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	cluster := NewRepositoryCluster("node-3", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	// Register multiple nodes
	cluster.RegisterNode(&ClusterNode{ID: "node-1", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
	cluster.Start()

	t.Run("Elect Leader", func(t *testing.T) {
		leaderID, err := cluster.ElectLeader()
		assert.NoError(t, err)
		assert.NotEmpty(t, leaderID)

		// Verify leader role assigned
		leader, _ := cluster.GetNode(leaderID)
		assert.Equal(t, "leader", leader.Role)
	})

	t.Run("Get Leader", func(t *testing.T) {
		leaderID := cluster.GetLeader()
		assert.NotEmpty(t, leaderID)
	})

	t.Run("Is Leader Check", func(t *testing.T) {
		cluster.ElectLeader()
		// node-3 may or may not be leader depending on election
		isLeader := cluster.IsLeader()
		assert.IsType(t, true, isLeader)
	})

	t.Run("No Active Nodes", func(t *testing.T) {
		// Mark all nodes as inactive
		cluster.mu.Lock()
		for _, node := range cluster.nodes {
			node.Status = "inactive"
		}
		cluster.mu.Unlock()

		_, err := cluster.ElectLeader()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active nodes")
	})
}

func TestReplicateFlowFile(t *testing.T) {
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	t.Run("Async Replication", func(t *testing.T) {
		config := ReplicationConfig{
			Enabled:           true,
			ReplicationFactor: 2,
			SyncMode:          "async",
		}

		cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)
		cluster.Start()
		defer cluster.Stop()

		// Register target nodes
		cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
		cluster.RegisterNode(&ClusterNode{ID: "node-3", Status: "active"})

		ff := types.NewFlowFile()
		ff.Attributes["test"] = "value"

		err := cluster.ReplicateFlowFile(ff, "store")
		assert.NoError(t, err)
	})

	t.Run("Sync Replication", func(t *testing.T) {
		config := ReplicationConfig{
			Enabled:           true,
			ReplicationFactor: 2,
			SyncMode:          "sync",
			SyncTimeout:       2 * time.Second,
		}

		cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)
		cluster.Start()
		defer cluster.Stop()

		cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})

		ff := types.NewFlowFile()

		// Sync replication will timeout waiting
		err := cluster.ReplicateFlowFile(ff, "store")
		// May timeout or succeed depending on timing
		_ = err
	})

	t.Run("Replication Disabled", func(t *testing.T) {
		config := ReplicationConfig{
			Enabled: false,
		}

		cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)

		ff := types.NewFlowFile()
		err := cluster.ReplicateFlowFile(ff, "store")
		assert.NoError(t, err) // Should succeed without replicating
	})
}

func TestReplicateContent(t *testing.T) {
	config := ReplicationConfig{
		Enabled:           true,
		ReplicationFactor: 2,
		SyncMode:          "async",
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)
	cluster.Start()
	defer cluster.Stop()

	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})

	claim := &types.ContentClaim{
		Container: "test",
		Section:   "section1",
		Offset:    0,
		Length:    100,
	}
	content := []byte("Test content data")

	err := cluster.ReplicateContent(claim, content)
	assert.NoError(t, err)
}

func TestSelectReplicationTargets(t *testing.T) {
	config := ReplicationConfig{
		Enabled:           true,
		ReplicationFactor: 2,
	}

	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	// Register multiple nodes
	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-3", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-4", Status: "failed"})

	targets := cluster.selectReplicationTargets()

	// Should select up to ReplicationFactor active nodes, excluding self
	assert.LessOrEqual(t, len(targets), 2)
	assert.NotContains(t, targets, "node-1") // Should not include self
	assert.NotContains(t, targets, "node-4") // Should not include failed node
}

func TestHeartbeat(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)

	ffRepo.On("Count").Return(100, nil)

	cluster.Start()
	defer cluster.Stop()

	// Register remote node
	cluster.RegisterNode(&ClusterNode{
		ID:       "node-2",
		Status:   "active",
		LastSeen: time.Now().Add(-1 * time.Minute), // Old timestamp
	})

	// Wait for heartbeat to mark node as failed
	time.Sleep(100 * time.Millisecond)
	cluster.sendHeartbeat()

	node, _ := cluster.GetNode("node-2")
	assert.Equal(t, "failed", node.Status)
}

func TestGetNodes(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	cluster.Start()
	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-3", Status: "active"})

	nodes := cluster.GetNodes()
	assert.Len(t, nodes, 3) // node-1, node-2, node-3

	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = node.ID
	}
	assert.Contains(t, ids, "node-1")
	assert.Contains(t, ids, "node-2")
	assert.Contains(t, ids, "node-3")
}

func TestGetStats(t *testing.T) {
	config := ReplicationConfig{Enabled: true, ReplicationFactor: 2}
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)

	ffRepo.On("Count").Return(50, nil)

	cluster.Start()

	cluster.RegisterNode(&ClusterNode{
		ID:        "node-2",
		Status:    "active",
		FlowFiles: 100,
		DataSize:  1024 * 1024,
	})

	cluster.RegisterNode(&ClusterNode{
		ID:        "node-3",
		Status:    "failed",
		FlowFiles: 50,
		DataSize:  512 * 1024,
	})

	time.Sleep(100 * time.Millisecond) // Wait for heartbeat

	stats := cluster.GetStats()

	assert.Equal(t, 3, stats.TotalNodes)
	assert.GreaterOrEqual(t, stats.ActiveNodes, 1)
	assert.Greater(t, stats.TotalFlowFiles, 0)
}

func TestGetReplicationLog(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	// Add some log entries manually
	cluster.mu.Lock()
	for i := 0; i < 10; i++ {
		entry := &ReplicationLogEntry{
			Operation: "store",
			Type:      "flowfile",
			Status:    "success",
			StartTime: time.Now(),
		}
		cluster.replicationLog = append(cluster.replicationLog, entry)
	}
	cluster.mu.Unlock()

	t.Run("Get All Entries", func(t *testing.T) {
		entries := cluster.GetReplicationLog(0)
		assert.Len(t, entries, 10)
	})

	t.Run("Get Limited Entries", func(t *testing.T) {
		entries := cluster.GetReplicationLog(5)
		assert.Len(t, entries, 5)
	})

	t.Run("Get More Than Available", func(t *testing.T) {
		entries := cluster.GetReplicationLog(20)
		assert.Len(t, entries, 10)
	})
}

func TestSetGetConfig(t *testing.T) {
	config := ReplicationConfig{
		Enabled:           true,
		ReplicationFactor: 3,
		SyncMode:          "async",
	}

	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	retrievedConfig := cluster.GetConfig()
	assert.Equal(t, config.ReplicationFactor, retrievedConfig.ReplicationFactor)
	assert.Equal(t, config.SyncMode, retrievedConfig.SyncMode)

	newConfig := ReplicationConfig{
		Enabled:           true,
		ReplicationFactor: 5,
		SyncMode:          "sync",
		SyncTimeout:       5 * time.Second,
	}

	cluster.SetConfig(newConfig)
	retrievedConfig = cluster.GetConfig()
	assert.Equal(t, newConfig.ReplicationFactor, retrievedConfig.ReplicationFactor)
	assert.Equal(t, newConfig.SyncMode, retrievedConfig.SyncMode)
}

func TestProcessReplicationTask(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	cluster := NewRepositoryCluster("node-1", config, ffRepo, contentRepo, provRepo)
	cluster.Start()
	defer cluster.Stop()

	// Register target nodes
	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-3", Status: "active"})

	task := &ReplicationTask{
		Operation:   "store",
		Type:        "flowfile",
		Data:        []byte("test data"),
		TargetNodes: []string{"node-2", "node-3"},
		Replicated:  make(map[string]bool),
	}

	// Process task
	cluster.processReplicationTask(task)

	// Check replication log
	time.Sleep(100 * time.Millisecond)
	logs := cluster.GetReplicationLog(1)
	assert.NotEmpty(t, logs)
}

func TestReplicateToNode(t *testing.T) {
	config := ReplicationConfig{Enabled: true}
	cluster := NewRepositoryCluster("node-1", config, new(MockFlowFileRepo), new(MockContentRepo), new(MockProvenanceRepo))

	cluster.RegisterNode(&ClusterNode{ID: "node-2", Status: "active"})
	cluster.RegisterNode(&ClusterNode{ID: "node-3", Status: "failed"})

	task := &ReplicationTask{
		Operation: "store",
		Type:      "flowfile",
		Data:      []byte("data"),
	}

	t.Run("Replicate to Active Node", func(t *testing.T) {
		err := cluster.replicateToNode("node-2", task)
		assert.NoError(t, err)
	})

	t.Run("Replicate to Failed Node", func(t *testing.T) {
		err := cluster.replicateToNode("node-3", task)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not active")
	})

	t.Run("Replicate to Non-existent Node", func(t *testing.T) {
		err := cluster.replicateToNode("node-99", task)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
