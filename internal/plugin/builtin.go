package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
)

// builtInRegistry stores built-in processor factories
var (
	builtInRegistry     = make(map[string]ProcessorFactory)
	builtInInfo         = make(map[string]PluginInfo)
	builtInMutex        sync.RWMutex
	registryInitialized bool
)

// RegisterBuiltInProcessor registers a statically compiled processor
// This should be called from init() functions in processor implementations
func RegisterBuiltInProcessor(id string, factory ProcessorFactory, info PluginInfo) {
	builtInMutex.Lock()
	defer builtInMutex.Unlock()

	if _, exists := builtInRegistry[id]; exists {
		panic(fmt.Sprintf("built-in processor %s already registered", id))
	}

	// Set defaults
	if info.ID == "" {
		info.ID = id
	}
	if info.Type == "" {
		info.Type = PluginTypeProcessor
	}
	if info.Status == "" {
		info.Status = PluginStatusActive
	}
	if info.LoadedAt.IsZero() {
		info.LoadedAt = time.Now()
	}
	if info.Path == "" {
		info.Path = "built-in"
	}

	builtInRegistry[id] = factory
	builtInInfo[id] = info
}

// BuiltInRegistryManager manages built-in processors
type BuiltInRegistryManager struct {
	registry *PluginRegistry
	logger   *logrus.Logger
	mu       sync.RWMutex
}

// NewBuiltInRegistryManager creates a new built-in registry manager
func NewBuiltInRegistryManager(registry *PluginRegistry, logger *logrus.Logger) *BuiltInRegistryManager {
	return &BuiltInRegistryManager{
		registry: registry,
		logger:   logger,
	}
}

// RegisterAllBuiltIns registers all built-in processors with the main registry
func (m *BuiltInRegistryManager) RegisterAllBuiltIns() error {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()

	registered := 0
	failed := 0

	for id, factory := range builtInRegistry {
		info := builtInInfo[id]

		if err := m.registry.RegisterProcessor(id, info, factory); err != nil {
			m.logger.WithError(err).Warnf("Failed to register built-in processor: %s", id)
			failed++
			continue
		}

		registered++
	}

	m.logger.WithFields(logrus.Fields{
		"registered": registered,
		"failed":     failed,
	}).Info("Registered built-in processors")

	if failed > 0 {
		return fmt.Errorf("failed to register %d built-in processors", failed)
	}

	registryInitialized = true
	return nil
}

// GetBuiltInProcessors returns all built-in processor IDs
func (m *BuiltInRegistryManager) GetBuiltInProcessors() []string {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()

	ids := make([]string, 0, len(builtInRegistry))
	for id := range builtInRegistry {
		ids = append(ids, id)
	}
	return ids
}

// IsBuiltIn checks if a processor is built-in
func (m *BuiltInRegistryManager) IsBuiltIn(id string) bool {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()

	_, exists := builtInRegistry[id]
	return exists
}

// GetBuiltInInfo returns info about a built-in processor
func (m *BuiltInRegistryManager) GetBuiltInInfo(id string) (PluginInfo, error) {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()

	info, exists := builtInInfo[id]
	if !exists {
		return PluginInfo{}, fmt.Errorf("built-in processor %s not found", id)
	}

	return info, nil
}

// ListBuiltInProcessors returns all built-in processor information
func (m *BuiltInRegistryManager) ListBuiltInProcessors() []PluginInfo {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()

	list := make([]PluginInfo, 0, len(builtInInfo))
	for _, info := range builtInInfo {
		list = append(list, info)
	}
	return list
}

// CreateBuiltInProcessor creates an instance of a built-in processor
func (m *BuiltInRegistryManager) CreateBuiltInProcessor(id string) (types.Processor, error) {
	builtInMutex.RLock()
	factory, exists := builtInRegistry[id]
	builtInMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("built-in processor %s not found", id)
	}

	processor := factory()
	if processor == nil {
		return nil, fmt.Errorf("factory for built-in processor %s returned nil", id)
	}

	return processor, nil
}

