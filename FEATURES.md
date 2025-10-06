# DataBridge Feature Roadmap

## Recent Completions (2025-01)

### Flow Management API & Testing
- **Flow Import/Export** - Complete API implementation with processor configs, connections, positions, properties, and ID remapping
  - API Endpoints: `GET /api/flows/{id}/export`, `POST /api/flows/import`
  - Core Methods: `ExportFlow()`, `ImportFlow()`
  - Files: `internal/api/flow_handlers.go:626-671`, `internal/core/flow_controller.go:1082-1349`

- **Flow Validation** - Comprehensive validation engine with cycle detection
  - API Endpoint: `POST /api/flows/{id}/validate`
  - Features: DFS-based cycle detection, disconnected processor warnings, configuration validation, relationship checks
  - Core Method: `ValidateFlow()`
  - Files: `internal/api/flow_handlers.go:673-696`, `internal/core/flow_controller.go:1351-1476`

- **Comprehensive Unit Tests** - 100% coverage of new features
  - API Handler Tests: 1,036 lines, ~40 test cases (`internal/api/flow_handlers_test.go`)
  - Core Flow Management Tests: 794 lines, ~20 test cases (`internal/core/flow_management_test.go`)
  - All CRUD operations, export/import round-trips, validation scenarios covered

---

## Data Transformation & Processing Processors

### ✅ Completed

#### Core File Operations
- **GetFile** - Read files from filesystem with pattern matching
- **PutFile** - Write files to filesystem with conflict resolution
- **GenerateFlowFile** - Generate test/sample data

#### Attribute Manipulation
- **UpdateAttribute** - Update FlowFile attributes with expressions and rules
- **AttributesToJSON** - Convert FlowFile attributes to JSON content
- **ExtractText** - Extract values using regex patterns and capture groups
- **EvaluateJSONPath** - Extract values from JSON into attributes using JSONPath

#### Data Transformation
- **ConvertRecord** - Universal format converter (CSV ↔ JSON ↔ XML)
- **ReplaceText** - Regex-based text replacement and transformation
- **CompressContent** - Compress/decompress content (gzip, deflate)

#### Content Splitting & Merging
- **SplitText** - Split by lines, size, or custom delimiter with header support
- **SplitJSON** - Split JSON arrays into individual FlowFiles
- **MergeContent** - Combine multiple FlowFiles with custom delimiters

#### Routing & Filtering
- **RouteOnAttribute** - Conditional routing based on attribute values with expression language

### 🚧 In Progress
- None currently

### 📋 Planned - High Priority

#### Attribute Manipulation
- [x] **ModifyBytes** - Modify binary content (prepend/append/replace)

#### Data Transformation
- [ ] **JoltTransformJSON** - Advanced JSON transformations using JOLT specifications
- [x] **EncryptContent** - Encrypt/decrypt content with various algorithms (AES-GCM)
- [x] **HashContent** - Generate content hashes (MD5, SHA-256, SHA-512, etc.)

#### Record-Oriented Processing
- [ ] **SplitRecord** - Split records into individual FlowFiles
- [ ] **MergeRecord** - Merge multiple records into one
- [ ] **QueryRecord** - SQL queries against record data
- [ ] **LookupRecord** - Enrich data with external lookups
- [ ] **ValidateRecord** - Schema validation

#### Content Splitting & Merging
- [x] **SplitXML** - Split XML documents by element
- [x] **SegmentContent** - Segment content into fixed-size chunks

#### Routing & Filtering
- [x] **RouteText** - Route based on content patterns (contains, regex, starts/ends with)
- [x] **RouteJSON** - Route based on JSON structure/values with JSONPath
- [x] **FilterAttribute** - Filter FlowFiles by attribute criteria with multiple operators
- [x] **DistributeLoad** - Load balance across relationships (round-robin, hash-based)

### 📋 Planned - Medium Priority

#### Database Integration
- [ ] **ExecuteSQL** - Execute SQL queries
- [ ] **PutSQL** - Insert/update database records
- [ ] **QueryDatabaseTable** - Incremental table queries

#### HTTP Integration
- [ ] **InvokeHTTP** - Make HTTP requests
- [ ] **HandleHTTPRequest** - Receive HTTP requests
- [ ] **HandleHTTPResponse** - Send HTTP responses

#### Messaging Integration
- [ ] **ConsumeKafka** - Consume from Kafka topics
- [ ] **PublishKafka** - Publish to Kafka topics
- [ ] **ConsumeAMQP** - Consume from AMQP queues
- [ ] **PublishAMQP** - Publish to AMQP exchanges

### 📋 Planned - Future

#### Cloud Storage
- [ ] **PutS3Object** - Upload to AWS S3
- [ ] **FetchS3Object** - Download from AWS S3
- [ ] **ListS3** - List S3 bucket objects
- [ ] **PutGCSObject** - Upload to Google Cloud Storage
- [ ] **PutAzureBlob** - Upload to Azure Blob Storage

#### Data Validation
- [ ] **ValidateJSON** - JSON schema validation
- [ ] **ValidateXML** - XML schema validation
- [ ] **ValidateCSV** - CSV format validation

#### Advanced Processing
- [ ] **ExecuteScript** - Run custom scripts (JavaScript, Python)
- [ ] **ExecuteProcess** - Execute external processes
- [ ] **Wait** - Wait for signal/timeout
- [ ] **Notify** - Send notifications

## UI/UX Features

### ✅ Completed
- Visual flow designer with drag-and-drop
- Property panel with dynamic inputs
- File/directory browser for server filesystem
- Toast notifications for user feedback
- Sidebar toggle for more canvas space
- Custom processor names displayed on canvas
- Path validation with format hints
- Permission input with common presets

