package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// HealthChecker monitors node health across the cluster
type HealthChecker struct {
	config       HealthCheckerConfig
	checks       map[string]*HealthCheck
	nodes        map[string]*ClusterNode
	logger       *logrus.Logger
	mu           sync.RWMutex
	failCallback func(nodeID string)
	running      bool
}

// HealthCheckerConfig holds health checker configuration
type HealthCheckerConfig struct {
	Interval      time.Duration // How often to check
	Timeout       time.Duration // Timeout for health checks
	FailThreshold int           // Consecutive failures before marking unhealthy
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(config HealthCheckerConfig, logger *logrus.Logger) *HealthChecker {
	// Set defaults
	if config.Interval == 0 {
		config.Interval = 5 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 3 * time.Second
	}
	if config.FailThreshold == 0 {
		config.FailThreshold = 3
	}

	return &HealthChecker{
		config:  config,
		checks:  make(map[string]*HealthCheck),
		nodes:   make(map[string]*ClusterNode),
		logger:  logger,
		running: false,
	}
}

// Start starts the health checker
func (hc *HealthChecker) Start(ctx context.Context, nodes map[string]*ClusterNode, failCallback func(string)) error {
	hc.mu.Lock()
	if hc.running {
		hc.mu.Unlock()
		return fmt.Errorf("health checker already running")
	}
	hc.running = true
	hc.nodes = nodes
	hc.failCallback = failCallback
	hc.mu.Unlock()

	hc.logger.Info("Starting health checker")

	// Initialize health checks for all nodes
	for nodeID, node := range nodes {
		hc.AddNode(node)
		hc.logger.WithField("nodeId", nodeID).Debug("Initialized health check")
	}

	// Start monitoring loop
	go hc.monitorLoop(ctx)

	return nil
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if !hc.running {
		return nil
	}

	hc.running = false
	hc.logger.Info("Health checker stopped")
	return nil
}

// AddNode adds a node to health monitoring
func (hc *HealthChecker) AddNode(node *ClusterNode) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.checks[node.ID]; exists {
		return
	}

	hc.checks[node.ID] = &HealthCheck{
		NodeID:           node.ID,
		LastCheck:        time.Now(),
		Latency:          0,
		ConsecutiveFails: 0,
		IsHealthy:        true,
	}

	hc.logger.WithField("nodeId", node.ID).Debug("Added node to health monitoring")
}

// RemoveNode removes a node from health monitoring
func (hc *HealthChecker) RemoveNode(nodeID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.checks, nodeID)
	hc.logger.WithField("nodeId", nodeID).Debug("Removed node from health monitoring")
}

// GetHealthCheck returns the health check for a node
func (hc *HealthChecker) GetHealthCheck(nodeID string) (*HealthCheck, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	check, exists := hc.checks[nodeID]
	return check, exists
}

// GetAllHealthChecks returns all health checks
func (hc *HealthChecker) GetAllHealthChecks() map[string]*HealthCheck {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	checks := make(map[string]*HealthCheck)
	for k, v := range hc.checks {
		checks[k] = v
	}
	return checks
}

// IsNodeHealthy checks if a node is healthy
func (hc *HealthChecker) IsNodeHealthy(nodeID string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	check, exists := hc.checks[nodeID]
	if !exists {
		return false
	}

	return check.IsHealthy
}

// GetHealthyNodeCount returns the count of healthy nodes
func (hc *HealthChecker) GetHealthyNodeCount() int {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	count := 0
	for _, check := range hc.checks {
		if check.IsHealthy {
			count++
		}
	}
	return count
}

// Private methods

func (hc *HealthChecker) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.performHealthChecks()
		}
	}
}

func (hc *HealthChecker) performHealthChecks() {
	hc.mu.RLock()
	nodes := make(map[string]*ClusterNode)
	for k, v := range hc.nodes {
		nodes[k] = v
	}
	hc.mu.RUnlock()

	for nodeID, node := range nodes {
		// Skip self (would be checked internally)
		// In production, would check actual node health via RPC/HTTP

		hc.checkNode(nodeID, node)
	}
}

func (hc *HealthChecker) checkNode(nodeID string, node *ClusterNode) {
	startTime := time.Now()

	// Perform health check
	isHealthy, latency := hc.executeHealthCheck(node)

	// Update health check
	hc.mu.Lock()
	check, exists := hc.checks[nodeID]
	if !exists {
		hc.mu.Unlock()
		return
	}

	check.LastCheck = time.Now()
	check.Latency = latency

	if !isHealthy {
		check.ConsecutiveFails++
		hc.logger.WithFields(logrus.Fields{
			"nodeId":           nodeID,
			"consecutiveFails": check.ConsecutiveFails,
			"threshold":        hc.config.FailThreshold,
		}).Warn("Health check failed")

		if check.ConsecutiveFails >= hc.config.FailThreshold && check.IsHealthy {
			// Mark as unhealthy
			check.IsHealthy = false
			hc.mu.Unlock()

			hc.logger.WithField("nodeId", nodeID).Error("Node marked as unhealthy")

			// Call failure callback
			if hc.failCallback != nil {
				hc.failCallback(nodeID)
			}
			return
		}
	} else {
		// Health check passed
		if check.ConsecutiveFails > 0 || !check.IsHealthy {
			hc.logger.WithFields(logrus.Fields{
				"nodeId":  nodeID,
				"latency": latency,
			}).Info("Node recovered")
		}

		check.ConsecutiveFails = 0
		check.IsHealthy = true
	}

	hc.mu.Unlock()

	// Log detailed check info at debug level
	hc.logger.WithFields(logrus.Fields{
		"nodeId":   nodeID,
		"healthy":  isHealthy,
		"latency":  latency,
		"duration": time.Since(startTime),
	}).Debug("Health check completed")
}

