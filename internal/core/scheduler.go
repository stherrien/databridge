package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ProcessScheduler manages the execution of processors
type ProcessScheduler struct {
	mu               sync.RWMutex
	flowController   *FlowController
	logger           *logrus.Logger
	ctx              context.Context
	cancel           context.CancelFunc

	// Scheduling
	scheduledTasks   map[uuid.UUID]*ScheduledTask
	timerScheduler   *time.Ticker
	timerInterval    time.Duration
	cronScheduler    *cron.Cron
	workerPool       *WorkerPool
	running          bool

	// Enhanced scheduling
	loadScheduler        *LoadBasedScheduler
	priorityScheduler    *PriorityScheduler
	conditionalScheduler *ConditionalScheduler
}

// ScheduledTask represents a scheduled processor execution
type ScheduledTask struct {
	ProcessorNode *ProcessorNode
	Schedule      ProcessorSchedule
	LastRun       time.Time
	NextRun       time.Time
	Running       bool
	RunCount      int64
	mu            sync.RWMutex
}

// ProcessorSchedule defines how a processor should be scheduled
type ProcessorSchedule struct {
	Type           types.ScheduleType `json:"type"`
	Interval       time.Duration      `json:"interval"`
	CronExpression string             `json:"cronExpression,omitempty"`
	Concurrency    int                `json:"concurrency"`
}

// WorkerPool manages concurrent processor execution
type WorkerPool struct {
	workers       chan *ProcessorExecution
	workerCount   int
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *logrus.Logger
	mu            sync.RWMutex
	activeWorkers int
	wg            sync.WaitGroup
}

// ProcessorExecution represents a processor execution task
type ProcessorExecution struct {
	ProcessorNode *ProcessorNode
	Session       types.ProcessSession
	Task          *ScheduledTask
	StartTime     time.Time
}

// NewProcessScheduler creates a new ProcessScheduler
func NewProcessScheduler(flowController *FlowController, logger *logrus.Logger) *ProcessScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	// Worker pool needs its own context so it can finish tasks during graceful shutdown
	workerCtx := context.Background()

	return &ProcessScheduler{
		flowController: flowController,
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		scheduledTasks: make(map[uuid.UUID]*ScheduledTask),
		timerInterval:  100 * time.Millisecond, // Check tasks every 100ms
		cronScheduler:  cron.New(cron.WithSeconds()),
		workerPool:     NewWorkerPool(10, workerCtx, logger), // Default 10 workers

		// Initialize enhanced schedulers
		loadScheduler:        NewLoadBasedScheduler(),
		priorityScheduler:    NewPriorityScheduler(),
		conditionalScheduler: NewConditionalScheduler(flowController),
	}
}

// Start starts the scheduler
func (s *ProcessScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	s.logger.Info("Starting ProcessScheduler")

	// Start cron scheduler
	s.cronScheduler.Start()

	// Start timer-driven scheduler
	s.timerScheduler = time.NewTicker(s.timerInterval)
	go s.timerScheduleLoop()

	// Start worker pool
	if err := s.workerPool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	s.running = true
	s.logger.Info("ProcessScheduler started")
	return nil
}

// Stop stops the scheduler
func (s *ProcessScheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping ProcessScheduler")

	// Cancel context first to stop timer loop
	s.cancel()

	// Stop timer scheduler
	if s.timerScheduler != nil {
		s.timerScheduler.Stop()
	}

	// Stop cron scheduler
	cronCtx := s.cronScheduler.Stop()
	<-cronCtx.Done()

	// Stop worker pool (waits for active tasks to complete)
	s.workerPool.Stop()

	s.logger.Info("ProcessScheduler stopped")
	return nil
}

