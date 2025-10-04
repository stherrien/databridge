# DataBridge Docker Deployment Guide

This guide covers Docker deployment options for DataBridge, including single-node, cluster, and monitoring configurations.

## Table of Contents

- [Quick Start](#quick-start)
- [Single Node Deployment](#single-node-deployment)
- [Cluster Deployment](#cluster-deployment)
- [Development Mode](#development-mode)
- [Monitoring Stack](#monitoring-stack)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)

## Quick Start

### Build and Run (Single Node)

```bash
# Build the Docker image
make docker-build

# Run single node
docker-compose up -d databridge

# Check logs
docker-compose logs -f databridge

# Access the application
# API: http://localhost:8080
# Frontend: http://localhost:3000
```

### Using Pre-built Commands

```bash
# Build Docker image
make docker-build

# Run container
make docker-run

# Stop and clean
make docker-clean
```

## Single Node Deployment

The simplest deployment runs a single DataBridge instance.

### Using Docker Compose

```bash
# Start single node
docker-compose up -d databridge

# View logs
docker-compose logs -f databridge

# Stop
docker-compose down
```

### Using Docker CLI

```bash
# Build
docker build -t databridge:latest .

# Run
docker run -d \
  --name databridge \
  -p 8080:8080 \
  -p 3000:3000 \
  -v databridge-data:/app/data \
  -e LOG_LEVEL=info \
  databridge:latest

# View logs
docker logs -f databridge

# Stop
docker stop databridge
docker rm databridge
```

### Configuration

Environment variables for single node:

```bash
DATA_DIR=/app/data              # Data storage directory
LOG_LEVEL=info                  # Logging level (debug, info, warn, error)
API_PORT=8080                   # REST API port
CLUSTER_ENABLED=false           # Disable clustering
```

## Cluster Deployment

Deploy a 3-node DataBridge cluster with Raft consensus.

### Start Cluster

```bash
# Start all 3 nodes
docker-compose --profile cluster up -d

# View all logs
docker-compose --profile cluster logs -f

# Check cluster status
curl http://localhost:8081/api/cluster/status
curl http://localhost:8082/api/cluster/status
curl http://localhost:8083/api/cluster/status
```

### Access Cluster Nodes

- **Node 1**: http://localhost:8081 (API), :9001 (Raft)
- **Node 2**: http://localhost:8082 (API), :9002 (Raft)
- **Node 3**: http://localhost:8083 (API), :9003 (Raft)

### Cluster Configuration

Each node requires these environment variables:

```bash
CLUSTER_ENABLED=true
CLUSTER_NODE_ID=node1                           # Unique node ID
CLUSTER_BIND_ADDR=0.0.0.0:9000                 # Raft bind address
CLUSTER_ADVERTISE_ADDR=databridge-node1:9000   # Raft advertise address
CLUSTER_PEERS=node1:9000,node2:9000,node3:9000 # Peer list
RAFT_DIR=/app/data/raft                        # Raft data directory
```

### Scale Cluster

Add more nodes by extending `docker-compose.yml`:

```yaml
databridge-node4:
  build:
    context: .
    dockerfile: Dockerfile
  image: databridge:latest
  container_name: databridge-node4
  ports:
    - "8084:8080"
    - "9004:9000"
  environment:
    - CLUSTER_ENABLED=true
    - CLUSTER_NODE_ID=node4
    - CLUSTER_BIND_ADDR=0.0.0.0:9000
    - CLUSTER_ADVERTISE_ADDR=databridge-node4:9000
    - CLUSTER_PEERS=databridge-node1:9000,databridge-node2:9000,databridge-node3:9000,databridge-node4:9000
  volumes:
    - databridge-node4-data:/app/data
  networks:
    - databridge-network
  profiles:
    - cluster
```

## Development Mode

Use hot-reload for rapid development.

### Start Development Container

```bash
# Using docker-compose
docker-compose -f docker-compose.dev.yml up

# Or using Docker directly
docker build -f Dockerfile.dev -t databridge:dev .
docker run -it --rm \
  -v $(pwd):/app \
  -p 8080:8080 \
  databridge:dev
```

### Features

- **Hot Reload**: Code changes trigger automatic rebuilds
- **Debug Logging**: Verbose logging enabled
- **Volume Mounts**: Source code mounted for live editing
- **Air**: Go hot-reload tool pre-configured

### Development Configuration

Create `docker-compose.dev.yml`:

```yaml
version: '3.8'

services:
  databridge-dev:
    build:
      context: .
      dockerfile: Dockerfile.dev
    container_name: databridge-dev
    ports:
      - "8080:8080"
    environment:
      - LOG_LEVEL=debug
      - DATA_DIR=/app/data-dev
    volumes:
      - .:/app
      - databridge-dev-data:/app/data-dev
    networks:
      - databridge-network

networks:
  databridge-network:
    driver: bridge

volumes:
  databridge-dev-data:
```

## Monitoring Stack

Deploy Prometheus and Grafana for comprehensive monitoring.

### Start Monitoring

```bash
# Start single node with monitoring
docker-compose --profile monitoring up -d

# Or start cluster with monitoring
docker-compose --profile cluster --profile monitoring up -d

# Access dashboards
# Grafana: http://localhost:3001 (admin/admin)
# Prometheus: http://localhost:9090
```

### Grafana Setup

1. Open http://localhost:3001
2. Login: `admin` / `admin` (change on first login)
3. Navigate to Dashboards → DataBridge
4. View metrics: CPU, memory, FlowFiles, throughput

### Prometheus Metrics

DataBridge exposes metrics at `/api/metrics`:

```bash
# Query metrics
curl http://localhost:8080/api/metrics

# Example metrics:
# - databridge_flowfiles_total
# - databridge_flowfiles_processed
# - databridge_processor_tasks_completed
# - databridge_processor_execution_time
# - databridge_queue_depth
# - go_goroutines
# - process_cpu_seconds_total
```

### Custom Monitoring Configuration

Edit `deploy/prometheus/prometheus.yml` to customize:

```yaml
scrape_configs:
  - job_name: 'databridge-custom'
    static_configs:
      - targets: ['your-host:8080']
    metrics_path: '/api/metrics'
    scrape_interval: 5s
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_DIR` | `/app/data` | Data storage directory |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `API_PORT` | `8080` | REST API port |
| `CLUSTER_ENABLED` | `false` | Enable clustering |
| `CLUSTER_NODE_ID` | - | Unique cluster node identifier |
| `CLUSTER_BIND_ADDR` | - | Raft consensus bind address |
| `CLUSTER_ADVERTISE_ADDR` | - | Raft consensus advertise address |
| `CLUSTER_PEERS` | - | Comma-separated list of cluster peers |
| `RAFT_DIR` | `/app/data/raft` | Raft data directory |

### Volumes

Persistent volumes for data storage:

```yaml
volumes:
  databridge-data:           # FlowFile and content repositories
  databridge-logs:           # Application logs
  databridge-node1-data:     # Cluster node 1 data
  databridge-node2-data:     # Cluster node 2 data
  databridge-node3-data:     # Cluster node 3 data
  prometheus-data:           # Prometheus time-series data
  grafana-data:              # Grafana dashboards and settings
```

### Network Configuration

Custom network for inter-container communication:

```yaml
networks:
  databridge-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.25.0.0/16
```

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker-compose logs databridge

# Check container status
docker ps -a

# Inspect container
docker inspect databridge

# Common issues:
# 1. Port conflicts - change ports in docker-compose.yml
# 2. Permission issues - check volume permissions
# 3. Memory limits - increase Docker resources
```

### Cluster Nodes Can't Connect

```bash
# Check network connectivity
docker exec databridge-node1 ping databridge-node2

# Check Raft logs
docker logs databridge-node1 | grep -i raft

# Verify peer configuration
docker exec databridge-node1 env | grep CLUSTER

# Common issues:
# 1. Incorrect CLUSTER_PEERS - ensure all nodes listed
# 2. Network isolation - check docker network
# 3. Firewall rules - ensure Raft ports accessible
```

### Performance Issues

```bash
# Check resource usage
docker stats

# Increase resources in docker-compose.yml:
services:
  databridge:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

# Enable debug logging
docker-compose up -e LOG_LEVEL=debug
```

### Data Persistence Issues

```bash
# List volumes
docker volume ls

# Inspect volume
docker volume inspect databridge-data

# Backup volume
docker run --rm -v databridge-data:/data -v $(pwd):/backup \
  alpine tar czf /backup/databridge-backup.tar.gz /data

# Restore volume
docker run --rm -v databridge-data:/data -v $(pwd):/backup \
  alpine tar xzf /backup/databridge-backup.tar.gz -C /
```

### Health Check Failures

```bash
# Manual health check
curl http://localhost:8080/api/health

# Disable health check temporarily in docker-compose.yml:
services:
  databridge:
    healthcheck:
      disable: true

# Check application logs for errors
docker logs databridge --tail 100
```

## Production Deployment

### Recommended Configuration

```yaml
version: '3.8'

services:
  databridge:
    image: databridge:1.0.0
    container_name: databridge-prod
    restart: always
    ports:
      - "8080:8080"
    environment:
      - LOG_LEVEL=warn
      - DATA_DIR=/app/data
      - API_PORT=8080
    volumes:
      - /var/lib/databridge/data:/app/data
      - /var/log/databridge:/app/data/logs
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 4G
        reservations:
          cpus: '2'
          memory: 2G
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "10"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
```

### Security Hardening

1. **Run as non-root**: Already configured in Dockerfile
2. **Use secrets**: Store sensitive data in Docker secrets
3. **Network isolation**: Use custom networks
4. **TLS/SSL**: Configure reverse proxy (nginx/traefik)
5. **Read-only filesystem**: Add `read_only: true` where possible

### Backup Strategy

```bash
# Automated backup script
#!/bin/bash
BACKUP_DIR=/backups/databridge/$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR

# Backup volumes
docker run --rm \
  -v databridge-data:/data \
  -v $BACKUP_DIR:/backup \
  alpine tar czf /backup/data.tar.gz /data

# Backup configuration
cp docker-compose.yml $BACKUP_DIR/
cp .env $BACKUP_DIR/

echo "Backup completed: $BACKUP_DIR"
```

## Advanced Topics

### Multi-Host Deployment

Use Docker Swarm or Kubernetes for multi-host deployments:

```bash
# Initialize Swarm
docker swarm init

# Deploy stack
docker stack deploy -c docker-compose.yml databridge

# Scale services
docker service scale databridge=5
```

### Custom Images

Build custom images with additional processors:

```dockerfile
FROM databridge:latest

# Copy custom processor plugins
COPY plugins/*.so /app/plugins/

# Additional configuration
COPY custom-config.yaml /app/config/
```

### Integration with CI/CD

```yaml
# .github/workflows/docker.yml
name: Docker Build and Push

on:
  push:
    tags:
      - 'v*'

jobs:
  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build and push
        run: |
          docker build -t databridge:${{ github.ref_name }} .
          docker push databridge:${{ github.ref_name }}
```

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/shawntherrien/databridge/issues
- Documentation: https://docs.databridge.io
- Community: https://community.databridge.io
