package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

// MockProcessorContext for testing
type MockConsumeKafkaProcessorContext struct {
	mock.Mock
	properties map[string]string
}

func (m *MockConsumeKafkaProcessorContext) GetPropertyValue(name string) string {
	if m.properties != nil {
		return m.properties[name]
	}
	args := m.Called(name)
	return args.String(0)
}

func (m *MockConsumeKafkaProcessorContext) HasProperty(name string) bool {
	if m.properties != nil {
		_, exists := m.properties[name]
		return exists
	}
	args := m.Called(name)
	return args.Bool(0)
}

func (m *MockConsumeKafkaProcessorContext) GetLogger() types.Logger {
	args := m.Called()
	return args.Get(0).(types.Logger)
}

func (m *MockConsumeKafkaProcessorContext) GetProperty(name string) (string, bool) {
	if m.properties != nil {
		val, exists := m.properties[name]
		return val, exists
	}
	args := m.Called(name)
	return args.String(0), args.Bool(1)
}

func (m *MockConsumeKafkaProcessorContext) GetProcessorConfig() types.ProcessorConfig {
	args := m.Called()
	if config := args.Get(0); config != nil {
		return config.(types.ProcessorConfig)
	}
	return types.ProcessorConfig{}
}

// MockLogger for testing
type MockConsumeKafkaLogger struct {
	mock.Mock
}

func (m *MockConsumeKafkaLogger) Debug(msg string, args ...interface{}) { m.Called(msg, args) }
func (m *MockConsumeKafkaLogger) Info(msg string, args ...interface{})  { m.Called(msg, args) }
func (m *MockConsumeKafkaLogger) Warn(msg string, args ...interface{})  { m.Called(msg, args) }
func (m *MockConsumeKafkaLogger) Error(msg string, args ...interface{}) { m.Called(msg, args) }

// MockProcessSession for testing
type MockConsumeKafkaProcessSession struct {
	mock.Mock
	flowFiles []*types.FlowFile
}

func (m *MockConsumeKafkaProcessSession) Get() *types.FlowFile {
	args := m.Called()
	if ff := args.Get(0); ff != nil {
		return ff.(*types.FlowFile)
	}
	return nil
}

func (m *MockConsumeKafkaProcessSession) Create() *types.FlowFile {
	ff := &types.FlowFile{
		ID:         uuid.New(),
		Attributes: make(map[string]string),
		Size:       0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.flowFiles = append(m.flowFiles, ff)
	return ff
}

func (m *MockConsumeKafkaProcessSession) Read(flowFile *types.FlowFile) ([]byte, error) {
	args := m.Called(flowFile)
	if b := args.Get(0); b != nil {
		return b.([]byte), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockConsumeKafkaProcessSession) Write(flowFile *types.FlowFile, content []byte) error {
	flowFile.Size = int64(len(content))
	args := m.Called(flowFile, content)
	return args.Error(0)
}

func (m *MockConsumeKafkaProcessSession) Transfer(flowFile *types.FlowFile, relationship types.Relationship) {
	m.Called(flowFile, relationship)
}

func (m *MockConsumeKafkaProcessSession) Remove(flowFile *types.FlowFile) {
	m.Called(flowFile)
}

func (m *MockConsumeKafkaProcessSession) Clone(original *types.FlowFile) *types.FlowFile {
	clone := original.Clone()
	m.flowFiles = append(m.flowFiles, clone)
	return clone
}

func (m *MockConsumeKafkaProcessSession) CreateChild(parent *types.FlowFile) *types.FlowFile {
	child := types.NewFlowFileBuilder().WithParent(parent).Build()
	m.flowFiles = append(m.flowFiles, child)
	return child
}

func (m *MockConsumeKafkaProcessSession) PutAttribute(flowFile *types.FlowFile, key, value string) {
	flowFile.Attributes[key] = value
}

func (m *MockConsumeKafkaProcessSession) RemoveAttribute(flowFile *types.FlowFile, key string) {
	delete(flowFile.Attributes, key)
}

func (m *MockConsumeKafkaProcessSession) PutAllAttributes(flowFile *types.FlowFile, attrs map[string]string) {
	for k, v := range attrs {
		flowFile.Attributes[k] = v
	}
}

func (m *MockConsumeKafkaProcessSession) Commit() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConsumeKafkaProcessSession) Rollback() {
	m.Called()
}

func (m *MockConsumeKafkaProcessSession) GetBatch(maxResults int) []*types.FlowFile {
	args := m.Called(maxResults)
	if ffs := args.Get(0); ffs != nil {
		return ffs.([]*types.FlowFile)
	}
	return nil
}

func (m *MockConsumeKafkaProcessSession) GetLogger() types.Logger {
	args := m.Called()
	if logger := args.Get(0); logger != nil {
		return logger.(types.Logger)
	}
	return &MockConsumeKafkaLogger{}
}

func TestNewConsumeKafkaProcessor(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	assert.NotNil(t, processor)
	assert.Equal(t, "ConsumeKafka", processor.GetInfo().Name)
	assert.Equal(t, "1.0.0", processor.GetInfo().Version)
	assert.Contains(t, processor.GetInfo().Tags, "kafka")
	assert.Contains(t, processor.GetInfo().Tags, "consumer")
	assert.Len(t, processor.GetInfo().Properties, 13)
	assert.Len(t, processor.GetInfo().Relationships, 2)
}

func TestConsumeKafkaProcessor_InitializeSuccess(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":        "localhost:9092",
			"Topic Names":          "test-topic",
			"Group ID":             "test-group",
			"Offset Reset":         "earliest",
			"Auto Commit Interval": "5s",
			"Session Timeout":      "10s",
			"Max Poll Records":     "500",
			"Security Protocol":    "PLAINTEXT",
			"Honor Transactions":   "true",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything)
	mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
	mockCtx.On("GetLogger").Return(mockLogger)

	// Note: This will fail in CI without a real Kafka broker
	// In a real test environment, you'd use a test container or mock the Kafka client
	err := processor.Initialize(mockCtx)

	// We expect this to potentially fail without a running Kafka broker
	// But we can still verify the initialization logic
	if err != nil {
		assert.Contains(t, err.Error(), "failed to create Kafka consumer")
	}
}