// ScheduleProcessor schedules a processor for execution
func (s *ProcessScheduler) ScheduleProcessor(processorNode *ProcessorNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, err := s.parseSchedule(processorNode.Config)
	if err != nil {
		return fmt.Errorf("failed to parse schedule: %w", err)
	}

	task := &ScheduledTask{
		ProcessorNode: processorNode,
		Schedule:      schedule,
		NextRun:       s.calculateNextRun(schedule, time.Now()),
	}

	s.scheduledTasks[processorNode.ID] = task

	// For cron-driven processors, add to cron scheduler
	if schedule.Type == types.ScheduleTypeCron {
		_, err := s.cronScheduler.AddFunc(schedule.CronExpression, func() {
			s.executeProcessor(task)
		})
		if err != nil {
			return fmt.Errorf("failed to schedule cron job: %w", err)
		}
	}

	s.logger.WithFields(logrus.Fields{
		"processorId":   processorNode.ID,
		"processorName": processorNode.Name,
		"scheduleType":  schedule.Type,
	}).Info("Scheduled processor")

	return nil
}

// UnscheduleProcessor removes a processor from scheduling
func (s *ProcessScheduler) UnscheduleProcessor(processorNode *ProcessorNode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.scheduledTasks, processorNode.ID)

	s.logger.WithFields(logrus.Fields{
		"processorId":   processorNode.ID,
		"processorName": processorNode.Name,
	}).Info("Unscheduled processor")
}

// Private methods

func (s *ProcessScheduler) timerScheduleLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.timerScheduler.C:
			s.checkTimerDrivenTasks()
		}
	}
}

func (s *ProcessScheduler) checkTimerDrivenTasks() {
	s.mu.RLock()
	tasks := make([]*ScheduledTask, 0, len(s.scheduledTasks))
	now := time.Now()

	for _, task := range s.scheduledTasks {
		if (task.Schedule.Type == types.ScheduleTypeTimer ||
			task.Schedule.Type == types.ScheduleTypeEvent) &&
			now.After(task.NextRun) && !task.Running {
			tasks = append(tasks, task)
		}
	}
	s.mu.RUnlock()

	// Execute eligible tasks
	for _, task := range tasks {
		s.executeProcessor(task)
	}
}

func (s *ProcessScheduler) executeProcessor(task *ScheduledTask) {
	// Check if scheduler is still running
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	if !running {
		return
	}

	// Check system load before execution
	if !s.loadScheduler.CheckLoad(s.workerPool) {
		s.logger.WithFields(logrus.Fields{
			"processorName": task.ProcessorNode.Name,
		}).Debug("Skipped processor execution - system load too high")
		return
	}

	// Check conditional rules
	shouldExecute, action := s.conditionalScheduler.ShouldExecute(task.ProcessorNode.ID, task)
	if !shouldExecute {
		s.logger.WithFields(logrus.Fields{
			"processorName": task.ProcessorNode.Name,
			"ruleAction":    action,
		}).Debug("Skipped processor execution - conditional rule")
		return
	}

	task.mu.Lock()
	if task.Running {
		task.mu.Unlock()
		return
	}
	task.Running = true
	task.LastRun = time.Now()
	task.NextRun = s.calculateNextRun(task.Schedule, task.LastRun)
	task.RunCount++
	task.mu.Unlock()

	// Create process session with processor's queues
	session := s.flowController.CreateProcessSession(task.ProcessorNode, &LogrusAdapter{Logger: s.logger})

	execution := &ProcessorExecution{
		ProcessorNode: task.ProcessorNode,
		Session:       session,
		Task:          task,
		StartTime:     time.Now(),
	}

	// Submit to worker pool
	select {
	case s.workerPool.workers <- execution:
		// Successfully submitted
	default:
		// Worker pool is full, skip this execution
		task.mu.Lock()
		task.Running = false
		task.mu.Unlock()

		s.logger.WithFields(logrus.Fields{
			"processorName": task.ProcessorNode.Name,
		}).Warn("Skipped processor execution - worker pool full")
	}
}

