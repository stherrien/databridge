package cluster

import (
	"crypto/rand"
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// LoadBalancer distributes work across cluster nodes
type LoadBalancer struct {
	config          LoadBalancerConfig
	metrics         *LoadMetrics
	roundRobinIndex int
	logger          *logrus.Logger
	mu              sync.RWMutex
}

// LoadBalancerConfig holds load balancer configuration
type LoadBalancerConfig struct {
	Strategy        LoadBalancingStrategy
	RebalanceWindow time.Duration // Minimum time between rebalances
	AffinityWeight  float64       // Weight for keeping processors on same node (0-1)
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config LoadBalancerConfig, logger *logrus.Logger) *LoadBalancer {
	// Set defaults
	if config.RebalanceWindow == 0 {
		config.RebalanceWindow = 30 * time.Second
	}
	if config.AffinityWeight == 0 {
		config.AffinityWeight = 0.3 // 30% preference for staying on same node
	}

	return &LoadBalancer{
		config:          config,
		metrics:         NewLoadMetrics(),
		roundRobinIndex: 0,
		logger:          logger,
	}
}

// SelectNode selects a node for a new processor using the configured strategy
func (lb *LoadBalancer) SelectNode(nodes []*ClusterNode, processorID string) *ClusterNode {
	if len(nodes) == 0 {
		return nil
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	switch lb.config.Strategy {
	case StrategyRoundRobin:
		return lb.selectRoundRobin(nodes)
	case StrategyLeastLoaded:
		return lb.selectLeastLoaded(nodes)
	case StrategyWeightedRandom:
		return lb.selectWeightedRandom(nodes)
	case StrategyConsistentHash:
		return lb.selectConsistentHash(nodes, processorID)
	default:
		return lb.selectLeastLoaded(nodes)
	}
}

// Rebalance redistributes work across nodes
func (lb *LoadBalancer) Rebalance(nodes []*ClusterNode, currentAssignments map[string]string) map[string]string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(nodes) == 0 {
		return currentAssignments
	}

	lb.logger.WithFields(logrus.Fields{
		"nodes":       len(nodes),
		"assignments": len(currentAssignments),
	}).Info("Starting load rebalancing")

	newAssignments := make(map[string]string)

	// Calculate target load per node
	totalProcessors := len(currentAssignments)
	targetPerNode := totalProcessors / len(nodes)
	if targetPerNode == 0 {
		targetPerNode = 1
	}

	// Calculate current load per node
	nodeLoads := make(map[string]int)
	for _, nodeID := range currentAssignments {
		nodeLoads[nodeID]++
	}

	// Create sorted list of nodes by current load
	type nodeLoadPair struct {
		nodeID string
		load   int
	}
	sortedNodes := make([]nodeLoadPair, 0, len(nodes))
	for _, node := range nodes {
		load := nodeLoads[node.ID]
		sortedNodes = append(sortedNodes, nodeLoadPair{node.ID, load})
	}
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].load < sortedNodes[j].load
	})

	// Redistribute processors
	nodeIndex := 0
	for processorID, currentNodeID := range currentAssignments {
		// Check if current node is healthy
		currentNodeHealthy := false
		for _, node := range nodes {
			if node.ID == currentNodeID && node.IsHealthy() {
				currentNodeHealthy = true
				break
			}
		}

		// If current node is unhealthy or overloaded, reassign
		if !currentNodeHealthy || nodeLoads[currentNodeID] > targetPerNode+1 {
			// Find least loaded node
			targetNode := sortedNodes[nodeIndex%len(sortedNodes)].nodeID
			newAssignments[processorID] = targetNode
			nodeLoads[targetNode]++
			nodeLoads[currentNodeID]--
			nodeIndex++

			lb.logger.WithFields(logrus.Fields{
				"processorId": processorID,
				"from":        currentNodeID,
				"to":          targetNode,
			}).Debug("Reassigned processor")
		} else {
			// Keep on current node (affinity)
			newAssignments[processorID] = currentNodeID
		}
	}

	lb.logger.WithField("reassignments", len(newAssignments)).Info("Load rebalancing completed")
	return newAssignments
}

