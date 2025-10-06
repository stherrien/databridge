package types

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Mock processor for testing
type mockProcessor struct {
	*BaseProcessor
	initCalled    bool
	triggerCalled bool
	stopCalled    bool
	shouldError   bool
}

func newMockProcessor() *mockProcessor {
	info := ProcessorInfo{
		Name:        "MockProcessor",
		Description: "A mock processor for testing",
		Version:     "1.0.0",
		Author:      "Test",
		Tags:        []string{"test", "mock"},
		Properties: []PropertySpec{
			{
				Name:         "required-prop",
				Description:  "A required property",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:         "optional-prop",
				Description:  "An optional property",
				Required:     false,
				DefaultValue: "default-value",
			},
		},
		Relationships: []Relationship{
			RelationshipSuccess,
			RelationshipFailure,
		},
	}

	return &mockProcessor{
		BaseProcessor: NewBaseProcessor(info),
	}
}

func (p *mockProcessor) Initialize(ctx ProcessorContext) error {
	p.initCalled = true
	if p.shouldError {
		return ErrMockError
	}
	return nil
}

func (p *mockProcessor) OnTrigger(ctx context.Context, session ProcessSession) error {
	p.triggerCalled = true
	if p.shouldError {
		return ErrMockError
	}
	return nil
}

func (p *mockProcessor) OnStopped(ctx context.Context) {
	p.stopCalled = true
}

var ErrMockError = NewMockError("mock processor error")

type MockError struct {
	message string
}

func NewMockError(message string) *MockError {
	return &MockError{message: message}
}

func (e *MockError) Error() string {
	return e.message
}

// Mock ProcessorContext for testing
type mockProcessorContext struct {
	properties map[string]string
	config     ProcessorConfig
}

func newMockProcessorContext(properties map[string]string) *mockProcessorContext {
	config := ProcessorConfig{
		ID:         uuid.New(),
		Name:       "Test Processor",
		Type:       "MockProcessor",
		Properties: properties,
	}

	return &mockProcessorContext{
		properties: properties,
		config:     config,
	}
}

func (ctx *mockProcessorContext) GetProperty(name string) (string, bool) {
	value, exists := ctx.properties[name]
	return value, exists
}

func (ctx *mockProcessorContext) GetPropertyValue(name string) string {
	value := ctx.properties[name]
	return value
}

func (ctx *mockProcessorContext) HasProperty(name string) bool {
	_, exists := ctx.properties[name]
	return exists
}

func (ctx *mockProcessorContext) GetProcessorConfig() ProcessorConfig {
	return ctx.config
}

func (ctx *mockProcessorContext) GetLogger() Logger {
	return &mockLogger{}
}

// Mock Logger for testing
type mockLogger struct {
	debugCalled bool
	infoCalled  bool
	warnCalled  bool
	errorCalled bool
}

func (l *mockLogger) Debug(msg string, fields ...interface{}) {
	l.debugCalled = true
}

func (l *mockLogger) Info(msg string, fields ...interface{}) {
	l.infoCalled = true
}

func (l *mockLogger) Warn(msg string, fields ...interface{}) {
	l.warnCalled = true
}

func (l *mockLogger) Error(msg string, fields ...interface{}) {
	l.errorCalled = true
}

// Mock ProcessSession for testing
type mockProcessSession struct {
	flowFiles    []*FlowFile
	createdFiles []*FlowFile
	transfers    map[uuid.UUID]Relationship
	removals     []uuid.UUID
	committed    bool
	rolledBack   bool
}

func newMockProcessSession() *mockProcessSession {
	return &mockProcessSession{
		flowFiles: make([]*FlowFile, 0),
		transfers: make(map[uuid.UUID]Relationship),
		removals:  make([]uuid.UUID, 0),
	}
}

func (s *mockProcessSession) Get() *FlowFile {
	if len(s.flowFiles) > 0 {
		flowFile := s.flowFiles[0]
		s.flowFiles = s.flowFiles[1:]
		return flowFile
	}
	return nil
}

func (s *mockProcessSession) GetBatch(maxResults int) []*FlowFile {
	result := make([]*FlowFile, 0)
	for i := 0; i < maxResults && len(s.flowFiles) > 0; i++ {
		result = append(result, s.Get())
	}
	return result
}

func (s *mockProcessSession) Create() *FlowFile {
	flowFile := NewFlowFile()
	s.createdFiles = append(s.createdFiles, flowFile)
	return flowFile
}

func (s *mockProcessSession) CreateChild(parent *FlowFile) *FlowFile {
	child := NewFlowFileBuilder().WithParent(parent).Build()
	s.createdFiles = append(s.createdFiles, child)
	return child
}

