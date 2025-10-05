package cluster

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Discovery handles node discovery in the cluster
type Discovery struct {
	config          DiscoveryConfig
	discoveredNodes map[string]*ClusterNode
	logger          *logrus.Logger
	mu              sync.RWMutex
	running         bool
	callback        func(*ClusterNode)
}

// DiscoveryConfig holds discovery configuration
type DiscoveryConfig struct {
	Method   DiscoveryMethod
	Seeds    []string
	Port     int
	Interval time.Duration
}

// NewDiscovery creates a new discovery instance
func NewDiscovery(config DiscoveryConfig, logger *logrus.Logger) (*Discovery, error) {
	// Set defaults
	if config.Interval == 0 {
		config.Interval = 10 * time.Second
	}
	if config.Port == 0 {
		config.Port = 8090
	}

	return &Discovery{
		config:          config,
		discoveredNodes: make(map[string]*ClusterNode),
		logger:          logger,
		running:         false,
	}, nil
}

// Start starts the discovery process
func (d *Discovery) Start(ctx context.Context, callback func(*ClusterNode)) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("discovery already running")
	}
	d.running = true
	d.callback = callback
	d.mu.Unlock()

	d.logger.WithField("method", d.config.Method).Info("Starting node discovery")

	// Perform initial discovery
	if err := d.discover(); err != nil {
		d.logger.WithError(err).Warn("Initial discovery failed")
	}

	// Start periodic discovery
	go d.periodicDiscovery(ctx)

	return nil
}

// Stop stops the discovery process
func (d *Discovery) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.running = false
	d.logger.Info("Node discovery stopped")
	return nil
}

// GetDiscoveredNodes returns all discovered nodes
func (d *Discovery) GetDiscoveredNodes() []*ClusterNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*ClusterNode, 0, len(d.discoveredNodes))
	for _, node := range d.discoveredNodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// Private methods

func (d *Discovery) periodicDiscovery(ctx context.Context) {
	ticker := time.NewTicker(d.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.discover(); err != nil {
				d.logger.WithError(err).Debug("Periodic discovery failed")
			}
		}
	}
}

func (d *Discovery) discover() error {
	switch d.config.Method {
	case DiscoveryStatic:
		return d.discoverStatic()
	case DiscoveryMulticast:
		return d.discoverMulticast()
	case DiscoveryDNS:
		return d.discoverDNS()
	case DiscoveryEtcd:
		return d.discoverEtcd()
	default:
		return d.discoverStatic()
	}
}

func (d *Discovery) discoverStatic() error {
	d.logger.Debug("Performing static discovery")

	// Parse seed nodes and create cluster nodes
	for _, seed := range d.config.Seeds {
		node, err := d.parseSeeded(seed)
		if err != nil {
			d.logger.WithError(err).WithField("seed", seed).Warn("Failed to parse seed node")
			continue
		}

		d.registerNode(node)
	}

	return nil
}

func (d *Discovery) discoverMulticast() error {
	d.logger.Debug("Performing multicast discovery")

	// In production, would use UDP multicast to discover nodes
	// For now, fall back to static discovery
	return d.discoverStatic()
}

func (d *Discovery) discoverDNS() error {
	d.logger.Debug("Performing DNS discovery")

	// In production, would query DNS SRV records
	// For now, fall back to static discovery
	return d.discoverStatic()
}

func (d *Discovery) discoverEtcd() error {
	d.logger.Debug("Performing etcd discovery")

	// In production, would query etcd for cluster membership
	// For now, fall back to static discovery
	return d.discoverStatic()
}

func (d *Discovery) parseSeeded(seed string) (*ClusterNode, error) {
	// Expected format: nodeID@host:port or host:port
	parts := strings.Split(seed, "@")

	var nodeID, address string
	var port int

	if len(parts) == 2 {
		// Format: nodeID@host:port
		nodeID = parts[0]
		address, port = d.parseAddress(parts[1])
	} else {
		// Format: host:port
		address, port = d.parseAddress(seed)
		// Generate node ID from address
		nodeID = fmt.Sprintf("node-%s-%d", address, port)
	}

	if address == "" {
		return nil, fmt.Errorf("invalid seed address: %s", seed)
	}

	node := &ClusterNode{
		ID:            nodeID,
		Address:       address,
		Port:          port,
		Role:          RoleWorker,
		State:         StateHealthy,
		LastHeartbeat: time.Now(),
		Metadata: map[string]string{
			"discovered": "static",
		},
		Capacity: ResourceCapacity{
			CPUCores:       8,
			MemoryGB:       16,
			MaxProcessors:  100,
			MaxConnections: 1000,
		},
	}

	return node, nil
}

func (d *Discovery) parseAddress(addr string) (string, int) {
	// Parse host:port
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return addr, d.config.Port
	}

	host := parts[0]
	port := d.config.Port

	// Try to parse port
	var p int
	if _, err := fmt.Sscanf(parts[1], "%d", &p); err == nil {
		port = p
	}

	return host, port
}

func (d *Discovery) registerNode(node *ClusterNode) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already discovered
	if _, exists := d.discoveredNodes[node.ID]; exists {
		return
	}

	d.discoveredNodes[node.ID] = node

	d.logger.WithFields(logrus.Fields{
		"nodeId":  node.ID,
		"address": node.Address,
		"port":    node.Port,
	}).Info("Discovered new node")

	// Call callback if set
	if d.callback != nil {
		// Call in goroutine to avoid blocking
		go d.callback(node)
	}
}

