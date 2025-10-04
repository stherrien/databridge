package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

// FlowController is the central orchestrator for data flow management
type FlowController struct {
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	logger            *logrus.Logger

	// Repositories
	flowFileRepo      FlowFileRepository
	contentRepo       ContentRepository
	provenanceRepo    ProvenanceRepository

	// Components
	processors        map[uuid.UUID]*ProcessorNode
	connections       map[uuid.UUID]*Connection
	processGroups     map[uuid.UUID]*ProcessGroup

	// Scheduling
	scheduler         *ProcessScheduler
	running           bool

	// Plugin Management
	pluginManager     *plugin.PluginManager

	// Cluster Management
	clusterManager    interface{}  // *cluster.ClusterManager (interface to avoid circular import)
	isClusteredMode   bool
}

// ProcessorNode wraps a processor with its configuration and runtime state
type ProcessorNode struct {
	ID              uuid.UUID                `json:"id"`
	Name            string                   `json:"name"`
	Type            string                   `json:"type"`
	Processor       types.Processor          `json:"-"`
	Config          types.ProcessorConfig    `json:"config"`
	Status          types.ProcessorStatus    `json:"status"`
	Context         *ProcessorContextImpl    `json:"-"`
	Connections     []*Connection           `json:"-"`
	RateLimiter     *ProcessorRateLimiter   `json:"-"`
	CircuitBreaker  *CircuitBreaker         `json:"-"`
	RetryPolicy     *RetryPolicy            `json:"-"`
	RetryQueue      *RetryQueue             `json:"-"`
	mu              sync.RWMutex
}

// Connection represents a connection between processors
type Connection struct {
	ID                uuid.UUID          `json:"id"`
	Name              string             `json:"name"`
	Source            *ProcessorNode     `json:"source"`
	Destination       *ProcessorNode     `json:"destination"`
	Relationship      types.Relationship `json:"relationship"`
	Queue             *FlowFileQueue     `json:"-"`
	BackPressureSize  int64             `json:"backPressureSize"`
	BackPressureConfig BackPressureConfig `json:"backPressureConfig"`
	RateLimitConfig   *RateLimitConfig   `json:"rateLimitConfig,omitempty"`
	lastPenaltyTime   *time.Time
	penaltyDuration   time.Duration
	mu                sync.RWMutex
}

// ProcessGroup provides hierarchical organization of components
type ProcessGroup struct {
	ID          uuid.UUID                    `json:"id"`
	Name        string                      `json:"name"`
	Parent      *ProcessGroup               `json:"parent,omitempty"`
	Children    map[uuid.UUID]*ProcessGroup `json:"children"`
	Processors  map[uuid.UUID]*ProcessorNode `json:"processors"`
	Connections map[uuid.UUID]*Connection   `json:"connections"`
	mu          sync.RWMutex
}

// FlowFileQueue manages queued FlowFiles between processors
type FlowFileQueue struct {
	connection  *Connection
	flowFiles   []*types.FlowFile
	maxSize     int64
	currentSize int64
	metrics     *BackPressureMetrics
	mu          sync.RWMutex
}

// NewFlowController creates a new FlowController
func NewFlowController(
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
	logger *logrus.Logger,
) *FlowController {
	return NewFlowControllerWithPlugins(flowFileRepo, contentRepo, provenanceRepo, nil, logger)
}

// NewFlowControllerWithPlugins creates a new FlowController with plugin management
func NewFlowControllerWithPlugins(
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
	pluginManager *plugin.PluginManager,
	logger *logrus.Logger,
) *FlowController {
	ctx, cancel := context.WithCancel(context.Background())

	fc := &FlowController{
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		flowFileRepo:   flowFileRepo,
		contentRepo:    contentRepo,
		provenanceRepo: provenanceRepo,
		processors:     make(map[uuid.UUID]*ProcessorNode),
		connections:    make(map[uuid.UUID]*Connection),
		processGroups:  make(map[uuid.UUID]*ProcessGroup),
		pluginManager:  pluginManager,
	}

	// Create root process group
	rootGroup := &ProcessGroup{
		ID:          uuid.New(),
		Name:        "root",
		Children:    make(map[uuid.UUID]*ProcessGroup),
		Processors:  make(map[uuid.UUID]*ProcessorNode),
		Connections: make(map[uuid.UUID]*Connection),
	}
	fc.processGroups[rootGroup.ID] = rootGroup

	// Initialize scheduler
	fc.scheduler = NewProcessScheduler(fc, logger)

	return fc
}

