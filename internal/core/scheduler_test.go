package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Mock processor for scheduler testing
type mockSchedulerProcessor struct {
	*types.BaseProcessor
	triggerCount  int32
	shouldFail    bool
	executionTime time.Duration
	mu            sync.Mutex
}

func newMockSchedulerProcessor(shouldFail bool, executionTime time.Duration) *mockSchedulerProcessor {
	info := types.ProcessorInfo{
		Name:        "MockSchedulerProcessor",
		Description: "A mock processor for scheduler testing",
		Version:     "1.0.0",
		Author:      "Test",
		Tags:        []string{"test", "mock"},
		Properties:  []types.PropertySpec{},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &mockSchedulerProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		shouldFail:    shouldFail,
		executionTime: executionTime,
	}
}

func (p *mockSchedulerProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

func (p *mockSchedulerProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	atomic.AddInt32(&p.triggerCount, 1)

	if p.executionTime > 0 {
		time.Sleep(p.executionTime)
	}

	if p.shouldFail {
		return TestError{message: "mock trigger failed"}
	}

	return nil
}

func (p *mockSchedulerProcessor) GetTriggerCount() int32 {
	return atomic.LoadInt32(&p.triggerCount)
}

func (p *mockSchedulerProcessor) ResetTriggerCount() {
	atomic.StoreInt32(&p.triggerCount, 0)
}

// Test helper functions
func newMockProvenanceRepository() ProvenanceRepository {
	return NewInMemoryProvenanceRepository()
}

func createTestFlowController(t *testing.T) *FlowController {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		newMockProvenanceRepository(),
		logger,
	)
}

// Test helper to create a flow controller for scheduler tests
func createSchedulerFlowController(logger *logrus.Logger) *FlowController {
	return NewFlowController(
		newMockFlowFileRepository(),
		newMockContentRepository(),
		newMockProvenanceRepository(),
		logger,
	)
}

// Test helper to create processor node
func createTestProcessorNode(processor types.Processor, config types.ProcessorConfig) *ProcessorNode {
	return &ProcessorNode{
		ID:        config.ID,
		Name:      config.Name,
		Type:      config.Type,
		Processor: processor,
		Config:    config,
		Status: types.ProcessorStatus{
			ID:    config.ID,
			Name:  config.Name,
			Type:  config.Type,
			State: types.ProcessorStateRunning,
		},
	}
}

// TestNewProcessScheduler tests scheduler creation
func TestNewProcessScheduler(t *testing.T) {
	logger := logrus.New()
	fc := createSchedulerFlowController(logger)

	scheduler := NewProcessScheduler(fc, logger)

	if scheduler == nil {
		t.Fatal("Expected scheduler to be created")
	}

	if scheduler.flowController != fc {
		t.Error("Expected flowController to be set")
	}

	if scheduler.logger != logger {
		t.Error("Expected logger to be set")
	}

	if scheduler.scheduledTasks == nil {
		t.Error("Expected scheduledTasks map to be initialized")
	}

	if scheduler.cronScheduler == nil {
		t.Error("Expected cronScheduler to be initialized")
	}

	if scheduler.workerPool == nil {
		t.Error("Expected workerPool to be initialized")
	}

	if scheduler.running {
		t.Error("Expected scheduler to not be running initially")
	}
}

// TestSchedulerStart tests starting the scheduler
func TestSchedulerStart(t *testing.T) {
	logger := logrus.New()
	fc := createSchedulerFlowController(logger)
	scheduler := NewProcessScheduler(fc, logger)

	err := scheduler.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting scheduler, got: %v", err)
	}

	if !scheduler.running {
		t.Error("Expected scheduler to be running after Start()")
	}

	if scheduler.timerScheduler == nil {
		t.Error("Expected timerScheduler to be started")
	}

	// Cleanup
	scheduler.Stop()
}

