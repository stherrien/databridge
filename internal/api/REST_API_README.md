# DataBridge REST API

This document describes the comprehensive REST API layer built for DataBridge using the Gin framework.

## Overview

The REST API provides complete control over DataBridge flow management, including:
- Flow (Process Group) management
- Processor lifecycle management
- Connection management between processors
- Real-time status monitoring

## Architecture

### Directory Structure

```
/Users/shawntherrien/Projects/databridge/internal/api/
├── handlers/           # Request handlers for each resource
│   ├── flow_handler.go
│   ├── processor_handler.go
│   ├── connection_handler.go
│   └── handler_test.go
├── middleware/         # HTTP middleware (CORS, logging, error handling)
│   └── middleware.go
├── models/             # DTOs (Data Transfer Objects)
│   └── dto.go
├── server.go           # Main Gin server setup (new Gin-based implementation)
└── server_old.go       # Previous mux-based server (if present)
```

### Components

#### 1. Server (`server.go`)
Main API server implementation using Gin framework:
- Configurable host, port, and CORS settings
- Middleware stack (logging, recovery, CORS, error handling)
- Route registration
- Graceful shutdown support
- Swagger documentation endpoint

#### 2. Handlers
Each handler manages a specific resource:

**FlowHandler** (`handlers/flow_handler.go`):
- Manages process groups (flows)
- CRUD operations for flows
- Flow status monitoring

**ProcessorHandler** (`handlers/processor_handler.go`):
- Manages processors within flows
- CRUD operations
- Start/stop processor operations
- Status monitoring

**ConnectionHandler** (`handlers/connection_handler.go`):
- Manages connections between processors
- CRUD operations
- Queue management

#### 3. Middleware (`middleware/middleware.go`)
- **Logger**: Request/response logging with metrics
- **CORS**: Configurable cross-origin resource sharing
- **Recovery**: Panic recovery with proper error responses
- **ErrorHandler**: Centralized error handling
- **RequestID**: Unique request ID generation and tracking

#### 4. Models (`models/dto.go`)
Data Transfer Objects for API requests/responses:
- ProcessGroupDTO
- ProcessorDTO
- ConnectionDTO
- Status DTOs
- Request/Response models

## API Endpoints

### Flows (Process Groups)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/flows` | List all flows |
| POST | `/api/flows` | Create new flow |
| GET | `/api/flows/:id` | Get flow details |
| PUT | `/api/flows/:id` | Update flow |
| DELETE | `/api/flows/:id` | Delete flow |
| GET | `/api/flows/:id/status` | Get flow status |

### Processors

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/processors` | List all processors |
| POST | `/api/processors` | Create new processor |
| GET | `/api/processors/:id` | Get processor details |
| PUT | `/api/processors/:id` | Update processor |
| DELETE | `/api/processors/:id` | Delete processor |
| PUT | `/api/processors/:id/start` | Start processor |
| PUT | `/api/processors/:id/stop` | Stop processor |
| GET | `/api/processors/:id/status` | Get processor status |

### Connections

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/connections` | List all connections |
| POST | `/api/connections` | Create new connection |
| GET | `/api/connections/:id` | Get connection details |
| PUT | `/api/connections/:id` | Update connection |
| DELETE | `/api/connections/:id` | Delete connection |

