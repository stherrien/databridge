# DataBridge Plugin Management System

## Overview

The DataBridge Plugin Management System provides a comprehensive framework for dynamically loading, managing, and executing plugins. It supports both statically compiled (built-in) and dynamically loaded plugins with features including resource isolation, validation, and hot-reloading.

## Architecture

### Core Components

```
plugin/
├── registry.go       - Central plugin registry
├── loader.go         - Dynamic plugin loading
├── builtin.go        - Built-in plugin management
├── isolation.go      - Resource isolation and limits
├── validator.go      - Plugin validation and security
├── api.go            - REST API handlers
└── *_test.go         - Comprehensive test suite
```

### Component Relationships

```
PluginManager (orchestrates all components)
    │
    ├─→ PluginRegistry (tracks plugins)
    │   └─→ ProcessorFactory map
    │
    ├─→ PluginLoader (loads external plugins)
    │   └─→ File system monitoring
    │
    ├─→ BuiltInRegistryManager (built-in plugins)
    │   └─→ Static registration
    │
    ├─→ PluginValidator (security)
    │   ├─→ Manifest validation
    │   ├─→ Checksum verification
    │   └─→ Security scanning
    │
    ├─→ NamespaceManager (isolation)
    │   └─→ Resource limits per plugin
    │
    └─→ ResourceMonitor (monitoring)
        └─→ CPU, memory, goroutine tracking
```

## Features

### 1. Plugin Registry

- **Centralized Management**: Single source of truth for all plugins
- **Type Safety**: Type-safe processor factories
- **Metadata Tracking**: Comprehensive plugin information
- **Search & Filter**: Query plugins by type, tags, status
- **Dependency Management**: Track and validate plugin dependencies
- **Enable/Disable**: Runtime control of plugins

### 2. Dynamic Loading

- **Hot Reload**: Load plugins without restarting
- **File Watching**: Automatic detection of plugin changes
- **Manifest-Based**: JSON manifests describe plugins
- **Validation**: Pre-load validation of plugin structure
- **Error Recovery**: Graceful handling of load failures

### 3. Built-in Registry

- **Static Compilation**: Compile plugins into binary
- **Init Registration**: Automatic registration via init()
- **Factory Pattern**: Processor factory functions
- **Zero Overhead**: No dynamic loading cost
- **Type Safety**: Full compile-time checking

### 4. Resource Isolation

- **Namespace Isolation**: Separate namespaces per plugin
- **Resource Limits**: CPU, memory, goroutine limits
- **Enforcement**: Runtime limit checking
- **Monitoring**: Real-time resource tracking
- **Alerts**: Callback for limit violations

### 5. Validation & Security

- **Manifest Validation**: Schema validation
- **Checksum Verification**: File integrity checks
- **Signature Support**: Code signing (extensible)
- **Trusted Publishers**: Whitelist of publishers
- **Security Scanning**: Hooks for security tools
- **Structure Validation**: Directory layout checks

### 6. REST API

Complete REST API for plugin management:

```
GET    /api/plugins              - List all plugins
GET    /api/plugins/{id}         - Get plugin details
PUT    /api/plugins/{id}/enable  - Enable plugin
PUT    /api/plugins/{id}/disable - Disable plugin
POST   /api/plugins/install      - Install plugin
POST   /api/plugins/{id}/reload  - Reload plugin
DELETE /api/plugins/{id}         - Uninstall plugin
GET    /api/plugins/search       - Search plugins
GET    /api/plugins/stats        - Plugin statistics
```

## Usage

### Creating a Built-in Plugin

```go
package myplugin

import (
    "github.com/shawntherrien/databridge/internal/plugin"
    "github.com/shawntherrien/databridge/pkg/types"
)

func init() {
    info := plugin.NewProcessorInfo(
        "MyProcessor",
        "My Custom Processor",
        "1.0.0",
        "Author",
        "Description",
        []string{"custom"},
    )

    plugin.RegisterBuiltInProcessor("MyProcessor", func() types.Processor {
        return NewMyProcessor()
    }, info)
}
```

### Initializing Plugin Manager

```go
// Create configuration
config := plugin.DefaultPluginManagerConfig()
config.PluginDir = "./plugins"
config.AutoLoad = true
config.AllowUnsigned = false

// Create manager
manager, err := plugin.NewPluginManager(config, logger)
if err != nil {
    log.Fatal(err)
}

// Initialize (registers built-ins, loads external)
if err := manager.Initialize(); err != nil {
    log.Fatal(err)
}
defer manager.Shutdown()
```

### Using Plugins

```go
// Get a processor instance
processor, err := manager.GetProcessor("MyProcessor")
if err != nil {
    log.Fatal(err)
}

// Use with flow controller
config := types.ProcessorConfig{
    ID:   uuid.New(),
    Name: "My Processor Instance",
    Type: "MyProcessor",
    Properties: map[string]string{
        "prop1": "value1",
    },
}

node, err := flowController.CreateProcessorByType("MyProcessor", config)
```