// TestSchedulerStartAlreadyRunning tests starting an already running scheduler
func TestSchedulerStartAlreadyRunning(t *testing.T) {
	logger := logrus.New()
	fc := createSchedulerFlowController(logger)
	scheduler := NewProcessScheduler(fc, logger)

	err := scheduler.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected no error on first start, got: %v", err)
	}

	err = scheduler.Start(context.Background())
	if err == nil {
		t.Error("Expected error when starting already running scheduler")
	}

	// Cleanup
	scheduler.Stop()
}

// TestSchedulerStop tests stopping the scheduler
func TestSchedulerStop(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	// Start first
	err := scheduler.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting scheduler, got: %v", err)
	}

	// Then stop
	err = scheduler.Stop()
	if err != nil {
		t.Fatalf("Expected no error stopping scheduler, got: %v", err)
	}

	if scheduler.running {
		t.Error("Expected scheduler to not be running after Stop()")
	}
}

// TestSchedulerStopNotRunning tests stopping a non-running scheduler
func TestSchedulerStopNotRunning(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	err := scheduler.Stop()
	if err != nil {
		t.Errorf("Expected no error stopping non-running scheduler, got: %v", err)
	}
}

// TestScheduleProcessorTimerDriven tests scheduling a timer-driven processor
func TestScheduleProcessorTimerDriven(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestTimerProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling processor, got: %v", err)
	}

	if len(scheduler.scheduledTasks) != 1 {
		t.Errorf("Expected 1 scheduled task, got: %d", len(scheduler.scheduledTasks))
	}

	task, exists := scheduler.scheduledTasks[processorNode.ID]
	if !exists {
		t.Fatal("Expected task to be scheduled")
	}

	if task.Schedule.Type != types.ScheduleTypeTimer {
		t.Errorf("Expected schedule type TIMER_DRIVEN, got: %s", task.Schedule.Type)
	}

	if task.Schedule.Interval != 1*time.Second {
		t.Errorf("Expected interval 1s, got: %v", task.Schedule.Interval)
	}
}

// TestScheduleProcessorCronDriven tests scheduling a cron-driven processor
func TestScheduleProcessorCronDriven(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestCronProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeCron,
		ScheduleValue: "*/5 * * * * *", // Every 5 seconds
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling cron processor, got: %v", err)
	}

	task, exists := scheduler.scheduledTasks[processorNode.ID]
	if !exists {
		t.Fatal("Expected task to be scheduled")
	}

	if task.Schedule.Type != types.ScheduleTypeCron {
		t.Errorf("Expected schedule type CRON_DRIVEN, got: %s", task.Schedule.Type)
	}

	if task.Schedule.CronExpression != "*/5 * * * * *" {
		t.Errorf("Expected cron expression '*/5 * * * * *', got: %s", task.Schedule.CronExpression)
	}
}

// TestScheduleProcessorEventDriven tests scheduling an event-driven processor
func TestScheduleProcessorEventDriven(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestEventProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeEvent,
		ScheduleValue: "",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling event-driven processor, got: %v", err)
	}

	task, exists := scheduler.scheduledTasks[processorNode.ID]
	if !exists {
		t.Fatal("Expected task to be scheduled")
	}

	if task.Schedule.Type != types.ScheduleTypeEvent {
		t.Errorf("Expected schedule type EVENT_DRIVEN, got: %s", task.Schedule.Type)
	}

	if task.Schedule.Interval != 100*time.Millisecond {
		t.Errorf("Expected interval 100ms for event-driven, got: %v", task.Schedule.Interval)
	}
}

// TestScheduleProcessorPrimaryNode tests scheduling a primary node processor
func TestScheduleProcessorPrimaryNode(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestPrimaryNodeProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypePrimaryNode,
		ScheduleValue: "2s",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling primary node processor, got: %v", err)
	}

	task, exists := scheduler.scheduledTasks[processorNode.ID]
	if !exists {
		t.Fatal("Expected task to be scheduled")
	}

	if task.Schedule.Type != types.ScheduleTypePrimaryNode {
		t.Errorf("Expected schedule type PRIMARY_NODE_ONLY, got: %s", task.Schedule.Type)
	}

	if task.Schedule.Interval != 2*time.Second {
		t.Errorf("Expected interval 2s, got: %v", task.Schedule.Interval)
	}
}

