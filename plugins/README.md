# DataBridge Built-in Processors

This directory contains the built-in processors for DataBridge. These processors provide essential data flow operations for ingesting, transforming, and routing data.

## Available Processors

### 1. GetFile

**Purpose:** File system ingestion with pattern matching and batch processing.

**Description:** Scans a directory for files matching specified criteria and creates FlowFiles from their content. Supports recursive directory scanning, file age filtering, and configurable batch processing.

**Properties:**
- `Input Directory` (required) - Directory to scan for files
- `File Filter` (default: "*") - Glob pattern for file matching (e.g., "*.txt", "*.json")
- `Keep Source File` (default: false) - Whether to keep or delete source file after ingestion
- `Recurse Subdirectories` (default: false) - Whether to scan subdirectories
- `Minimum File Age` (default: "0s") - Minimum age before file is picked up (e.g., "10s", "1m")
- `Maximum File Age` (default: "0s") - Maximum age (0 = no limit)
- `Batch Size` (default: 10) - Max files to process per execution

**Relationships:**
- `success` - Files successfully ingested
- `failure` - Failed to read file

**Attributes Added:**
- `filename` - Name of the file
- `path` - Directory path
- `absolute.path` - Full path to the file
- `file.size` - Size in bytes
- `file.lastModified` - Last modification timestamp (RFC3339)
- `file.permissions` - File permissions string

**Example Use Cases:**
- Ingest log files from a directory
- Pick up CSV files for processing
- Monitor a drop directory for incoming data files

---

### 2. PutFile

**Purpose:** Write FlowFiles to the file system with configurable conflict resolution.

**Description:** Writes FlowFile content to files on disk. Supports multiple conflict resolution strategies, automatic directory creation, and file permission control.

**Properties:**
- `Directory` (required) - Output directory where files will be written
- `Conflict Resolution Strategy` (default: "fail") - How to handle existing files
  - `fail` - Fail if file exists
  - `replace` - Overwrite existing file
  - `ignore` - Skip if file exists
  - `rename` - Add numeric suffix to create unique filename
- `Create Missing Directories` (default: true) - Auto-create directories if they don't exist
- `Permissions` (default: "0644") - File permissions in octal format
- `Maximum File Count` (default: -1) - Max files in directory (-1 = unlimited)

**Relationships:**
- `success` - File written successfully
- `failure` - Failed to write file

**Attributes Added:**
- `absolute.path` - Full path to written file
- `filename` - Name of the written file
- `path` - Directory path

**Example Use Cases:**
- Export processed data to files
- Archive data to filesystem
- Create report files

---

### 3. InvokeHTTP

**Purpose:** HTTP client for REST API interactions.

**Description:** Makes HTTP requests to external APIs or services. Supports all standard HTTP methods, authentication, custom headers, and intelligent response routing based on status codes.

**Properties:**
- `HTTP Method` (required) - HTTP method: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS
- `Remote URL` (required) - Target URL for the request
- `Connection Timeout` (default: "5s") - Timeout for establishing connection
- `Read Timeout` (default: "30s") - Timeout for reading response
- `Send Message Body` (default: true) - Send FlowFile content as request body
- `Attributes to Send` (default: "") - Comma-separated attribute names to send as headers (e.g., "request-id,user-id")
- `Follow Redirects` (default: true) - Automatically follow HTTP redirects
- `Basic Auth Username` (optional) - Username for basic authentication
- `Basic Auth Password` (optional, sensitive) - Password for basic authentication
- `Content-Type` (default: "application/octet-stream") - Content-Type header for request

**Relationships:**
- `success` - 2xx status codes (successful)
- `failure` - Connection errors, timeouts
- `retry` - 5xx status codes (server errors, retryable)
- `no retry` - 4xx status codes (client errors, not retryable)

**Attributes Added:**
- `http.status.code` - HTTP response status code
- `http.status.message` - HTTP status message
- `http.response.time` - Request duration
- `http.remote.url` - Target URL
- `http.method` - HTTP method used
- `http.header.*` - Response headers (lowercased)

**Example Use Cases:**
- Call REST APIs
- Submit data to webhooks
- Query external services
- Integrate with cloud services

---

### 4. TransformJSON

**Purpose:** JSON transformation using simplified JSONPath expressions.

**Description:** Extracts and transforms JSON data using path expressions. Supports nested objects, arrays, and flexible output formatting.

**Properties:**
- `JSON Path Expression` (required) - Simplified JSONPath (e.g., ".data.items[0].name")
  - Use dot notation for object properties: `.user.name`
  - Use brackets for array access: `.items[0]`
  - Use `[*]` for array wildcard: `.users[*]`
  - Use `.` for root object
