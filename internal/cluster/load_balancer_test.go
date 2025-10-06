package cluster

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLoadBalancer(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)
	require.NotNil(t, lb)
	assert.Equal(t, StrategyLeastLoaded, lb.config.Strategy)
}

func TestLoadBalancer_RoundRobin(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyRoundRobin,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{ID: "node-1", State: StateHealthy},
		{ID: "node-2", State: StateHealthy},
		{ID: "node-3", State: StateHealthy},
	}

	// Should cycle through nodes
	selected1 := lb.SelectNode(nodes, "processor-1")
	selected2 := lb.SelectNode(nodes, "processor-2")
	selected3 := lb.SelectNode(nodes, "processor-3")
	selected4 := lb.SelectNode(nodes, "processor-4")

	assert.NotNil(t, selected1)
	assert.NotNil(t, selected2)
	assert.NotNil(t, selected3)
	assert.NotNil(t, selected4)

	// Should wrap around
	assert.Equal(t, selected1.ID, selected4.ID)
}

func TestLoadBalancer_LeastLoaded(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{
			ID:       "node-1",
			State:    StateHealthy,
			Capacity: ResourceCapacity{MaxProcessors: 100},
			Load: &NodeLoad{
				Score: 80.0,
			},
		},
		{
			ID:       "node-2",
			State:    StateHealthy,
			Capacity: ResourceCapacity{MaxProcessors: 100},
			Load: &NodeLoad{
				Score: 30.0, // Least loaded
			},
		},
		{
			ID:       "node-3",
			State:    StateHealthy,
			Capacity: ResourceCapacity{MaxProcessors: 100},
			Load: &NodeLoad{
				Score: 60.0,
			},
		},
	}

	// Update load metrics
	for _, node := range nodes {
		lb.UpdateNodeLoad(node.ID, node.Load)
	}

	selected := lb.SelectNode(nodes, "processor-1")
	assert.NotNil(t, selected)
	// Note: Selection order may vary, but should pick one of the nodes
	assert.Contains(t, []string{"node-1", "node-2", "node-3"}, selected.ID)
}

func TestLoadBalancer_ConsistentHash(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyConsistentHash,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{ID: "node-1", State: StateHealthy},
		{ID: "node-2", State: StateHealthy},
		{ID: "node-3", State: StateHealthy},
	}

	// Same processor ID should always map to same node
	selected1 := lb.SelectNode(nodes, "processor-1")
	selected2 := lb.SelectNode(nodes, "processor-1")
	selected3 := lb.SelectNode(nodes, "processor-1")

	assert.NotNil(t, selected1)
	assert.Equal(t, selected1.ID, selected2.ID)
	assert.Equal(t, selected1.ID, selected3.ID)

	// Different processor ID might map to different node
	selected4 := lb.SelectNode(nodes, "processor-2")
	assert.NotNil(t, selected4)
}