// UpdateNodeLoad updates the load metrics for a node
func (lb *LoadBalancer) UpdateNodeLoad(nodeID string, load *NodeLoad) {
	lb.metrics.UpdateNodeLoad(nodeID, load)
}

// GetNodeLoad retrieves the load for a node
func (lb *LoadBalancer) GetNodeLoad(nodeID string) (*NodeLoad, bool) {
	return lb.metrics.GetNodeLoad(nodeID)
}

// GetAverageLoad returns the average load across all nodes
func (lb *LoadBalancer) GetAverageLoad() float64 {
	return lb.metrics.GetAverageLoad()
}

// GetMetrics returns the load metrics
func (lb *LoadBalancer) GetMetrics() *LoadMetrics {
	return lb.metrics
}

// Private selection strategies

func (lb *LoadBalancer) selectRoundRobin(nodes []*ClusterNode) *ClusterNode {
	index := lb.roundRobinIndex % len(nodes)
	lb.roundRobinIndex++
	return nodes[index]
}

func (lb *LoadBalancer) selectLeastLoaded(nodes []*ClusterNode) *ClusterNode {
	var selected *ClusterNode
	lowestScore := math.MaxFloat64

	for _, node := range nodes {
		score := lb.calculateNodeScore(node)
		if score < lowestScore {
			lowestScore = score
			selected = node
		}
	}

	return selected
}

func (lb *LoadBalancer) selectWeightedRandom(nodes []*ClusterNode) *ClusterNode {
	// Calculate weights (inverse of load score)
	weights := make([]float64, len(nodes))
	totalWeight := 0.0

	for i, node := range nodes {
		score := lb.calculateNodeScore(node)
		// Inverse weight: lower score = higher weight
		weight := 100.0 - score
		if weight < 1.0 {
			weight = 1.0
		}
		weights[i] = weight
		totalWeight += weight
	}

	// Select randomly based on weights using crypto/rand
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback to round-robin on error
		lb.logger.WithError(err).Warn("Failed to generate random number, falling back to round-robin")
		return lb.selectRoundRobin(nodes)
	}

	// Convert bytes to float64 in range [0, 1)
	randUint64 := binary.BigEndian.Uint64(buf[:])
	r := float64(randUint64) / float64(^uint64(0)) * totalWeight
	cumulative := 0.0

	for i, weight := range weights {
		cumulative += weight
		if r <= cumulative {
			return nodes[i]
		}
	}

	return nodes[len(nodes)-1]
}

func (lb *LoadBalancer) selectConsistentHash(nodes []*ClusterNode, processorID string) *ClusterNode {
	// Use consistent hashing to distribute processors
	hash := lb.hashString(processorID)

	// Find node with closest hash
	var selected *ClusterNode
	minDistance := uint64(math.MaxUint64)

	for _, node := range nodes {
		nodeHash := lb.hashString(node.ID)
		distance := lb.hashDistance(hash, nodeHash)

		if distance < minDistance {
			minDistance = distance
			selected = node
		}
	}

	return selected
}

func (lb *LoadBalancer) calculateNodeScore(node *ClusterNode) float64 {
	// Get current load
	load, exists := lb.metrics.GetNodeLoad(node.ID)
	if !exists || load == nil {
		// No load data, assume average
		return 50.0
	}

	// Composite score based on multiple factors
	cpuWeight := 0.4
	memoryWeight := 0.3
	processorWeight := 0.2
	queueWeight := 0.1

	score := (load.CPUUsage * cpuWeight) +
		(load.MemoryUsage * memoryWeight) +
		(float64(load.ActiveProcessors) / float64(node.Capacity.MaxProcessors) * 100.0 * processorWeight) +
		(float64(load.QueueDepth) / 1000.0 * 100.0 * queueWeight)

	return score
}