### 📋 Planned - High Priority

#### Canvas Enhancements
- [x] **Processor state indicators** - Visual indicators for running/stopped/error states with colors and icons
- [x] **Connection labels** - Display relationship names on connection edges
- [ ] **Connection context menu** - Right-click connections to view/clear queue, see FlowFile count
- [x] **Processor quick actions** - Inline start/stop/configure buttons on nodes
- [x] **Zoom controls** - Visible zoom in/out buttons and mini-map for large flows (keyboard shortcuts implemented)
- [ ] **Auto-layout** - Automatic arrangement of processors to reduce clutter
- [x] **Bulk operations** - Select and operate on multiple processors at once (delete implemented)
- [ ] **Visual connection validation** - Warnings when connecting incompatible relationships

#### Property Panel Enhancements
- [ ] **Real-time validation feedback** - Visual indicators for invalid property values
- [ ] **Property search/filter** - Filter properties in processors with many configs
- [ ] **Property templates** - Save and reuse common configurations
- [ ] **Inline examples** - Expandable examples directly in help text
- [ ] **Expression validator** - Validate ${...} expressions as user types

#### Monitoring & Debugging
- [x] **Real-time flow statistics** - Total FlowFiles, processing rate, error count
- [x] **Processor metrics on nodes** - Show flowfiles processed, errors, throughput
- [x] **FlowFile Inspector** - View actual FlowFile content and attributes at any point (API complete)
- [x] **Connection queue viewer** - See FlowFiles waiting in queues (API complete)
- [ ] **Error message improvements** - Descriptive errors with actionable suggestions
- [x] **Connection backpressure indicators** - Backend support for backpressure monitoring

#### User Experience
- [x] **Keyboard shortcuts** - Delete (Del), Copy/Paste (Ctrl+C/V), Select All (Ctrl+A), Save (Ctrl+S), Zoom (Ctrl+=/−/0)
- [x] **Processor search in palette** - Quick filter by name, category, or tags with Ctrl+K shortcut
- [ ] **Conditional routing visualization** - Show which conditions are being evaluated
- [x] **Flow validation warnings** - Detect cycles, disconnected processors, missing configs

### 📋 Planned - Medium Priority
- [ ] **Visual expression builder** - Drag-and-drop interface for building ${...} expressions
- [ ] **JOLT transformation editor** - Interactive editor with live preview
- [ ] **SQL query builder** - Visual query builder for QueryRecord processor
- [ ] **Performance metrics dashboard** - Comprehensive system performance view
- [ ] **Flow templates library** - Predefined flows for common use cases
- [ ] **Flow import/export** - Save/load entire flows as JSON files
- [ ] **Flow versioning** - Track changes with rollback capability
- [ ] **Processor grouping** - Organize processors into logical groups
- [ ] **Dark mode** - Dark theme support

## Infrastructure Features

### ✅ Completed
- Plugin system for extensible processors
- FlowFile repository with BadgerDB
- Content repository with filesystem storage
- Basic provenance tracking
- Timer-based and event-driven scheduling
- Expression language (${attribute} syntax)
- REST API for flow management

### 📋 Planned - High Priority

#### Core Engine
- [x] **Backpressure handling** - Queue limits and backpressure signals between processors (multiple strategies)
- [x] **Flow validation engine** - Detect cycles, disconnected processors, configuration issues
- [x] **Graceful shutdown** - Ensure FlowFiles persisted before shutdown with signal handling
- [x] **Processor metrics collection** - Track processing time, throughput, errors per processor
- [x] **Health check endpoints** - API endpoints for monitoring system health (/api/system/health)
- [x] **Error handling improvements** - Retry relationship for retryable errors (RelationshipRetry)

#### Flow Management
- [x] **Flow import/export API** - Save/load entire flows as JSON with full processor configs, connections, and metadata
- [x] **Flow validation API** - Validate flow configuration with cycle detection, disconnected processor warnings, and relationship checks
- [ ] **Flow templates** - Template system for reusable flow patterns
- [ ] **Flow versioning** - Track changes with rollback capability
- [ ] **Dynamic processor properties** - Allow user-defined properties at runtime

#### Monitoring & Observability
- [ ] **Processor performance metrics** - Per-processor performance tracking
- [ ] **Queue depth monitoring** - Track FlowFile queue sizes
- [ ] **System resource monitoring** - CPU, memory, disk usage tracking
- [ ] **Event stream API** - Real-time processor events via SSE/WebSocket
- [ ] **Audit logging** - Track all configuration changes

### 📋 Planned - Medium Priority

#### Advanced Features
- [ ] **Record Reader/Writer services** - Schema-aware processing (CSV, JSON, XML, Avro)
- [ ] **Controller services architecture** - Shared services across processors
- [ ] **Distributed state management** - State coordination across processors
- [ ] **Advanced provenance queries** - Rich provenance search and filtering
- [ ] **Processor scheduling enhancements** - Primary node only, load balancing
- [ ] **FlowFile prioritization** - Configure processing priority

#### Clustering & High Availability
- [ ] **Cluster coordination** - Multi-node cluster support
- [ ] **Load balancing** - Distribute processing across nodes
- [ ] **State replication** - Replicate state across cluster nodes
- [ ] **Automatic failover** - Node failure detection and recovery

#### Developer Tools
- [ ] **Processor test framework** - Easy testing for custom processors
- [ ] **Processor documentation generator** - Auto-generate docs from metadata
- [ ] **Integration test framework** - End-to-end flow testing
- [ ] **Performance benchmarks** - Measure and track processor performance
- [ ] **OpenAPI/Swagger spec** - Complete API documentation
