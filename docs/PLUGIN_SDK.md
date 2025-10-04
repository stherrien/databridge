# DataBridge Plugin SDK

## Overview

The DataBridge Plugin SDK allows developers to create custom processors, controllers, and reporting components that seamlessly integrate with the DataBridge data flow processing engine. This document provides comprehensive guidance on developing, testing, and distributing plugins.

## Table of Contents

1. [Getting Started](#getting-started)
2. [Plugin Architecture](#plugin-architecture)
3. [Creating a Custom Processor](#creating-a-custom-processor)
4. [Built-in vs. Dynamic Plugins](#built-in-vs-dynamic-plugins)
5. [Plugin Manifest](#plugin-manifest)
6. [Testing Plugins](#testing-plugins)
7. [Packaging and Distribution](#packaging-and-distribution)
8. [Best Practices](#best-practices)
9. [API Reference](#api-reference)

---

## Getting Started

### Prerequisites

- Go 1.19 or higher
- DataBridge source code or binary
- Basic understanding of Go interfaces and packages

### Quick Start

The fastest way to create a plugin is to register it as a built-in processor:

```go
package myplugin

import (
    "context"
    "github.com/shawntherrien/databridge/internal/plugin"
    "github.com/shawntherrien/databridge/pkg/types"
)

func init() {
    info := plugin.NewProcessorInfo(
        "MyProcessor",
        "My Custom Processor",
        "1.0.0",
        "Your Name",
        "Processes FlowFiles in a custom way",
        []string{"custom", "transform"},
    )

    plugin.RegisterBuiltInProcessor("MyProcessor", func() types.Processor {
        return NewMyProcessor()
    }, info)
}

type MyProcessor struct {
    *types.BaseProcessor
}

func NewMyProcessor() *MyProcessor {
    info := types.ProcessorInfo{
        Name:        "MyProcessor",
        Description: "My custom processor implementation",
        Version:     "1.0.0",
        Author:      "Your Name",
        Tags:        []string{"custom", "transform"},
        Properties:  []types.PropertySpec{
            {
                Name:         "Property1",
                Description:  "First property",
                Required:     true,
                DefaultValue: "default",
            },
        },
        Relationships: []types.Relationship{
            types.RelationshipSuccess,
            types.RelationshipFailure,
        },
    }

    return &MyProcessor{
        BaseProcessor: types.NewBaseProcessor(info),
    }
}

func (p *MyProcessor) Initialize(ctx types.ProcessorContext) error {
    // Initialization logic
    return nil
}

func (p *MyProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
    // Get a FlowFile from input
    flowFile := session.Get()
    if flowFile == nil {
        return nil
    }

    // Process the FlowFile
    // ... your custom logic here ...

    // Transfer to success relationship
    session.Transfer(flowFile, types.RelationshipSuccess)

    return nil
}

func (p *MyProcessor) OnStopped(ctx context.Context) {
    // Cleanup logic
}
```

---

## Plugin Architecture

### Plugin Types

DataBridge supports three types of plugins:

1. **Processor Plugins**: Transform, route, or generate FlowFiles
2. **Controller Plugins**: Provide shared services (e.g., connection pools)
3. **Reporting Plugins**: Generate analytics and reports

### Plugin Lifecycle

```
Registration -> Validation -> Loading -> Initialization -> Active -> Stopped -> Unloaded
```

1. **Registration**: Plugin is registered with the plugin registry
2. **Validation**: Plugin manifest and structure are validated
3. **Loading**: Plugin code is loaded into memory
4. **Initialization**: `Initialize()` is called with processor context
5. **Active**: Plugin processes FlowFiles via `OnTrigger()`
6. **Stopped**: `OnStopped()` is called for cleanup
7. **Unloaded**: Plugin is removed from memory

### Component Relationships

```
PluginManager
    ├── PluginRegistry (tracks all plugins)
    ├── PluginLoader (loads dynamic plugins)
    ├── BuiltInRegistryManager (manages built-in plugins)
    ├── PluginValidator (validates plugins)
    ├── NamespaceManager (resource isolation)
    └── ResourceMonitor (monitors resource usage)
```

---

## Creating a Custom Processor

### Step 1: Define Processor Interface

All processors must implement the `types.Processor` interface:

```go
type Processor interface {
    Initialize(ctx ProcessorContext) error
    OnTrigger(ctx context.Context, session ProcessSession) error
    GetInfo() ProcessorInfo
    Validate(config ProcessorConfig) []ValidationResult
    OnStopped(ctx context.Context)
}
```

### Step 2: Implement Required Methods

#### Initialize

Called once when the processor is created:

```go
func (p *MyProcessor) Initialize(ctx types.ProcessorContext) error {
    logger := ctx.GetLogger()
    logger.Info("Initializing MyProcessor")

    // Validate required properties
    if !ctx.HasProperty("RequiredProperty") {
        return fmt.Errorf("RequiredProperty is missing")
    }

    // Initialize internal state
    // ...

    return nil
}
```

#### OnTrigger

Called to process FlowFiles:

```go
func (p *MyProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
    // Get processor context
    processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
    if !ok {
        return fmt.Errorf("processor context not available")
    }

    logger := session.GetLogger()

    // Get FlowFile(s)
    flowFile := session.Get()
    if flowFile == nil {
        return nil // No work to do
    }

    // Read properties
    myProperty := processorCtx.GetPropertyValue("MyProperty")

    // Read FlowFile content
    content, err := session.Read(flowFile)
    if err != nil {
        logger.Error("Failed to read FlowFile", "error", err)
        session.Transfer(flowFile, types.RelationshipFailure)
        return err
    }

    // Process content
    processedContent := processData(content, myProperty)

    // Write modified content
    if err := session.Write(flowFile, processedContent); err != nil {
        logger.Error("Failed to write FlowFile", "error", err)
        session.Transfer(flowFile, types.RelationshipFailure)
        return err
    }

    // Update attributes
    session.PutAttribute(flowFile, "processed.timestamp", time.Now().Format(time.RFC3339))

    // Transfer to success
    session.Transfer(flowFile, types.RelationshipSuccess)

    return nil
}
```

#### Validate

Validates processor configuration:

```go
func (p *MyProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
    results := p.BaseProcessor.Validate(config)

    // Custom validation
    if value, exists := config.Properties["MyProperty"]; exists {
        if len(value) == 0 {
            results = append(results, types.ValidationResult{
                Property: "MyProperty",
                Valid:    false,
                Message:  "MyProperty cannot be empty",
            })
        }
    }

    return results
}
```

### Step 3: Define Properties

Properties allow users to configure your processor:

```go
Properties: []types.PropertySpec{
    {
        Name:          "Input Format",
        Description:   "Format of input data",
        Required:      true,
        DefaultValue:  "json",
        AllowedValues: []string{"json", "xml", "csv"},
    },
    {
        Name:        "Output Directory",
        Description: "Directory for output files",
        Required:    true,
        Pattern:     `^/.*`,
    },
    {
        Name:        "Buffer Size",
        Description: "Size of processing buffer in KB",
        Required:    false,
        DefaultValue: "1024",
        Pattern:     `^\d+$`,
    },
},
```

### Step 4: Define Relationships

Relationships determine how FlowFiles are routed:

```go
Relationships: []types.Relationship{
    types.RelationshipSuccess,  // Successfully processed
    types.RelationshipFailure,  // Processing failed
    {
        Name:        "invalid",
        Description: "FlowFiles that failed validation",
    },
    {
        Name:        "retry",
        Description: "FlowFiles to retry processing",
    },
},
```

---

## Built-in vs. Dynamic Plugins

### Built-in Plugins

Built-in plugins are compiled directly into the DataBridge binary.

**Advantages:**
- No dynamic loading overhead
- Type-safe at compile time
- Easier to debug
- Better performance

**Registration:**

```go
func init() {
    info := plugin.NewProcessorInfo(
        "MyProcessor",
        "My Processor",
        "1.0.0",
        "Author",
        "Description",
        []string{"tag1", "tag2"},
    )

    plugin.RegisterBuiltInProcessor("MyProcessor", func() types.Processor {
        return NewMyProcessor()
    }, info)
}
```

### Dynamic Plugins

Dynamic plugins are loaded at runtime from `.so` files (Linux/Mac).

**Advantages:**
- Can be updated without recompiling DataBridge
- Third-party distribution
- Hot reload capability

**Limitations:**
- Platform-specific (`.so` files)
- More complex deployment
- Potential version compatibility issues

**Creating a Dynamic Plugin:**

1. Create plugin directory structure:
```
my-plugin/
├── manifest.json
├── processor.go
└── plugin.so (built)
```

2. Implement the plugin with exported symbols:

```go
package main

import "github.com/shawntherrien/databridge/pkg/types"

// Exported variable for plugin info
var PluginInfo = plugin.PluginInfo{
    ID:          "MyDynamicProcessor",
    Name:        "My Dynamic Processor",
    Version:     "1.0.0",
    Author:      "Your Name",
    Description: "A dynamically loaded processor",
    Type:        plugin.PluginTypeProcessor,
}

// Exported function to create processor instance
func NewProcessor() types.Processor {
    return &MyDynamicProcessor{}
}

type MyDynamicProcessor struct {
    // ... implementation ...
}
```

3. Build as plugin:
```bash
go build -buildmode=plugin -o plugin.so processor.go
```

---

## Plugin Manifest

For dynamic plugins, create a `manifest.json` file:

```json
{
  "id": "my-custom-processor",
  "name": "My Custom Processor",
  "version": "1.0.0",
  "author": "Your Name",
  "description": "Detailed description of what this processor does",
  "type": "processor",
  "entryPoint": "plugin.so",
  "dependencies": [],
  "tags": ["transform", "custom"],
  "config": {
    "maxMemoryMB": "256",
    "maxCPUPercent": "50"
  },
  "minVersion": "1.0.0",
  "requiredFeatures": []
}
```

### Manifest Fields

- **id**: Unique identifier (required)
- **name**: Human-readable name (required)
- **version**: Semantic version (required)
- **author**: Plugin author
- **description**: Detailed description
- **type**: Plugin type (processor, controller, reporting)
- **entryPoint**: Path to .so file for dynamic plugins
- **dependencies**: List of required plugin IDs
- **tags**: Search tags
- **config**: Plugin-specific configuration
- **minVersion**: Minimum DataBridge version
- **requiredFeatures**: Required DataBridge features

---

## Testing Plugins

### Unit Testing

```go
package myplugin

import (
    "context"
    "testing"
    "github.com/shawntherrien/databridge/pkg/types"
)

func TestMyProcessor_OnTrigger(t *testing.T) {
    // Create processor
    processor := NewMyProcessor()

    // Create mock context
    config := types.ProcessorConfig{
        Properties: map[string]string{
            "MyProperty": "test-value",
        },
    }
    ctx := &mockProcessorContext{config: config}

    // Initialize
    if err := processor.Initialize(ctx); err != nil {
        t.Fatalf("Failed to initialize: %v", err)
    }

    // Create mock session
    session := &mockProcessSession{}

    // Test processing
    err := processor.OnTrigger(context.Background(), session)
    if err != nil {
        t.Errorf("OnTrigger failed: %v", err)
    }

    // Verify results
    // ...
}

// Mock implementations
type mockProcessorContext struct {
    config types.ProcessorConfig
}

func (m *mockProcessorContext) GetProperty(name string) (string, bool) {
    value, exists := m.config.Properties[name]
    return value, exists
}

// ... implement other required methods ...
```

### Integration Testing

Test your plugin with the actual DataBridge flow controller:

```go
func TestMyProcessor_Integration(t *testing.T) {
    // Setup DataBridge components
    logger := logrus.New()
    flowFileRepo := core.NewInMemoryFlowFileRepository()
    contentRepo := core.NewInMemoryContentRepository()
    provenanceRepo := core.NewInMemoryProvenanceRepository()

    // Initialize plugin manager
    pluginManager, _ := plugin.NewPluginManager(plugin.DefaultPluginManagerConfig(), logger)
    pluginManager.Initialize()

    // Create flow controller
    fc := core.NewFlowControllerWithPlugins(
        flowFileRepo,
        contentRepo,
        provenanceRepo,
        pluginManager,
        logger,
    )

    // Add and test your processor
    config := types.ProcessorConfig{
        ID:   uuid.New(),
        Name: "Test Processor",
        Type: "MyProcessor",
        Properties: map[string]string{
            "MyProperty": "test",
        },
    }

    node, err := fc.CreateProcessorByType("MyProcessor", config)
    if err != nil {
        t.Fatalf("Failed to create processor: %v", err)
    }

    // Test processing
    // ...
}
```

---

## Packaging and Distribution

### For Built-in Plugins

1. Add your plugin package to `plugins/` directory
2. Import in `cmd/databridge/main.go`:
   ```go
   _ "github.com/shawntherrien/databridge/plugins/myplugin"
   ```
3. Rebuild DataBridge

### For Dynamic Plugins

1. Build your plugin:
   ```bash
   go build -buildmode=plugin -o plugin.so
   ```

2. Create directory structure:
   ```
   my-plugin/
   ├── manifest.json
   ├── plugin.so
   ├── README.md
   └── LICENSE
   ```

3. Optional: Create checksum file:
   ```bash
   sha256sum plugin.so > checksum.txt
   ```

4. Distribute as archive:
   ```bash
   tar czf my-plugin-1.0.0.tar.gz my-plugin/
   ```

### Installation

Users can install your plugin by:

1. **Manual installation:**
   ```bash
   mkdir -p $DATABRIDGE_HOME/plugins/my-plugin
   tar xzf my-plugin-1.0.0.tar.gz -C $DATABRIDGE_HOME/plugins/
   ```

2. **REST API:**
   ```bash
   curl -X POST http://localhost:8080/api/plugins/install \
     -H "Content-Type: application/json" \
     -d '{"source": "/path/to/my-plugin", "validate": true}'
   ```

---

## Best Practices

### Performance

1. **Minimize OnTrigger overhead**: Keep OnTrigger fast, move heavy initialization to Initialize()
2. **Batch processing**: Process multiple FlowFiles when possible:
   ```go
   flowFiles := session.GetBatch(10)
   for _, ff := range flowFiles {
       // Process
   }
   ```
3. **Avoid blocking**: Don't block in OnTrigger unless necessary
4. **Resource cleanup**: Always clean up resources in OnStopped()

### Error Handling

1. **Route errors appropriately**:
   ```go
   if err != nil {
       logger.Error("Processing failed", "error", err)
       session.Transfer(flowFile, types.RelationshipFailure)
       return err
   }
   ```

2. **Validate inputs early**: Check configuration in Initialize() and Validate()
3. **Log meaningful messages**: Include context in log messages

### Memory Management

1. **Process incrementally**: Don't load entire FlowFile content if not needed
2. **Use streaming**: For large files, use streaming APIs
3. **Clean up**: Remove temporary data structures after use

### Security

1. **Validate inputs**: Never trust user-provided data
2. **Limit resource usage**: Set reasonable limits in manifest
3. **Avoid sensitive data in logs**: Don't log passwords, keys, etc.
4. **Use secure defaults**: Default to secure configurations

### Testing

1. **Test edge cases**: Empty inputs, large inputs, invalid inputs
2. **Test concurrency**: Ensure thread-safety
3. **Test failure scenarios**: Network failures, disk full, etc.
4. **Performance testing**: Benchmark critical paths

---

## API Reference

### Core Interfaces

#### types.Processor

Main processor interface that all processors must implement.

#### types.ProcessSession

Provides transactional access to FlowFiles:

- `Get()` - Get single FlowFile
- `GetBatch(int)` - Get multiple FlowFiles
- `Create()` - Create new FlowFile
- `Transfer(flowFile, relationship)` - Route FlowFile
- `Remove(flowFile)` - Delete FlowFile
- `Read(flowFile)` - Read content
- `Write(flowFile, content)` - Write content
- `PutAttribute(flowFile, key, value)` - Set attribute

#### types.ProcessorContext

Access to processor configuration:

- `GetProperty(name)` - Get property with existence check
- `GetPropertyValue(name)` - Get property value
- `HasProperty(name)` - Check property existence
- `GetLogger()` - Get logger instance

### Plugin Management

#### plugin.RegisterBuiltInProcessor

Register a built-in processor:

```go
func RegisterBuiltInProcessor(
    id string,
    factory ProcessorFactory,
    info PluginInfo,
)
```

#### plugin.NewProcessorInfo

Create plugin information:

```go
func NewProcessorInfo(
    id, name, version, author, description string,
    tags []string,
) PluginInfo
```

### REST API Endpoints

#### List Plugins
- **GET** `/api/plugins`
- Returns all registered plugins

#### Get Plugin
- **GET** `/api/plugins/{id}`
- Returns specific plugin details

#### Enable Plugin
- **PUT** `/api/plugins/{id}/enable`
- Enables a plugin

#### Disable Plugin
- **PUT** `/api/plugins/{id}/disable`
- Disables a plugin

#### Install Plugin
- **POST** `/api/plugins/install`
- Install plugin from path or URL

#### Reload Plugin
- **POST** `/api/plugins/{id}/reload`
- Reloads a plugin

#### Uninstall Plugin
- **DELETE** `/api/plugins/{id}`
- Uninstalls a plugin

#### Search Plugins
- **GET** `/api/plugins/search?q=query&type=processor&tags=tag1,tag2`
- Search for plugins

---

## Examples

See the following examples in the repository:

- `plugins/generate_flowfile.go` - Complete built-in processor example
- `examples/custom-transform/` - Custom transformation processor
- `examples/external-api/` - Processor that calls external APIs
- `examples/dynamic-plugin/` - Dynamic plugin example

---

## Support

For issues, questions, or contributions:

- GitHub Issues: https://github.com/yourusername/databridge/issues
- Documentation: https://databridge.readthedocs.io
- Community Forum: https://community.databridge.io

---

## License

This SDK and DataBridge are released under the MIT License. See LICENSE file for details.
