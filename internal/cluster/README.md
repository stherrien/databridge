# DataBridge Clustering System

This package implements comprehensive clustering and distribution support for DataBridge, enabling multi-node deployments with automatic failover and load balancing.

## Features

### 1. Cluster Manager (`manager.go`)
The central orchestrator for cluster operations:
- **Node Discovery**: Automatic detection of cluster members
- **Health Monitoring**: Continuous health checks with heartbeats
- **Leader Election**: Automatic leader selection using Raft consensus
- **Processor Assignment**: Intelligent distribution of processors across nodes
- **Load Balancing**: Dynamic rebalancing based on node capacity
- **Event System**: Cluster-wide event propagation

### 2. Coordinator (`coordinator.go`)
Distributed coordination using Raft-like consensus:
- **Consensus Algorithm**: Simplified Raft implementation for cluster state
- **Leader Election**: Automatic failover with quorum-based voting
- **State Replication**: Synchronized cluster state across all nodes
- **Configuration Management**: Shared configuration distribution
- **Snapshot Support**: State recovery and persistence

### 3. Health Checker (`health_checker.go`)
Continuous health monitoring:
- **Periodic Health Checks**: Configurable interval-based checks
- **Latency Tracking**: Network latency monitoring
- **Failure Detection**: Automatic unhealthy node detection
- **Threshold-based Quarantine**: Configurable consecutive failure limits
- **Recovery Detection**: Automatic health restoration

### 4. Load Balancer (`load_balancer.go`)
Intelligent work distribution:
- **Multiple Strategies**:
  - Round Robin: Even distribution across nodes
  - Least Loaded: Assign to node with lowest load
  - Weighted Random: Random with load-based weights
  - Consistent Hash: Deterministic assignment for affinity
- **Dynamic Rebalancing**: Automatic workload redistribution
- **Load Metrics**: CPU, memory, processor count, queue depth
- **Optimal Distribution**: Capacity-aware processor placement

### 5. State Replicator (`replication.go`)
State replication across nodes:
- **Replication Strategies**:
  - Sync: Wait for all replicas
  - Async: Fire-and-forget
  - Quorum: Wait for majority
- **Retry Logic**: Automatic retry of failed replications
- **Acknowledgment Tracking**: Monitor replication success
- **Cleanup**: Automatic removal of old replication requests

### 6. Node Discovery (`discovery.go`)
Automatic node discovery:
- **Discovery Methods**:
  - Static: Predefined seed nodes
  - Multicast: UDP multicast discovery
  - DNS: DNS SRV record lookup
  - etcd: Service registry integration
- **Dynamic Membership**: Automatic cluster join/leave
- **Stale Node Purging**: Remove inactive nodes

## Usage

### Basic Cluster Setup

```go
// Configure cluster
config := cluster.ClusterConfig{
    Enabled:          true,
    NodeID:           "node-1",
    BindAddress:      "127.0.0.1",
    BindPort:         8090,
    SeedNodes:        []string{"127.0.0.1:8091", "127.0.0.1:8092"},
    Discovery:        "static",
    ReplicaCount:     3,
    HeartbeatTimeout: 10 * time.Second,
    ElectionTimeout:  30 * time.Second,
}

// Create cluster manager
clusterManager, err := cluster.NewClusterManager(config, logger)
if err != nil {
    log.Fatal(err)
}

// Start cluster
if err := clusterManager.Start(); err != nil {
    log.Fatal(err)
}
defer clusterManager.Stop()

// Check if this node is leader
if clusterManager.IsLeader() {
    fmt.Println("This node is the cluster leader")
}
```

### Processor Assignment

```go
// Assign processor to a node (leader only)
processorID := uuid.New()
nodeID, err := clusterManager.AssignProcessor(processorID)
if err != nil {
    log.Printf("Failed to assign processor: %v", err)
} else {
    log.Printf("Assigned processor %s to node %s", processorID, nodeID)
}

// Check processor assignment
assignedNode, exists := clusterManager.GetProcessorAssignment(processorID)
if exists {
    fmt.Printf("Processor is assigned to: %s\n", assignedNode)
}
```

### Load Balancing

```go
// Get load balancer
lb := clusterManager.GetLoadBalancer()

// Change strategy
lb.SetStrategy(cluster.StrategyLeastLoaded)

// Check if rebalancing is needed
if lb.ShouldRebalance() {
    // Trigger rebalance (leader only)
    if err := clusterManager.Rebalance(); err != nil {
        log.Printf("Rebalance failed: %v", err)
    }
}

// Get load distribution
distribution := lb.GetLoadDistribution()
fmt.Printf("Average load: %.2f\n", distribution.AverageScore)
fmt.Printf("Load range: %.2f\n", distribution.ScoreRange)
```

### Health Monitoring

```go
// Get health checker
hc := clusterManager.GetHealthChecker()

// Get statistics
stats := hc.GetStatistics()
fmt.Printf("Healthy nodes: %d/%d\n", stats.HealthyNodes, stats.TotalNodes)

// Check specific node
if hc.IsNodeHealthy("node-2") {
    fmt.Println("Node 2 is healthy")
}

// Force health check
hc.ForceHealthCheck("node-2")
```

