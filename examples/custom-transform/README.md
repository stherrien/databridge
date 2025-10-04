# Custom Transform Processor Example

This is a complete example of a custom DataBridge processor that demonstrates best practices for plugin development.

## Overview

The Custom Transform Processor transforms FlowFile content and attributes using configurable rules. It supports multiple transformation types including:

- **uppercase**: Convert to uppercase
- **lowercase**: Convert to lowercase
- **replace**: Replace strings
- **prefix**: Add prefix to content
- **suffix**: Add suffix to content
- **json-pretty**: Pretty-print JSON content

## Features Demonstrated

1. **Built-in Plugin Registration**: Shows how to register a processor as a built-in plugin
2. **Property Configuration**: Multiple configurable properties with validation
3. **Content Transformation**: Reading, transforming, and writing FlowFile content
4. **Attribute Manipulation**: Working with FlowFile attributes
5. **Error Handling**: Proper error handling and routing to failure relationships
6. **Logging**: Structured logging with context
7. **Validation**: Configuration validation with detailed error messages
8. **Multiple Relationships**: Success, failure, and custom (invalid) relationships

## Configuration

### Properties

- **Transform Type** (required): Type of transformation
  - Values: uppercase, lowercase, replace, prefix, suffix, json-pretty
  - Default: uppercase

- **Replace Target** (optional): String to replace (for 'replace' type)

- **Replace Value** (optional): Replacement string

- **Transform Content** (optional): Whether to transform content
  - Values: true, false
  - Default: true

- **Transform Attributes** (optional): Whether to transform attributes
  - Values: true, false
  - Default: false

- **Attribute Pattern** (optional): Only transform matching attributes

### Relationships

- **success**: Successfully transformed FlowFiles
- **failure**: Processing failed
- **invalid**: Invalid content (e.g., malformed JSON)

## Usage Example

### Example 1: Convert to Uppercase

```go
config := types.ProcessorConfig{
    ID:   uuid.New(),
    Name: "Uppercase Transform",
    Type: "CustomTransform",
    Properties: map[string]string{
        "Transform Type":    "uppercase",
        "Transform Content": "true",
    },
}
```

Input: "hello world"
Output: "HELLO WORLD"

### Example 2: Replace Text

```go
config := types.ProcessorConfig{
    ID:   uuid.New(),
    Name: "Replace Transform",
    Type: "CustomTransform",
    Properties: map[string]string{
        "Transform Type":    "replace",
        "Replace Target":    "foo",
        "Replace Value":     "bar",
        "Transform Content": "true",
    },
}
```

Input: "foo is great, foo is awesome"
Output: "bar is great, bar is awesome"

### Example 3: Pretty Print JSON

```go
config := types.ProcessorConfig{
    ID:   uuid.New(),
    Name: "JSON Pretty Print",
    Type: "CustomTransform",
    Properties: map[string]string{
        "Transform Type":    "json-pretty",
        "Transform Content": "true",
    },
}
```

Input: `{"name":"John","age":30}`
Output:
```json
{
  "name": "John",
  "age": 30
}
```

### Example 4: Transform Attributes

```go
config := types.ProcessorConfig{
    ID:   uuid.New(),
    Name: "Attribute Transform",
    Type: "CustomTransform",
    Properties: map[string]string{
        "Transform Type":       "uppercase",
        "Transform Content":    "false",
        "Transform Attributes": "true",
        "Attribute Pattern":    "custom.",
    },
}
```

Transforms only attributes containing "custom." in their names.

## Integration

### As Built-in Plugin

1. Copy this package to `plugins/custom-transform/`

2. Import in your main package:
```go
import _ "github.com/shawntherrien/databridge/plugins/custom-transform"
```

3. Rebuild DataBridge:
```bash
go build ./cmd/databridge
```

### Using in Flow

```go
// Create processor using plugin manager
node, err := flowController.CreateProcessorByType("CustomTransform", config)
if err != nil {
    log.Fatal(err)
}

// Start the processor
if err := flowController.StartProcessor(node.ID); err != nil {
    log.Fatal(err)
}
```

### REST API

```bash
# List available plugins
curl http://localhost:8080/api/plugins

# Get CustomTransform info
curl http://localhost:8080/api/plugins/CustomTransform

# Create processor via API
curl -X POST http://localhost:8080/api/processors \
  -H "Content-Type: application/json" \
  -d '{
    "type": "CustomTransform",
    "name": "My Transform",
    "properties": {
      "Transform Type": "uppercase"
    }
  }'
```

## Testing

Run the included tests:

```bash
go test -v ./examples/custom-transform/
```

## Key Implementation Details

### Initialization

The `Initialize()` method validates required configuration:

```go
func (p *CustomTransformProcessor) Initialize(ctx types.ProcessorContext) error {
    if !ctx.HasProperty("Transform Type") {
        return fmt.Errorf("Transform Type property is required")
    }
    // ... additional validation
    return nil
}
```

### Processing

The `OnTrigger()` method handles the core transformation logic:

```go
func (p *CustomTransformProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
    flowFile := session.Get()
    if flowFile == nil {
        return nil
    }

    // Read content
    content, err := session.Read(flowFile)

    // Transform
    transformed, err := p.transformData(content, ...)

    // Write back
    session.Write(flowFile, transformed)

    // Route
    session.Transfer(flowFile, types.RelationshipSuccess)

    return nil
}
```

### Validation

Configuration is validated before the processor starts:

```go
func (p *CustomTransformProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
    results := p.BaseProcessor.Validate(config)

    // Custom validation logic
    if transformType == "replace" {
        if target == "" {
            results = append(results, types.ValidationResult{
                Property: "Replace Target",
                Valid:    false,
                Message:  "Required for replace type",
            })
        }
    }

    return results
}
```

## Best Practices Demonstrated

1. **Clear Property Definitions**: Properties have descriptions, defaults, and allowed values
2. **Comprehensive Validation**: Early validation prevents runtime errors
3. **Structured Logging**: Context-rich log messages for debugging
4. **Error Handling**: Errors routed to appropriate relationships
5. **Metadata**: Transformation metadata added as attributes
6. **Flexible Configuration**: Multiple options for different use cases
7. **Documentation**: Extensive inline and external documentation

## Extending This Example

You can extend this processor by:

1. Adding new transformation types
2. Supporting regex patterns for replacement
3. Adding conditional transformations
4. Supporting transformation chains
5. Adding performance metrics
6. Supporting streaming for large files

## License

MIT License - See LICENSE file for details
