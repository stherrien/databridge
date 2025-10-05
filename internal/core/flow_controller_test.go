package core

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Mock processor for testing
type mockTestProcessor struct {
	*types.BaseProcessor
	initCalled    bool
	triggerCalled bool
	stopCalled    bool
	shouldFail    bool
}

func newMockTestProcessor(shouldFail bool) *mockTestProcessor {
	info := types.ProcessorInfo{
		Name:        "MockTestProcessor",
		Description: "A mock processor for flow controller testing",
		Version:     "1.0.0",
		Author:      "Test",
		Tags:        []string{"test", "mock"},
		Properties: []types.PropertySpec{
			{
				Name:         "test-prop",
				Description:  "A test property",
				Required:     false,
				DefaultValue: "default",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &mockTestProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		shouldFail:    shouldFail,
	}
}

func (p *mockTestProcessor) Initialize(ctx types.ProcessorContext) error {
	p.initCalled = true
	if p.shouldFail {
		return TestError{message: "mock initialization failed"}
	}
	return nil
}

func (p *mockTestProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	p.triggerCalled = true
	if p.shouldFail {
		return TestError{message: "mock trigger failed"}
	}

	// Simple processing: get a FlowFile and transfer it to success
	flowFile := session.Get()
	if flowFile != nil {
		session.PutAttribute(flowFile, "processed", "true")
		session.Transfer(flowFile, types.RelationshipSuccess)
	}

	return nil
}

func (p *mockTestProcessor) OnStopped(ctx context.Context) {
	p.stopCalled = true
}

type TestError struct {
	message string
}

func (e TestError) Error() string {
	return e.message
}

// Mock repositories for testing
type mockFlowFileRepository struct {
	flowFiles map[uuid.UUID]*types.FlowFile
}

func newMockFlowFileRepository() *mockFlowFileRepository {
	return &mockFlowFileRepository{
		flowFiles: make(map[uuid.UUID]*types.FlowFile),
	}
}

func (r *mockFlowFileRepository) Store(flowFile *types.FlowFile) error {
	r.flowFiles[flowFile.ID] = flowFile
	return nil
}

func (r *mockFlowFileRepository) Get(id uuid.UUID) (*types.FlowFile, error) {
	if flowFile, exists := r.flowFiles[id]; exists {
		return flowFile, nil
	}
	return nil, TestError{message: "FlowFile not found"}
}

func (r *mockFlowFileRepository) Delete(id uuid.UUID) error {
	delete(r.flowFiles, id)
	return nil
}

func (r *mockFlowFileRepository) List(limit, offset int) ([]*types.FlowFile, error) {
	var result []*types.FlowFile
	for _, ff := range r.flowFiles {
		result = append(result, ff)
	}
	return result, nil
}

func (r *mockFlowFileRepository) UpdateAttributes(id uuid.UUID, attributes map[string]string) error {
	if flowFile, exists := r.flowFiles[id]; exists {
		for k, v := range attributes {
			flowFile.Attributes[k] = v
		}
		return nil
	}
	return TestError{message: "FlowFile not found"}
}

func (r *mockFlowFileRepository) Close() error {
	return nil
}

func (r *mockFlowFileRepository) Count() (int, error) {
	return len(r.flowFiles), nil
}

type mockContentRepository struct {
	content map[uuid.UUID][]byte
}

func newMockContentRepository() *mockContentRepository {
	return &mockContentRepository{
		content: make(map[uuid.UUID][]byte),
	}
}

func (r *mockContentRepository) Store(content []byte) (*types.ContentClaim, error) {
	claim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "mock",
		Section:   "test",
		Offset:    0,
		Length:    int64(len(content)),
		RefCount:  1,
	}
	r.content[claim.ID] = content
	return claim, nil
}

func (r *mockContentRepository) Get(claim *types.ContentClaim) ([]byte, error) {
	if content, exists := r.content[claim.ID]; exists {
		return content, nil
	}
	return nil, TestError{message: "Content not found"}
}

func (r *mockContentRepository) GetReader(claim *types.ContentClaim) (io.ReadCloser, error) {
	return nil, TestError{message: "GetReader not implemented in mock"}
}

func (r *mockContentRepository) GetWriter() (ContentWriter, error) {
	return nil, TestError{message: "GetWriter not implemented in mock"}
}

func (r *mockContentRepository) Delete(claim *types.ContentClaim) error {
	delete(r.content, claim.ID)
	return nil
}

func (r *mockContentRepository) IncrementRef(claim *types.ContentClaim) error {
	claim.RefCount++
	return nil
}

func (r *mockContentRepository) DecrementRef(claim *types.ContentClaim) error {
	claim.RefCount--
	return nil
}

func (r *mockContentRepository) Close() error {
	return nil
}

func (r *mockContentRepository) ListClaims() ([]*types.ContentClaim, error) {
	claims := make([]*types.ContentClaim, 0, len(r.content))
	for id, content := range r.content {
		claims = append(claims, &types.ContentClaim{
			ID:        id,
			Container: "mock",
			Section:   "test",
			Offset:    0,
			Length:    int64(len(content)),
			RefCount:  1,
		})
	}
	return claims, nil
}

func (r *mockContentRepository) Read(claim *types.ContentClaim) (io.ReadCloser, error) {
	if content, exists := r.content[claim.ID]; exists {
		return io.NopCloser(bytes.NewReader(content)), nil
	}
	return nil, TestError{message: "Content not found"}
}

func (r *mockContentRepository) Write(claim *types.ContentClaim, data []byte) error {
	r.content[claim.ID] = data
	return nil
}

func TestNewFlowController(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	flowFileRepo := newMockFlowFileRepository()
	contentRepo := newMockContentRepository()
	provenanceRepo := NewInMemoryProvenanceRepository()

	fc := NewFlowController(flowFileRepo, contentRepo, provenanceRepo, logger)

	if fc == nil {
		t.Fatal("NewFlowController should return a valid controller")
	}

	if len(fc.processors) != 0 {
		t.Error("New FlowController should have no processors")
	}

	if len(fc.connections) != 0 {
		t.Error("New FlowController should have no connections")
	}

	if len(fc.processGroups) != 1 {
		t.Error("New FlowController should have root process group")
	}

	if fc.running {
		t.Error("New FlowController should not be running")
	}
}

func TestFlowControllerAddProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	processor := newMockTestProcessor(false)
	config := types.ProcessorConfig{
		ID:           uuid.New(),
		Name:         "Test Processor",
		Type:         "MockTestProcessor",
		ScheduleType: types.ScheduleTypeTimer,
		ScheduleValue: "5s",
		Concurrency:  1,
		Properties: map[string]string{
			"test-prop": "test-value",
		},
	}

	node, err := fc.AddProcessor(processor, config)
	if err != nil {
		t.Errorf("AddProcessor should not return error: %v", err)
	}

	if node == nil {
		t.Fatal("AddProcessor should return a processor node")
	}

	if node.ID != config.ID {
		t.Error("Processor node should have correct ID")
	}

	if node.Name != config.Name {
		t.Error("Processor node should have correct name")
	}

	if node.Type != config.Type {
		t.Error("Processor node should have correct type")
	}

	if node.Status.State != types.ProcessorStateStopped {
		t.Error("New processor should be in stopped state")
	}

	// Verify processor was added to controller
	if len(fc.processors) != 1 {
		t.Errorf("FlowController should have 1 processor, got %d", len(fc.processors))
	}

	if _, exists := fc.processors[config.ID]; !exists {
		t.Error("Processor should be stored with correct ID")
	}
}