### Cluster Statistics

```go
// Get cluster-wide statistics
stats := clusterManager.GetStatistics()
fmt.Printf("Total nodes: %d\n", stats.TotalNodes)
fmt.Printf("Healthy nodes: %d\n", stats.HealthyNodes)
fmt.Printf("Leader: %s\n", stats.Leader)
fmt.Printf("Average load: %.2f\n", stats.AverageLoad)

// Get per-node statistics
for nodeID, nodeStats := range stats.NodeStats {
    fmt.Printf("Node %s: %d processors, %d queued\n",
        nodeID, nodeStats.ProcessorsCount, nodeStats.QueueDepth)
}
```

## Configuration

### Command-Line Flags

```bash
# Enable clustering
--cluster-enabled=true

# Set node ID (auto-generated if empty)
--cluster-node-id=node-1

# Set bind address and port
--cluster-bind-address=127.0.0.1
--cluster-bind-port=8090

# Specify seed nodes
--cluster-seeds=127.0.0.1:8091,127.0.0.1:8092

# Set discovery method
--cluster-discovery=static

# Set replica count
--cluster-replica-count=3
```

### Configuration File (YAML)

```yaml
cluster:
  enabled: true
  nodeId: "node-1"
  bindAddress: "127.0.0.1"
  bindPort: 8090
  seeds:
    - "127.0.0.1:8091"
    - "127.0.0.1:8092"
  discovery: "static"
  replicaCount: 3
```

## Architecture

### Node Roles

- **Primary**: Main coordinator (auto-elected leader)
- **Secondary**: Backup coordinator
- **Worker**: Standard processing node

### Cluster State

The cluster maintains synchronized state for:
- Node membership and health
- Processor assignments
- Flow assignments
- Shared configuration

### Fault Tolerance

- **Split-Brain Prevention**: Quorum-based decisions
- **Automatic Failover**: Leader re-election on failure
- **Health-based Routing**: Only healthy nodes receive work
- **Graceful Degradation**: Continues with reduced capacity

## Multi-Node Deployment

### 3-Node Cluster Example

**Node 1:**
```bash
./databridge \
  --cluster-enabled=true \
  --cluster-node-id=node-1 \
  --cluster-bind-address=10.0.1.1 \
  --cluster-bind-port=8090 \
  --cluster-seeds=10.0.1.2:8090,10.0.1.3:8090
```

**Node 2:**
```bash
./databridge \
  --cluster-enabled=true \
  --cluster-node-id=node-2 \
  --cluster-bind-address=10.0.1.2 \
  --cluster-bind-port=8090 \
  --cluster-seeds=10.0.1.1:8090,10.0.1.3:8090
```

**Node 3:**
```bash
./databridge \
  --cluster-enabled=true \
  --cluster-node-id=node-3 \
  --cluster-bind-address=10.0.1.3 \
  --cluster-bind-port=8090 \
  --cluster-seeds=10.0.1.1:8090,10.0.1.2:8090
```

## REST API

The cluster system exposes REST endpoints for management:

```
GET    /api/cluster/nodes           - List all nodes
GET    /api/cluster/nodes/{id}      - Get node details
GET    /api/cluster/leader          - Get current leader
POST   /api/cluster/join            - Join cluster
DELETE /api/cluster/nodes/{id}      - Remove node
GET    /api/cluster/status          - Cluster health status
GET    /api/cluster/assignments     - View work assignments
POST   /api/cluster/rebalance       - Trigger rebalancing
```

## Testing

Run tests with:

```bash
# Unit tests
go test ./internal/cluster/

# With coverage
go test -cover ./internal/cluster/

# Integration tests
go test -v ./internal/cluster/

# Skip integration tests
go test -short ./internal/cluster/
```

## Performance Considerations

- **Heartbeat Interval**: Balance between detection speed and network overhead (default: 10s)
- **Election Timeout**: Must be longer than heartbeat interval (default: 30s)
- **Rebalance Window**: Minimum time between rebalances to prevent thrashing (default: 30s)
- **Replica Count**: Higher values increase reliability but add overhead (default: 3)

## Troubleshooting

### Split Brain
- Ensure proper quorum size (n/2 + 1)
- Check network connectivity between nodes
- Verify heartbeat timeouts are appropriate

### Slow Leader Election
- Reduce election timeout
- Check network latency
- Ensure seed nodes are reachable

### Unbalanced Load
- Verify load metrics are being collected
- Check rebalancing strategy
- Ensure healthy node capacity

## Future Enhancements

- Integration with external Raft libraries (hashicorp/raft)
- Consul/etcd service discovery
- TLS/mTLS for inter-node communication
- Advanced load balancing algorithms
- Processor migration without downtime
- Cluster-aware flow file routing
- Cross-datacenter replication

## License

Part of DataBridge project.
