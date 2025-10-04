package plugins

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Mock implementations for testing
type mockProcessorContext struct {
	properties map[string]string
}

func (ctx *mockProcessorContext) GetProperty(name string) (string, bool) {
	value, exists := ctx.properties[name]
	return value, exists
}

func (ctx *mockProcessorContext) GetPropertyValue(name string) string {
	value, _ := ctx.properties[name]
	return value
}

func (ctx *mockProcessorContext) HasProperty(name string) bool {
	_, exists := ctx.properties[name]
	return exists
}

func (ctx *mockProcessorContext) GetProcessorConfig() types.ProcessorConfig {
	return types.ProcessorConfig{
		ID:         uuid.New(),
		Name:       "Test Generate FlowFile",
		Type:       "GenerateFlowFile",
		Properties: ctx.properties,
	}
}

func (ctx *mockProcessorContext) GetLogger() types.Logger {
	return &mockLogger{}
}

type mockLogger struct{}

func (l *mockLogger) Debug(msg string, fields ...interface{}) {}
func (l *mockLogger) Info(msg string, fields ...interface{})  {}
func (l *mockLogger) Warn(msg string, fields ...interface{})  {}
func (l *mockLogger) Error(msg string, fields ...interface{}) {}

type mockProcessSession struct {
	createdFlowFiles []*types.FlowFile
	writtenContent   map[uuid.UUID][]byte
	transfers        map[uuid.UUID]types.Relationship
	removals         []uuid.UUID
	attributes       map[uuid.UUID]map[string]string
}

func newMockProcessSession() *mockProcessSession {
	return &mockProcessSession{
		createdFlowFiles: make([]*types.FlowFile, 0),
		writtenContent:   make(map[uuid.UUID][]byte),
		transfers:        make(map[uuid.UUID]types.Relationship),
		removals:         make([]uuid.UUID, 0),
		attributes:       make(map[uuid.UUID]map[string]string),
	}
}

func (s *mockProcessSession) Get() *types.FlowFile {
	if len(s.createdFlowFiles) > 0 {
		ff := s.createdFlowFiles[0]
		s.createdFlowFiles = s.createdFlowFiles[1:]
		return ff
	}
	return nil
}

func (s *mockProcessSession) GetBatch(maxResults int) []*types.FlowFile { return nil }

func (s *mockProcessSession) CreateChild(parent *types.FlowFile) *types.FlowFile {
	flowFile := types.NewFlowFile()
	flowFile.Lineage.ParentUUIDs = []uuid.UUID{parent.ID}
	s.createdFlowFiles = append(s.createdFlowFiles, flowFile)
	s.attributes[flowFile.ID] = make(map[string]string)
	return flowFile
}

func (s *mockProcessSession) Clone(original *types.FlowFile) *types.FlowFile { return nil }

func (s *mockProcessSession) Read(flowFile *types.FlowFile) ([]byte, error) {
	if content, exists := s.writtenContent[flowFile.ID]; exists {
		return content, nil
	}
	return nil, nil
}
func (s *mockProcessSession) Commit() error                                            { return nil }
func (s *mockProcessSession) Rollback()                                                {}
func (s *mockProcessSession) GetLogger() types.Logger                                  { return &mockLogger{} }
func (s *mockProcessSession) RemoveAttribute(flowFile *types.FlowFile, key string)    {}
func (s *mockProcessSession) PutAllAttributes(flowFile *types.FlowFile, attrs map[string]string) {}

func (s *mockProcessSession) Create() *types.FlowFile {
	flowFile := types.NewFlowFile()
	s.createdFlowFiles = append(s.createdFlowFiles, flowFile)
	s.attributes[flowFile.ID] = make(map[string]string)
	return flowFile
}

func (s *mockProcessSession) Transfer(flowFile *types.FlowFile, relationship types.Relationship) {
	s.transfers[flowFile.ID] = relationship
}

func (s *mockProcessSession) Remove(flowFile *types.FlowFile) {
	s.removals = append(s.removals, flowFile.ID)
}