// Start starts the flow controller
func (fc *FlowController) Start() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.running {
		return fmt.Errorf("flow controller is already running")
	}

	fc.logger.Info("Starting FlowController")

	// Start scheduler
	if err := fc.scheduler.Start(fc.ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Initialize all processors
	for _, processorNode := range fc.processors {
		if err := fc.initializeProcessor(processorNode); err != nil {
			fc.logger.WithError(err).Errorf("Failed to initialize processor %s", processorNode.Name)
		}
	}

	fc.running = true
	fc.logger.Info("FlowController started successfully")
	return nil
}

// Stop stops the flow controller
func (fc *FlowController) Stop() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if !fc.running {
		return nil
	}

	fc.logger.Info("Stopping FlowController")

	// Stop scheduler
	if err := fc.scheduler.Stop(); err != nil {
		fc.logger.WithError(err).Error("Error stopping scheduler")
	}

	// Stop all processors
	for _, processorNode := range fc.processors {
		fc.stopProcessor(processorNode)
	}

	// Cancel context
	fc.cancel()

	fc.running = false
	fc.logger.Info("FlowController stopped")
	return nil
}

// AddProcessor adds a processor to the flow
func (fc *FlowController) AddProcessor(processor types.Processor, config types.ProcessorConfig) (*ProcessorNode, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Initialize flow control components
	rateLimiterConfig := DefaultRateLimitConfig()
	circuitBreakerConfig := DefaultCircuitBreakerConfig()
	retryConfig := DefaultRetryConfig()

	retryPolicy, err := NewRetryPolicy(retryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create retry policy: %w", err)
	}

	processorNode := &ProcessorNode{
		ID:             config.ID,
		Name:           config.Name,
		Type:           config.Type,
		Processor:      processor,
		Config:         config,
		Status: types.ProcessorStatus{
			ID:    config.ID,
			Name:  config.Name,
			Type:  config.Type,
			State: types.ProcessorStateStopped,
		},
		Context:        NewProcessorContext(config, fc.logger),
		RateLimiter:    NewProcessorRateLimiter(rateLimiterConfig),
		CircuitBreaker: NewCircuitBreaker(config.Name, circuitBreakerConfig),
		RetryPolicy:    retryPolicy,
		RetryQueue:     NewRetryQueue(retryPolicy, 1000),
	}

	fc.processors[processorNode.ID] = processorNode

	// Add to root process group for now
	for _, pg := range fc.processGroups {
		if pg.Parent == nil { // Root group
			pg.mu.Lock()
			pg.Processors[processorNode.ID] = processorNode
			pg.mu.Unlock()
			break
		}
	}

	fc.logger.WithFields(logrus.Fields{
		"processorId":   processorNode.ID,
		"processorName": processorNode.Name,
		"processorType": processorNode.Type,
	}).Info("Added processor")

	return processorNode, nil
}