- `Destination` (default: "flowfile-content") - Where to put result
  - `flowfile-content` - Replace FlowFile content with result
  - `flowfile-attribute` - Store result in an attribute
- `Destination Attribute` (default: "json.result") - Attribute name when destination is flowfile-attribute
- `Return Type` (default: "auto") - How to format the result
  - `auto` - String values as-is, objects/arrays as JSON
  - `json` - Always return as JSON
  - `string` - Convert to string representation

**Relationships:**
- `success` - Transformation successful
- `failure` - Invalid JSON or path error

**Attributes Added:**
- `json.path` - The JSONPath expression used
- `json.destination` - Where the result was written
- `json.error` - Error message (on failure)

**Example Use Cases:**
- Extract specific fields from JSON responses
- Transform JSON structure
- Filter JSON arrays
- Parse API responses

---

### 5. SplitText

**Purpose:** Split text content by delimiter or line count into multiple FlowFiles.

**Description:** Divides text content into multiple smaller FlowFiles based on line count. Supports headers that are prepended to each split, size limits, and configurable line ending handling.

**Properties:**
- `Line Split Count` (default: 1) - Number of lines per split
- `Remove Trailing Newlines` (default: true) - Remove trailing newlines from splits
- `Header Line Count` (default: 0) - Number of header lines to prepend to each split
- `Maximum Fragment Size` (default: 0) - Max size in bytes per fragment (0 = unlimited)

**Relationships:**
- `success` - Original FlowFile (after splitting)
- `splits` - Individual split FlowFiles
- `failure` - Error reading or splitting content

**Attributes Added (to splits):**
- `fragment.index` - Zero-based index of this fragment
- `fragment.count` - Total number of fragments created
- `fragment.identifier` - UUID of the parent FlowFile
- `segment.original.filename` - Original filename
- `filename` - Generated filename with split index

**Example Use Cases:**
- Split large log files into smaller chunks
- Process CSV files line by line
- Divide batch files for parallel processing
- Split text data for distributed processing

---

## Processor Development

To create a new processor:

1. Implement the `types.Processor` interface:
   - `Initialize(ctx ProcessorContext) error`
   - `OnTrigger(ctx context.Context, session ProcessSession) error`
   - `GetInfo() ProcessorInfo`
   - `Validate(config ProcessorConfig) []ValidationResult`
   - `OnStopped(ctx context.Context)`

2. Extend `types.BaseProcessor` for common functionality

3. Define processor metadata in `ProcessorInfo`:
   - Name, Description, Version, Author
   - Properties with validation rules
   - Relationships for routing FlowFiles

4. Create comprehensive tests with 90%+ coverage

5. Document the processor in this README

## Testing

Run all processor tests:
```bash
go test ./plugins/... -v
```

Run tests with coverage:
```bash
go test ./plugins/... -cover
```

Run specific processor tests:
```bash
go test ./plugins/... -run TestGetFile -v
```

## Common Patterns

### Reading FlowFile Content
```go
content, err := session.Read(flowFile)
if err != nil {
    session.Transfer(flowFile, types.RelationshipFailure)
    return nil
}
```

### Writing FlowFile Content
```go
if err := session.Write(flowFile, []byte(newContent)); err != nil {
    session.Transfer(flowFile, types.RelationshipFailure)
    return nil
}
```

### Setting Attributes
```go
session.PutAttribute(flowFile, "attribute.name", "value")
```

### Creating Child FlowFiles
```go
child := session.CreateChild(parentFlowFile)
session.Write(child, childContent)
session.Transfer(child, types.RelationshipSuccess)
```

### Error Handling
```go
if err != nil {
    logger.Error("Operation failed", "error", err)
    session.PutAttribute(flowFile, "error.message", err.Error())
    session.Transfer(flowFile, types.RelationshipFailure)
    return nil
}
```

## Performance Considerations

- **GetFile**: Use appropriate batch sizes to balance throughput and memory usage
- **PutFile**: Consider file count limits to avoid filling up disk
- **InvokeHTTP**: Set appropriate timeouts based on expected API response times
- **TransformJSON**: Large JSON documents may require more memory
- **SplitText**: Large files will create many FlowFiles; consider memory implications

## Version History

- **v1.0.0** - Initial release with 5 essential processors
  - GetFile: File system ingestion
  - PutFile: File system output
  - InvokeHTTP: HTTP client
  - TransformJSON: JSON transformation
  - SplitText: Text splitting