func (s *mockProcessSession) PutAttribute(flowFile *types.FlowFile, key, value string) {
	if s.attributes[flowFile.ID] == nil {
		s.attributes[flowFile.ID] = make(map[string]string)
	}
	s.attributes[flowFile.ID][key] = value
	flowFile.UpdateAttribute(key, value)
}

func (s *mockProcessSession) Write(flowFile *types.FlowFile, content []byte) error {
	s.writtenContent[flowFile.ID] = content
	flowFile.Size = int64(len(content))
	return nil
}

func TestNewGenerateFlowFileProcessor(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()

	if processor == nil {
		t.Fatal("NewGenerateFlowFileProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "GenerateFlowFile" {
		t.Errorf("Expected processor name GenerateFlowFile, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 4 {
		t.Errorf("Expected 4 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 1 {
		t.Errorf("Expected 1 relationship, got %d", len(info.Relationships))
	}

	if info.Relationships[0] != types.RelationshipSuccess {
		t.Error("Expected success relationship")
	}

	// Check specific properties
	expectedProperties := map[string]bool{
		"File Size":        true,
		"Content":          false,
		"Unique FlowFiles": false,
		"Custom Text":      false,
	}

	for _, prop := range info.Properties {
		expectedRequired, exists := expectedProperties[prop.Name]
		if !exists {
			t.Errorf("Unexpected property: %s", prop.Name)
			continue
		}

		if prop.Required != expectedRequired {
			t.Errorf("Property %s required should be %v, got %v", prop.Name, expectedRequired, prop.Required)
		}
	}
}

func TestGenerateFlowFileProcessorInitialize(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"File Size": "1024",
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid properties: %v", err)
	}

	// Test initialization without File Size property
	ctxNoFileSize := &mockProcessorContext{
		properties: map[string]string{},
	}

	err = processor.Initialize(ctxNoFileSize)
	if err == nil {
		t.Error("Initialize should fail without File Size property")
	}
}

func TestGenerateFlowFileProcessorValidate(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"File Size":        "1024",
			"Content":          "Test content",
			"Unique FlowFiles": "true",
			"Custom Text":      "Custom",
		},
	}

	results := processor.Validate(validConfig)
	validCount := 0
	for _, result := range results {
		if result.Valid {
			validCount++
		}
	}

	if len(results) > 0 && validCount != len(results) {
		t.Error("Valid configuration should pass all validations")
	}

	// Test invalid file size - non-numeric
	invalidConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"File Size": "not-a-number",
		},
	}

	results = processor.Validate(invalidConfig)
	hasFileError := false
	for _, result := range results {
		if result.Property == "File Size" && !result.Valid {
			hasFileError = true
			break
		}
	}

	if !hasFileError {
		t.Error("Invalid file size should cause validation error")
	}

	// Test file size too large
	tooLargeConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"File Size": "200000000", // 200MB, over 100MB limit
		},
	}

	results = processor.Validate(tooLargeConfig)
	hasFileError = false
	for _, result := range results {
		if result.Property == "File Size" && !result.Valid {
			hasFileError = true
			break
		}
	}

	if !hasFileError {
		t.Error("File size over limit should cause validation error")
	}

	// Test zero file size
	zeroSizeConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"File Size": "0",
		},
	}

	results = processor.Validate(zeroSizeConfig)
	hasFileError = false
	for _, result := range results {
		if result.Property == "File Size" && !result.Valid {
			hasFileError = true
			break
		}
	}

	if !hasFileError {
		t.Error("Zero file size should cause validation error")
	}
}