// AddConnection creates a connection between processors
func (fc *FlowController) AddConnection(sourceID, destID uuid.UUID, relationship types.Relationship) (*Connection, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	source, exists := fc.processors[sourceID]
	if !exists {
		return nil, fmt.Errorf("source processor not found: %s", sourceID)
	}

	dest, exists := fc.processors[destID]
	if !exists {
		return nil, fmt.Errorf("destination processor not found: %s", destID)
	}

	connection := &Connection{
		ID:                 uuid.New(),
		Name:               fmt.Sprintf("%s -> %s", source.Name, dest.Name),
		Source:             source,
		Destination:        dest,
		Relationship:       relationship,
		BackPressureSize:   10000, // Default back pressure size
		BackPressureConfig: DefaultBackPressureConfig(),
	}

	// Create queue
	connection.Queue = &FlowFileQueue{
		connection: connection,
		flowFiles:  make([]*types.FlowFile, 0),
		maxSize:    connection.BackPressureSize,
		metrics:    &BackPressureMetrics{},
	}

	fc.connections[connection.ID] = connection

	// Add connection to processors
	source.mu.Lock()
	source.Connections = append(source.Connections, connection)
	source.mu.Unlock()

	// Add to root process group
	for _, pg := range fc.processGroups {
		if pg.Parent == nil { // Root group
			pg.mu.Lock()
			pg.Connections[connection.ID] = connection
			pg.mu.Unlock()
			break
		}
	}

	fc.logger.WithFields(logrus.Fields{
		"connectionId": connection.ID,
		"source":       source.Name,
		"destination":  dest.Name,
		"relationship": relationship.Name,
	}).Info("Added connection")

	return connection, nil
}

// StartProcessor starts a specific processor
func (fc *FlowController) StartProcessor(processorID uuid.UUID) error {
	fc.mu.RLock()
	processorNode, exists := fc.processors[processorID]
	fc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("processor not found: %s", processorID)
	}

	// Check if in clustered mode and if this node should run this processor
	if fc.isClusteredMode && fc.clusterManager != nil {
		// Use type assertion to call cluster manager methods
		if cm, ok := fc.clusterManager.(interface {
			GetNodeID() string
			GetProcessorAssignment(uuid.UUID) (string, bool)
		}); ok {
			assignedNode, exists := cm.GetProcessorAssignment(processorID)
			if exists && assignedNode != cm.GetNodeID() {
				return fmt.Errorf("processor assigned to different node: %s", assignedNode)
			}
		}
	}

	processorNode.mu.Lock()
	defer processorNode.mu.Unlock()

	if processorNode.Status.State != types.ProcessorStateStopped {
		return fmt.Errorf("processor %s is not in stopped state", processorNode.Name)
	}

	// Validate configuration
	validationResults := processorNode.Processor.Validate(processorNode.Config)
	var errors []string
	for _, result := range validationResults {
		if !result.Valid {
			errors = append(errors, fmt.Sprintf("%s: %s", result.Property, result.Message))
		}
	}

	if len(errors) > 0 {
		processorNode.Status.State = types.ProcessorStateInvalid
		processorNode.Status.ValidationErrors = errors
		return fmt.Errorf("processor validation failed: %v", errors)
	}

	// Initialize if not already done
	if processorNode.Status.State == types.ProcessorStateStopped {
		if err := fc.initializeProcessor(processorNode); err != nil {
			return fmt.Errorf("failed to initialize processor: %w", err)
		}
	}

	// Schedule processor
	fc.scheduler.ScheduleProcessor(processorNode)

	processorNode.Status.State = types.ProcessorStateRunning
	processorNode.Status.ValidationErrors = nil

	fc.logger.WithField("processorName", processorNode.Name).Info("Started processor")
	return nil
}

// StopProcessor stops a specific processor
func (fc *FlowController) StopProcessor(processorID uuid.UUID) error {
	fc.mu.RLock()
	processorNode, exists := fc.processors[processorID]
	fc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("processor not found: %s", processorID)
	}

	fc.stopProcessor(processorNode)
	return nil
}

// Private methods

func (fc *FlowController) initializeProcessor(processorNode *ProcessorNode) error {
	if err := processorNode.Processor.Initialize(processorNode.Context); err != nil {
		return fmt.Errorf("processor initialization failed: %w", err)
	}
	return nil
}

func (fc *FlowController) stopProcessor(processorNode *ProcessorNode) {
	processorNode.mu.Lock()
	defer processorNode.mu.Unlock()

	if processorNode.Status.State != types.ProcessorStateRunning {
		return
	}

	// Unschedule processor
	fc.scheduler.UnscheduleProcessor(processorNode)

	// Call processor's stop method
	processorNode.Processor.OnStopped(fc.ctx)

	processorNode.Status.State = types.ProcessorStateStopped

	fc.logger.WithField("processorName", processorNode.Name).Info("Stopped processor")
}

