package cluster

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// NodeRole defines the role of a node in the cluster
type NodeRole string

const (
	RolePrimary   NodeRole = "primary"
	RoleSecondary NodeRole = "secondary"
	RoleWorker    NodeRole = "worker"
)

// NodeState tracks the health state of a node
type NodeState string

const (
	StateHealthy      NodeState = "healthy"
	StateDegraded     NodeState = "degraded"
	StateUnhealthy    NodeState = "unhealthy"
	StateDisconnected NodeState = "disconnected"
)

// LoadBalancingStrategy defines how work is distributed across nodes
type LoadBalancingStrategy string

const (
	StrategyRoundRobin     LoadBalancingStrategy = "round_robin"
	StrategyLeastLoaded    LoadBalancingStrategy = "least_loaded"
	StrategyWeightedRandom LoadBalancingStrategy = "weighted_random"
	StrategyConsistentHash LoadBalancingStrategy = "consistent_hash"
)

// ReplicationStrategy defines how state is replicated
type ReplicationStrategy string

const (
	ReplicationSync   ReplicationStrategy = "sync"   // Wait for all replicas
	ReplicationAsync  ReplicationStrategy = "async"  // Fire and forget
	ReplicationQuorum ReplicationStrategy = "quorum" // Wait for majority
)

// DiscoveryMethod defines how nodes discover each other
type DiscoveryMethod string

const (
	DiscoveryStatic    DiscoveryMethod = "static"    // Static seed list
	DiscoveryMulticast DiscoveryMethod = "multicast" // UDP multicast
	DiscoveryDNS       DiscoveryMethod = "dns"       // DNS SRV records
	DiscoveryEtcd      DiscoveryMethod = "etcd"      // etcd registry
)