// UnregisterBuiltIn removes a built-in processor (useful for testing)
func (m *BuiltInRegistryManager) UnregisterBuiltIn(id string) error {
	builtInMutex.Lock()
	defer builtInMutex.Unlock()

	if _, exists := builtInRegistry[id]; !exists {
		return fmt.Errorf("built-in processor %s not found", id)
	}

	delete(builtInRegistry, id)
	delete(builtInInfo, id)

	m.logger.WithField("processorId", id).Debug("Unregistered built-in processor")
	return nil
}

// ReloadBuiltIns clears and reloads all built-in processors
func (m *BuiltInRegistryManager) ReloadBuiltIns() error {
	m.logger.Info("Reloading built-in processors")

	// Unregister all current built-ins from main registry
	builtIns := m.GetBuiltInProcessors()
	for _, id := range builtIns {
		if err := m.registry.UnregisterProcessor(id); err != nil {
			m.logger.WithError(err).Warnf("Failed to unregister %s", id)
		}
	}

	// Re-register all
	return m.RegisterAllBuiltIns()
}

// GetBuiltInCount returns the number of registered built-in processors
func (m *BuiltInRegistryManager) GetBuiltInCount() int {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()
	return len(builtInRegistry)
}

// IsRegistryInitialized checks if the built-in registry has been initialized
func IsRegistryInitialized() bool {
	builtInMutex.RLock()
	defer builtInMutex.RUnlock()
	return registryInitialized
}

// Helper functions for processors to create standardized PluginInfo

// NewProcessorInfo creates a standard PluginInfo for a processor
func NewProcessorInfo(id, name, version, author, description string, tags []string) PluginInfo {
	return PluginInfo{
		ID:          id,
		Name:        name,
		Version:     version,
		Author:      author,
		Description: description,
		Type:        PluginTypeProcessor,
		Status:      PluginStatusActive,
		LoadedAt:    time.Now(),
		Path:        "built-in",
		Tags:        tags,
	}
}

// ProcessorInfoFromProcessorInfo converts types.ProcessorInfo to PluginInfo
func ProcessorInfoFromProcessorInfo(id string, procInfo types.ProcessorInfo) PluginInfo {
	return PluginInfo{
		ID:          id,
		Name:        procInfo.Name,
		Version:     procInfo.Version,
		Author:      procInfo.Author,
		Description: procInfo.Description,
		Type:        PluginTypeProcessor,
		Status:      PluginStatusActive,
		LoadedAt:    time.Now(),
		Path:        "built-in",
		Tags:        procInfo.Tags,
	}
}

// PluginManager combines all plugin management functionality
type PluginManager struct {
	Registry   *PluginRegistry
	Loader     *PluginLoader
	BuiltIns   *BuiltInRegistryManager
	Validator  *PluginValidator
	Namespaces *NamespaceManager
	Monitor    *ResourceMonitor
	logger     *logrus.Logger
}

// PluginManagerConfig configures the plugin manager
type PluginManagerConfig struct {
	PluginDir         string
	AutoLoad          bool
	WatchChanges      bool
	AllowUnsigned     bool
	TrustedPublishers []string
	RequireChecksum   bool
	MonitorInterval   time.Duration
}

// DefaultPluginManagerConfig returns default configuration
func DefaultPluginManagerConfig() PluginManagerConfig {
	return PluginManagerConfig{
		PluginDir:         "./plugins",
		AutoLoad:          true,
		WatchChanges:      false,
		AllowUnsigned:     true,
		TrustedPublishers: []string{},
		RequireChecksum:   false,
		MonitorInterval:   30 * time.Second,
	}
}