// GetProcessor retrieves a processor by ID
func (fc *FlowController) GetProcessor(id uuid.UUID) (*ProcessorNode, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	processor, exists := fc.processors[id]
	return processor, exists
}

// GetProcessors returns all processors
func (fc *FlowController) GetProcessors() map[uuid.UUID]*ProcessorNode {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	result := make(map[uuid.UUID]*ProcessorNode)
	for k, v := range fc.processors {
		result[k] = v
	}
	return result
}

// GetConnection retrieves a connection by ID
func (fc *FlowController) GetConnection(id uuid.UUID) (*Connection, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	connection, exists := fc.connections[id]
	return connection, exists
}

// GetConnections returns all connections
func (fc *FlowController) GetConnections() map[uuid.UUID]*Connection {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	result := make(map[uuid.UUID]*Connection)
	for k, v := range fc.connections {
		result[k] = v
	}
	return result
}

// CreateProcessSession creates a new process session for a processor
func (fc *FlowController) CreateProcessSession(processorNode *ProcessorNode, logger types.Logger) types.ProcessSession {
	// Get input queues from incoming connections
	inputQueues := fc.getInputQueues(processorNode)

	// Get output connections from this processor
	outputConnections := processorNode.Connections

	return NewProcessSession(
		fc.flowFileRepo,
		fc.contentRepo,
		fc.provenanceRepo,
		logger,
		fc.ctx,
		inputQueues,
		outputConnections,
	)
}

// getInputQueues returns all input queues for a processor
func (fc *FlowController) getInputQueues(processorNode *ProcessorNode) []*FlowFileQueue {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var queues []*FlowFileQueue
	for _, conn := range fc.connections {
		if conn.Destination != nil && conn.Destination.ID == processorNode.ID {
			queues = append(queues, conn.Queue)
		}
	}

	return queues
}

// FlowFileQueue methods

// Enqueue adds a FlowFile to the queue
func (q *FlowFileQueue) Enqueue(flowFile *types.FlowFile) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.currentSize >= q.maxSize {
		return fmt.Errorf("queue is full, back pressure triggered")
	}

	q.flowFiles = append(q.flowFiles, flowFile)
	q.currentSize++
	return nil
}

// Dequeue removes and returns a FlowFile from the queue
func (q *FlowFileQueue) Dequeue() *types.FlowFile {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.flowFiles) == 0 {
		return nil
	}

	flowFile := q.flowFiles[0]
	q.flowFiles = q.flowFiles[1:]
	q.currentSize--
	return flowFile
}

// Size returns the current queue size
func (q *FlowFileQueue) Size() int64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.currentSize
}

// IsEmpty returns whether the queue is empty
func (q *FlowFileQueue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.flowFiles) == 0
}

// SetMaxSize sets the maximum queue size
func (q *FlowFileQueue) SetMaxSize(size int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.maxSize = size
}

// ProcessorContextImpl implements ProcessorContext
type ProcessorContextImpl struct {
	config types.ProcessorConfig
	logger types.Logger
}

// NewProcessorContext creates a new ProcessorContext
func NewProcessorContext(config types.ProcessorConfig, logger *logrus.Logger) *ProcessorContextImpl {
	return &ProcessorContextImpl{
		config: config,
		logger: &LogrusAdapter{Logger: logger},
	}
}

// GetProperty retrieves a property value
func (ctx *ProcessorContextImpl) GetProperty(name string) (string, bool) {
	value, exists := ctx.config.Properties[name]
	return value, exists
}

// GetPropertyValue retrieves a property value or empty string
func (ctx *ProcessorContextImpl) GetPropertyValue(name string) string {
	value, _ := ctx.config.Properties[name]
	return value
}

// HasProperty checks if a property exists
func (ctx *ProcessorContextImpl) HasProperty(name string) bool {
	_, exists := ctx.config.Properties[name]
	return exists
}

// GetProcessorConfig returns the processor configuration
func (ctx *ProcessorContextImpl) GetProcessorConfig() types.ProcessorConfig {
	return ctx.config
}