func TestFlowControllerAddConnection(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	// Add two processors
	processor1 := newMockTestProcessor(false)
	config1 := types.ProcessorConfig{
		ID:   uuid.New(),
		Name: "Source Processor",
		Type: "MockTestProcessor",
	}

	processor2 := newMockTestProcessor(false)
	config2 := types.ProcessorConfig{
		ID:   uuid.New(),
		Name: "Destination Processor",
		Type: "MockTestProcessor",
	}

	node1, _ := fc.AddProcessor(processor1, config1)
	node2, _ := fc.AddProcessor(processor2, config2)

	// Add connection
	connection, err := fc.AddConnection(node1.ID, node2.ID, types.RelationshipSuccess)
	if err != nil {
		t.Errorf("AddConnection should not return error: %v", err)
	}

	if connection == nil {
		t.Fatal("AddConnection should return a connection")
	}

	if connection.Source.ID != node1.ID {
		t.Error("Connection should have correct source")
	}

	if connection.Destination.ID != node2.ID {
		t.Error("Connection should have correct destination")
	}

	if connection.Relationship != types.RelationshipSuccess {
		t.Error("Connection should have correct relationship")
	}

	if connection.Queue == nil {
		t.Error("Connection should have a queue")
	}

	// Verify connection was added
	if len(fc.connections) != 1 {
		t.Errorf("FlowController should have 1 connection, got %d", len(fc.connections))
	}

	// Test adding connection with non-existent processor
	_, err = fc.AddConnection(uuid.New(), node2.ID, types.RelationshipSuccess)
	if err == nil {
		t.Error("AddConnection should return error for non-existent source processor")
	}

	_, err = fc.AddConnection(node1.ID, uuid.New(), types.RelationshipSuccess)
	if err == nil {
		t.Error("AddConnection should return error for non-existent destination processor")
	}
}