func (s *ProcessScheduler) parseSchedule(config types.ProcessorConfig) (ProcessorSchedule, error) {
	schedule := ProcessorSchedule{
		Type:        config.ScheduleType,
		Concurrency: config.Concurrency,
	}

	if schedule.Concurrency <= 0 {
		schedule.Concurrency = 1
	}

	switch config.ScheduleType {
	case types.ScheduleTypeTimer:
		duration, err := time.ParseDuration(config.ScheduleValue)
		if err != nil {
			return schedule, fmt.Errorf("invalid timer duration: %w", err)
		}
		schedule.Interval = duration
	case types.ScheduleTypeCron:
		schedule.CronExpression = config.ScheduleValue
	case types.ScheduleTypeEvent:
		// Event-driven processors run immediately when data is available
		schedule.Interval = 100 * time.Millisecond // Quick polling for events
	case types.ScheduleTypePrimaryNode:
		// Primary node only - treat as timer-driven for now
		duration, err := time.ParseDuration(config.ScheduleValue)
		if err != nil {
			return schedule, fmt.Errorf("invalid timer duration for primary node: %w", err)
		}
		schedule.Interval = duration
	default:
		return schedule, fmt.Errorf("unsupported schedule type: %s", config.ScheduleType)
	}

	return schedule, nil
}

func (s *ProcessScheduler) calculateNextRun(schedule ProcessorSchedule, lastRun time.Time) time.Time {
	switch schedule.Type {
	case types.ScheduleTypeTimer, types.ScheduleTypeEvent, types.ScheduleTypePrimaryNode:
		return lastRun.Add(schedule.Interval)
	case types.ScheduleTypeCron:
		// For cron jobs, the cron scheduler handles the next run calculation
		return time.Now().Add(24 * time.Hour) // Placeholder
	default:
		return time.Now().Add(1 * time.Minute) // Default fallback
	}
}

// WorkerPool implementation