// GetLogger returns the logger
func (ctx *ProcessorContextImpl) GetLogger() types.Logger {
	return ctx.logger
}

// LogrusAdapter adapts logrus.Logger to types.Logger interface
type LogrusAdapter struct {
	*logrus.Logger
}

// Debug logs a debug message
func (l *LogrusAdapter) Debug(msg string, fields ...interface{}) {
	entry := l.Logger.WithFields(logrus.Fields{})
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok && i+1 < len(fields) {
			entry = entry.WithField(key, fields[i+1])
		}
	}
	entry.Debug(msg)
}

// Info logs an info message
func (l *LogrusAdapter) Info(msg string, fields ...interface{}) {
	entry := l.Logger.WithFields(logrus.Fields{})
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok && i+1 < len(fields) {
			entry = entry.WithField(key, fields[i+1])
		}
	}
	entry.Info(msg)
}

// Warn logs a warning message
func (l *LogrusAdapter) Warn(msg string, fields ...interface{}) {
	entry := l.Logger.WithFields(logrus.Fields{})
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok && i+1 < len(fields) {
			entry = entry.WithField(key, fields[i+1])
		}
	}
	entry.Warn(msg)
}

// Error logs an error message
func (l *LogrusAdapter) Error(msg string, fields ...interface{}) {
	entry := l.Logger.WithFields(logrus.Fields{})
	for i := 0; i < len(fields)-1; i += 2 {
		if key, ok := fields[i].(string); ok && i+1 < len(fields) {
			entry = entry.WithField(key, fields[i+1])
		}
	}
	entry.Error(msg)
}

// ProcessGroup methods for locking
func (pg *ProcessGroup) RLock() {
	pg.mu.RLock()
}

func (pg *ProcessGroup) RUnlock() {
	pg.mu.RUnlock()
}

// ProcessorNode methods for locking
func (pn *ProcessorNode) RLock() {
	pn.mu.RLock()
}

func (pn *ProcessorNode) RUnlock() {
	pn.mu.RUnlock()
}

// Connection methods for locking
func (c *Connection) RLock() {
	c.mu.RLock()
}

func (c *Connection) RUnlock() {
	c.mu.RUnlock()
}

// GetProcessGroups returns all process groups
func (fc *FlowController) GetProcessGroups() map[uuid.UUID]*ProcessGroup {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	result := make(map[uuid.UUID]*ProcessGroup)
	for k, v := range fc.processGroups {
		result[k] = v
	}
	return result
}

// GetProcessGroup retrieves a process group by ID
func (fc *FlowController) GetProcessGroup(id uuid.UUID) (*ProcessGroup, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	pg, exists := fc.processGroups[id]
	return pg, exists
}

// GetScheduler returns the process scheduler
func (fc *FlowController) GetScheduler() *ProcessScheduler {
	return fc.scheduler
}

// CreateProcessGroup creates a new process group
func (fc *FlowController) CreateProcessGroup(name string, parentID *uuid.UUID) (*ProcessGroup, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	pg := &ProcessGroup{
		ID:          uuid.New(),
		Name:        name,
		Children:    make(map[uuid.UUID]*ProcessGroup),
		Processors:  make(map[uuid.UUID]*ProcessorNode),
		Connections: make(map[uuid.UUID]*Connection),
	}

	// Set parent if specified
	if parentID != nil {
		parent, exists := fc.processGroups[*parentID]
		if !exists {
			return nil, fmt.Errorf("parent process group not found")
		}
		pg.Parent = parent
		parent.mu.Lock()
		parent.Children[pg.ID] = pg
		parent.mu.Unlock()
	}

	fc.processGroups[pg.ID] = pg

	fc.logger.WithFields(logrus.Fields{
		"processGroupId":   pg.ID,
		"processGroupName": pg.Name,
	}).Info("Created process group")

	return pg, nil
}