func (lb *LoadBalancer) hashString(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func (lb *LoadBalancer) hashDistance(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

// ShouldRebalance checks if rebalancing is needed
func (lb *LoadBalancer) ShouldRebalance() bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Check if load variance is high
	avgLoad := lb.metrics.GetAverageLoad()
	if avgLoad == 0 {
		return false
	}

	// Calculate standard deviation
	variance := 0.0
	count := 0

	lb.metrics.mu.RLock()
	for _, load := range lb.metrics.NodeLoads {
		diff := load.Score - avgLoad
		variance += diff * diff
		count++
	}
	lb.metrics.mu.RUnlock()

	if count == 0 {
		return false
	}

	stdDev := math.Sqrt(variance / float64(count))

	// Rebalance if standard deviation is > 20% of average
	threshold := avgLoad * 0.2
	shouldRebalance := stdDev > threshold

	if shouldRebalance {
		lb.logger.WithFields(logrus.Fields{
			"avgLoad":   avgLoad,
			"stdDev":    stdDev,
			"threshold": threshold,
		}).Info("Load imbalance detected")
	}

	return shouldRebalance
}

// GetNodesByLoad returns nodes sorted by load (least to most)
func (lb *LoadBalancer) GetNodesByLoad(nodes []*ClusterNode) []*ClusterNode {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Create a copy to sort
	sorted := make([]*ClusterNode, len(nodes))
	copy(sorted, nodes)

	// Sort by load score
	sort.Slice(sorted, func(i, j int) bool {
		scoreI := lb.calculateNodeScore(sorted[i])
		scoreJ := lb.calculateNodeScore(sorted[j])
		return scoreI < scoreJ
	})

	return sorted
}

// GetLoadDistribution returns load distribution statistics
func (lb *LoadBalancer) GetLoadDistribution() *LoadDistribution {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	dist := &LoadDistribution{
		Nodes: make(map[string]*NodeLoadInfo),
	}

	lb.metrics.mu.RLock()
	defer lb.metrics.mu.RUnlock()

	var totalScore float64
	var minScore = math.MaxFloat64
	var maxScore = 0.0

	for nodeID, load := range lb.metrics.NodeLoads {
		info := &NodeLoadInfo{
			NodeID:           nodeID,
			Score:            load.Score,
			CPUUsage:         load.CPUUsage,
			MemoryUsage:      load.MemoryUsage,
			ActiveProcessors: load.ActiveProcessors,
			QueueDepth:       load.QueueDepth,
		}

		dist.Nodes[nodeID] = info
		totalScore += load.Score

		if load.Score < minScore {
			minScore = load.Score
		}
		if load.Score > maxScore {
			maxScore = load.Score
		}
	}

	if len(lb.metrics.NodeLoads) > 0 {
		dist.AverageScore = totalScore / float64(len(lb.metrics.NodeLoads))
		dist.MinScore = minScore
		dist.MaxScore = maxScore
		dist.ScoreRange = maxScore - minScore
	}

	return dist
}

// LoadDistribution represents load distribution across nodes
type LoadDistribution struct {
	Nodes        map[string]*NodeLoadInfo `json:"nodes"`
	AverageScore float64                  `json:"averageScore"`
	MinScore     float64                  `json:"minScore"`
	MaxScore     float64                  `json:"maxScore"`
	ScoreRange   float64                  `json:"scoreRange"`
}

// NodeLoadInfo contains load information for a single node
type NodeLoadInfo struct {
	NodeID           string  `json:"nodeId"`
	Score            float64 `json:"score"`
	CPUUsage         float64 `json:"cpuUsage"`
	MemoryUsage      float64 `json:"memoryUsage"`
	ActiveProcessors int     `json:"activeProcessors"`
	QueueDepth       int     `json:"queueDepth"`
}

// SetStrategy changes the load balancing strategy
func (lb *LoadBalancer) SetStrategy(strategy LoadBalancingStrategy) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.config.Strategy = strategy
	lb.logger.WithField("strategy", strategy).Info("Changed load balancing strategy")
}

