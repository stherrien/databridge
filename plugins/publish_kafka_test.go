package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewPublishKafkaProcessor(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	assert.NotNil(t, processor)
	assert.Equal(t, "PublishKafka", processor.GetInfo().Name)
	assert.Equal(t, "1.0.0", processor.GetInfo().Version)
	assert.Contains(t, processor.GetInfo().Tags, "kafka")
	assert.Contains(t, processor.GetInfo().Tags, "producer")
	assert.Len(t, processor.GetInfo().Properties, 21)
	assert.Len(t, processor.GetInfo().Relationships, 2)
}

func TestPublishKafkaProcessor_InitializeSuccess(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":      "localhost:9092",
			"Topic Name":         "test-topic",
			"Delivery Guarantee": "all",
			"Compression Type":   "gzip",
			"Max Message Size":   "1048576",
			"Request Timeout":    "30s",
			"Max Retries":        "3",
			"Async Mode":         "false",
			"Security Protocol":  "PLAINTEXT",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
	mockCtx.On("GetLogger").Return(mockLogger)

	// Note: This will fail without a real Kafka broker
	err := processor.Initialize(mockCtx)

	if err != nil {
		assert.Contains(t, err.Error(), "failed to create")
	}
}

func TestPublishKafkaProcessor_InitializeMissingBrokers(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Topic Name": "test-topic",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Kafka Brokers property is required")
}

func TestPublishKafkaProcessor_InitializeMissingTopic(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Topic Name property is required")
}

func TestPublishKafkaProcessor_InitializeInvalidMaxMessageSize(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":    "localhost:9092",
			"Topic Name":       "test-topic",
			"Max Message Size": "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Max Message Size")
}

func TestPublishKafkaProcessor_InitializeInvalidTimeout(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":   "localhost:9092",
			"Topic Name":      "test-topic",
			"Request Timeout": "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Request Timeout")
}

func TestPublishKafkaProcessor_InitializeInvalidRetries(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Topic Name":    "test-topic",
			"Max Retries":   "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Max Retries")
}

func TestPublishKafkaProcessor_InitializeAsyncMode(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Topic Name":    "test-topic",
			"Async Mode":    "true",
			"Batch Size":    "100",
			"Linger Time":   "10ms",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	if err != nil {
		assert.Contains(t, err.Error(), "failed to create")
	}
	assert.True(t, processor.isAsync)
}

func TestPublishKafkaProcessor_InitializeInvalidBatchSize(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Topic Name":    "test-topic",
			"Async Mode":    "true",
			"Batch Size":    "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Batch Size")
}

func TestPublishKafkaProcessor_InitializeInvalidLingerTime(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers": "localhost:9092",
			"Topic Name":    "test-topic",
			"Async Mode":    "true",
			"Linger Time":   "invalid",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Linger Time")
}

func TestPublishKafkaProcessor_InitializeTransactions(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":    "localhost:9092",
			"Topic Name":       "test-topic",
			"Use Transactions": "true",
			"Transactional ID": "test-tx-id",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	if err != nil {
		assert.Contains(t, err.Error(), "failed to create")
	}
}