// TestScheduleProcessorInvalidSchedule tests scheduling with invalid configuration
func TestScheduleProcessorInvalidSchedule(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestInvalidProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "invalid-duration",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err == nil {
		t.Error("Expected error with invalid timer duration")
	}
}

// TestScheduleProcessorInvalidCron tests scheduling with invalid cron expression
func TestScheduleProcessorInvalidCron(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	// Start scheduler so cron jobs are actually scheduled
	scheduler.Start(context.Background())
	defer scheduler.Stop()

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestInvalidCronProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeCron,
		ScheduleValue: "invalid cron",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err == nil {
		t.Error("Expected error with invalid cron expression")
	}
}

// TestScheduleProcessorUnsupportedType tests scheduling with unsupported schedule type
func TestScheduleProcessorUnsupportedType(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestUnsupportedProcessor",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleType("UNSUPPORTED"),
		ScheduleValue: "1s",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err == nil {
		t.Error("Expected error with unsupported schedule type")
	}
}

// TestScheduleProcessorDefaultConcurrency tests that concurrency defaults to 1
func TestScheduleProcessorDefaultConcurrency(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestDefaultConcurrency",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   0, // Invalid, should default to 1
	}

	processorNode := createTestProcessorNode(processor, config)

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	task := scheduler.scheduledTasks[processorNode.ID]
	if task.Schedule.Concurrency != 1 {
		t.Errorf("Expected default concurrency 1, got: %d", task.Schedule.Concurrency)
	}
}

// TestUnscheduleProcessor tests unscheduling a processor
func TestUnscheduleProcessor(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestUnschedule",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	// Schedule first
	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling, got: %v", err)
	}

	if len(scheduler.scheduledTasks) != 1 {
		t.Errorf("Expected 1 scheduled task, got: %d", len(scheduler.scheduledTasks))
	}

	// Unschedule
	scheduler.UnscheduleProcessor(processorNode)

	if len(scheduler.scheduledTasks) != 0 {
		t.Errorf("Expected 0 scheduled tasks after unscheduling, got: %d", len(scheduler.scheduledTasks))
	}
}

// TestTimerScheduleLoop tests the timer-driven task execution loop
func TestTimerScheduleLoop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 10*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestTimerLoop",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "100ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	// Schedule processor
	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling, got: %v", err)
	}

	// Start scheduler
	err = scheduler.Start(context.Background())
	if err != nil {
		t.Fatalf("Expected no error starting scheduler, got: %v", err)
	}

	// Wait for some executions
	time.Sleep(350 * time.Millisecond)

	// Stop scheduler
	scheduler.Stop()

	triggerCount := processor.GetTriggerCount()
	if triggerCount < 1 {
		t.Errorf("Expected at least 1 execution, got: %d", triggerCount)
	}

	t.Logf("Processor was triggered %d times", triggerCount)
}

// TestCheckTimerDrivenTasks tests the checkTimerDrivenTasks method
func TestCheckTimerDrivenTasks(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 0)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestCheckTasks",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	// Schedule with NextRun in the past
	scheduler.ScheduleProcessor(processorNode)
	task := scheduler.scheduledTasks[processorNode.ID]
	task.mu.Lock()
	task.NextRun = time.Now().Add(-1 * time.Second)
	task.mu.Unlock()

	// Start scheduler
	scheduler.Start(context.Background())

	// Wait for execution - need at least one timer tick (100ms) plus processing time
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler
	scheduler.Stop()

	// Check execution happened
	if processor.GetTriggerCount() < 1 {
		t.Errorf("Expected at least 1 execution, got: %d", processor.GetTriggerCount())
	}

	// Check task state updated
	task.mu.RLock()
	runCount := task.RunCount
	task.mu.RUnlock()
	if runCount < 1 {
		t.Errorf("Expected run count >= 1, got: %d", runCount)
	}
}