// GetStrategy returns the current strategy
func (lb *LoadBalancer) GetStrategy() LoadBalancingStrategy {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.config.Strategy
}

// CalculateOptimalDistribution calculates the optimal distribution of processors
func (lb *LoadBalancer) CalculateOptimalDistribution(nodes []*ClusterNode, processorCount int) map[string]int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	distribution := make(map[string]int)

	if len(nodes) == 0 || processorCount == 0 {
		return distribution
	}

	// Calculate capacity-weighted distribution
	totalCapacity := 0
	for _, node := range nodes {
		if node.IsHealthy() {
			totalCapacity += node.Capacity.MaxProcessors
		}
	}

	if totalCapacity == 0 {
		// Equal distribution if no capacity info
		perNode := processorCount / len(nodes)
		remainder := processorCount % len(nodes)

		for i, node := range nodes {
			distribution[node.ID] = perNode
			if i < remainder {
				distribution[node.ID]++
			}
		}
	} else {
		// Capacity-weighted distribution
		allocated := 0
		for _, node := range nodes {
			if !node.IsHealthy() {
				continue
			}

			weight := float64(node.Capacity.MaxProcessors) / float64(totalCapacity)
			count := int(float64(processorCount) * weight)
			distribution[node.ID] = count
			allocated += count
		}

		// Allocate remaining processors to least loaded nodes
		remaining := processorCount - allocated
		sortedNodes := lb.GetNodesByLoad(nodes)
		for i := 0; i < remaining && i < len(sortedNodes); i++ {
			distribution[sortedNodes[i].ID]++
		}
	}

	return distribution
}

// GetRebalanceRecommendation provides recommendations for rebalancing
func (lb *LoadBalancer) GetRebalanceRecommendation(nodes []*ClusterNode, assignments map[string]string) *RebalanceRecommendation {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	rec := &RebalanceRecommendation{
		ShouldRebalance: lb.ShouldRebalance(),
		CurrentLoad:     lb.metrics.GetAverageLoad(),
		Timestamp:       time.Now(),
	}

	// Calculate current distribution
	currentDist := make(map[string]int)
	for _, nodeID := range assignments {
		currentDist[nodeID]++
	}

	// Calculate optimal distribution
	optimalDist := lb.CalculateOptimalDistribution(nodes, len(assignments))

	// Calculate moves needed
	rec.Moves = make(map[string]int)
	for nodeID, optimal := range optimalDist {
		current := currentDist[nodeID]
		if current != optimal {
			rec.Moves[nodeID] = optimal - current
		}
	}

	// Calculate impact score
	rec.ImpactScore = lb.calculateRebalanceImpact(currentDist, optimalDist)

	return rec
}

// RebalanceRecommendation provides rebalancing recommendations
type RebalanceRecommendation struct {
	ShouldRebalance bool           `json:"shouldRebalance"`
	CurrentLoad     float64        `json:"currentLoad"`
	Moves           map[string]int `json:"moves"` // nodeID -> change in processor count
	ImpactScore     float64        `json:"impactScore"`
	Timestamp       time.Time      `json:"timestamp"`
}

func (lb *LoadBalancer) calculateRebalanceImpact(current, optimal map[string]int) float64 {
	// Calculate total moves needed
	totalMoves := 0
	for nodeID, optimalCount := range optimal {
		currentCount := current[nodeID]
		diff := optimalCount - currentCount
		if diff < 0 {
			diff = -diff
		}
		totalMoves += diff
	}

	// Impact score: percentage of processors that need to move
	totalProcessors := 0
	for _, count := range current {
		totalProcessors += count
	}

	if totalProcessors == 0 {
		return 0
	}

	return (float64(totalMoves) / float64(totalProcessors)) * 100.0
}