// NewPluginManager creates a comprehensive plugin manager
func NewPluginManager(config PluginManagerConfig, logger *logrus.Logger) (*PluginManager, error) {
	// Create registry
	registry := NewPluginRegistry(config.PluginDir, logger)

	// Create loader
	loader := NewPluginLoader(registry, LoaderConfig{
		PluginDir:    config.PluginDir,
		AutoLoad:     config.AutoLoad,
		WatchChanges: config.WatchChanges,
	}, logger)

	// Create built-in manager
	builtIns := NewBuiltInRegistryManager(registry, logger)

	// Create validator
	validator := NewPluginValidator(ValidatorConfig{
		AllowUnsigned:     config.AllowUnsigned,
		TrustedPublishers: config.TrustedPublishers,
		RequireChecksum:   config.RequireChecksum,
	}, logger)

	// Create namespace manager
	namespaces := NewNamespaceManager(logger)

	// Create resource monitor
	monitor := NewResourceMonitor(namespaces, logger, config.MonitorInterval)

	return &PluginManager{
		Registry:   registry,
		Loader:     loader,
		BuiltIns:   builtIns,
		Validator:  validator,
		Namespaces: namespaces,
		Monitor:    monitor,
		logger:     logger,
	}, nil
}

// Initialize initializes the plugin manager
func (pm *PluginManager) Initialize() error {
	pm.logger.Info("Initializing plugin manager")

	// Register all built-in processors
	if err := pm.BuiltIns.RegisterAllBuiltIns(); err != nil {
		return fmt.Errorf("failed to register built-in processors: %w", err)
	}

	// Load external plugins if auto-load is enabled
	if pm.Loader.autoLoad {
		if err := pm.Loader.LoadAll(); err != nil {
			pm.logger.WithError(err).Warn("Failed to load some external plugins")
		}
	}

	// Start resource monitor
	pm.Monitor.Start()

	pm.logger.Info("Plugin manager initialized successfully")
	return nil
}

// Shutdown shuts down the plugin manager
func (pm *PluginManager) Shutdown() error {
	pm.logger.Info("Shutting down plugin manager")

	// Stop resource monitor
	pm.Monitor.Stop()

	// Stop loader watching if active
	pm.Loader.StopWatching()

	pm.logger.Info("Plugin manager shut down")
	return nil
}

// GetProcessor retrieves a processor by ID (tries registry, then built-ins)
func (pm *PluginManager) GetProcessor(id string) (types.Processor, error) {
	// Try registry first
	processor, err := pm.Registry.GetProcessor(id)
	if err == nil {
		return processor, nil
	}

	// Try built-ins
	return pm.BuiltIns.CreateBuiltInProcessor(id)
}

// GetPluginInfo retrieves plugin information
func (pm *PluginManager) GetPluginInfo(id string) (*PluginInfo, error) {
	// Try registry first
	info, err := pm.Registry.GetPluginInfo(id)
	if err == nil {
		return info, nil
	}

	// Try built-ins
	builtInInfo, err := pm.BuiltIns.GetBuiltInInfo(id)
	if err != nil {
		return nil, fmt.Errorf("plugin %s not found", id)
	}

	return &builtInInfo, nil
}

// ListAllPlugins returns all plugins (registry + built-ins)
func (pm *PluginManager) ListAllPlugins() []*PluginInfo {
	// Get from registry
	plugins := pm.Registry.ListPlugins()

	// Add built-ins that aren't already in registry
	builtIns := pm.BuiltIns.ListBuiltInProcessors()
	for _, builtIn := range builtIns {
		found := false
		for _, plugin := range plugins {
			if plugin.ID == builtIn.ID {
				found = true
				break
			}
		}
		if !found {
			builtInCopy := builtIn
			plugins = append(plugins, &builtInCopy)
		}
	}

	return plugins
}

// GetStats returns comprehensive plugin statistics
func (pm *PluginManager) GetStats() map[string]interface{} {
	stats := pm.Registry.GetPluginStats()
	stats["builtInCount"] = pm.BuiltIns.GetBuiltInCount()

	// Add namespace stats
	namespaces := pm.Namespaces.ListNamespaces()
	stats["namespaceCount"] = len(namespaces)

	// Add resource metrics
	stats["resourceMetrics"] = pm.Monitor.GetAllMetrics()

	return stats
}
