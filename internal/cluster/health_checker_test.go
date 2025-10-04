package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      5 * time.Second,
		Timeout:       3 * time.Second,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)
	require.NotNil(t, hc)
	assert.Equal(t, 5*time.Second, hc.config.Interval)
	assert.Equal(t, 3*time.Second, hc.config.Timeout)
	assert.Equal(t, 3, hc.config.FailThreshold)
}

func TestHealthChecker_AddRemoveNode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node := &ClusterNode{
		ID:            "test-node",
		Address:       "127.0.0.1",
		Port:          8090,
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
	}

	// Add node
	hc.AddNode(node)

	check, exists := hc.GetHealthCheck("test-node")
	assert.True(t, exists)
	assert.NotNil(t, check)
	assert.True(t, check.IsHealthy)

	// Remove node
	hc.RemoveNode("test-node")

	_, exists = hc.GetHealthCheck("test-node")
	assert.False(t, exists)
}

func TestHealthChecker_StartStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	nodes := map[string]*ClusterNode{
		"node-1": {
			ID:            "node-1",
			State:         StateHealthy,
			LastHeartbeat: time.Now(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	failCount := 0
	failCallback := func(nodeID string) {
		failCount++
	}

	err := hc.Start(ctx, nodes, failCallback)
	require.NoError(t, err)

	// Test double start
	err = hc.Start(ctx, nodes, failCallback)
	assert.Error(t, err)

	err = hc.Stop()
	require.NoError(t, err)

	// Test double stop
	err = hc.Stop()
	require.NoError(t, err)
}

func TestHealthChecker_IsNodeHealthy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node := &ClusterNode{
		ID:            "test-node",
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
	}

	hc.AddNode(node)

	isHealthy := hc.IsNodeHealthy("test-node")
	assert.True(t, isHealthy)

	// Non-existent node
	isHealthy = hc.IsNodeHealthy("non-existent")
	assert.False(t, isHealthy)
}

func TestHealthChecker_GetStatistics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node1 := &ClusterNode{ID: "node-1", State: StateHealthy, LastHeartbeat: time.Now()}
	node2 := &ClusterNode{ID: "node-2", State: StateHealthy, LastHeartbeat: time.Now()}

	hc.AddNode(node1)
	hc.AddNode(node2)

	stats := hc.GetStatistics()
	assert.NotNil(t, stats)
	assert.Equal(t, 2, stats.TotalNodes)
	assert.Equal(t, 2, stats.HealthyNodes)
	assert.Equal(t, 0, stats.UnhealthyNodes)
}

func TestHealthChecker_MarkNodeUnhealthy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node := &ClusterNode{
		ID:            "test-node",
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
	}

	hc.AddNode(node)

	// Mark as unhealthy
	err := hc.MarkNodeUnhealthy("test-node")
	require.NoError(t, err)

	isHealthy := hc.IsNodeHealthy("test-node")
	assert.False(t, isHealthy)

	stats := hc.GetStatistics()
	assert.Equal(t, 0, stats.HealthyNodes)
	assert.Equal(t, 1, stats.UnhealthyNodes)
}

func TestHealthChecker_MarkNodeHealthy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node := &ClusterNode{
		ID:            "test-node",
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
	}

	hc.AddNode(node)

	// Mark as unhealthy first
	hc.MarkNodeUnhealthy("test-node")
	assert.False(t, hc.IsNodeHealthy("test-node"))

	// Mark as healthy
	err := hc.MarkNodeHealthy("test-node")
	require.NoError(t, err)

	isHealthy := hc.IsNodeHealthy("test-node")
	assert.True(t, isHealthy)
}

func TestHealthChecker_GetHealthyNodeCount(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	node1 := &ClusterNode{ID: "node-1", State: StateHealthy, LastHeartbeat: time.Now()}
	node2 := &ClusterNode{ID: "node-2", State: StateHealthy, LastHeartbeat: time.Now()}
	node3 := &ClusterNode{ID: "node-3", State: StateHealthy, LastHeartbeat: time.Now()}

	hc.AddNode(node1)
	hc.AddNode(node2)
	hc.AddNode(node3)

	assert.Equal(t, 3, hc.GetHealthyNodeCount())

	hc.MarkNodeUnhealthy("node-2")
	assert.Equal(t, 2, hc.GetHealthyNodeCount())
}

func TestHealthChecker_UpdateNodeHeartbeat(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	initialTime := time.Now().Add(-10 * time.Second)
	node := &ClusterNode{
		ID:            "test-node",
		State:         StateHealthy,
		LastHeartbeat: initialTime,
	}

	nodes := map[string]*ClusterNode{
		"test-node": node,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc.Start(ctx, nodes, nil)
	defer hc.Stop()

	// Update heartbeat
	hc.UpdateNodeHeartbeat("test-node")

	// Heartbeat should be more recent
	assert.True(t, node.LastHeartbeat.After(initialTime))
}

func TestHealthChecker_SetFailThreshold(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	assert.Equal(t, 3, hc.config.FailThreshold)

	hc.SetFailThreshold(5)
	assert.Equal(t, 5, hc.config.FailThreshold)
}

func TestHealthChecker_SetCheckInterval(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := HealthCheckerConfig{
		Interval:      1 * time.Second,
		Timeout:       500 * time.Millisecond,
		FailThreshold: 3,
	}

	hc := NewHealthChecker(config, logger)

	assert.Equal(t, 1*time.Second, hc.config.Interval)

	hc.SetCheckInterval(2 * time.Second)
	assert.Equal(t, 2*time.Second, hc.config.Interval)
}