func (s *mockProcessSession) Clone(original *FlowFile) *FlowFile {
	clone := original.Clone()
	s.createdFiles = append(s.createdFiles, clone)
	return clone
}

func (s *mockProcessSession) Transfer(flowFile *FlowFile, relationship Relationship) {
	s.transfers[flowFile.ID] = relationship
}

func (s *mockProcessSession) Remove(flowFile *FlowFile) {
	s.removals = append(s.removals, flowFile.ID)
}

func (s *mockProcessSession) PutAttribute(flowFile *FlowFile, key, value string) {
	flowFile.UpdateAttribute(key, value)
}

func (s *mockProcessSession) PutAllAttributes(flowFile *FlowFile, attributes map[string]string) {
	for k, v := range attributes {
		flowFile.UpdateAttribute(k, v)
	}
}

func (s *mockProcessSession) RemoveAttribute(flowFile *FlowFile, key string) {
	flowFile.RemoveAttribute(key)
}

func (s *mockProcessSession) Write(flowFile *FlowFile, content []byte) error {
	// Mock implementation - just update size
	flowFile.Size = int64(len(content))
	return nil
}

func (s *mockProcessSession) Read(flowFile *FlowFile) ([]byte, error) {
	// Mock implementation - return dummy content
	return []byte("mock content"), nil
}

func (s *mockProcessSession) Commit() error {
	s.committed = true
	return nil
}

func (s *mockProcessSession) Rollback() {
	s.rolledBack = true
}

func (s *mockProcessSession) GetLogger() Logger {
	return &mockLogger{}
}

// Add a helper method to add FlowFiles to the session for testing
func (s *mockProcessSession) addFlowFile(flowFile *FlowFile) {
	s.flowFiles = append(s.flowFiles, flowFile)
}

// Test cases
func TestProcessorInfo(t *testing.T) {
	processor := newMockProcessor()
	info := processor.GetInfo()

	if info.Name != "MockProcessor" {
		t.Errorf("Expected name=MockProcessor, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version=1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 2 {
		t.Errorf("Expected 2 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(info.Relationships))
	}

	// Test specific property
	requiredProp := info.Properties[0]
	if requiredProp.Name != "required-prop" {
		t.Errorf("Expected required-prop, got %s", requiredProp.Name)
	}

	if !requiredProp.Required {
		t.Error("First property should be required")
	}

	optionalProp := info.Properties[1]
	if optionalProp.Required {
		t.Error("Second property should be optional")
	}

	if optionalProp.DefaultValue != "default-value" {
		t.Errorf("Expected default-value, got %s", optionalProp.DefaultValue)
	}
}

func TestProcessorValidation(t *testing.T) {
	processor := newMockProcessor()

	// Test valid configuration
	validConfig := ProcessorConfig{
		ID:   uuid.New(),
		Name: "Test Processor",
		Type: "MockProcessor",
		Properties: map[string]string{
			"required-prop": "some-value",
			"optional-prop": "custom-value",
		},
	}

	results := processor.Validate(validConfig)
	if len(results) != 0 {
		t.Errorf("Valid configuration should have no validation errors, got %d", len(results))
	}

	// Test invalid configuration - missing required property
	invalidConfig := ProcessorConfig{
		ID:   uuid.New(),
		Name: "Test Processor",
		Type: "MockProcessor",
		Properties: map[string]string{
			"optional-prop": "custom-value",
		},
	}

	results = processor.Validate(invalidConfig)
	if len(results) != 1 {
		t.Errorf("Invalid configuration should have 1 validation error, got %d", len(results))
	}

	if results[0].Property != "required-prop" {
		t.Errorf("Validation error should be for required-prop, got %s", results[0].Property)
	}

	if results[0].Valid {
		t.Error("Validation result should be invalid")
	}

	// Test empty required property
	emptyRequiredConfig := ProcessorConfig{
		ID:   uuid.New(),
		Name: "Test Processor",
		Type: "MockProcessor",
		Properties: map[string]string{
			"required-prop": "",
			"optional-prop": "custom-value",
		},
	}

	results = processor.Validate(emptyRequiredConfig)
	if len(results) != 1 {
		t.Errorf("Configuration with empty required property should have 1 validation error, got %d", len(results))
	}
}

func TestProcessorInitialize(t *testing.T) {
	processor := newMockProcessor()
	ctx := newMockProcessorContext(map[string]string{
		"required-prop": "test-value",
	})

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should not return error, got %v", err)
	}

	if !processor.initCalled {
		t.Error("Initialize method should be called")
	}

	// Test initialize with error
	processor2 := newMockProcessor()
	processor2.shouldError = true

	err = processor2.Initialize(ctx)
	if err == nil {
		t.Error("Initialize should return error when shouldError is true")
	}

	if err != ErrMockError {
		t.Errorf("Expected ErrMockError, got %v", err)
	}
}

func TestProcessorOnTrigger(t *testing.T) {
	processor := newMockProcessor()
	session := newMockProcessSession()
	ctx := context.Background()

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error, got %v", err)
	}

	if !processor.triggerCalled {
		t.Error("OnTrigger method should be called")
	}

	// Test OnTrigger with error
	processor2 := newMockProcessor()
	processor2.shouldError = true

	err = processor2.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when shouldError is true")
	}
}

