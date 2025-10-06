package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

// PublishKafkaProcessor publishes FlowFiles to Apache Kafka topics
type PublishKafkaProcessor struct {
	*types.BaseProcessor
	producer      sarama.SyncProducer
	asyncProducer sarama.AsyncProducer
	isAsync       bool
	processorCtx  types.ProcessorContext
	isInitialized bool
	mu            sync.Mutex
	wg            sync.WaitGroup
}

// NewPublishKafkaProcessor creates a new PublishKafka processor
func NewPublishKafkaProcessor() *PublishKafkaProcessor {
	info := types.ProcessorInfo{
		Name:        "PublishKafka",
		Description: "Publishes FlowFile content as messages to Apache Kafka topics",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"kafka", "producer", "messaging", "streaming", "publish"},
		Properties: []types.PropertySpec{
			{
				Name:         "Kafka Brokers",
				Description:  "Comma-separated list of Kafka broker addresses (e.g., localhost:9092,broker2:9092)",
				Required:     true,
				DefaultValue: "localhost:9092",
			},
			{
				Name:         "Topic Name",
				Description:  "Name of the Kafka topic to publish to (supports attribute expression language)",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:          "Delivery Guarantee",
				Description:   "Level of acknowledgement required from Kafka brokers",
				Required:      false,
				DefaultValue:  "all",
				AllowedValues: []string{"none", "leader", "all"},
			},
			{
				Name:         "Message Key Field",
				Description:  "FlowFile attribute name to use as Kafka message key (empty = no key)",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:          "Partition Strategy",
				Description:   "Strategy for determining message partition",
				Required:      false,
				DefaultValue:  "default",
				AllowedValues: []string{"default", "round-robin", "manual"},
			},
			{
				Name:         "Partition",
				Description:  "Partition number when using manual partition strategy",
				Required:     false,
				DefaultValue: "0",
			},
			{
				Name:          "Compression Type",
				Description:   "Compression codec for message data",
				Required:      false,
				DefaultValue:  "none",
				AllowedValues: []string{"none", "gzip", "snappy", "lz4", "zstd"},
			},
			{
				Name:         "Max Message Size",
				Description:  "Maximum size of a single message in bytes",
				Required:     false,
				DefaultValue: "1048576",
			},
			{
				Name:         "Request Timeout",
				Description:  "Timeout for produce requests (e.g., 30s, 1m)",
				Required:     false,
				DefaultValue: "30s",
			},
			{
				Name:         "Max Retries",
				Description:  "Maximum number of retry attempts for failed sends",
				Required:     false,
				DefaultValue: "3",
			},
			{
				Name:          "Async Mode",
				Description:   "Use asynchronous producer for higher throughput",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Batch Size",
				Description:  "Number of messages to batch together (async mode only)",
				Required:     false,
				DefaultValue: "100",
			},
			{
				Name:         "Linger Time",
				Description:  "Time to wait before sending batch (e.g., 10ms, 100ms)",
				Required:     false,
				DefaultValue: "10ms",
			},
			{
				Name:          "Security Protocol",
				Description:   "Security protocol for Kafka connection",
				Required:      false,
				DefaultValue:  "PLAINTEXT",
				AllowedValues: []string{"PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"},
			},
			{
				Name:          "SASL Mechanism",
				Description:   "SASL mechanism for authentication",
				Required:      false,
				DefaultValue:  "PLAIN",
				AllowedValues: []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"},
			},
			{
				Name:         "SASL Username",
				Description:  "Username for SASL authentication",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:         "SASL Password",
				Description:  "Password for SASL authentication",
				Required:     false,
				DefaultValue: "",
				Sensitive:    true,
			},
			{
				Name:          "Use Transactions",
				Description:   "Enable transactional message delivery",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Transactional ID",
				Description:  "Unique identifier for transactional producer",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:         "Attributes as Headers",
				Description:  "Comma-separated list of FlowFile attributes to send as Kafka headers",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:          "Failure Strategy",
				Description:   "How to handle publish failures",
				Required:      false,
				DefaultValue:  "retry",
				AllowedValues: []string{"retry", "fail", "ignore"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &PublishKafkaProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the Kafka producer
func (p *PublishKafkaProcessor) Initialize(ctx types.ProcessorContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isInitialized {
		return nil
	}

	logger := ctx.GetLogger()
	logger.Info("Initializing PublishKafka processor")

	p.processorCtx = ctx

	// Validate and parse brokers and topic
	brokers, topicName, err := p.validateAndParseBrokers(ctx)
	if err != nil {
		return err
	}

	// Create Kafka configuration
	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	// Configure producer settings
	if err := p.configureProducerSettings(ctx, config); err != nil {
		return err
	}

	// Configure async mode if enabled
	if err := p.configureAsyncMode(ctx, config); err != nil {
		return err
	}

	// Configure transactions if enabled
	if err := p.configureTransactions(ctx, config); err != nil {
		return err
	}

	// Configure security
	if err := p.configureSecurity(ctx, config); err != nil {
		return err
	}

	// Create producer
	if err := p.createProducer(brokers, config); err != nil {
		return err
	}

	p.isInitialized = true

	logger.Info(fmt.Sprintf("Kafka producer initialized successfully - brokers: %v, topic: %s, async: %v", brokers, topicName, p.isAsync))

	return nil
}

// validateAndParseBrokers validates and parses required properties
func (p *PublishKafkaProcessor) validateAndParseBrokers(ctx types.ProcessorContext) ([]string, string, error) {
	if !ctx.HasProperty("Kafka Brokers") {
		return nil, "", fmt.Errorf("Kafka Brokers property is required")
	}

	if !ctx.HasProperty("Topic Name") {
		return nil, "", fmt.Errorf("Topic Name property is required")
	}

	brokersStr := ctx.GetPropertyValue("Kafka Brokers")
	brokers := strings.Split(brokersStr, ",")
	for i, broker := range brokers {
		brokers[i] = strings.TrimSpace(broker)
	}

	topicName := ctx.GetPropertyValue("Topic Name")
	if topicName == "" {
		return nil, "", fmt.Errorf("Topic Name cannot be empty")
	}

	return brokers, topicName, nil
}

// configureProducerSettings configures basic producer settings
func (p *PublishKafkaProcessor) configureProducerSettings(ctx types.ProcessorContext, config *sarama.Config) error {
	// Delivery guarantee
	deliveryGuarantee := ctx.GetPropertyValue("Delivery Guarantee")
	switch deliveryGuarantee {
	case "none":
		config.Producer.RequiredAcks = sarama.NoResponse
	case "leader":
		config.Producer.RequiredAcks = sarama.WaitForLocal
	case "all":
		config.Producer.RequiredAcks = sarama.WaitForAll
	default:
		config.Producer.RequiredAcks = sarama.WaitForAll
	}

	// Compression
	compressionType := ctx.GetPropertyValue("Compression Type")
	switch compressionType {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	// Max message size
	if maxMsgSizeStr := ctx.GetPropertyValue("Max Message Size"); maxMsgSizeStr != "" {
		maxMsgSize, err := strconv.Atoi(maxMsgSizeStr)
		if err != nil {
			return fmt.Errorf("invalid Max Message Size: %w", err)
		}
		config.Producer.MaxMessageBytes = maxMsgSize
	}

	// Request timeout
	if timeoutStr := ctx.GetPropertyValue("Request Timeout"); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid Request Timeout: %w", err)
		}
		config.Producer.Timeout = timeout
	}

	// Max retries
	if maxRetriesStr := ctx.GetPropertyValue("Max Retries"); maxRetriesStr != "" {
		maxRetries, err := strconv.Atoi(maxRetriesStr)
		if err != nil {
			return fmt.Errorf("invalid Max Retries: %w", err)
		}
		config.Producer.Retry.Max = maxRetries
	}

	// Partition strategy
	partitionStrategy := ctx.GetPropertyValue("Partition Strategy")
	switch partitionStrategy {
	case "round-robin":
		config.Producer.Partitioner = sarama.NewRoundRobinPartitioner
	case "manual":
		config.Producer.Partitioner = sarama.NewManualPartitioner
	default:
		config.Producer.Partitioner = sarama.NewHashPartitioner
	}

	return nil
}

// configureAsyncMode configures async mode settings if enabled
func (p *PublishKafkaProcessor) configureAsyncMode(ctx types.ProcessorContext, config *sarama.Config) error {
	asyncModeStr := ctx.GetPropertyValue("Async Mode")
	p.isAsync = asyncModeStr == "true"

	if !p.isAsync {
		return nil
	}

	// Batch size
	if batchSizeStr := ctx.GetPropertyValue("Batch Size"); batchSizeStr != "" {
		batchSize, err := strconv.Atoi(batchSizeStr)
		if err != nil {
			return fmt.Errorf("invalid Batch Size: %w", err)
		}
		config.Producer.Flush.MaxMessages = batchSize
	}

	// Linger time
	if lingerStr := ctx.GetPropertyValue("Linger Time"); lingerStr != "" {
		linger, err := time.ParseDuration(lingerStr)
		if err != nil {
			return fmt.Errorf("invalid Linger Time: %w", err)
		}
		config.Producer.Flush.Frequency = linger
	}

	return nil
}

// configureTransactions configures transaction settings if enabled
func (p *PublishKafkaProcessor) configureTransactions(ctx types.ProcessorContext, config *sarama.Config) error {
	useTxStr := ctx.GetPropertyValue("Use Transactions")
	if useTxStr != "true" {
		return nil
	}

	txID := ctx.GetPropertyValue("Transactional ID")
	if txID == "" {
		return fmt.Errorf("Transactional ID is required when Use Transactions is enabled")
	}

	config.Producer.Idempotent = true
	config.Producer.Transaction.ID = txID
	config.Net.MaxOpenRequests = 1

	return nil
}

// configureSecurity configures security settings
func (p *PublishKafkaProcessor) configureSecurity(ctx types.ProcessorContext, config *sarama.Config) error {
	securityProtocol := ctx.GetPropertyValue("Security Protocol")

	switch securityProtocol {
	case "SSL":
		config.Net.TLS.Enable = true
	case "SASL_PLAINTEXT":
		config.Net.SASL.Enable = true
		return p.configureSASL(ctx, config, false)
	case "SASL_SSL":
		config.Net.TLS.Enable = true
		config.Net.SASL.Enable = true
		return p.configureSASL(ctx, config, true)
	}

	return nil
}

// configureSASL configures SASL authentication
func (p *PublishKafkaProcessor) configureSASL(ctx types.ProcessorContext, config *sarama.Config, enableTLS bool) error {
	config.Net.SASL.Mechanism = sarama.SASLTypePlaintext

	saslMechanism := ctx.GetPropertyValue("SASL Mechanism")
	switch saslMechanism {
	case "PLAIN":
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	case "SCRAM-SHA-256":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
	}

	username := ctx.GetPropertyValue("SASL Username")
	password := ctx.GetPropertyValue("SASL Password")
	if username != "" && password != "" {
		config.Net.SASL.User = username
		config.Net.SASL.Password = password
	}

	return nil
}

// createProducer creates the appropriate producer based on async mode
func (p *PublishKafkaProcessor) createProducer(brokers []string, config *sarama.Config) error {
	var err error

	if p.isAsync {
		p.asyncProducer, err = sarama.NewAsyncProducer(brokers, config)
		if err != nil {
			return fmt.Errorf("failed to create async Kafka producer: %w", err)
		}

		// Start error handler
		p.wg.Add(1)
		go p.handleAsyncErrors()
	} else {
		p.producer, err = sarama.NewSyncProducer(brokers, config)
		if err != nil {
			return fmt.Errorf("failed to create sync Kafka producer: %w", err)
		}
	}

	return nil
}

// OnTrigger publishes FlowFiles to Kafka
func (p *PublishKafkaProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	if !p.isInitialized {
		return fmt.Errorf("processor not initialized")
	}

	logger := p.processorCtx.GetLogger()

	// Get FlowFile from session
	flowFile := session.Get()
	if flowFile == nil {
		return nil // No FlowFile available
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content: " + err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Get topic name (support attribute expression)
	topicName := p.processorCtx.GetPropertyValue("Topic Name")
	if strings.Contains(topicName, "${") {
		// Simple attribute replacement
		for key, value := range flowFile.Attributes {
			placeholder := "${" + key + "}"
			topicName = strings.ReplaceAll(topicName, placeholder, value)
		}
	}

	// Create Kafka message
	msg := &sarama.ProducerMessage{
		Topic: topicName,
		Value: sarama.ByteEncoder(content),
	}

	// Set message key if specified
	keyField := p.processorCtx.GetPropertyValue("Message Key Field")
	if keyField != "" {
		if keyValue, exists := flowFile.Attributes[keyField]; exists {
			msg.Key = sarama.StringEncoder(keyValue)
		}
	}

	// Set partition if manual strategy
	partitionStrategy := p.processorCtx.GetPropertyValue("Partition Strategy")
	if partitionStrategy == "manual" {
		partitionStr := p.processorCtx.GetPropertyValue("Partition")
		partition, parseErr := strconv.ParseInt(partitionStr, 10, 32)
		if parseErr == nil {
			msg.Partition = int32(partition)
		}
	}

	// Add FlowFile attributes as Kafka headers
	attrsToSend := p.processorCtx.GetPropertyValue("Attributes as Headers")
	if attrsToSend != "" {
		attrNames := strings.Split(attrsToSend, ",")
		for _, attrName := range attrNames {
			attrName = strings.TrimSpace(attrName)
			if attrValue, exists := flowFile.Attributes[attrName]; exists {
				msg.Headers = append(msg.Headers, sarama.RecordHeader{
					Key:   []byte(attrName),
					Value: []byte(attrValue),
				})
			}
		}
	}

	// Send message
	if p.isAsync {
		err = p.sendAsync(msg)
	} else {
		err = p.sendSync(msg, flowFile)
	}

	if err != nil {
		failureStrategy := p.processorCtx.GetPropertyValue("Failure Strategy")
		switch failureStrategy {
		case "retry":
			logger.Info("Failed to publish to Kafka, will retry: " + err.Error())
			session.Transfer(flowFile, types.RelationshipFailure)
		case "fail":
			logger.Error("Failed to publish to Kafka: " + err.Error())
			session.Transfer(flowFile, types.RelationshipFailure)
		case "ignore":
			logger.Debug("Failed to publish to Kafka, ignoring: " + err.Error())
			session.Transfer(flowFile, types.RelationshipSuccess)
		}
		return err
	}

	// Add Kafka metadata to FlowFile
	session.PutAttribute(flowFile, "kafka.topic", topicName)
	if msg.Partition >= 0 {
		session.PutAttribute(flowFile, "kafka.partition", strconv.FormatInt(int64(msg.Partition), 10))
	}
	if msg.Offset >= 0 {
		session.PutAttribute(flowFile, "kafka.offset", strconv.FormatInt(msg.Offset, 10))
	}

	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Debug(fmt.Sprintf("Successfully published message to Kafka - topic: %s, partition: %d, offset: %d, size: %d", topicName, msg.Partition, msg.Offset, len(content)))

	return nil
}

// sendSync sends a message synchronously
func (p *PublishKafkaProcessor) sendSync(msg *sarama.ProducerMessage, flowFile *types.FlowFile) error {
	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	msg.Partition = partition
	msg.Offset = offset
	return nil
}

// sendAsync sends a message asynchronously
func (p *PublishKafkaProcessor) sendAsync(msg *sarama.ProducerMessage) error {
	select {
	case p.asyncProducer.Input() <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending message to async producer")
	}
}

// handleAsyncErrors handles errors from async producer
func (p *PublishKafkaProcessor) handleAsyncErrors() {
	defer p.wg.Done()

	logger := p.processorCtx.GetLogger()

	for {
		select {
		case err := <-p.asyncProducer.Errors():
			if err == nil {
				return
			}
			logger.Error(fmt.Sprintf("Async producer error - topic: %s, error: %s", err.Msg.Topic, err.Err.Error()))
		case success := <-p.asyncProducer.Successes():
			if success == nil {
				return
			}
			logger.Debug(fmt.Sprintf("Message sent successfully - topic: %s, partition: %d, offset: %d", success.Topic, success.Partition, success.Offset))
		}
	}
}

// Cleanup cleans up resources
func (p *PublishKafkaProcessor) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isInitialized {
		return nil
	}

	logger := p.processorCtx.GetLogger()
	logger.Info("Cleaning up PublishKafka processor")

	// Close producer
	if p.isAsync {
		if p.asyncProducer != nil {
			if err := p.asyncProducer.Close(); err != nil {
				logger.Error("Error closing async Kafka producer: " + err.Error())
				return err
			}
			p.wg.Wait() // Wait for error handler to finish
		}
	} else {
		if p.producer != nil {
			if err := p.producer.Close(); err != nil {
				logger.Error("Error closing sync Kafka producer: " + err.Error())
				return err
			}
		}
	}

	p.isInitialized = false
	logger.Info("PublishKafka processor cleaned up successfully")

	return nil
}

// getPublishKafkaInfo returns processor metadata
func getPublishKafkaInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"PublishKafka",
		"PublishKafka",
		"1.0.0",
		"DataBridge",
		"Publishes FlowFile content as messages to Apache Kafka topics",
		[]string{"kafka", "publish", "producer", "messaging"},
	)
}

// Register the processor
func init() {
	plugin.RegisterBuiltInProcessor("PublishKafka", func() types.Processor {
		return NewPublishKafkaProcessor()
	}, getPublishKafkaInfo())
}