// ClusterNode represents a node in the cluster
type ClusterNode struct {
	ID            string            `json:"id"`
	Address       string            `json:"address"`
	Port          int               `json:"port"`
	Role          NodeRole          `json:"role"`
	State         NodeState         `json:"state"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	Metadata      map[string]string `json:"metadata"`
	Capacity      ResourceCapacity  `json:"capacity"`
	Load          *NodeLoad         `json:"load,omitempty"`
	mu            sync.RWMutex
}

// ResourceCapacity defines the resource capacity of a node
type ResourceCapacity struct {
	CPUCores       int   `json:"cpuCores"`
	MemoryGB       int64 `json:"memoryGB"`
	MaxProcessors  int   `json:"maxProcessors"`
	MaxConnections int   `json:"maxConnections"`
}

// NodeLoad represents the current load on a node
type NodeLoad struct {
	CPUUsage         float64   `json:"cpuUsage"`
	MemoryUsage      float64   `json:"memoryUsage"`
	ActiveProcessors int       `json:"activeProcessors"`
	QueueDepth       int       `json:"queueDepth"`
	Score            float64   `json:"score"` // Composite load score (0-100)
	LastUpdated      time.Time `json:"lastUpdated"`
}

// ClusterState holds the replicated cluster state
type ClusterState struct {
	Nodes           map[string]*ClusterNode `json:"nodes"`
	FlowAssignments map[string]string       `json:"flowAssignments"` // flowID -> nodeID
	ProcessorLeases map[string]string       `json:"processorLeases"` // processorID -> nodeID
	SharedConfig    map[string]interface{}  `json:"sharedConfig"`
	Version         uint64                  `json:"version"`
	mu              sync.RWMutex
}

// HealthCheck tracks the health status of a node
type HealthCheck struct {
	NodeID           string        `json:"nodeId"`
	LastCheck        time.Time     `json:"lastCheck"`
	Latency          time.Duration `json:"latency"`
	ConsecutiveFails int           `json:"consecutiveFails"`
	IsHealthy        bool          `json:"isHealthy"`
}

// LoadMetrics tracks load metrics across the cluster
type LoadMetrics struct {
	NodeLoads   map[string]*NodeLoad `json:"nodeLoads"`
	LastUpdated time.Time            `json:"lastUpdated"`
	mu          sync.RWMutex
}

// ClusterConfig holds cluster configuration
type ClusterConfig struct {
	Enabled          bool          `json:"enabled"`
	NodeID           string        `json:"nodeId"`
	BindAddress      string        `json:"bindAddress"`
	BindPort         int           `json:"bindPort"`
	SeedNodes        []string      `json:"seedNodes"`
	Discovery        string        `json:"discovery"`
	ReplicaCount     int           `json:"replicaCount"`
	HeartbeatTimeout time.Duration `json:"heartbeatTimeout"`
	ElectionTimeout  time.Duration `json:"electionTimeout"`
}

// ClusterEvent represents an event in the cluster
type ClusterEvent struct {
	Type      EventType              `json:"type"`
	NodeID    string                 `json:"nodeId"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventType defines types of cluster events
type EventType string

const (
	EventNodeJoined      EventType = "node_joined"
	EventNodeLeft        EventType = "node_left"
	EventNodeFailed      EventType = "node_failed"
	EventLeaderElected   EventType = "leader_elected"
	EventWorkAssigned    EventType = "work_assigned"
	EventWorkRebalanced  EventType = "work_rebalanced"
	EventStateReplicated EventType = "state_replicated"
)

// ProcessorAssignment represents a processor assigned to a node
type ProcessorAssignment struct {
	ProcessorID uuid.UUID `json:"processorId"`
	NodeID      string    `json:"nodeId"`
	AssignedAt  time.Time `json:"assignedAt"`
	Affinity    int       `json:"affinity"` // Preference score for keeping on this node
}

// FlowAssignment represents a flow assigned to a node
type FlowAssignment struct {
	FlowID     uuid.UUID `json:"flowId"`
	NodeID     string    `json:"nodeId"`
	AssignedAt time.Time `json:"assignedAt"`
}

// ReplicationRequest represents a state replication request
type ReplicationRequest struct {
	RequestID   string      `json:"requestId"`
	StateType   string      `json:"stateType"`
	Data        interface{} `json:"data"`
	Version     uint64      `json:"version"`
	Timestamp   time.Time   `json:"timestamp"`
	AckRequired bool        `json:"ackRequired"`
}

// ReplicationResponse represents a replication acknowledgment
type ReplicationResponse struct {
	RequestID string    `json:"requestId"`
	NodeID    string    `json:"nodeId"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ClusterStatistics holds cluster-wide statistics
type ClusterStatistics struct {
	TotalNodes       int                   `json:"totalNodes"`
	HealthyNodes     int                   `json:"healthyNodes"`
	Leader           string                `json:"leader"`
	TotalProcessors  int                   `json:"totalProcessors"`
	ActiveProcessors int                   `json:"activeProcessors"`
	AverageLoad      float64               `json:"averageLoad"`
	NodeStats        map[string]*NodeStats `json:"nodeStats"`
	LastUpdated      time.Time             `json:"lastUpdated"`
}

// NodeStats holds statistics for a single node
type NodeStats struct {
	NodeID          string        `json:"nodeId"`
	Uptime          time.Duration `json:"uptime"`
	ProcessorsCount int           `json:"processorsCount"`
	QueueDepth      int           `json:"queueDepth"`
	ProcessedCount  int64         `json:"processedCount"`
	LastHeartbeat   time.Time     `json:"lastHeartbeat"`
}

// Helper methods for ClusterNode
func (n *ClusterNode) UpdateState(state NodeState) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.State = state
}

func (n *ClusterNode) UpdateHeartbeat() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.LastHeartbeat = time.Now()
}

func (n *ClusterNode) GetState() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.State
}

func (n *ClusterNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.State == StateHealthy || n.State == StateDegraded
}

// Helper methods for ClusterState
func (cs *ClusterState) GetNode(nodeID string) (*ClusterNode, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	node, exists := cs.Nodes[nodeID]
	return node, exists
}

func (cs *ClusterState) AddNode(node *ClusterNode) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Nodes[node.ID] = node
	cs.Version++
}

func (cs *ClusterState) RemoveNode(nodeID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.Nodes, nodeID)
	cs.Version++
}

func (cs *ClusterState) AssignProcessor(processorID uuid.UUID, nodeID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.ProcessorLeases[processorID.String()] = nodeID
	cs.Version++
}

func (cs *ClusterState) GetProcessorAssignment(processorID uuid.UUID) (string, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	nodeID, exists := cs.ProcessorLeases[processorID.String()]
	return nodeID, exists
}

func (cs *ClusterState) GetNodeCount() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.Nodes)
}

// Helper methods for LoadMetrics
func (lm *LoadMetrics) UpdateNodeLoad(nodeID string, load *NodeLoad) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.NodeLoads[nodeID] = load
	lm.LastUpdated = time.Now()
}

func (lm *LoadMetrics) GetNodeLoad(nodeID string) (*NodeLoad, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	load, exists := lm.NodeLoads[nodeID]
	return load, exists
}

func (lm *LoadMetrics) GetAverageLoad() float64 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if len(lm.NodeLoads) == 0 {
		return 0.0
	}

	var total float64
	for _, load := range lm.NodeLoads {
		total += load.Score
	}

	return total / float64(len(lm.NodeLoads))
}

// NewClusterState creates a new cluster state
func NewClusterState() *ClusterState {
	return &ClusterState{
		Nodes:           make(map[string]*ClusterNode),
		FlowAssignments: make(map[string]string),
		ProcessorLeases: make(map[string]string),
		SharedConfig:    make(map[string]interface{}),
		Version:         0,
	}
}

// NewLoadMetrics creates a new load metrics tracker
func NewLoadMetrics() *LoadMetrics {
	return &LoadMetrics{
		NodeLoads:   make(map[string]*NodeLoad),
		LastUpdated: time.Now(),
	}
}