func TestFlowControllerLifecycle(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	// Start FlowController
	err := fc.Start()
	if err != nil {
		t.Errorf("Start should not return error: %v", err)
	}

	if !fc.running {
		t.Error("FlowController should be running after Start")
	}

	// Starting again should return error
	err = fc.Start()
	if err == nil {
		t.Error("Starting already running FlowController should return error")
	}

	// Stop FlowController
	err = fc.Stop()
	if err != nil {
		t.Errorf("Stop should not return error: %v", err)
	}

	if fc.running {
		t.Error("FlowController should not be running after Stop")
	}

	// Stopping again should not error
	err = fc.Stop()
	if err != nil {
		t.Error("Stopping already stopped FlowController should not error")
	}
}

func TestFlowControllerProcessorLifecycle(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	// Start FlowController first
	fc.Start()
	defer fc.Stop()

	processor := newMockTestProcessor(false)
	config := types.ProcessorConfig{
		ID:           uuid.New(),
		Name:         "Test Processor",
		Type:         "MockTestProcessor",
		ScheduleType: types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:  1,
		Properties: map[string]string{
			"test-prop": "test-value",
		},
	}

	node, _ := fc.AddProcessor(processor, config)

	// Start processor
	err := fc.StartProcessor(node.ID)
	if err != nil {
		t.Errorf("StartProcessor should not return error: %v", err)
	}

	if node.Status.State != types.ProcessorStateRunning {
		t.Error("Processor should be running after StartProcessor")
	}

	if !processor.initCalled {
		t.Error("Processor Initialize should be called")
	}

	// Starting already running processor should return error
	err = fc.StartProcessor(node.ID)
	if err == nil {
		t.Error("Starting already running processor should return error")
	}

	// Stop processor
	err = fc.StopProcessor(node.ID)
	if err != nil {
		t.Errorf("StopProcessor should not return error: %v", err)
	}

	// Give a moment for the processor to actually stop
	time.Sleep(10 * time.Millisecond)

	if node.Status.State != types.ProcessorStateStopped {
		t.Error("Processor should be stopped after StopProcessor")
	}

	if !processor.stopCalled {
		t.Error("Processor OnStopped should be called")
	}
}

func TestFlowControllerProcessorValidation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	fc.Start()
	defer fc.Stop()

	// Create processor that fails validation
	processor := newMockTestProcessor(true)
	config := types.ProcessorConfig{
		ID:           uuid.New(),
		Name:         "Failing Processor",
		Type:         "MockTestProcessor",
		ScheduleType: types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:  1,
		Properties:   make(map[string]string), // Empty properties might cause validation to fail
	}

	node, _ := fc.AddProcessor(processor, config)

	// Try to start processor - should fail initialization
	err := fc.StartProcessor(node.ID)
	if err == nil {
		t.Error("StartProcessor should return error for failing processor")
	}

	if node.Status.State == types.ProcessorStateRunning {
		t.Error("Processor should not be running after failed start")
	}
}

func TestFlowControllerGetters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	// Initially should be empty
	processors := fc.GetProcessors()
	if len(processors) != 0 {
		t.Error("Initially should have no processors")
	}

	connections := fc.GetConnections()
	if len(connections) != 0 {
		t.Error("Initially should have no connections")
	}

	// Add processor and verify
	processor := newMockTestProcessor(false)
	config := types.ProcessorConfig{
		ID:   uuid.New(),
		Name: "Test Processor",
		Type: "MockTestProcessor",
	}

	node, _ := fc.AddProcessor(processor, config)

	processors = fc.GetProcessors()
	if len(processors) != 1 {
		t.Errorf("Should have 1 processor, got %d", len(processors))
	}

	retrievedNode, exists := fc.GetProcessor(node.ID)
	if !exists {
		t.Error("GetProcessor should find added processor")
	}

	if retrievedNode.ID != node.ID {
		t.Error("Retrieved processor should have correct ID")
	}

	// Test GetProcessor with non-existent ID
	_, exists = fc.GetProcessor(uuid.New())
	if exists {
		t.Error("GetProcessor should return false for non-existent processor")
	}
}

func TestFlowControllerCreateProcessSession(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	// Create a test processor node
	processorNode := &ProcessorNode{
		ID:   uuid.New(),
		Name: "TestProcessor",
	}

	mockLogger := &LogrusAdapter{Logger: logger}
	session := fc.CreateProcessSession(processorNode, mockLogger)

	if session == nil {
		t.Fatal("CreateProcessSession should return a session")
	}

	// Test that session can be used
	flowFile := session.Create()
	if flowFile == nil {
		t.Error("Session should be able to create FlowFiles")
	}

	session.PutAttribute(flowFile, "test", "value")
	if flowFile.Attributes["test"] != "value" {
		t.Error("Session should be able to set attributes")
	}
}