// NewWorkerPool creates a new WorkerPool
func NewWorkerPool(workerCount int, ctx context.Context, logger *logrus.Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &WorkerPool{
		workers:     make(chan *ProcessorExecution, workerCount*2), // Buffer for queued executions
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() error {
	wp.logger.WithField("workerCount", wp.workerCount).Info("Starting worker pool")

	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	return nil
}

// Stop stops the worker pool and waits for all active tasks to complete
func (wp *WorkerPool) Stop() {
	wp.logger.Info("Stopping worker pool")
	close(wp.workers)
	wp.wg.Wait()
	wp.logger.Debug("All workers stopped")
}

// worker is the main worker goroutine
func (wp *WorkerPool) worker(workerID int) {
	defer wp.wg.Done()
	wp.logger.WithField("workerId", workerID).Debug("Worker started")

	for {
		select {
		case <-wp.ctx.Done():
			wp.logger.WithField("workerId", workerID).Debug("Worker stopped")
			return
		case execution, ok := <-wp.workers:
			if !ok {
				wp.logger.WithField("workerId", workerID).Debug("Worker channel closed")
				return
			}
			wp.executeProcessorTask(workerID, execution)
		}
	}
}

func (wp *WorkerPool) executeProcessorTask(workerID int, execution *ProcessorExecution) {
	wp.mu.Lock()
	wp.activeWorkers++
	wp.mu.Unlock()

	defer func() {
		wp.mu.Lock()
		wp.activeWorkers--
		wp.mu.Unlock()

		// Mark task as no longer running
		if execution.Task != nil {
			execution.Task.mu.Lock()
			execution.Task.Running = false
			execution.Task.mu.Unlock()
		}

		// End rate limiter execution tracking
		if execution.ProcessorNode.RateLimiter != nil {
			execution.ProcessorNode.RateLimiter.EndExecution()
		}
	}()

	logger := wp.logger.WithFields(logrus.Fields{
		"workerId":      workerID,
		"processorName": execution.ProcessorNode.Name,
		"processorId":   execution.ProcessorNode.ID,
	})

	// Check circuit breaker
	if execution.ProcessorNode.CircuitBreaker != nil {
		if execution.ProcessorNode.CircuitBreaker.GetState() == CircuitOpen {
			logger.Warn("Circuit breaker is open, skipping execution")
			return
		}
	}

	// Check rate limiter
	if execution.ProcessorNode.RateLimiter != nil && execution.ProcessorNode.RateLimiter.IsEnabled() {
		if !execution.ProcessorNode.RateLimiter.CanExecute() {
			logger.Debug("Rate limit reached, skipping execution")
			return
		}
		execution.ProcessorNode.RateLimiter.BeginExecution()
	}

	// Check for penalized connections
	for _, conn := range execution.ProcessorNode.Connections {
		if conn.IsPenalized() {
			logger.WithField("connection", conn.Name).Debug("Connection is penalized, skipping execution")
			return
		}
	}

	logger.Debug("Executing processor")

	startTime := time.Now()

	// Create context with processor context injected
	ctx := context.WithValue(wp.ctx, "processorContext", execution.ProcessorNode.Context)

	// Execute processor with circuit breaker protection
	var err error
	if execution.ProcessorNode.CircuitBreaker != nil {
		err = execution.ProcessorNode.CircuitBreaker.Call(func() error {
			return execution.ProcessorNode.Processor.OnTrigger(ctx, execution.Session)
		})
	} else {
		err = execution.ProcessorNode.Processor.OnTrigger(ctx, execution.Session)
	}

	duration := time.Since(startTime)

	// Update processor statistics
	execution.ProcessorNode.mu.Lock()
	execution.ProcessorNode.Status.TasksCompleted++
	execution.ProcessorNode.Status.LastRun = startTime
	execution.ProcessorNode.Status.AverageTaskTime =
		(execution.ProcessorNode.Status.AverageTaskTime + duration) / 2
	execution.ProcessorNode.mu.Unlock()

	if err != nil {
		logger.WithError(err).Error("Processor execution failed")
		execution.Session.Rollback()

		// Check if should retry
		if execution.ProcessorNode.RetryPolicy != nil && execution.ProcessorNode.RetryQueue != nil {
			// Get failed FlowFiles from session (this would need session API enhancement)
			// For now, we just log the retry possibility
			logger.Debug("Retry policy available for failed execution")
		}
	} else {
		if err := execution.Session.Commit(); err != nil {
			logger.WithError(err).Error("Failed to commit process session")
		} else {
			logger.WithField("duration", duration).Debug("Processor execution completed successfully")
		}
	}

	// Mark scheduled task as no longer running
	// This would typically be done through a callback or task reference
}

// Enhanced Scheduling API

// GetLoadScheduler returns the load-based scheduler
func (s *ProcessScheduler) GetLoadScheduler() *LoadBasedScheduler {
	return s.loadScheduler
}

// GetPriorityScheduler returns the priority scheduler
func (s *ProcessScheduler) GetPriorityScheduler() *PriorityScheduler {
	return s.priorityScheduler
}

// GetConditionalScheduler returns the conditional scheduler
func (s *ProcessScheduler) GetConditionalScheduler() *ConditionalScheduler {
	return s.conditionalScheduler
}

// SetLoadThresholds configures load-based scheduling thresholds
func (s *ProcessScheduler) SetLoadThresholds(cpu, memory float64, goroutines int) {
	s.loadScheduler.SetThresholds(cpu, memory, goroutines)
}

// AddExecutionRule adds a conditional execution rule
func (s *ProcessScheduler) AddExecutionRule(rule *ExecutionRule) error {
	return s.conditionalScheduler.AddRule(rule)
}

// RemoveExecutionRule removes a conditional execution rule
func (s *ProcessScheduler) RemoveExecutionRule(ruleID uuid.UUID) {
	s.conditionalScheduler.RemoveRule(ruleID)
}

// GetExecutionRules returns all execution rules for a processor
func (s *ProcessScheduler) GetExecutionRules(processorID uuid.UUID) []*ExecutionRule {
	return s.conditionalScheduler.GetRules(processorID)
}

// GetSystemLoad returns current system load metrics
func (s *ProcessScheduler) GetSystemLoad() SystemLoad {
	s.loadScheduler.mu.RLock()
	defer s.loadScheduler.mu.RUnlock()
	return s.loadScheduler.currentLoad
}

// GetAverageLoad returns average system load
func (s *ProcessScheduler) GetAverageLoad() SystemLoad {
	return s.loadScheduler.GetAverageLoad()
}

// GetPriorityQueueDepths returns the depth of each priority queue
func (s *ProcessScheduler) GetPriorityQueueDepths() map[ProcessorPriority]int {
	return s.priorityScheduler.GetQueueDepths()
}