func TestProcessorOnStopped(t *testing.T) {
	processor := newMockProcessor()
	ctx := context.Background()

	processor.OnStopped(ctx)

	if !processor.stopCalled {
		t.Error("OnStopped method should be called")
	}
}

func TestProcessorContext(t *testing.T) {
	properties := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	ctx := newMockProcessorContext(properties)

	// Test GetProperty
	value, exists := ctx.GetProperty("key1")
	if !exists {
		t.Error("GetProperty should return true for existing property")
	}

	if value != "value1" {
		t.Errorf("Expected value1, got %s", value)
	}

	// Test GetProperty for non-existent key
	_, exists = ctx.GetProperty("nonexistent")
	if exists {
		t.Error("GetProperty should return false for non-existent property")
	}

	// Test GetPropertyValue
	value = ctx.GetPropertyValue("key2")
	if value != "value2" {
		t.Errorf("Expected value2, got %s", value)
	}

	// Test GetPropertyValue for non-existent key
	value = ctx.GetPropertyValue("nonexistent")
	if value != "" {
		t.Errorf("Expected empty string for non-existent property, got %s", value)
	}

	// Test HasProperty
	if !ctx.HasProperty("key1") {
		t.Error("HasProperty should return true for existing property")
	}

	if ctx.HasProperty("nonexistent") {
		t.Error("HasProperty should return false for non-existent property")
	}

	// Test GetProcessorConfig
	config := ctx.GetProcessorConfig()
	if config.Name != "Test Processor" {
		t.Errorf("Expected Test Processor, got %s", config.Name)
	}

	if len(config.Properties) != len(properties) {
		t.Errorf("Expected %d properties, got %d", len(properties), len(config.Properties))
	}
}

func TestProcessSession(t *testing.T) {
	session := newMockProcessSession()

	// Test Create
	created := session.Create()
	if created == nil {
		t.Error("Create should return a FlowFile")
	}

	if len(session.createdFiles) != 1 {
		t.Errorf("Expected 1 created file, got %d", len(session.createdFiles))
	}

	// Test CreateChild
	parent := NewFlowFile()
	child := session.CreateChild(parent)
	if child == nil {
		t.Error("CreateChild should return a FlowFile")
	}

	if len(child.Lineage.ParentUUIDs) != 1 {
		t.Errorf("Child should have 1 parent, got %d", len(child.Lineage.ParentUUIDs))
	}

	if child.Lineage.ParentUUIDs[0] != parent.ID {
		t.Error("Child should have correct parent ID")
	}

	// Test Clone
	original := NewFlowFile()
	original.Attributes["test"] = "value"
	clone := session.Clone(original)

	if clone.ID == original.ID {
		t.Error("Clone should have different ID")
	}

	if clone.Attributes["test"] != "value" {
		t.Error("Clone should inherit attributes")
	}

	// Test Get with no FlowFiles
	retrieved := session.Get()
	if retrieved != nil {
		t.Error("Get should return nil when no FlowFiles available")
	}

	// Test Get with FlowFiles
	testFlowFile := NewFlowFile()
	session.addFlowFile(testFlowFile)

	retrieved = session.Get()
	if retrieved == nil {
		t.Error("Get should return FlowFile when available")
	}

	if retrieved.ID != testFlowFile.ID {
		t.Error("Get should return correct FlowFile")
	}

	// Test GetBatch
	for i := 0; i < 5; i++ {
		session.addFlowFile(NewFlowFile())
	}

	batch := session.GetBatch(3)
	if len(batch) != 3 {
		t.Errorf("GetBatch should return 3 FlowFiles, got %d", len(batch))
	}

	// Test Transfer
	flowFile := NewFlowFile()
	session.Transfer(flowFile, RelationshipSuccess)

	if rel, exists := session.transfers[flowFile.ID]; !exists || rel != RelationshipSuccess {
		t.Error("Transfer should record the relationship")
	}

	// Test Remove
	session.Remove(flowFile)
	if len(session.removals) != 1 {
		t.Errorf("Expected 1 removal, got %d", len(session.removals))
	}

	if session.removals[0] != flowFile.ID {
		t.Error("Remove should record correct FlowFile ID")
	}

	// Test attribute operations
	testFile := NewFlowFile()
	session.PutAttribute(testFile, "testKey", "testValue")

	if testFile.Attributes["testKey"] != "testValue" {
		t.Error("PutAttribute should set attribute")
	}

	attributes := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	session.PutAllAttributes(testFile, attributes)

	for k, v := range attributes {
		if testFile.Attributes[k] != v {
			t.Errorf("PutAllAttributes should set %s=%s", k, v)
		}
	}

	session.RemoveAttribute(testFile, "testKey")
	if _, exists := testFile.Attributes["testKey"]; exists {
		t.Error("RemoveAttribute should remove attribute")
	}

	// Test Write and Read
	err := session.Write(testFile, []byte("test content"))
	if err != nil {
		t.Errorf("Write should not return error, got %v", err)
	}

	content, err := session.Read(testFile)
	if err != nil {
		t.Errorf("Read should not return error, got %v", err)
	}

	if string(content) != "mock content" {
		t.Errorf("Expected mock content, got %s", string(content))
	}

	// Test Commit and Rollback
	err = session.Commit()
	if err != nil {
		t.Errorf("Commit should not return error, got %v", err)
	}

	if !session.committed {
		t.Error("Session should be marked as committed")
	}

	session2 := newMockProcessSession()
	session2.Rollback()

	if !session2.rolledBack {
		t.Error("Session should be marked as rolled back")
	}
}