func (hc *HealthChecker) executeHealthCheck(node *ClusterNode) (bool, time.Duration) {
	startTime := time.Now()

	// In production, would perform actual health check:
	// - TCP connection test
	// - HTTP/gRPC health endpoint
	// - Ping/heartbeat
	//
	// For now, simulate based on last heartbeat time
	timeSinceHeartbeat := time.Since(node.LastHeartbeat)

	// Consider healthy if heartbeat is recent
	maxHeartbeatAge := hc.config.Interval * 3
	isHealthy := timeSinceHeartbeat < maxHeartbeatAge

	latency := time.Since(startTime)

	// Simulate some latency variance
	if !isHealthy {
		latency = hc.config.Timeout
	}

	return isHealthy, latency
}

// ForceHealthCheck immediately performs a health check for a specific node
func (hc *HealthChecker) ForceHealthCheck(nodeID string) error {
	hc.mu.RLock()
	node, exists := hc.nodes[nodeID]
	hc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	hc.checkNode(nodeID, node)
	return nil
}

// UpdateNodeHeartbeat updates the last heartbeat time for a node
func (hc *HealthChecker) UpdateNodeHeartbeat(nodeID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if node, exists := hc.nodes[nodeID]; exists {
		node.UpdateHeartbeat()
	}
}

// GetStatistics returns health checker statistics
func (hc *HealthChecker) GetStatistics() *HealthStatistics {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	stats := &HealthStatistics{
		TotalNodes:     len(hc.checks),
		HealthyNodes:   0,
		UnhealthyNodes: 0,
		Checks:         make(map[string]*HealthCheckStatus),
	}

	var totalLatency time.Duration
	for nodeID, check := range hc.checks {
		if check.IsHealthy {
			stats.HealthyNodes++
		} else {
			stats.UnhealthyNodes++
		}

		totalLatency += check.Latency

		stats.Checks[nodeID] = &HealthCheckStatus{
			NodeID:           check.NodeID,
			IsHealthy:        check.IsHealthy,
			LastCheck:        check.LastCheck,
			Latency:          check.Latency,
			ConsecutiveFails: check.ConsecutiveFails,
		}
	}

	if len(hc.checks) > 0 {
		stats.AverageLatency = totalLatency / time.Duration(len(hc.checks))
	}

	return stats
}

// HealthStatistics holds health checker statistics
type HealthStatistics struct {
	TotalNodes     int                           `json:"totalNodes"`
	HealthyNodes   int                           `json:"healthyNodes"`
	UnhealthyNodes int                           `json:"unhealthyNodes"`
	AverageLatency time.Duration                 `json:"averageLatency"`
	Checks         map[string]*HealthCheckStatus `json:"checks"`
}

// HealthCheckStatus represents the status of a health check
type HealthCheckStatus struct {
	NodeID           string        `json:"nodeId"`
	IsHealthy        bool          `json:"isHealthy"`
	LastCheck        time.Time     `json:"lastCheck"`
	Latency          time.Duration `json:"latency"`
	ConsecutiveFails int           `json:"consecutiveFails"`
}

// SetFailThreshold updates the failure threshold
func (hc *HealthChecker) SetFailThreshold(threshold int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.config.FailThreshold = threshold
	hc.logger.WithField("threshold", threshold).Info("Updated failure threshold")
}

// SetCheckInterval updates the check interval
func (hc *HealthChecker) SetCheckInterval(interval time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.config.Interval = interval
	hc.logger.WithField("interval", interval).Info("Updated check interval")
}

// MarkNodeHealthy manually marks a node as healthy
func (hc *HealthChecker) MarkNodeHealthy(nodeID string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	check, exists := hc.checks[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	check.IsHealthy = true
	check.ConsecutiveFails = 0
	hc.logger.WithField("nodeId", nodeID).Info("Manually marked node as healthy")

	return nil
}

// MarkNodeUnhealthy manually marks a node as unhealthy
func (hc *HealthChecker) MarkNodeUnhealthy(nodeID string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	check, exists := hc.checks[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	wasHealthy := check.IsHealthy
	check.IsHealthy = false
	check.ConsecutiveFails = hc.config.FailThreshold

	hc.mu.Unlock()

	hc.logger.WithField("nodeId", nodeID).Warn("Manually marked node as unhealthy")

	// Call failure callback if node was previously healthy
	if wasHealthy && hc.failCallback != nil {
		hc.failCallback(nodeID)
	}

	hc.mu.Lock()
	return nil
}