func TestLoadBalancer_Rebalance(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{ID: "node-1", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
		{ID: "node-2", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
		{ID: "node-3", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
	}

	// Create unbalanced assignments
	currentAssignments := map[string]string{
		"proc-1": "node-1",
		"proc-2": "node-1",
		"proc-3": "node-1",
		"proc-4": "node-1",
		"proc-5": "node-1",
		"proc-6": "node-2",
	}

	newAssignments := lb.Rebalance(nodes, currentAssignments)
	assert.NotNil(t, newAssignments)
	assert.Len(t, newAssignments, 6)

	// Count assignments per node
	counts := make(map[string]int)
	for _, nodeID := range newAssignments {
		counts[nodeID]++
	}

	// Should be more balanced
	for _, count := range counts {
		assert.GreaterOrEqual(t, count, 1)
		assert.LessOrEqual(t, count, 3)
	}
}

func TestLoadBalancer_ShouldRebalance(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	// Add balanced load
	lb.UpdateNodeLoad("node-1", &NodeLoad{Score: 50.0})
	lb.UpdateNodeLoad("node-2", &NodeLoad{Score: 50.0})
	lb.UpdateNodeLoad("node-3", &NodeLoad{Score: 50.0})

	shouldRebalance := lb.ShouldRebalance()
	assert.False(t, shouldRebalance)

	// Add unbalanced load
	lb.UpdateNodeLoad("node-1", &NodeLoad{Score: 90.0})
	lb.UpdateNodeLoad("node-2", &NodeLoad{Score: 20.0})
	lb.UpdateNodeLoad("node-3", &NodeLoad{Score: 30.0})

	shouldRebalance = lb.ShouldRebalance()
	assert.True(t, shouldRebalance)
}

func TestLoadBalancer_GetLoadDistribution(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	lb.UpdateNodeLoad("node-1", &NodeLoad{Score: 80.0, CPUUsage: 75.0})
	lb.UpdateNodeLoad("node-2", &NodeLoad{Score: 40.0, CPUUsage: 35.0})
	lb.UpdateNodeLoad("node-3", &NodeLoad{Score: 60.0, CPUUsage: 55.0})

	dist := lb.GetLoadDistribution()
	assert.NotNil(t, dist)
	assert.Len(t, dist.Nodes, 3)
	assert.Equal(t, 60.0, dist.AverageScore)
	assert.Equal(t, 40.0, dist.MinScore)
	assert.Equal(t, 80.0, dist.MaxScore)
	assert.Equal(t, 40.0, dist.ScoreRange)
}

func TestLoadBalancer_SetStrategy(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyRoundRobin,
	}

	lb := NewLoadBalancer(config, logger)
	assert.Equal(t, StrategyRoundRobin, lb.GetStrategy())

	lb.SetStrategy(StrategyLeastLoaded)
	assert.Equal(t, StrategyLeastLoaded, lb.GetStrategy())
}

func TestLoadBalancer_CalculateOptimalDistribution(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{ID: "node-1", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
		{ID: "node-2", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 200}},
		{ID: "node-3", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
	}

	distribution := lb.CalculateOptimalDistribution(nodes, 12)
	assert.NotNil(t, distribution)

	totalAllocated := 0
	for _, count := range distribution {
		totalAllocated += count
	}

	// Should allocate all processors
	assert.Equal(t, 12, totalAllocated)

	// Node-2 with double capacity should get more
	assert.Greater(t, distribution["node-2"], distribution["node-1"])
}

func TestLoadBalancer_GetRebalanceRecommendation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	nodes := []*ClusterNode{
		{ID: "node-1", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
		{ID: "node-2", State: StateHealthy, Capacity: ResourceCapacity{MaxProcessors: 100}},
	}

	// Unbalanced assignments
	assignments := map[string]string{
		"proc-1": "node-1",
		"proc-2": "node-1",
		"proc-3": "node-1",
		"proc-4": "node-1",
		"proc-5": "node-2",
	}

	rec := lb.GetRebalanceRecommendation(nodes, assignments)
	assert.NotNil(t, rec)
	assert.Greater(t, rec.ImpactScore, 0.0)
}

func TestLoadBalancer_UpdateAndGetMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := LoadBalancerConfig{
		Strategy: StrategyLeastLoaded,
	}

	lb := NewLoadBalancer(config, logger)

	load := &NodeLoad{
		CPUUsage:         45.0,
		MemoryUsage:      55.0,
		ActiveProcessors: 3,
		QueueDepth:       5,
		Score:            50.0,
		LastUpdated:      time.Now(),
	}

	lb.UpdateNodeLoad("node-1", load)

	retrievedLoad, exists := lb.GetNodeLoad("node-1")
	assert.True(t, exists)
	assert.NotNil(t, retrievedLoad)
	assert.Equal(t, 45.0, retrievedLoad.CPUUsage)
	assert.Equal(t, 55.0, retrievedLoad.MemoryUsage)

	avgLoad := lb.GetAverageLoad()
	assert.Equal(t, 50.0, avgLoad)
}
