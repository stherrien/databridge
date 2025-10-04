# DataBridge Monitoring API

## Overview

The DataBridge Monitoring API provides real-time visibility into system performance, processor metrics, and data flow statistics. It offers both REST endpoints and Server-Sent Events (SSE) for streaming updates.

## Features

- System status and health checks
- Processor-level metrics and statistics
- Connection queue monitoring
- Real-time streaming updates via SSE
- Metrics caching for improved performance
- CORS support for web clients

## API Endpoints

### System Status Endpoints

#### GET /api/system/status
Returns overall system status including uptime, active processors, and scheduler state.

**Response:**
```json
{
  "status": "running",
  "uptime": "PT1H30M",
  "uptimeSeconds": 5400,
  "flowController": {
    "running": true,
    "state": "RUNNING"
  },
  "scheduler": {
    "running": true,
    "scheduledTasks": 5,
    "activeWorkers": 2,
    "maxWorkers": 10
  },
  "activeProcessors": 3,
  "totalProcessors": 5,
  "totalConnections": 4,
  "timestamp": "2025-09-30T12:00:00Z"
}
```

#### GET /api/system/health
Health check endpoint with component-level health status.

**Response:**
```json
{
  "status": "healthy",
  "components": {
    "flowController": {
      "status": "healthy",
      "message": "FlowController is running"
    },
    "scheduler": {
      "status": "healthy",
      "message": "ProcessScheduler is running"
    },
    "processors": {
      "status": "healthy",
      "message": "All processors are valid"
    }
  },
  "timestamp": "2025-09-30T12:00:00Z"
}
```

**Status Codes:**
- `200 OK` - System is healthy or degraded
- `503 Service Unavailable` - System is unhealthy

#### GET /api/system/metrics
Returns system-wide metrics including throughput, memory, CPU, and repository statistics.

**Response:**
```json
{
  "throughput": {
    "flowFilesProcessed": 1000,
    "flowFilesPerSecond": 10.5,
    "flowFilesPerMinute": 630,
    "flowFilesPerHour": 37800,
    "bytesProcessed": 1048576,
    "bytesPerSecond": 1024.5,
    "totalTransactions": 1000
  },
  "memory": {
    "allocMB": 45.2,
    "totalAllocMB": 120.5,
    "sysMB": 85.3,
    "numGC": 15,
    "gcPauseMS": 1.2
  },
  "cpu": {
    "numCPU": 8,
    "numGoroutine": 25
  },
  "repository": {
    "flowFileCount": 1000,
    "contentClaimCount": 950,
    "flowFileRepoSizeMB": 10,
    "contentRepoSizeMB": 250
  },
  "timestamp": "2025-09-30T12:00:00Z"
}
```

#### GET /api/system/stats
Returns a comprehensive statistics summary.

**Response:**
```json
{
  "totalFlowFilesProcessed": 1000,
  "totalBytesProcessed": 1048576,
  "activeProcessors": 3,
  "totalProcessors": 5,
  "totalConnections": 4,
  "averageThroughput": 10.5,
  "uptime": "PT1H30M",
  "processorStats": [
    {
      "id": "uuid",
      "name": "Generate FlowFiles",
      "type": "GenerateFlowFile",
      "state": "RUNNING",
      "tasksCompleted": 500,
      "tasksFailed": 0,
      "flowFilesIn": 0,
      "flowFilesOut": 500,
      "bytesIn": 0,
      "bytesOut": 512000
    }
  ],
  "connectionStats": [
    {
      "id": "uuid",
      "name": "Generate -> Process",
      "queueDepth": 5,
      "flowFilesQueued": 5
    }
  ],
  "timestamp": "2025-09-30T12:00:00Z"
}
```

### Component Monitoring Endpoints

#### GET /api/monitoring/processors
Returns metrics for all processors.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Generate FlowFiles",
    "type": "GenerateFlowFile",
    "state": "RUNNING",
    "tasksCompleted": 500,
    "tasksFailed": 0,
    "tasksRunning": 1,
    "flowFilesIn": 0,
    "flowFilesOut": 500,
    "bytesIn": 0,
    "bytesOut": 512000,
    "averageExecutionTime": "PT0.050S",
    "averageExecutionTimeMS": 50.0,
    "lastRunTime": "2025-09-30T12:00:00Z",
    "runCount": 500,
    "throughput": {
      "flowFilesPerSecond": 10.0,
      "flowFilesPerMinute": 600.0,
      "bytesPerSecond": 10240.0
    },
    "timestamp": "2025-09-30T12:00:00Z"
  }
]
```

#### GET /api/monitoring/processors/{id}
Returns metrics for a specific processor.

**Response:** Same as single processor object above.

#### GET /api/monitoring/connections
Returns metrics for all connections.

**Response:**
```json
[
  {
    "id": "uuid",
    "name": "Generate -> Process",
    "sourceId": "uuid",
    "sourceName": "Generate FlowFiles",
    "destinationId": "uuid",
    "destinationName": "Process Data",
    "relationship": "success",
    "queueDepth": 5,
    "maxQueueSize": 10000,
    "flowFilesQueued": 5,
    "backPressureTriggered": 0,
    "percentFull": 0.05,
    "throughput": {
      "flowFilesPerSecond": 10.0,
      "enqueueRate": 10.5,
      "dequeueRate": 10.0
    },
    "timestamp": "2025-09-30T12:00:00Z"
  }
]
```

#### GET /api/monitoring/connections/{id}
Returns metrics for a specific connection.

**Response:** Same as single connection object above.

#### GET /api/monitoring/queues
Returns aggregated queue metrics and individual queue statistics.

**Response:**
```json
{
  "totalQueued": 15,
  "totalCapacity": 40000,
  "percentFull": 0.0375,
  "backPressureCount": 0,
  "queues": [
    {
      "id": "uuid",
      "name": "Generate -> Process",
      "queueDepth": 5,
      "maxQueueSize": 10000,
      "percentFull": 0.05
    }
  ],
  "timestamp": "2025-09-30T12:00:00Z"
}
```

### Real-Time Streaming

#### GET /api/monitoring/stream
Server-Sent Events (SSE) endpoint for real-time monitoring updates.

**Event Types:**
- `system_status` - System status updates
- `throughput` - Throughput metrics
- `processor_metrics` - Individual processor metrics
- `queue_metrics` - Queue depth and statistics

**Example Event:**
```
event: system_status
data: {"eventType":"system_status","data":{...},"timestamp":"2025-09-30T12:00:00Z"}