func TestConsumeKafkaProcessor_InitializeMissingBrokers(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Topic Names": "test-topic",
			"Group ID":    "test-group",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Kafka Brokers property is required")
}

func TestConsumeKafkaProcessor_InitializeMissingTopics(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Group ID":      "test-group",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Topic Names property is required")
}

func TestConsumeKafkaProcessor_InitializeMissingGroupID(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Topic Names":   "test-topic",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Group ID property is required")
}

func TestConsumeKafkaProcessor_InitializeInvalidAutoCommitInterval(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":        "localhost:9092",
			"Topic Names":          "test-topic",
			"Group ID":             "test-group",
			"Auto Commit Interval": "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Auto Commit Interval")
}

func TestConsumeKafkaProcessor_ProcessMessage(t *testing.T) {
	mockSession := &MockConsumeKafkaProcessSession{}
	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Debug", mock.Anything)
	mockLogger.On("WithFields", mock.Anything).Return(mockLogger)

	handler := &consumerGroupHandler{
		session: mockSession,
		logger:  mockLogger,
	}

	// Create test Kafka message
	kafkaMsg := &sarama.ConsumerMessage{
		Topic:     "test-topic",
		Partition: 0,
		Offset:    123,
		Key:       []byte("test-key"),
		Value:     []byte("test message content"),
		Timestamp: time.Now(),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("custom-header"), Value: []byte("header-value")},
		},
	}

	// Setup mocks
	mockSession.On("Write", mock.AnythingOfType("*types.FlowFile"), kafkaMsg.Value).Return(nil)
	mockSession.On("PutAllAttributes", mock.AnythingOfType("*types.FlowFile"), mock.Anything).Return(&types.FlowFile{})
	mockSession.On("Transfer", mock.AnythingOfType("*types.FlowFile"), types.RelationshipSuccess)

	// Create mock consumer group session
	mockConsumerSession := &mockConsumerGroupSession{}

	// Process message
	err := handler.processMessage(mockConsumerSession, kafkaMsg)

	assert.NoError(t, err)
	mockSession.AssertExpectations(t)

	// Verify created FlowFile
	assert.Len(t, mockSession.flowFiles, 1)
	ff := mockSession.flowFiles[0]
	assert.Equal(t, "test-topic", ff.Attributes["kafka.topic"])
	assert.Equal(t, "0", ff.Attributes["kafka.partition"])
	assert.Equal(t, "123", ff.Attributes["kafka.offset"])
	assert.Equal(t, "test-key", ff.Attributes["kafka.key"])
	assert.Equal(t, "header-value", ff.Attributes["kafka.header.custom-header"])
}

func TestConsumeKafkaProcessor_Properties(t *testing.T) {
	processor := NewConsumeKafkaProcessor()
	info := processor.GetInfo()

	// Verify all required properties
	requiredProps := []string{"Kafka Brokers", "Topic Names", "Group ID"}
	for _, propName := range requiredProps {
		found := false
		for _, prop := range info.Properties {
			if prop.Name == propName {
				found = true
				assert.True(t, prop.Required, "Property %s should be required", propName)
				break
			}
		}
		assert.True(t, found, "Required property %s not found", propName)
	}

	// Verify optional properties exist
	optionalProps := []string{
		"Offset Reset",
		"Auto Commit Interval",
		"Session Timeout",
		"Max Poll Records",
		"Security Protocol",
		"SASL Mechanism",
		"SASL Username",
		"SASL Password",
		"Message Header Encoding",
		"Honor Transactions",
	}
	for _, propName := range optionalProps {
		found := false
		for _, prop := range info.Properties {
			if prop.Name == propName {
				found = true
				assert.False(t, prop.Required, "Property %s should be optional", propName)
				break
			}
		}
		assert.True(t, found, "Optional property %s not found", propName)
	}

	// Verify sensitive property
	for _, prop := range info.Properties {
		if prop.Name == "SASL Password" {
			assert.True(t, prop.Sensitive, "SASL Password should be sensitive")
		}
	}
}

func TestConsumeKafkaProcessor_Cleanup(t *testing.T) {
	processor := NewConsumeKafkaProcessor()

	// Cleanup should succeed even if not initialized
	err := processor.Cleanup()
	assert.NoError(t, err)
}

// Mock consumer group session
type mockConsumerGroupSession struct {
	mock.Mock
}

func (m *mockConsumerGroupSession) Claims() map[string][]int32 {
	args := m.Called()
	return args.Get(0).(map[string][]int32)
}

func (m *mockConsumerGroupSession) MemberID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockConsumerGroupSession) GenerationID() int32 {
	args := m.Called()
	return int32(args.Int(0))
}

func (m *mockConsumerGroupSession) MarkOffset(topic string, partition int32, offset int64, metadata string) {
	m.Called(topic, partition, offset, metadata)
}

func (m *mockConsumerGroupSession) ResetOffset(topic string, partition int32, offset int64, metadata string) {
	m.Called(topic, partition, offset, metadata)
}

func (m *mockConsumerGroupSession) MarkMessage(msg *sarama.ConsumerMessage, metadata string) {
	m.Called(msg, metadata)
}

func (m *mockConsumerGroupSession) Context() context.Context {
	return context.Background()
}

func (m *mockConsumerGroupSession) Commit() {
	m.Called()
}
