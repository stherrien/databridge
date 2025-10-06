package plugin

import (
	"context"
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
)

func TestNewPluginRegistry(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	registry := NewPluginRegistry("/tmp/plugins", logger)

	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}

	if registry.pluginDir != "/tmp/plugins" {
		t.Errorf("Expected plugin dir /tmp/plugins, got %s", registry.pluginDir)
	}
}

func TestRegisterProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	info := PluginInfo{
		ID:          "test-processor",
		Name:        "Test Processor",
		Version:     "1.0.0",
		Author:      "Test Author",
		Description: "Test processor",
		Type:        PluginTypeProcessor,
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	err := registry.RegisterProcessor("test-processor", info, factory)
	if err != nil {
		t.Fatalf("Failed to register processor: %v", err)
	}

	// Try to register again - should fail
	err = registry.RegisterProcessor("test-processor", info, factory)
	if err == nil {
		t.Error("Expected error when registering duplicate processor")
	}
}

func TestGetProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	info := PluginInfo{
		ID:      "test-processor",
		Name:    "Test Processor",
		Version: "1.0.0",
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	registry.RegisterProcessor("test-processor", info, factory)

	// Get processor
	processor, err := registry.GetProcessor("test-processor")
	if err != nil {
		t.Fatalf("Failed to get processor: %v", err)
	}

	if processor == nil {
		t.Error("Expected non-nil processor")
	}

	// Try to get non-existent processor
	_, err = registry.GetProcessor("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent processor")
	}
}

func TestUnregisterProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	info := PluginInfo{
		ID:   "test-processor",
		Name: "Test Processor",
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	registry.RegisterProcessor("test-processor", info, factory)

	// Unregister
	err := registry.UnregisterProcessor("test-processor")
	if err != nil {
		t.Fatalf("Failed to unregister processor: %v", err)
	}

	// Try to get unregistered processor
	_, err = registry.GetProcessor("test-processor")
	if err == nil {
		t.Error("Expected error for unregistered processor")
	}

	// Try to unregister non-existent processor
	err = registry.UnregisterProcessor("non-existent")
	if err == nil {
		t.Error("Expected error when unregistering non-existent processor")
	}
}

func TestListPlugins(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	// Register multiple processors
	for i := 1; i <= 3; i++ {
		info := PluginInfo{
			ID:   string(rune('a'+i-1)) + "-processor",
			Name: "Processor " + string(rune('0'+i)),
		}

		factory := func() types.Processor {
			return &mockProcessor{}
		}

		registry.RegisterProcessor(info.ID, info, factory)
	}

	plugins := registry.ListPlugins()
	if len(plugins) != 3 {
		t.Errorf("Expected 3 plugins, got %d", len(plugins))
	}
}

func TestEnableDisablePlugin(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	info := PluginInfo{
		ID:   "test-processor",
		Name: "Test Processor",
	}

	factory := func() types.Processor {
		return &mockProcessor{}
	}

	registry.RegisterProcessor("test-processor", info, factory)

	// Should be enabled by default
	if !registry.IsEnabled("test-processor") {
		t.Error("Expected processor to be enabled by default")
	}

	// Disable
	err := registry.DisablePlugin("test-processor")
	if err != nil {
		t.Fatalf("Failed to disable plugin: %v", err)
	}

	if registry.IsEnabled("test-processor") {
		t.Error("Expected processor to be disabled")
	}

	// Try to get disabled processor
	_, err = registry.GetProcessor("test-processor")
	if err == nil {
		t.Error("Expected error when getting disabled processor")
	}

	// Enable
	err = registry.EnablePlugin("test-processor")
	if err != nil {
		t.Fatalf("Failed to enable plugin: %v", err)
	}

	if !registry.IsEnabled("test-processor") {
		t.Error("Expected processor to be enabled")
	}
}

func TestSearchPlugins(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	// Register processors with different attributes
	processors := []PluginInfo{
		{
			ID:          "generate-processor",
			Name:        "Generate FlowFile",
			Description: "Generates data",
			Type:        PluginTypeProcessor,
			Tags:        []string{"source", "generate"},
		},
		{
			ID:          "transform-processor",
			Name:        "Transform Data",
			Description: "Transforms data",
			Type:        PluginTypeProcessor,
			Tags:        []string{"transform", "modify"},
		},
		{
			ID:          "report-controller",
			Name:        "Report Controller",
			Description: "Generates reports",
			Type:        PluginTypeController,
			Tags:        []string{"report", "analytics"},
		},
	}

	for _, info := range processors {
		factory := func() types.Processor {
			return &mockProcessor{}
		}
		registry.RegisterProcessor(info.ID, info, factory)
	}

	// Search by query
	results := registry.SearchPlugins("generate", "", nil)
	if len(results) != 2 { // Should match "generate" and "Generates"
		t.Errorf("Expected 2 results for 'generate', got %d", len(results))
	}

	// Search by type
	results = registry.SearchPlugins("", PluginTypeProcessor, nil)
	if len(results) != 2 {
		t.Errorf("Expected 2 processor results, got %d", len(results))
	}

	// Search by tags
	results = registry.SearchPlugins("", "", []string{"report"})
	if len(results) != 1 {
		t.Errorf("Expected 1 result for tag 'report', got %d", len(results))
	}
}