### System

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/` | API root info |
| GET | `/swagger/*` | Swagger documentation |

## Configuration

### Via Command-line Flags

```bash
databridge \
  --api-host=0.0.0.0 \
  --api-port=8080 \
  --api-cors-origins="*" \
  --api-enable-swagger=true
```

### Via Configuration File (`databridge.yaml`)

```yaml
api:
  host: "0.0.0.0"
  port: 8080
  corsOrigins:
    - "*"
  enableSwagger: true
```

### Via Code

```go
import "github.com/shawntherrien/databridge/internal/api"

config := &api.Config{
    Host:           "0.0.0.0",
    Port:           8080,
    AllowedOrigins: []string{"*"},
    ReadTimeout:    10 * time.Second,
    WriteTimeout:   10 * time.Second,
    IdleTimeout:    60 * time.Second,
    EnableSwagger:  true,
    Mode:           gin.ReleaseMode,
}

apiServer := api.NewServer(flowController, logger, config)
apiServer.Start()
```

## Example Usage

### Create a Flow

```bash
curl -X POST http://localhost:8080/api/flows \
  -H "Content-Type: application/json" \
  -d '{"name": "My Data Flow"}'
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "My Data Flow",
  "processors": [],
  "connections": []
}
```

### List All Processors

```bash
curl http://localhost:8080/api/processors
```

Response:
```json
{
  "items": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440000",
      "name": "Generate Sample Data",
      "type": "GenerateFlowFile",
      "state": "RUNNING",
      "properties": {
        "File Size": "512"
      }
    }
  ],
  "total": 1
}
```

### Start a Processor

```bash
curl -X PUT http://localhost:8080/api/processors/660e8400-e29b-41d4-a716-446655440000/start
```

Response:
```json
{
  "success": true,
  "message": "Processor started successfully"
}
```

### Create a Connection

```bash
curl -X POST http://localhost:8080/api/connections \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Success Connection",
    "sourceId": "660e8400-e29b-41d4-a716-446655440000",
    "destinationId": "770e8400-e29b-41d4-a716-446655440000",
    "relationship": "success",
    "backPressureSize": 10000
  }'
```

## Testing

Comprehensive tests are provided in `handlers/handler_test.go`:

```bash
# Run all API tests
go test ./internal/api/handlers/...

# Run with verbose output
go test -v ./internal/api/handlers/...

# Run specific test
go test -v ./internal/api/handlers/ -run TestFlowHandlers
```

### Test Coverage

The test suite covers:
- All CRUD operations for flows, processors, and connections
- Start/stop processor operations
- Status endpoints
- Error cases (not found, invalid input, etc.)
- Proper HTTP status codes
- Response validation

## Error Handling

All errors are returned in a consistent format:

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": {
    "additional": "context"
  }
}
```

Common error codes:
- `invalid_id`: Invalid UUID format
- `not_found`: Resource not found
- `invalid_request`: Invalid request payload
- `creation_failed`: Failed to create resource
- `update_failed`: Failed to update resource
- `deletion_failed`: Failed to delete resource
- `start_failed`: Failed to start processor
- `stop_failed`: Failed to stop processor

## Integration with Main Application

The API server is integrated into `cmd/databridge/main.go`:

```go
// Create and start API server
apiConfig := &api.Config{
    Host:           viper.GetString("api.host"),
    Port:           viper.GetInt("api.port"),
    AllowedOrigins: viper.GetStringSlice("api.corsOrigins"),
    EnableSwagger:  viper.GetBool("api.enableSwagger"),
    Mode:           "release",
}

apiServer := api.NewServer(flowController, log, apiConfig)
if err := apiServer.Start(); err != nil {
    return fmt.Errorf("failed to start API server: %w", err)
}

// Graceful shutdown
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := apiServer.Stop(shutdownCtx); err != nil {
        log.WithError(err).Error("Error stopping API server")
    }
}()
```

## Swagger Documentation

When Swagger is enabled, interactive API documentation is available at:

```
http://localhost:8080/swagger/index.html
```

To generate Swagger docs:

```bash
# Install swag
go install github.com/swaggo/swag/cmd/swag@latest

# Generate docs
swag init -g cmd/databridge/main.go
```

## Security Considerations

1. **CORS**: Configure `AllowedOrigins` appropriately for production
2. **Rate Limiting**: Consider adding rate limiting middleware
3. **Authentication**: Add JWT or API key authentication middleware as needed
4. **Input Validation**: All inputs are validated using Gin's binding
5. **Error Messages**: Production errors don't expose internal details

## Performance

- **Gin Framework**: High-performance HTTP router
- **Concurrent Safe**: Thread-safe access to FlowController
- **Connection Pooling**: HTTP/2 support with proper timeouts
- **Graceful Shutdown**: Ensures in-flight requests complete

## Future Enhancements

Potential additions:
1. **WebSocket Support**: Real-time flow status updates
2. **Batch Operations**: Bulk processor start/stop
3. **Flow Templates**: Predefined flow configurations
4. **Metrics Export**: Prometheus/OpenMetrics endpoints
5. **GraphQL API**: Alternative to REST
6. **API Versioning**: Support for v2, v3, etc.
7. **Pagination**: For large result sets
8. **Filtering & Sorting**: Query parameters for list endpoints

## Dependencies

- **Gin**: HTTP web framework
- **Swaggo**: Swagger documentation generation
- **Logrus**: Structured logging
- **Viper**: Configuration management
- **Testify**: Testing utilities

## License

Part of the DataBridge project.