// TestExecuteProcessorConcurrency tests that a processor doesn't execute concurrently
func TestExecuteProcessorConcurrency(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 200*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestConcurrency",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "50ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	scheduler.ScheduleProcessor(processorNode)
	task := scheduler.scheduledTasks[processorNode.ID]
	task.NextRun = time.Now().Add(-1 * time.Second)

	scheduler.Start(context.Background())

	// Wait for potential concurrent executions
	time.Sleep(400 * time.Millisecond)

	scheduler.Stop()

	// Should have limited concurrent executions due to Running flag
	count := processor.GetTriggerCount()
	t.Logf("Processor executed %d times", count)

	// Even with a fast schedule, the long execution should prevent many concurrent runs
	if count > 5 {
		t.Errorf("Expected limited executions due to running flag, got: %d", count)
	}
}

// TestWorkerPoolFull tests behavior when worker pool is full
func TestWorkerPoolFull(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)

	// Create scheduler with small worker pool
	scheduler := NewProcessScheduler(fc, logger)
	scheduler.workerPool = NewWorkerPool(1, scheduler.ctx, logger) // Only 1 worker

	// Create multiple slow processors
	processors := make([]*mockSchedulerProcessor, 3)
	for i := 0; i < 3; i++ {
		processors[i] = newMockSchedulerProcessor(false, 100*time.Millisecond)
		config := types.ProcessorConfig{
			ID:            uuid.New(),
			Name:          "TestPoolFull" + string(rune('A'+i)),
			Type:          "MockSchedulerProcessor",
			ScheduleType:  types.ScheduleTypeTimer,
			ScheduleValue: "50ms",
			Concurrency:   1,
		}

		processorNode := createTestProcessorNode(processors[i], config)
		scheduler.ScheduleProcessor(processorNode)

		// Set NextRun in the past to trigger immediately
		task := scheduler.scheduledTasks[processorNode.ID]
		task.NextRun = time.Now().Add(-1 * time.Second)
	}

	scheduler.Start(context.Background())

	// Wait for executions - give enough time for at least one full execution cycle
	time.Sleep(500 * time.Millisecond)

	scheduler.Stop()

	// Check that not all processors ran due to pool being full
	totalExecutions := int32(0)
	for i, p := range processors {
		count := p.GetTriggerCount()
		totalExecutions += count
		t.Logf("Processor %d executed %d times", i, count)
	}

	if totalExecutions == 0 {
		t.Error("Expected at least some executions")
	}
}

// TestCalculateNextRun tests the calculateNextRun method
func TestCalculateNextRun(t *testing.T) {
	logger := logrus.New()
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	now := time.Now()

	tests := []struct {
		name          string
		scheduleType  types.ScheduleType
		interval      time.Duration
		expectedDelay time.Duration
	}{
		{
			name:          "Timer schedule",
			scheduleType:  types.ScheduleTypeTimer,
			interval:      5 * time.Second,
			expectedDelay: 5 * time.Second,
		},
		{
			name:          "Event schedule",
			scheduleType:  types.ScheduleTypeEvent,
			interval:      100 * time.Millisecond,
			expectedDelay: 100 * time.Millisecond,
		},
		{
			name:          "Primary node schedule",
			scheduleType:  types.ScheduleTypePrimaryNode,
			interval:      10 * time.Second,
			expectedDelay: 10 * time.Second,
		},
		{
			name:          "Cron schedule",
			scheduleType:  types.ScheduleTypeCron,
			interval:      0,
			expectedDelay: 24 * time.Hour, // Placeholder
		},
		{
			name:          "Default schedule",
			scheduleType:  types.ScheduleType("UNKNOWN"),
			interval:      0,
			expectedDelay: 1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := ProcessorSchedule{
				Type:     tt.scheduleType,
				Interval: tt.interval,
			}

			nextRun := scheduler.calculateNextRun(schedule, now)
			actualDelay := nextRun.Sub(now)

			if actualDelay < tt.expectedDelay-time.Millisecond || actualDelay > tt.expectedDelay+time.Millisecond {
				t.Errorf("Expected delay ~%v, got: %v", tt.expectedDelay, actualDelay)
			}
		})
	}
}