func TestGetPluginsByType(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	// Register different types
	registry.RegisterProcessor("proc1", PluginInfo{
		ID:   "proc1",
		Type: PluginTypeProcessor,
	}, func() types.Processor { return &mockProcessor{} })

	registry.RegisterProcessor("proc2", PluginInfo{
		ID:   "proc2",
		Type: PluginTypeProcessor,
	}, func() types.Processor { return &mockProcessor{} })

	registry.RegisterProcessor("ctrl1", PluginInfo{
		ID:   "ctrl1",
		Type: PluginTypeController,
	}, func() types.Processor { return &mockProcessor{} })

	processors := registry.GetPluginsByType(PluginTypeProcessor)
	if len(processors) != 2 {
		t.Errorf("Expected 2 processors, got %d", len(processors))
	}

	controllers := registry.GetPluginsByType(PluginTypeController)
	if len(controllers) != 1 {
		t.Errorf("Expected 1 controller, got %d", len(controllers))
	}
}

func TestGetPluginsByStatus(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	registry.RegisterProcessor("proc1", PluginInfo{
		ID:     "proc1",
		Status: PluginStatusActive,
	}, func() types.Processor { return &mockProcessor{} })

	registry.RegisterProcessor("proc2", PluginInfo{
		ID:     "proc2",
		Status: PluginStatusActive,
	}, func() types.Processor { return &mockProcessor{} })

	// Change one to error
	registry.UpdatePluginStatus("proc2", PluginStatusError)

	active := registry.GetPluginsByStatus(PluginStatusActive)
	if len(active) != 1 {
		t.Errorf("Expected 1 active plugin, got %d", len(active))
	}

	errored := registry.GetPluginsByStatus(PluginStatusError)
	if len(errored) != 1 {
		t.Errorf("Expected 1 errored plugin, got %d", len(errored))
	}
}

func TestValidateDependencies(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	// Register dependency
	registry.RegisterProcessor("dep1", PluginInfo{
		ID:     "dep1",
		Status: PluginStatusActive,
	}, func() types.Processor { return &mockProcessor{} })

	// Register plugin with dependency
	registry.RegisterProcessor("proc1", PluginInfo{
		ID:           "proc1",
		Dependencies: []string{"dep1"},
	}, func() types.Processor { return &mockProcessor{} })

	// Validate - should pass
	err := registry.ValidateDependencies("proc1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Register plugin with missing dependency
	registry.RegisterProcessor("proc2", PluginInfo{
		ID:           "proc2",
		Dependencies: []string{"missing-dep"},
	}, func() types.Processor { return &mockProcessor{} })

	// Validate - should fail
	err = registry.ValidateDependencies("proc2")
	if err == nil {
		t.Error("Expected error for missing dependency")
	}
}

func TestGetPluginStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	registry := NewPluginRegistry("/tmp/plugins", logger)

	// Register some plugins
	registry.RegisterProcessor("proc1", PluginInfo{
		ID:   "proc1",
		Type: PluginTypeProcessor,
	}, func() types.Processor { return &mockProcessor{} })

	registry.RegisterProcessor("proc2", PluginInfo{
		ID:   "proc2",
		Type: PluginTypeProcessor,
	}, func() types.Processor { return &mockProcessor{} })

	stats := registry.GetPluginStats()

	total, ok := stats["total"].(int)
	if !ok || total != 2 {
		t.Errorf("Expected total 2, got %v", stats["total"])
	}
}

// Mock processor for testing
type mockProcessor struct {
	types.BaseProcessor
}

func (m *mockProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

func (m *mockProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	return nil
}

func (m *mockProcessor) GetInfo() types.ProcessorInfo {
	return types.ProcessorInfo{
		Name:    "Mock Processor",
		Version: "1.0.0",
	}
}

func (m *mockProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	return nil
}

func (m *mockProcessor) OnStopped(ctx context.Context) {}