// RegisterNode manually registers a node
func (d *Discovery) RegisterNode(node *ClusterNode) {
	d.registerNode(node)
}

// UnregisterNode removes a node from discovered nodes
func (d *Discovery) UnregisterNode(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.discoveredNodes, nodeID)
	d.logger.WithField("nodeId", nodeID).Info("Unregistered node")
}

// GetNode retrieves a specific discovered node
func (d *Discovery) GetNode(nodeID string) (*ClusterNode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	node, exists := d.discoveredNodes[nodeID]
	return node, exists
}

// GetNodeCount returns the count of discovered nodes
func (d *Discovery) GetNodeCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.discoveredNodes)
}

// AnnounceNode announces this node to the cluster
func (d *Discovery) AnnounceNode(node *ClusterNode) error {
	d.logger.WithFields(logrus.Fields{
		"nodeId":  node.ID,
		"address": node.Address,
		"port":    node.Port,
	}).Info("Announcing node to cluster")

	// In production, would broadcast announcement based on discovery method
	// For static discovery, this is a no-op
	// For multicast, would send UDP announcement
	// For DNS, would register DNS record
	// For etcd, would register in etcd

	switch d.config.Method {
	case DiscoveryMulticast:
		return d.announceMulticast(node)
	case DiscoveryEtcd:
		return d.announceEtcd(node)
	default:
		// Static discovery doesn't need announcements
		return nil
	}
}

func (d *Discovery) announceMulticast(node *ClusterNode) error {
	// In production, would send UDP multicast announcement
	d.logger.Debug("Multicast announcement (simulated)")
	return nil
}

func (d *Discovery) announceEtcd(node *ClusterNode) error {
	// In production, would register node in etcd
	d.logger.Debug("Etcd announcement (simulated)")
	return nil
}

// RefreshNodes refreshes the list of discovered nodes
func (d *Discovery) RefreshNodes() error {
	d.logger.Info("Refreshing node list")
	return d.discover()
}

// GetMethod returns the discovery method
func (d *Discovery) GetMethod() DiscoveryMethod {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config.Method
}

// SetSeeds updates the seed nodes for static discovery
func (d *Discovery) SetSeeds(seeds []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.config.Seeds = seeds
	d.logger.WithField("seeds", seeds).Info("Updated seed nodes")

	// Re-discover with new seeds
	if d.running {
		go d.discover()
	}

	return nil
}

// GetSeeds returns the configured seed nodes
func (d *Discovery) GetSeeds() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config.Seeds
}

// UpdateNodeMetadata updates metadata for a discovered node
func (d *Discovery) UpdateNodeMetadata(nodeID string, metadata map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.discoveredNodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	// Merge metadata
	if node.Metadata == nil {
		node.Metadata = make(map[string]string)
	}

	for k, v := range metadata {
		node.Metadata[k] = v
	}

	d.logger.WithField("nodeId", nodeID).Debug("Updated node metadata")
	return nil
}

// GetStatistics returns discovery statistics
func (d *Discovery) GetStatistics() *DiscoveryStatistics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := &DiscoveryStatistics{
		Method:          d.config.Method,
		DiscoveredNodes: len(d.discoveredNodes),
		SeedNodes:       len(d.config.Seeds),
		LastDiscovery:   time.Now(),
		Nodes:           make([]string, 0, len(d.discoveredNodes)),
	}

	for nodeID := range d.discoveredNodes {
		stats.Nodes = append(stats.Nodes, nodeID)
	}

	return stats
}

// DiscoveryStatistics holds discovery statistics
type DiscoveryStatistics struct {
	Method          DiscoveryMethod `json:"method"`
	DiscoveredNodes int             `json:"discoveredNodes"`
	SeedNodes       int             `json:"seedNodes"`
	LastDiscovery   time.Time       `json:"lastDiscovery"`
	Nodes           []string        `json:"nodes"`
}

// WaitForNodes waits until at least the specified number of nodes are discovered
func (d *Discovery) WaitForNodes(ctx context.Context, minNodes int, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for %d nodes", minNodes)
		case <-ticker.C:
			if d.GetNodeCount() >= minNodes {
				d.logger.WithField("count", d.GetNodeCount()).Info("Required nodes discovered")
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// IsNodeDiscovered checks if a node has been discovered
func (d *Discovery) IsNodeDiscovered(nodeID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.discoveredNodes[nodeID]
	return exists
}

// GetHealthyNodes returns only healthy discovered nodes
func (d *Discovery) GetHealthyNodes() []*ClusterNode {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes := make([]*ClusterNode, 0)
	for _, node := range d.discoveredNodes {
		if node.IsHealthy() {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// PurgeStaleNodes removes nodes that haven't been seen in a while
func (d *Discovery) PurgeStaleNodes(maxAge time.Duration) int {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	purged := 0

	for nodeID, node := range d.discoveredNodes {
		if node.LastHeartbeat.Before(cutoff) {
			delete(d.discoveredNodes, nodeID)
			purged++
			d.logger.WithField("nodeId", nodeID).Info("Purged stale node")
		}
	}

	return purged
}