event: throughput
data: {"eventType":"throughput","data":{...},"timestamp":"2025-09-30T12:00:00Z"}
```

**Usage Example (JavaScript):**
```javascript
const eventSource = new EventSource('http://localhost:8080/api/monitoring/stream');

eventSource.addEventListener('system_status', (event) => {
  const data = JSON.parse(event.data);
  console.log('System Status:', data);
});

eventSource.addEventListener('processor_metrics', (event) => {
  const data = JSON.parse(event.data);
  console.log('Processor Metrics:', data);
});

eventSource.onerror = (error) => {
  console.error('SSE Error:', error);
};
```

## Configuration

The monitoring API can be configured via command-line flags or configuration file:

```bash
# Command-line flags
./databridge --port 8080 --log-level info

# Configuration file (databridge.yaml)
port: 8080
logLevel: info
```

### Configuration Options

- `port` - HTTP server port (default: 8080)
- `logLevel` - Logging level: debug, info, warn, error (default: info)
- Metrics cache interval: 5 seconds (configured in code)
- SSE update interval: 2 seconds (configured in code)

## Performance Considerations

### Metrics Caching

The API implements intelligent metrics caching to reduce overhead:

- System metrics are cached for 5 seconds
- Processor and connection metrics are computed on-demand
- Cache interval is configurable

### SSE Performance

- Updates are broadcast every 2 seconds
- Client buffer size: 10 events
- Automatic cleanup of disconnected clients

## Error Handling

The API returns appropriate HTTP status codes and error messages:

**Example Error Response:**
```json
{
  "error": "Processor not found"
}
```

**Common Status Codes:**
- `200 OK` - Successful request
- `400 Bad Request` - Invalid request parameters
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Server error
- `503 Service Unavailable` - Service unhealthy

## Testing

Comprehensive test suite covering all endpoints:

```bash
# Run all API tests
go test ./internal/api/... -v

# Run specific test
go test ./internal/api -v -run TestHandleSystemStatus

# Run with coverage
go test ./internal/api -cover
```

## Architecture

### Components

1. **Server** (`server.go`) - HTTP server with Gorilla Mux router
2. **Monitoring Handlers** (`monitoring_handlers.go`) - REST endpoint handlers
3. **SSE Handler** (`sse_handler.go`) - Server-Sent Events implementation
4. **Metrics Collector** (`metrics_collector.go`) - Metrics aggregation and caching
5. **Models** (`models.go`) - Data transfer objects (DTOs)

### Design Patterns

- **Repository Pattern** - Abstracts data access
- **Handler Pattern** - Separates HTTP handling from business logic
- **Observer Pattern** - SSE client management
- **Caching Pattern** - Performance optimization

## Examples

### Monitoring Dashboard

```javascript
// Simple monitoring dashboard
async function fetchSystemStatus() {
  const response = await fetch('http://localhost:8080/api/system/status');
  const status = await response.json();

  document.getElementById('uptime').textContent = status.uptimeSeconds + 's';
  document.getElementById('activeProcessors').textContent = status.activeProcessors;
  document.getElementById('totalConnections').textContent = status.totalConnections;
}

// Refresh every 5 seconds
setInterval(fetchSystemStatus, 5000);
```

### Health Check Integration

```bash
# Kubernetes liveness probe
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10

# Readiness probe
readinessProbe:
  httpGet:
    path: /api/system/health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Prometheus Integration

The API can be extended to provide Prometheus metrics:

```go
// Example Prometheus metrics endpoint
api.HandleFunc("/metrics", promhttp.Handler())
```

## Security Considerations

- CORS is enabled by default with `Access-Control-Allow-Origin: *`
- For production, configure specific allowed origins
- Consider adding authentication/authorization
- Use HTTPS in production environments
- Implement rate limiting for public endpoints

## Future Enhancements

- [ ] Prometheus metrics export
- [ ] Grafana dashboard templates
- [ ] Historical metrics storage
- [ ] Alerting and notifications
- [ ] Custom metric aggregations
- [ ] Performance profiling endpoints
- [ ] Audit logging