// UpdateProcessGroup updates a process group
func (fc *FlowController) UpdateProcessGroup(id uuid.UUID, name string) (*ProcessGroup, error) {
	fc.mu.RLock()
	pg, exists := fc.processGroups[id]
	fc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("process group not found")
	}

	pg.mu.Lock()
	pg.Name = name
	pg.mu.Unlock()

	fc.logger.WithFields(logrus.Fields{
		"processGroupId":   pg.ID,
		"processGroupName": pg.Name,
	}).Info("Updated process group")

	return pg, nil
}

// DeleteProcessGroup deletes a process group
func (fc *FlowController) DeleteProcessGroup(id uuid.UUID) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	pg, exists := fc.processGroups[id]
	if !exists {
		return fmt.Errorf("process group not found")
	}

	// Check if it's the root group
	if pg.Parent == nil {
		return fmt.Errorf("cannot delete root process group")
	}

	pg.mu.Lock()
	defer pg.mu.Unlock()

	// Check if the group has any processors or connections
	if len(pg.Processors) > 0 {
		return fmt.Errorf("cannot delete process group with processors")
	}
	if len(pg.Connections) > 0 {
		return fmt.Errorf("cannot delete process group with connections")
	}
	if len(pg.Children) > 0 {
		return fmt.Errorf("cannot delete process group with children")
	}

	// Remove from parent
	if pg.Parent != nil {
		pg.Parent.mu.Lock()
		delete(pg.Parent.Children, pg.ID)
		pg.Parent.mu.Unlock()
	}

	delete(fc.processGroups, id)

	fc.logger.WithFields(logrus.Fields{
		"processGroupId": id,
	}).Info("Deleted process group")

	return nil
}

// RemoveProcessor removes a processor from the flow controller
func (fc *FlowController) RemoveProcessor(id uuid.UUID) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	processor, exists := fc.processors[id]
	if !exists {
		return fmt.Errorf("processor not found")
	}

	// Stop processor if running
	if processor.Status.State == types.ProcessorStateRunning {
		fc.stopProcessor(processor)
	}

	// Remove from process groups
	for _, pg := range fc.processGroups {
		pg.mu.Lock()
		delete(pg.Processors, id)
		pg.mu.Unlock()
	}

	// Remove any connections involving this processor
	connectionsToRemove := []uuid.UUID{}
	for connID, conn := range fc.connections {
		if conn.Source.ID == id || conn.Destination.ID == id {
			connectionsToRemove = append(connectionsToRemove, connID)
		}
	}

	for _, connID := range connectionsToRemove {
		fc.removeConnection(connID)
	}

	delete(fc.processors, id)

	fc.logger.WithFields(logrus.Fields{
		"processorId": id,
	}).Info("Removed processor")

	return nil
}

// RemoveConnection removes a connection
func (fc *FlowController) RemoveConnection(id uuid.UUID) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	return fc.removeConnection(id)
}

// removeConnection is an internal method that removes a connection (caller must hold lock)
func (fc *FlowController) removeConnection(id uuid.UUID) error {
	_, exists := fc.connections[id]
	if !exists {
		return fmt.Errorf("connection not found")
	}

	// Remove from process groups
	for _, pg := range fc.processGroups {
		pg.mu.Lock()
		delete(pg.Connections, id)
		pg.mu.Unlock()
	}

	delete(fc.connections, id)

	fc.logger.WithFields(logrus.Fields{
		"connectionId": id,
	}).Info("Removed connection")

	return nil
}

// UpdateProcessor updates a processor's configuration
func (fc *FlowController) UpdateProcessor(id uuid.UUID, config types.ProcessorConfig) (*ProcessorNode, error) {
	fc.mu.RLock()
	processor, exists := fc.processors[id]
	fc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("processor not found")
	}

	processor.mu.Lock()
	defer processor.mu.Unlock()

	// Update configuration
	if config.Name != "" {
		processor.Name = config.Name
		processor.Config.Name = config.Name
	}
	if config.Properties != nil {
		processor.Config.Properties = config.Properties
	}
	if config.ScheduleType != "" {
		processor.Config.ScheduleType = config.ScheduleType
	}
	if config.ScheduleValue != "" {
		processor.Config.ScheduleValue = config.ScheduleValue
	}
	if config.Concurrency > 0 {
		processor.Config.Concurrency = config.Concurrency
	}
	if config.AutoTerminate != nil {
		processor.Config.AutoTerminate = config.AutoTerminate
	}

	// Re-validate if processor is running
	if processor.Status.State == types.ProcessorStateRunning {
		validationResults := processor.Processor.Validate(processor.Config)
		var errors []string
		for _, result := range validationResults {
			if !result.Valid {
				errors = append(errors, fmt.Sprintf("%s: %s", result.Property, result.Message))
			}
		}
		if len(errors) > 0 {
			processor.Status.State = types.ProcessorStateInvalid
			processor.Status.ValidationErrors = errors
		}
	}

	fc.logger.WithFields(logrus.Fields{
		"processorId":   id,
		"processorName": processor.Name,
	}).Info("Updated processor")

	return processor, nil
}