func TestPublishKafkaProcessor_InitializeTransactionsMissingID(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	mockCtx := &MockConsumeKafkaProcessorContext{
		properties: map[string]string{
			"Kafka Brokers":    "localhost:9092",
			"Topic Name":       "test-topic",
			"Use Transactions": "true",
		},
	}

	mockLogger := &MockConsumeKafkaLogger{}
	mockLogger.On("Info", mock.Anything, mock.Anything)
	mockCtx.On("GetLogger").Return(mockLogger)

	err := processor.Initialize(mockCtx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Transactional ID is required")
}

func TestPublishKafkaProcessor_Properties(t *testing.T) {
	processor := NewPublishKafkaProcessor()
	info := processor.GetInfo()

	// Verify required properties
	requiredProps := []string{"Kafka Brokers", "Topic Name"}
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

	// Verify optional properties
	optionalProps := []string{
		"Delivery Guarantee",
		"Message Key Field",
		"Partition Strategy",
		"Partition",
		"Compression Type",
		"Max Message Size",
		"Request Timeout",
		"Max Retries",
		"Async Mode",
		"Batch Size",
		"Linger Time",
		"Security Protocol",
		"SASL Mechanism",
		"SASL Username",
		"SASL Password",
		"Use Transactions",
		"Transactional ID",
		"Attributes as Headers",
		"Failure Strategy",
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

func TestPublishKafkaProcessor_AllowedValues(t *testing.T) {
	processor := NewPublishKafkaProcessor()
	info := processor.GetInfo()

	// Test Delivery Guarantee allowed values
	for _, prop := range info.Properties {
		if prop.Name == "Delivery Guarantee" {
			assert.ElementsMatch(t, []string{"none", "leader", "all"}, prop.AllowedValues)
		}
		if prop.Name == "Partition Strategy" {
			assert.ElementsMatch(t, []string{"default", "round-robin", "manual"}, prop.AllowedValues)
		}
		if prop.Name == "Compression Type" {
			assert.ElementsMatch(t, []string{"none", "gzip", "snappy", "lz4", "zstd"}, prop.AllowedValues)
		}
		if prop.Name == "Security Protocol" {
			assert.ElementsMatch(t, []string{"PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"}, prop.AllowedValues)
		}
		if prop.Name == "SASL Mechanism" {
			assert.ElementsMatch(t, []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}, prop.AllowedValues)
		}
		if prop.Name == "Failure Strategy" {
			assert.ElementsMatch(t, []string{"retry", "fail", "ignore"}, prop.AllowedValues)
		}
	}
}

func TestPublishKafkaProcessor_Cleanup(t *testing.T) {
	processor := NewPublishKafkaProcessor()

	// Cleanup should succeed even if not initialized
	err := processor.Cleanup()
	assert.NoError(t, err)
}

func TestPublishKafkaProcessor_CompressionTypes(t *testing.T) {
	tests := []struct {
		name           string
		compressionStr string
	}{
		{"No compression", "none"},
		{"GZIP compression", "gzip"},
		{"Snappy compression", "snappy"},
		{"LZ4 compression", "lz4"},
		{"ZSTD compression", "zstd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewPublishKafkaProcessor()

			mockCtx := &MockConsumeKafkaProcessorContext{
				properties: map[string]string{
					"Kafka Brokers":    "localhost:9092",
					"Topic Name":       "test-topic",
					"Compression Type": tt.compressionStr,
				},
			}

			mockLogger := &MockConsumeKafkaLogger{}
			mockLogger.On("Info", mock.Anything, mock.Anything)
			mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
			mockCtx.On("GetLogger").Return(mockLogger)

			err := processor.Initialize(mockCtx)
			if err != nil {
				assert.Contains(t, err.Error(), "failed to create")
			}
		})
	}
}

func TestPublishKafkaProcessor_PartitionStrategies(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
	}{
		{"Default partitioner", "default"},
		{"Round-robin partitioner", "round-robin"},
		{"Manual partitioner", "manual"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewPublishKafkaProcessor()

			mockCtx := &MockConsumeKafkaProcessorContext{
				properties: map[string]string{
					"Kafka Brokers":      "localhost:9092",
					"Topic Name":         "test-topic",
					"Partition Strategy": tt.strategy,
					"Partition":          "0",
				},
			}

			mockLogger := &MockConsumeKafkaLogger{}
			mockLogger.On("Info", mock.Anything, mock.Anything)
			mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
			mockCtx.On("GetLogger").Return(mockLogger)

			err := processor.Initialize(mockCtx)
			if err != nil {
				assert.Contains(t, err.Error(), "failed to create")
			}
		})
	}
}

func TestPublishKafkaProcessor_DeliveryGuarantees(t *testing.T) {
	tests := []struct {
		name      string
		guarantee string
	}{
		{"No acknowledgement", "none"},
		{"Leader acknowledgement", "leader"},
		{"All replicas acknowledgement", "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewPublishKafkaProcessor()

			mockCtx := &MockConsumeKafkaProcessorContext{
				properties: map[string]string{
					"Kafka Brokers":      "localhost:9092",
					"Topic Name":         "test-topic",
					"Delivery Guarantee": tt.guarantee,
				},
			}

			mockLogger := &MockConsumeKafkaLogger{}
			mockLogger.On("Info", mock.Anything, mock.Anything)
			mockLogger.On("WithFields", mock.Anything).Return(mockLogger)
			mockCtx.On("GetLogger").Return(mockLogger)

			err := processor.Initialize(mockCtx)
			if err != nil {
				assert.Contains(t, err.Error(), "failed to create")
			}
		})
	}
}