func TestRelationships(t *testing.T) {
	// Test predefined relationships
	if RelationshipSuccess.Name != "success" {
		t.Errorf("Expected success relationship name, got %s", RelationshipSuccess.Name)
	}

	if RelationshipFailure.Name != "failure" {
		t.Errorf("Expected failure relationship name, got %s", RelationshipFailure.Name)
	}

	if RelationshipRetry.Name != "retry" {
		t.Errorf("Expected retry relationship name, got %s", RelationshipRetry.Name)
	}

	if RelationshipOriginal.Name != "original" {
		t.Errorf("Expected original relationship name, got %s", RelationshipOriginal.Name)
	}
}

func TestScheduleTypes(t *testing.T) {
	// Test schedule type constants
	if ScheduleTypeTimer != "TIMER_DRIVEN" {
		t.Errorf("Expected TIMER_DRIVEN, got %s", ScheduleTypeTimer)
	}

	if ScheduleTypeEvent != "EVENT_DRIVEN" {
		t.Errorf("Expected EVENT_DRIVEN, got %s", ScheduleTypeEvent)
	}

	if ScheduleTypeCron != "CRON_DRIVEN" {
		t.Errorf("Expected CRON_DRIVEN, got %s", ScheduleTypeCron)
	}

	if ScheduleTypePrimaryNode != "PRIMARY_NODE_ONLY" {
		t.Errorf("Expected PRIMARY_NODE_ONLY, got %s", ScheduleTypePrimaryNode)
	}
}

func TestProcessorStates(t *testing.T) {
	// Test processor state constants
	if ProcessorStateStopped != "STOPPED" {
		t.Errorf("Expected STOPPED, got %s", ProcessorStateStopped)
	}

	if ProcessorStateRunning != "RUNNING" {
		t.Errorf("Expected RUNNING, got %s", ProcessorStateRunning)
	}

	if ProcessorStateDisabled != "DISABLED" {
		t.Errorf("Expected DISABLED, got %s", ProcessorStateDisabled)
	}

	if ProcessorStateInvalid != "INVALID" {
		t.Errorf("Expected INVALID, got %s", ProcessorStateInvalid)
	}
}

// Benchmark tests
func BenchmarkProcessorValidate(b *testing.B) {
	processor := newMockProcessor()
	config := ProcessorConfig{
		ID:   uuid.New(),
		Name: "Test Processor",
		Type: "MockProcessor",
		Properties: map[string]string{
			"required-prop": "value",
			"optional-prop": "value",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.Validate(config)
	}
}

func BenchmarkProcessorOnTrigger(b *testing.B) {
	processor := newMockProcessor()
	session := newMockProcessSession()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.OnTrigger(ctx, session)
	}
}
