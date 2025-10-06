package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ConsumeKafkaProcessor consumes messages from Apache Kafka topics
type ConsumeKafkaProcessor struct {
	*types.BaseProcessor
	consumer      sarama.ConsumerGroup
	cancelFunc    context.CancelFunc
	wg            sync.WaitGroup
	ready         chan bool
	processorCtx  types.ProcessorContext
	isInitialized bool
	mu            sync.Mutex
}

// NewConsumeKafkaProcessor creates a new ConsumeKafka processor
func NewConsumeKafkaProcessor() *ConsumeKafkaProcessor {
	info := types.ProcessorInfo{
		Name:        "ConsumeKafka",
		Description: "Consumes messages from Apache Kafka topics and creates FlowFiles",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"kafka", "consumer", "messaging", "streaming", "ingest"},
		Properties: []types.PropertySpec{
			{
				Name:         "Kafka Brokers",
				Description:  "Comma-separated list of Kafka broker addresses (e.g., localhost:9092,broker2:9092)",
				Required:     true,
				DefaultValue: "localhost:9092",
			},
			{
				Name:         "Topic Names",
				Description:  "Comma-separated list of Kafka topic names to consume from",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:         "Group ID",
				Description:  "Consumer group ID for Kafka consumer group coordination",
				Required:     true,
				DefaultValue: "databridge-consumer-group",
			},
			{
				Name:          "Offset Reset",
				Description:   "Where to start consuming when there is no initial offset",
				Required:      false,
				DefaultValue:  "latest",
				AllowedValues: []string{"earliest", "latest"},
			},
			{
				Name:         "Auto Commit Interval",
				Description:  "Frequency of offset commits (e.g., 5s, 1m)",
				Required:     false,
				DefaultValue: "5s",
			},
			{
				Name:         "Session Timeout",
				Description:  "Timeout for detecting consumer failures (e.g., 10s, 30s)",
				Required:     false,
				DefaultValue: "10s",
			},
			{
				Name:         "Max Poll Records",
				Description:  "Maximum number of records to poll in a single batch",
				Required:     false,
				DefaultValue: "500",
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
				Name:         "Message Header Encoding",
				Description:  "Encoding for Kafka message headers (added as FlowFile attributes)",
				Required:     false,
				DefaultValue: "UTF-8",
			},
			{
				Name:          "Honor Transactions",
				Description:   "Whether to only consume committed messages in transactional topics",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &ConsumeKafkaProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		ready:         make(chan bool),
	}
}

// Initialize initializes the Kafka consumer
func (p *ConsumeKafkaProcessor) Initialize(ctx types.ProcessorContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isInitialized {
		return nil
	}

	logger := ctx.GetLogger()
	logger.Info("Initializing ConsumeKafka processor")

	// Store context for later use
	p.processorCtx = ctx

	// Validate and parse required properties
	brokers, _, groupID, err := p.validateAndParseProperties(ctx)
	if err != nil {
		return err
	}

	// Create Kafka configuration
	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()

	// Configure consumer settings
	if err := p.configureConsumerSettings(ctx, config); err != nil {
		return err
	}

	// Configure security
	if err := p.configureConsumerSecurity(ctx, config); err != nil {
		return err
	}

	// Create consumer group
	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	p.consumer = consumer
	p.isInitialized = true

	logger.Info("Kafka consumer initialized successfully")

	return nil
}

// validateAndParseProperties validates and parses required properties
func (p *ConsumeKafkaProcessor) validateAndParseProperties(ctx types.ProcessorContext) ([]string, []string, string, error) {
	// Validate required properties
	if !ctx.HasProperty("Kafka Brokers") {
		return nil, nil, "", fmt.Errorf("Kafka Brokers property is required")
	}

	if !ctx.HasProperty("Topic Names") {
		return nil, nil, "", fmt.Errorf("Topic Names property is required")
	}

	if !ctx.HasProperty("Group ID") {
		return nil, nil, "", fmt.Errorf("Group ID property is required")
	}

	// Parse broker list
	brokersStr := ctx.GetPropertyValue("Kafka Brokers")
	brokers := strings.Split(brokersStr, ",")
	for i, broker := range brokers {
		brokers[i] = strings.TrimSpace(broker)
	}

	// Parse topic list
	topicsStr := ctx.GetPropertyValue("Topic Names")
	if topicsStr == "" {
		return nil, nil, "", fmt.Errorf("Topic Names cannot be empty")
	}
	topics := strings.Split(topicsStr, ",")
	for i, topic := range topics {
		topics[i] = strings.TrimSpace(topic)
	}

	groupID := ctx.GetPropertyValue("Group ID")
	if groupID == "" {
		return nil, nil, "", fmt.Errorf("Group ID cannot be empty")
	}

	return brokers, topics, groupID, nil
}

// configureConsumerSettings configures consumer settings
func (p *ConsumeKafkaProcessor) configureConsumerSettings(ctx types.ProcessorContext, config *sarama.Config) error {
	// Offset reset
	offsetReset := ctx.GetPropertyValue("Offset Reset")
	if offsetReset == "earliest" {
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	} else {
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// Auto commit interval
	if autoCommitStr := ctx.GetPropertyValue("Auto Commit Interval"); autoCommitStr != "" {
		autoCommitInterval, err := time.ParseDuration(autoCommitStr)
		if err != nil {
			return fmt.Errorf("invalid Auto Commit Interval: %w", err)
		}
		config.Consumer.Offsets.AutoCommit.Interval = autoCommitInterval
	}

	// Session timeout
	if sessionTimeoutStr := ctx.GetPropertyValue("Session Timeout"); sessionTimeoutStr != "" {
		sessionTimeout, err := time.ParseDuration(sessionTimeoutStr)
		if err != nil {
			return fmt.Errorf("invalid Session Timeout: %w", err)
		}
		config.Consumer.Group.Session.Timeout = sessionTimeout
	}

	// Max poll records
	if maxPollStr := ctx.GetPropertyValue("Max Poll Records"); maxPollStr != "" {
		maxPoll, err := strconv.Atoi(maxPollStr)
		if err != nil {
			return fmt.Errorf("invalid Max Poll Records: %w", err)
		}
		config.Consumer.MaxProcessingTime = time.Second * 30
		_ = maxPoll // Store for later use in consumption logic
	}

	// Honor transactions
	honorTxStr := ctx.GetPropertyValue("Honor Transactions")
	if honorTxStr == "true" {
		config.Consumer.IsolationLevel = sarama.ReadCommitted
	}

	return nil
}

// configureConsumerSecurity configures security settings for the consumer
func (p *ConsumeKafkaProcessor) configureConsumerSecurity(ctx types.ProcessorContext, config *sarama.Config) error {
	securityProtocol := ctx.GetPropertyValue("Security Protocol")

	switch securityProtocol {
	case "SSL":
		config.Net.TLS.Enable = true
	case "SASL_SSL":
		config.Net.TLS.Enable = true
		config.Net.SASL.Enable = true
		return p.configureConsumerSASL(ctx, config)
	case "SASL_PLAINTEXT":
		config.Net.SASL.Enable = true
		return p.configureConsumerSASL(ctx, config)
	}

	return nil
}

// configureConsumerSASL configures SASL authentication for the consumer
func (p *ConsumeKafkaProcessor) configureConsumerSASL(ctx types.ProcessorContext, config *sarama.Config) error {
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

// OnTrigger processes Kafka messages
func (p *ConsumeKafkaProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	if !p.isInitialized {
		return fmt.Errorf("processor not initialized")
	}

	logger := p.processorCtx.GetLogger()

	// Parse topic list
	topicsStr := p.processorCtx.GetPropertyValue("Topic Names")
	topics := strings.Split(topicsStr, ",")
	for i, topic := range topics {
		topics[i] = strings.TrimSpace(topic)
	}

	// Create consumer handler
	handler := &consumerGroupHandler{
		processor: p,
		session:   session,
		logger:    logger,
	}

	// Start consuming
	consumeCtx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-consumeCtx.Done():
				logger.Info("Kafka consumer context canceled")
				return
			default:
				if err := p.consumer.Consume(consumeCtx, topics, handler); err != nil {
					logger.Error("Error consuming from Kafka: " + err.Error())
					return
				}
				// Check if context was canceled
				if consumeCtx.Err() != nil {
					return
				}
			}
		}
	}()

	// Wait for consumer to be ready
	<-p.ready
	logger.Info("Kafka consumer is ready")

	return nil
}