### Resource Isolation

```go
// Create namespace with limits
limits := plugin.ResourceLimits{
    MaxMemoryMB:    512,
    MaxCPUPercent:  50.0,
    MaxGoroutines:  100,
    MaxExecutionMS: 30000,
}

ns, err := manager.Namespaces.CreateNamespace("my-ns", "My Namespace", limits)

// Use resource guard
guard := ns.NewResourceGuard(context.Background())
defer guard.Release()

// Check limits
if err := guard.CheckLimits(); err != nil {
    // Handle violation
}
```

### Plugin Validation

```go
// Create validator
validator := plugin.NewPluginValidator(plugin.ValidatorConfig{
    AllowUnsigned:     false,
    TrustedPublishers: []string{"DataBridge", "TrustedVendor"},
    RequireChecksum:   true,
}, logger)

// Validate plugin
result := validator.Validate("/path/to/plugin")
if !result.Valid {
    for _, err := range result.Errors {
        log.Printf("Error: %s", err)
    }
}
```

## Configuration

### Plugin Manager Config

```go
type PluginManagerConfig struct {
    PluginDir         string        // Plugin directory
    AutoLoad          bool          // Auto-load on startup
    WatchChanges      bool          // Watch for changes
    AllowUnsigned     bool          // Allow unsigned plugins
    TrustedPublishers []string      // Trusted publishers
    RequireChecksum   bool          // Require checksums
    MonitorInterval   time.Duration // Resource monitor interval
}
```

### Resource Limits

```go
type ResourceLimits struct {
    MaxMemoryMB    int64   // Max memory in MB
    MaxCPUPercent  float64 // Max CPU percentage
    MaxGoroutines  int     // Max goroutines
    MaxFileHandles int     // Max file handles
    MaxExecutionMS int64   // Max execution time in ms
}
```

## Testing

### Running Tests

```bash
# Run all plugin tests
go test ./internal/plugin/ -v

# Run specific test
go test ./internal/plugin/ -run TestRegisterProcessor -v

# Run with coverage
go test ./internal/plugin/ -cover

# Run with race detector
go test ./internal/plugin/ -race
```

### Test Coverage

Current test coverage:
- Registry: 85%+
- Loader: 75%+
- Built-in: 90%+
- Isolation: 80%+
- Validator: 75%+

## Performance Considerations

### Built-in vs Dynamic Plugins

**Built-in Plugins:**
- ✓ Zero loading overhead
- ✓ Type-safe at compile time
- ✓ Better performance
- ✗ Requires recompilation

**Dynamic Plugins:**
- ✓ Hot reload capability
- ✓ No recompilation needed
- ✗ Platform-specific (.so)
- ✗ Loading overhead

### Best Practices

1. **Use Built-in for Core Functionality**: Common processors should be built-in
2. **Dynamic for Extensions**: Third-party or optional features as dynamic
3. **Resource Limits**: Always set appropriate limits
4. **Validation**: Enable validation in production
5. **Monitoring**: Monitor resource usage
6. **Error Handling**: Handle plugin failures gracefully

## Security

### Validation Layers

1. **Manifest Validation**: Schema and required fields
2. **Checksum Verification**: File integrity
3. **Signature Verification**: Code signing (extensible)
4. **Publisher Trust**: Whitelist mechanism
5. **Structure Validation**: Expected layout
6. **Security Scanning**: Integration hooks

### Security Best Practices

1. Enable checksum verification in production
2. Maintain a whitelist of trusted publishers
3. Use resource limits to prevent DoS
4. Regular security audits of plugins
5. Validate all user inputs in plugins
6. Never disable security checks in production

## Troubleshooting

### Common Issues

**Plugin Not Loading:**
- Check manifest.json syntax
- Verify file permissions
- Check plugin directory path
- Review logs for errors

**Resource Limit Violations:**
- Increase limits if legitimate
- Check for memory leaks
- Profile plugin performance
- Review goroutine usage

**Registration Failures:**
- Check for duplicate IDs
- Verify factory function signature
- Ensure processor implements interface
- Check dependency requirements

### Debug Mode

Enable debug logging:

```go
logger.SetLevel(logrus.DebugLevel)
```

## Future Enhancements

Planned features:
- [ ] Plugin marketplace integration
- [ ] Automatic dependency resolution
- [ ] Plugin versioning and updates
- [ ] Enhanced security scanning
- [ ] Performance profiling tools
- [ ] Plugin templates/generator
- [ ] Cross-platform dynamic loading

## Contributing

When contributing plugin system improvements:

1. Add tests for new features
2. Update documentation
3. Follow existing patterns
4. Consider backward compatibility
5. Add example usage

## References

- [Plugin SDK Documentation](../../docs/PLUGIN_SDK.md)
- [Example Plugins](../../examples/)
- [Processor Interface](../../pkg/types/processor.go)
- [API Documentation](../../docs/API.md)

## License

MIT License - See LICENSE file for details