// UpdateConnection updates a connection
func (fc *FlowController) UpdateConnection(id uuid.UUID, name string, backPressureSize int64) (*Connection, error) {
	fc.mu.RLock()
	connection, exists := fc.connections[id]
	fc.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("connection not found")
	}

	connection.mu.Lock()
	defer connection.mu.Unlock()

	if name != "" {
		connection.Name = name
	}
	if backPressureSize > 0 {
		connection.BackPressureSize = backPressureSize
		connection.Queue.maxSize = backPressureSize
	}

	fc.logger.WithFields(logrus.Fields{
		"connectionId": id,
	}).Info("Updated connection")

	return connection, nil
}

// CreateProcessorByType creates a processor by type name using the plugin registry
func (fc *FlowController) CreateProcessorByType(processorType string, config types.ProcessorConfig) (*ProcessorNode, error) {
	if fc.pluginManager == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}

	// Get processor from plugin manager
	processor, err := fc.pluginManager.GetProcessor(processorType)
	if err != nil {
		return nil, fmt.Errorf("failed to get processor type %s: %w", processorType, err)
	}

	// Use the existing AddProcessor method
	return fc.AddProcessor(processor, config)
}

// AddProcessorToGroup adds an existing processor to a specific ProcessGroup
func (fc *FlowController) AddProcessorToGroup(groupID uuid.UUID, processor *ProcessorNode) error {
	processGroup, exists := fc.GetProcessGroup(groupID)
	if !exists {
		return fmt.Errorf("process group not found: %s", groupID)
	}

	processGroup.mu.Lock()
	processGroup.Processors[processor.ID] = processor
	processGroup.mu.Unlock()

	return nil
}

// GetPluginManager returns the plugin manager
func (fc *FlowController) GetPluginManager() *plugin.PluginManager {
	return fc.pluginManager
}

// SetPluginManager sets the plugin manager
func (fc *FlowController) SetPluginManager(manager *plugin.PluginManager) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.pluginManager = manager
}

// SetClusterManager sets the cluster manager
func (fc *FlowController) SetClusterManager(clusterManager interface{}) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.clusterManager = clusterManager
	fc.isClusteredMode = clusterManager != nil

	if fc.isClusteredMode {
		fc.logger.Info("Cluster mode enabled")
	}
}

// GetClusterManager returns the cluster manager
func (fc *FlowController) GetClusterManager() interface{} {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.clusterManager
}

// IsClusteredMode returns whether clustering is enabled
func (fc *FlowController) IsClusteredMode() bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.isClusteredMode
}

// AssignProcessorToNode assigns a processor to a specific node in the cluster
func (fc *FlowController) AssignProcessorToNode(processorID uuid.UUID, nodeID string) error {
	if !fc.isClusteredMode || fc.clusterManager == nil {
		return fmt.Errorf("clustering is not enabled")
	}

	// Use type assertion to call cluster manager methods
	if cm, ok := fc.clusterManager.(interface {
		GetNodeID() string
		IsLeader() bool
	}); ok {
		if !cm.IsLeader() {
			return fmt.Errorf("only cluster leader can assign processors")
		}
	}

	fc.logger.WithFields(logrus.Fields{
		"processorId": processorID,
		"nodeId":      nodeID,
	}).Info("Assigned processor to node")

	return nil
}