func TestFlowFileQueue(t *testing.T) {
	// Create a connection with queue
	source := &ProcessorNode{ID: uuid.New(), Name: "Source"}
	dest := &ProcessorNode{ID: uuid.New(), Name: "Dest"}

	connection := &Connection{
		ID:               uuid.New(),
		Source:           source,
		Destination:      dest,
		Relationship:     types.RelationshipSuccess,
		BackPressureSize: 3, // Small size for testing
	}

	connection.Queue = &FlowFileQueue{
		connection: connection,
		flowFiles:  make([]*types.FlowFile, 0),
		maxSize:    connection.BackPressureSize,
	}

	// Test empty queue
	if !connection.Queue.IsEmpty() {
		t.Error("New queue should be empty")
	}

	if connection.Queue.Size() != 0 {
		t.Errorf("Empty queue size should be 0, got %d", connection.Queue.Size())
	}

	dequeued := connection.Queue.Dequeue()
	if dequeued != nil {
		t.Error("Dequeue from empty queue should return nil")
	}

	// Test enqueue
	flowFile1 := types.NewFlowFile()
	err := connection.Queue.Enqueue(flowFile1)
	if err != nil {
		t.Errorf("Enqueue should not return error: %v", err)
	}

	if connection.Queue.IsEmpty() {
		t.Error("Queue should not be empty after enqueue")
	}

	if connection.Queue.Size() != 1 {
		t.Errorf("Queue size should be 1, got %d", connection.Queue.Size())
	}

	// Test enqueue multiple
	flowFile2 := types.NewFlowFile()
	flowFile3 := types.NewFlowFile()

	connection.Queue.Enqueue(flowFile2)
	connection.Queue.Enqueue(flowFile3)

	if connection.Queue.Size() != 3 {
		t.Errorf("Queue size should be 3, got %d", connection.Queue.Size())
	}

	// Test back pressure (queue is full)
	flowFile4 := types.NewFlowFile()
	err = connection.Queue.Enqueue(flowFile4)
	if err == nil {
		t.Error("Enqueue should return error when queue is full")
	}

	// Test dequeue FIFO order
	dequeued = connection.Queue.Dequeue()
	if dequeued == nil {
		t.Fatal("Dequeue should return a FlowFile")
	}

	if dequeued.ID != flowFile1.ID {
		t.Error("Dequeue should return first enqueued FlowFile (FIFO)")
	}

	if connection.Queue.Size() != 2 {
		t.Errorf("Queue size should be 2 after dequeue, got %d", connection.Queue.Size())
	}

	// Dequeue remaining
	dequeued = connection.Queue.Dequeue()
	if dequeued.ID != flowFile2.ID {
		t.Error("Second dequeue should return second FlowFile")
	}

	dequeued = connection.Queue.Dequeue()
	if dequeued.ID != flowFile3.ID {
		t.Error("Third dequeue should return third FlowFile")
	}

	if !connection.Queue.IsEmpty() {
		t.Error("Queue should be empty after dequeuing all FlowFiles")
	}
}

// Benchmark tests
func BenchmarkFlowControllerAddProcessor(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor := newMockTestProcessor(false)
		config := types.ProcessorConfig{
			ID:   uuid.New(),
			Name: "Benchmark Processor",
			Type: "MockTestProcessor",
		}
		fc.AddProcessor(processor, config)
	}
}

func BenchmarkFlowControllerCreateProcessSession(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	fc := NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		NewInMemoryProvenanceRepository(),
		logger,
	)

	processorNode := &ProcessorNode{
		ID:   uuid.New(),
		Name: "TestProcessor",
	}

	mockLogger := &LogrusAdapter{Logger: logger}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fc.CreateProcessSession(processorNode, mockLogger)
	}
}

func BenchmarkFlowFileQueueOperations(b *testing.B) {
	queue := &FlowFileQueue{
		flowFiles: make([]*types.FlowFile, 0),
		maxSize:   1000,
	}

	flowFiles := make([]*types.FlowFile, b.N)
	for i := 0; i < b.N; i++ {
		flowFiles[i] = types.NewFlowFile()
	}

	b.ResetTimer()
	// Benchmark enqueue and dequeue operations
	for i := 0; i < b.N; i++ {
		queue.Enqueue(flowFiles[i])
	}

	for i := 0; i < b.N; i++ {
		queue.Dequeue()
	}
}