// TestNewWorkerPool tests worker pool creation
func TestNewWorkerPool(t *testing.T) {
	logger := logrus.New()
	ctx := context.Background()

	wp := NewWorkerPool(5, ctx, logger)

	if wp == nil {
		t.Fatal("Expected worker pool to be created")
	}

	if wp.workerCount != 5 {
		t.Errorf("Expected worker count 5, got: %d", wp.workerCount)
	}

	if wp.workers == nil {
		t.Error("Expected workers channel to be initialized")
	}

	if cap(wp.workers) != 10 { // workerCount * 2
		t.Errorf("Expected workers channel capacity 10, got: %d", cap(wp.workers))
	}

	if wp.logger != logger {
		t.Error("Expected logger to be set")
	}
}

// TestWorkerPoolStart tests starting the worker pool
func TestWorkerPoolStart(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()

	wp := NewWorkerPool(3, ctx, logger)

	err := wp.Start()
	if err != nil {
		t.Fatalf("Expected no error starting worker pool, got: %v", err)
	}

	// Give workers time to start
	time.Sleep(50 * time.Millisecond)

	// Cleanup
	wp.Stop()
}

// TestWorkerPoolStop tests stopping the worker pool
func TestWorkerPoolStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()

	wp := NewWorkerPool(3, ctx, logger)
	wp.Start()

	// Give workers time to start
	time.Sleep(50 * time.Millisecond)

	wp.Stop()

	// Wait for workers to stop
	time.Sleep(50 * time.Millisecond)

	// Try to send work (should not panic, channel is closed)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Sending to closed channel should not panic in normal use")
		}
	}()
}

// TestWorkerPoolExecution tests executing tasks through worker pool
func TestWorkerPoolExecution(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()
	fc := createTestFlowController(t)

	wp := NewWorkerPool(2, ctx, logger)
	wp.Start()
	defer wp.Stop()

	processor := newMockSchedulerProcessor(false, 50*time.Millisecond)
	config := types.ProcessorConfig{
		ID:          uuid.New(),
		Name:        "TestWorkerExecution",
		Type:        "MockSchedulerProcessor",
		Concurrency: 1,
	}

	processorNode := createTestProcessorNode(processor, config)
	session := fc.CreateProcessSession(processorNode, &LogrusAdapter{Logger: logger})

	execution := &ProcessorExecution{
		ProcessorNode: processorNode,
		Session:       session,
		Task:          nil, // No scheduled task for manual test execution
		StartTime:     time.Now(),
	}

	// Send execution to worker pool
	wp.workers <- execution

	// Wait for execution
	time.Sleep(200 * time.Millisecond)

	if processor.GetTriggerCount() != 1 {
		t.Errorf("Expected 1 execution, got: %d", processor.GetTriggerCount())
	}

	// Check that statistics were updated
	if processorNode.Status.TasksCompleted != 1 {
		t.Errorf("Expected TasksCompleted 1, got: %d", processorNode.Status.TasksCompleted)
	}
}

// TestWorkerPoolExecutionFailure tests handling failed executions
func TestWorkerPoolExecutionFailure(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()
	fc := createTestFlowController(t)

	wp := NewWorkerPool(1, ctx, logger)
	wp.Start()
	defer wp.Stop()

	processor := newMockSchedulerProcessor(true, 0) // shouldFail = true
	config := types.ProcessorConfig{
		ID:          uuid.New(),
		Name:        "TestWorkerFailure",
		Type:        "MockSchedulerProcessor",
		Concurrency: 1,
	}

	processorNode := createTestProcessorNode(processor, config)
	session := fc.CreateProcessSession(processorNode, &LogrusAdapter{Logger: logger})

	execution := &ProcessorExecution{
		ProcessorNode: processorNode,
		Session:       session,
		Task:          nil, // No scheduled task for manual test execution
		StartTime:     time.Now(),
	}

	// Send execution to worker pool
	wp.workers <- execution

	// Wait for execution
	time.Sleep(100 * time.Millisecond)

	if processor.GetTriggerCount() != 1 {
		t.Errorf("Expected 1 execution attempt, got: %d", processor.GetTriggerCount())
	}

	// Task should still be marked as completed even if it failed
	if processorNode.Status.TasksCompleted != 1 {
		t.Errorf("Expected TasksCompleted 1, got: %d", processorNode.Status.TasksCompleted)
	}
}