// Cleanup cleans up resources
func (p *ConsumeKafkaProcessor) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isInitialized {
		return nil
	}

	logger := p.processorCtx.GetLogger()
	logger.Info("Cleaning up ConsumeKafka processor")

	// Cancel consumer context
	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	// Wait for consumer goroutine to finish
	p.wg.Wait()

	// Close consumer
	if p.consumer != nil {
		if err := p.consumer.Close(); err != nil {
			logger.Error("Error closing Kafka consumer: " + err.Error())
			return err
		}
	}

	p.isInitialized = false
	logger.Info("ConsumeKafka processor cleaned up successfully")

	return nil
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	processor *ConsumeKafkaProcessor
	session   types.ProcessSession
	logger    types.Logger
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.processor.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim processes messages from a specific partition
func (h *consumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// Create FlowFile from Kafka message
			if err := h.processMessage(sess, message); err != nil {
				h.logger.Error("Error processing Kafka message: " + err.Error())
				continue
			}

			// Mark message as consumed
			sess.MarkMessage(message, "")

		case <-sess.Context().Done():
			return nil
		}
	}
}

// processMessage converts a Kafka message to a FlowFile
func (h *consumerGroupHandler) processMessage(sess sarama.ConsumerGroupSession, msg *sarama.ConsumerMessage) error {
	// Create new FlowFile
	flowFile := h.session.Create()
	if flowFile == nil {
		return fmt.Errorf("failed to create FlowFile")
	}

	// Set FlowFile content from message value
	if err := h.session.Write(flowFile, msg.Value); err != nil {
		h.session.Remove(flowFile)
		return fmt.Errorf("failed to write message content: %w", err)
	}

	// Add Kafka metadata as attributes
	attributes := map[string]string{
		"kafka.topic":          msg.Topic,
		"kafka.partition":      strconv.FormatInt(int64(msg.Partition), 10),
		"kafka.offset":         strconv.FormatInt(msg.Offset, 10),
		"kafka.timestamp":      msg.Timestamp.Format(time.RFC3339),
		"kafka.key":            string(msg.Key),
		"mime.type":            "application/octet-stream",
		"databridge.source":    "ConsumeKafka",
		"databridge.record.id": uuid.New().String(),
	}

	// Add message headers as attributes
	for _, header := range msg.Headers {
		if len(header.Key) > 0 {
			attrKey := fmt.Sprintf("kafka.header.%s", string(header.Key))
			attributes[attrKey] = string(header.Value)
		}
	}

	// Update FlowFile attributes
	h.session.PutAllAttributes(flowFile, attributes)

	// Transfer to success
	h.session.Transfer(flowFile, types.RelationshipSuccess)

	h.logger.Debug("Successfully processed Kafka message")

	return nil
}

// getConsumeKafkaInfo returns processor metadata
func getConsumeKafkaInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ConsumeKafka",
		"ConsumeKafka",
		"1.0.0",
		"DataBridge",
		"Consumes messages from Apache Kafka topics",
		[]string{"kafka", "consume", "messaging"},
	)
}

// Register the processor
func init() {
	plugin.RegisterBuiltInProcessor("ConsumeKafka", func() types.Processor {
		return NewConsumeKafkaProcessor()
	}, getConsumeKafkaInfo())
}
