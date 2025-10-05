package plugin

import (
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
)

func TestRegisterBuiltInProcessor(t *testing.T) {
	// Clear registry for test
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	info := PluginInfo{
		ID:      "test-builtin",
		Name:    "Test Built-in",
		Version: "1.0.0",
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	// Should not panic
	RegisterBuiltInProcessor("test-builtin", factory, info)

	// Verify registration
	builtInMutex.RLock()
	_, exists := builtInRegistry["test-builtin"]
	builtInMutex.RUnlock()

	if !exists {
		t.Error("Expected processor to be registered in built-in registry")
	}
}

func TestRegisterBuiltInProcessorDuplicate(t *testing.T) {
	// Clear registry for test
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	info := PluginInfo{
		ID:   "test-builtin",
		Name: "Test Built-in",
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	RegisterBuiltInProcessor("test-builtin", factory, info)

	// Try to register again - should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when registering duplicate built-in processor")
		}
	}()

	RegisterBuiltInProcessor("test-builtin", factory, info)
}

func TestBuiltInRegistryManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear and setup registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	// Register some built-ins
	RegisterBuiltInProcessor("builtin1", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin1", Name: "Built-in 1"})

	RegisterBuiltInProcessor("builtin2", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin2", Name: "Built-in 2"})

	// Create manager
	registry := NewPluginRegistry("/tmp/plugins", logger)
	manager := NewBuiltInRegistryManager(registry, logger)

	// Register all built-ins
	err := manager.RegisterAllBuiltIns()
	if err != nil {
		t.Fatalf("Failed to register built-ins: %v", err)
	}

	// Verify registration
	processor, err := registry.GetProcessor("builtin1")
	if err != nil {
		t.Errorf("Failed to get built-in processor: %v", err)
	}
	if processor == nil {
		t.Error("Expected non-nil processor")
	}
}

func TestIsBuiltIn(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear and setup registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	RegisterBuiltInProcessor("builtin1", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin1"})

	registry := NewPluginRegistry("/tmp/plugins", logger)
	manager := NewBuiltInRegistryManager(registry, logger)

	if !manager.IsBuiltIn("builtin1") {
		t.Error("Expected builtin1 to be a built-in")
	}

	if manager.IsBuiltIn("non-existent") {
		t.Error("Expected non-existent to not be a built-in")
	}
}

func TestGetBuiltInInfo(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear and setup registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	expectedInfo := PluginInfo{
		ID:      "builtin1",
		Name:    "Built-in 1",
		Version: "1.0.0",
	}

	RegisterBuiltInProcessor("builtin1", func() types.Processor {
		return &mockProcessor{}
	}, expectedInfo)

	registry := NewPluginRegistry("/tmp/plugins", logger)
	manager := NewBuiltInRegistryManager(registry, logger)

	info, err := manager.GetBuiltInInfo("builtin1")
	if err != nil {
		t.Fatalf("Failed to get built-in info: %v", err)
	}

	if info.ID != expectedInfo.ID || info.Name != expectedInfo.Name {
		t.Errorf("Info mismatch: got %+v, expected %+v", info, expectedInfo)
	}

	// Try non-existent
	_, err = manager.GetBuiltInInfo("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent built-in")
	}
}

func TestListBuiltInProcessors(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear and setup registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	RegisterBuiltInProcessor("builtin1", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin1"})

	RegisterBuiltInProcessor("builtin2", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin2"})

	registry := NewPluginRegistry("/tmp/plugins", logger)
	manager := NewBuiltInRegistryManager(registry, logger)

	list := manager.ListBuiltInProcessors()
	if len(list) != 2 {
		t.Errorf("Expected 2 built-in processors, got %d", len(list))
	}
}

func TestCreateBuiltInProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear and setup registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	builtInMutex.Unlock()

	RegisterBuiltInProcessor("builtin1", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "builtin1"})

	registry := NewPluginRegistry("/tmp/plugins", logger)
	manager := NewBuiltInRegistryManager(registry, logger)

	processor, err := manager.CreateBuiltInProcessor("builtin1")
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	if processor == nil {
		t.Error("Expected non-nil processor")
	}

	// Try non-existent
	_, err = manager.CreateBuiltInProcessor("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent built-in")
	}
}

func TestNewProcessorInfo(t *testing.T) {
	info := NewProcessorInfo("test-id", "Test Processor", "1.0.0", "Test Author", "Test description", []string{"test", "demo"})

	if info.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %s", info.ID)
	}

	if info.Type != PluginTypeProcessor {
		t.Errorf("Expected type %s, got %s", PluginTypeProcessor, info.Type)
	}

	if info.Status != PluginStatusActive {
		t.Errorf("Expected status %s, got %s", PluginStatusActive, info.Status)
	}

	if len(info.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(info.Tags))
	}
}

func TestPluginManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Clear built-in registry
	builtInMutex.Lock()
	builtInRegistry = make(map[string]ProcessorFactory)
	builtInInfo = make(map[string]PluginInfo)
	registryInitialized = false
	builtInMutex.Unlock()

	// Register a built-in
	RegisterBuiltInProcessor("test-builtin", func() types.Processor {
		return &mockProcessor{}
	}, PluginInfo{ID: "test-builtin", Name: "Test Built-in"})

	config := DefaultPluginManagerConfig()
	config.PluginDir = "/tmp/test-plugins"

	manager, err := NewPluginManager(config, logger)
	if err != nil {
		t.Fatalf("Failed to create plugin manager: %v", err)
	}

	// Initialize
	err = manager.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize plugin manager: %v", err)
	}

	// Get processor
	processor, err := manager.GetProcessor("test-builtin")
	if err != nil {
		t.Errorf("Failed to get processor: %v", err)
	}
	if processor == nil {
		t.Error("Expected non-nil processor")
	}

	// Get plugin info
	info, err := manager.GetPluginInfo("test-builtin")
	if err != nil {
		t.Errorf("Failed to get plugin info: %v", err)
	}
	if info.ID != "test-builtin" {
		t.Errorf("Expected ID 'test-builtin', got %s", info.ID)
	}

	// List all plugins
	plugins := manager.ListAllPlugins()
	if len(plugins) == 0 {
		t.Error("Expected at least one plugin")
	}

	// Get stats
	stats := manager.GetStats()
	if stats == nil {
		t.Error("Expected non-nil stats")
	}

	// Shutdown
	err = manager.Shutdown()
	if err != nil {
		t.Errorf("Failed to shutdown plugin manager: %v", err)
	}
}

func TestPluginManagerConfig(t *testing.T) {
	config := DefaultPluginManagerConfig()

	if config.PluginDir != "./plugins" {
		t.Errorf("Expected default plugin dir './plugins', got %s", config.PluginDir)
	}

	if !config.AutoLoad {
		t.Error("Expected AutoLoad to be true by default")
	}

	if !config.AllowUnsigned {
		t.Error("Expected AllowUnsigned to be true by default")
	}
}