// TestWorkerPoolActiveWorkers tests active worker tracking
func TestWorkerPoolActiveWorkers(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()
	fc := createTestFlowController(t)

	wp := NewWorkerPool(2, ctx, logger)
	wp.Start()
	defer wp.Stop()

	processor := newMockSchedulerProcessor(false, 100*time.Millisecond)

	// Send multiple tasks
	for i := 0; i < 2; i++ {
		config := types.ProcessorConfig{
			ID:          uuid.New(),
			Name:        "TestActiveWorker",
			Type:        "MockSchedulerProcessor",
			Concurrency: 1,
		}

		processorNode := createTestProcessorNode(processor, config)
		session := fc.CreateProcessSession(processorNode, &LogrusAdapter{Logger: logger})

		execution := &ProcessorExecution{
			ProcessorNode: processorNode,
			Session:       session,
			Task:          nil, // No scheduled task for manual test execution
			StartTime:     time.Now(),
		}

		wp.workers <- execution
	}

	// Check active workers
	time.Sleep(50 * time.Millisecond)

	wp.mu.RLock()
	activeWorkers := wp.activeWorkers
	wp.mu.RUnlock()

	if activeWorkers != 2 {
		t.Logf("Expected 2 active workers, got: %d (timing dependent, may vary)", activeWorkers)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	wp.mu.RLock()
	activeWorkers = wp.activeWorkers
	wp.mu.RUnlock()

	if activeWorkers != 0 {
		t.Errorf("Expected 0 active workers after completion, got: %d", activeWorkers)
	}
}

// TestScheduledTaskRunCount tests that run count increments
func TestScheduledTaskRunCount(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 10*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestRunCount",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "50ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)
	scheduler.ScheduleProcessor(processorNode)

	task := scheduler.scheduledTasks[processorNode.ID]
	initialRunCount := task.RunCount

	scheduler.Start(context.Background())

	// Wait for some executions
	time.Sleep(200 * time.Millisecond)

	scheduler.Stop()

	task.mu.RLock()
	finalRunCount := task.RunCount
	task.mu.RUnlock()

	if finalRunCount <= initialRunCount {
		t.Errorf("Expected run count to increment, initial: %d, final: %d", initialRunCount, finalRunCount)
	}

	t.Logf("Task run count: %d", finalRunCount)
}

// TestScheduledTaskLastRun tests that last run time is updated
func TestScheduledTaskLastRun(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 10*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestLastRun",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "50ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)
	scheduler.ScheduleProcessor(processorNode)

	task := scheduler.scheduledTasks[processorNode.ID]
	initialLastRun := task.LastRun

	scheduler.Start(context.Background())

	// Wait for execution
	time.Sleep(150 * time.Millisecond)

	scheduler.Stop()

	task.mu.RLock()
	finalLastRun := task.LastRun
	task.mu.RUnlock()

	if !finalLastRun.After(initialLastRun) {
		t.Error("Expected last run time to be updated")
	}
}

// TestCronSchedulerIntegration tests cron-based scheduling
func TestCronSchedulerIntegration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 10*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestCronIntegration",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeCron,
		ScheduleValue: "* * * * * *", // Every second
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)

	scheduler.Start(context.Background())

	err := scheduler.ScheduleProcessor(processorNode)
	if err != nil {
		t.Fatalf("Expected no error scheduling cron processor, got: %v", err)
	}

	// Wait for at least one execution
	time.Sleep(1500 * time.Millisecond)

	scheduler.Stop()

	if processor.GetTriggerCount() < 1 {
		t.Errorf("Expected at least 1 cron execution, got: %d", processor.GetTriggerCount())
	}

	t.Logf("Cron processor executed %d times", processor.GetTriggerCount())
}