func TestGenerateFlowFileProcessorOnTrigger(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()
	session := newMockProcessSession()

	// Create context with processor context
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"File Size":        "100",
			"Content":          "Test content",
			"Unique FlowFiles": "true",
			"Custom Text":      "Test custom",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	// Test OnTrigger
	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify FlowFile was created
	if len(session.createdFlowFiles) != 1 {
		t.Errorf("Expected 1 created FlowFile, got %d", len(session.createdFlowFiles))
	}

	flowFile := session.createdFlowFiles[0]

	// Verify content was written
	if content, exists := session.writtenContent[flowFile.ID]; !exists {
		t.Error("Content should be written to FlowFile")
	} else {
		if len(content) != 100 {
			t.Errorf("Expected content length 100, got %d", len(content))
		}

		contentStr := string(content)
		if !contains(contentStr, "Test content") {
			t.Error("Content should contain base content")
		}

		if !contains(contentStr, "Test custom") {
			t.Error("Content should contain custom text")
		}
	}

	// Verify attributes
	attrs := session.attributes[flowFile.ID]
	if attrs == nil {
		t.Fatal("FlowFile should have attributes")
	}

	expectedAttrs := []string{"filename", "generated.timestamp", "generated.size", "generator.type", "custom.text"}
	for _, expectedAttr := range expectedAttrs {
		if _, exists := attrs[expectedAttr]; !exists {
			t.Errorf("FlowFile should have attribute %s", expectedAttr)
		}
	}

	// Verify specific attribute values
	if attrs["generator.type"] != "GenerateFlowFile" {
		t.Errorf("Expected generator.type=GenerateFlowFile, got %s", attrs["generator.type"])
	}

	if attrs["generated.size"] != "100" {
		t.Errorf("Expected generated.size=100, got %s", attrs["generated.size"])
	}

	if attrs["custom.text"] != "Test custom" {
		t.Errorf("Expected custom.text=Test custom, got %s", attrs["custom.text"])
	}

	// Verify transfer
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel != types.RelationshipSuccess {
		t.Error("FlowFile should be transferred to success relationship")
	}
}

func TestGenerateFlowFileProcessorOnTriggerWithoutProcessorContext(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestGenerateFlowFileProcessorContentGeneration(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()

	// Test different content scenarios
	testCases := []struct {
		baseContent  string
		customText   string
		unique       bool
		targetSize   int64
		description  string
	}{
		{"Hello", "", false, 50, "Simple content repetition"},
		{"Test", "Custom", false, 100, "With custom text"},
		{"Data", "Unique", true, 150, "With unique identifier"},
		{"", "OnlyCustom", false, 100, "Empty base content"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			content := processor.generateContent(tc.baseContent, tc.customText, tc.unique, tc.targetSize)

			if int64(len(content)) != tc.targetSize {
				t.Errorf("Generated content length should be %d, got %d", tc.targetSize, len(content))
			}

			if tc.baseContent != "" && !contains(content, tc.baseContent) {
				t.Errorf("Content should contain base content '%s'", tc.baseContent)
			}

			if tc.customText != "" && !contains(content, tc.customText) {
				t.Errorf("Content should contain custom text '%s', got: %s", tc.customText, content)
			}

			// Content should always have timestamp
			if !contains(content, "[") || !contains(content, "]") {
				t.Error("Content should contain timestamp markers")
			}

			if tc.unique {
				// Should contain some UUID-like pattern
				hasUUIDPattern := false
				for i := 0; i < len(content)-8; i++ {
					if content[i] == '[' && i+9 < len(content) && content[i+9] == ']' {
						hasUUIDPattern = true
						break
					}
				}
				if !hasUUIDPattern {
					t.Error("Unique content should contain UUID pattern")
				}
			}
		})
	}
}

func TestGenerateFlowFileProcessorOnStopped(t *testing.T) {
	processor := NewGenerateFlowFileProcessor()
	ctx := context.Background()

	// OnStopped should not panic and should complete normally
	processor.OnStopped(ctx)
}

// Helper function - proper substring search
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkGenerateFlowFileProcessorOnTrigger(b *testing.B) {
	processor := NewGenerateFlowFileProcessor()

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"File Size":        "1024",
			"Content":          "Benchmark content",
			"Unique FlowFiles": "true",
			"Custom Text":      "Benchmark",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		session := newMockProcessSession()
		processor.OnTrigger(ctx, session)
	}
}

func BenchmarkGenerateFlowFileContentGeneration(b *testing.B) {
	processor := NewGenerateFlowFileProcessor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.generateContent("Benchmark content", "Custom text", true, 1024)
	}
}