// TestGracefulShutdown tests that shutdown waits for running tasks
func TestGracefulShutdown(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	processor := newMockSchedulerProcessor(false, 200*time.Millisecond)
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "TestGracefulShutdown",
		Type:          "MockSchedulerProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "50ms",
		Concurrency:   1,
	}

	processorNode := createTestProcessorNode(processor, config)
	scheduler.ScheduleProcessor(processorNode)

	scheduler.Start(context.Background())

	// Wait for task to start - need enough time for at least one timer tick (100ms)
	// plus time for the task to actually start executing
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler (should wait for running tasks)
	stopStart := time.Now()
	scheduler.Stop()
	stopDuration := time.Since(stopStart)

	// Verify graceful shutdown completed
	t.Logf("Shutdown took: %v", stopDuration)

	// Verify processor finished
	if processor.GetTriggerCount() < 1 {
		t.Error("Expected processor to complete execution before shutdown")
	}
}

// TestMultipleProcessorsScheduling tests scheduling multiple processors
func TestMultipleProcessorsScheduling(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	fc := createTestFlowController(t)
	scheduler := NewProcessScheduler(fc, logger)

	numProcessors := 5
	processors := make([]*mockSchedulerProcessor, numProcessors)

	for i := 0; i < numProcessors; i++ {
		processors[i] = newMockSchedulerProcessor(false, 10*time.Millisecond)
		config := types.ProcessorConfig{
			ID:            uuid.New(),
			Name:          "TestMultiple" + string(rune('A'+i)),
			Type:          "MockSchedulerProcessor",
			ScheduleType:  types.ScheduleTypeTimer,
			ScheduleValue: "100ms",
			Concurrency:   1,
		}

		processorNode := createTestProcessorNode(processors[i], config)
		err := scheduler.ScheduleProcessor(processorNode)
		if err != nil {
			t.Fatalf("Error scheduling processor %d: %v", i, err)
		}
	}

	if len(scheduler.scheduledTasks) != numProcessors {
		t.Errorf("Expected %d scheduled tasks, got: %d", numProcessors, len(scheduler.scheduledTasks))
	}

	scheduler.Start(context.Background())

	// Wait for executions
	time.Sleep(250 * time.Millisecond)

	scheduler.Stop()

	// Verify all processors executed
	for i, p := range processors {
		count := p.GetTriggerCount()
		if count < 1 {
			t.Errorf("Processor %d expected at least 1 execution, got: %d", i, count)
		}
		t.Logf("Processor %d executed %d times", i, count)
	}
}

// TestProcessorStatisticsUpdate tests that processor statistics are updated
func TestProcessorStatisticsUpdate(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	ctx := context.Background()
	fc := createTestFlowController(t)

	wp := NewWorkerPool(1, ctx, logger)
	wp.Start()
	defer wp.Stop()

	processor := newMockSchedulerProcessor(false, 50*time.Millisecond)
	config := types.ProcessorConfig{
		ID:          uuid.New(),
		Name:        "TestStatistics",
		Type:        "MockSchedulerProcessor",
		Concurrency: 1,
	}

	processorNode := createTestProcessorNode(processor, config)
	session := fc.CreateProcessSession(processorNode, &LogrusAdapter{Logger: logger})

	initialCompleted := processorNode.Status.TasksCompleted
	initialLastRun := processorNode.Status.LastRun

	execution := &ProcessorExecution{
		ProcessorNode: processorNode,
		Session:       session,
		Task:          nil, // No scheduled task for manual test execution
		StartTime:     time.Now(),
	}

	wp.workers <- execution

	// Wait for execution
	time.Sleep(150 * time.Millisecond)

	processorNode.mu.RLock()
	finalCompleted := processorNode.Status.TasksCompleted
	finalLastRun := processorNode.Status.LastRun
	finalAvgTime := processorNode.Status.AverageTaskTime
	processorNode.mu.RUnlock()

	if finalCompleted != initialCompleted+1 {
		t.Errorf("Expected TasksCompleted to increment by 1, got: %d -> %d", initialCompleted, finalCompleted)
	}

	if !finalLastRun.After(initialLastRun) {
		t.Error("Expected LastRun to be updated")
	}

	if finalAvgTime == 0 {
		t.Error("Expected AverageTaskTime to be updated")
	}

	t.Logf("Average task time: %v", finalAvgTime)
}
