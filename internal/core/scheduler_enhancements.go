package core

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LoadBasedScheduler extends scheduling with system load awareness
type LoadBasedScheduler struct {
	mu                    sync.RWMutex
	enabled               bool
	cpuThreshold          float64 // 0.0-1.0, percentage
	memoryThreshold       float64 // 0.0-1.0, percentage
	goroutineThreshold    int
	checkInterval         time.Duration
	lastCheck             time.Time
	currentLoad           SystemLoad
	loadHistory           []SystemLoad
	maxHistorySize        int
}

// SystemLoad represents current system resource utilization
type SystemLoad struct {
	CPUPercent        float64
	MemoryPercent     float64
	GoroutineCount    int
	ActiveWorkers     int
	QueueDepth        int
	Timestamp         time.Time
}

// PriorityScheduler adds priority-based task scheduling
type PriorityScheduler struct {
	mu              sync.RWMutex
	priorityQueues  map[ProcessorPriority]*PriorityQueue
	enabled         bool
}

// ProcessorPriority defines execution priority levels
type ProcessorPriority int

const (
	PriorityCritical ProcessorPriority = 4
	PriorityHigh     ProcessorPriority = 3
	PriorityNormal   ProcessorPriority = 2
	PriorityLow      ProcessorPriority = 1
	PriorityIdle     ProcessorPriority = 0
)

// PriorityQueue manages tasks at a specific priority level
type PriorityQueue struct {
	mu      sync.RWMutex
	tasks   []*PriorityTask
	maxSize int
}

// PriorityTask wraps a scheduled task with priority metadata
type PriorityTask struct {
	Task      *ScheduledTask
	Priority  ProcessorPriority
	QueueTime time.Time
	Deadline  time.Time
}

// ConditionalScheduler enables rule-based conditional execution
type ConditionalScheduler struct {
	mu         sync.RWMutex
	rules      map[uuid.UUID]*ExecutionRule
	evaluator  *RuleEvaluator
	enabled    bool
}

// ExecutionRule defines conditions for processor execution
type ExecutionRule struct {
	ID            uuid.UUID
	ProcessorID   uuid.UUID
	Name          string
	Conditions    []Condition
	Action        RuleAction
	Enabled       bool
	EvaluateMode  EvaluateMode // ALL, ANY, NONE
}

// Condition represents a single rule condition
type Condition struct {
	Type      ConditionType
	Operator  Operator
	Value     interface{}
	Attribute string // For attribute-based conditions
}

// ConditionType defines types of conditions
type ConditionType string

const (
	ConditionQueueDepth        ConditionType = "QUEUE_DEPTH"
	ConditionFlowFileCount     ConditionType = "FLOWFILE_COUNT"
	ConditionAttribute         ConditionType = "ATTRIBUTE"
	ConditionTimeOfDay         ConditionType = "TIME_OF_DAY"
	ConditionDayOfWeek         ConditionType = "DAY_OF_WEEK"
	ConditionSystemLoad        ConditionType = "SYSTEM_LOAD"
	ConditionUpstreamState     ConditionType = "UPSTREAM_STATE"
	ConditionDownstreamState   ConditionType = "DOWNSTREAM_STATE"
)

// Operator defines comparison operators
type Operator string

const (
	OperatorEquals            Operator = "EQUALS"
	OperatorNotEquals         Operator = "NOT_EQUALS"
	OperatorGreaterThan       Operator = "GREATER_THAN"
	OperatorLessThan          Operator = "LESS_THAN"
	OperatorGreaterThanEquals Operator = "GREATER_THAN_EQUALS"
	OperatorLessThanEquals    Operator = "LESS_THAN_EQUALS"
	OperatorContains          Operator = "CONTAINS"
	OperatorMatches           Operator = "MATCHES" // Regex
)

// RuleAction defines what happens when rule conditions are met
type RuleAction string

const (
	ActionExecute         RuleAction = "EXECUTE"
	ActionSkip            RuleAction = "SKIP"
	ActionDelay           RuleAction = "DELAY"
	ActionChangePriority  RuleAction = "CHANGE_PRIORITY"
)

// EvaluateMode defines how multiple conditions are evaluated
type EvaluateMode string

const (
	EvaluateModeAll  EvaluateMode = "ALL"  // All conditions must be true
	EvaluateModeAny  EvaluateMode = "ANY"  // Any condition must be true
	EvaluateModeNone EvaluateMode = "NONE" // No conditions should be true
)

// RuleEvaluator evaluates execution rules
type RuleEvaluator struct {
	flowController *FlowController
}

// NewLoadBasedScheduler creates a new load-based scheduler
func NewLoadBasedScheduler() *LoadBasedScheduler {
	return &LoadBasedScheduler{
		enabled:            true,
		cpuThreshold:       0.80,  // 80% CPU threshold
		memoryThreshold:    0.85,  // 85% memory threshold
		goroutineThreshold: 10000, // Max goroutines
		checkInterval:      1 * time.Second,
		lastCheck:          time.Now(),
		loadHistory:        make([]SystemLoad, 0, 60), // Keep 60 samples
		maxHistorySize:     60,
	}
}

// CheckLoad determines if system load allows execution
func (lbs *LoadBasedScheduler) CheckLoad(workerPool *WorkerPool) bool {
	lbs.mu.Lock()
	defer lbs.mu.Unlock()

	if !lbs.enabled {
		return true
	}

	// Check if we need to update load metrics
	if time.Since(lbs.lastCheck) < lbs.checkInterval {
		return lbs.isLoadAcceptable()
	}

	// Update load metrics
	lbs.updateLoad(workerPool)
	lbs.lastCheck = time.Now()

	return lbs.isLoadAcceptable()
}

// updateLoad collects current system load metrics
func (lbs *LoadBasedScheduler) updateLoad(workerPool *WorkerPool) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	load := SystemLoad{
		MemoryPercent:  float64(memStats.Alloc) / float64(memStats.Sys),
		GoroutineCount: runtime.NumGoroutine(),
		Timestamp:      time.Now(),
	}

	if workerPool != nil {
		workerPool.mu.RLock()
		load.ActiveWorkers = workerPool.activeWorkers
		load.QueueDepth = len(workerPool.workers)
		workerPool.mu.RUnlock()
	}

	// Simple CPU estimation based on goroutine count and worker utilization
	if workerPool != nil {
		utilizationRatio := float64(load.ActiveWorkers) / float64(workerPool.workerCount)
		load.CPUPercent = utilizationRatio * 0.5 // Rough estimation
	}

	lbs.currentLoad = load

	// Add to history
	lbs.loadHistory = append(lbs.loadHistory, load)
	if len(lbs.loadHistory) > lbs.maxHistorySize {
		lbs.loadHistory = lbs.loadHistory[1:]
	}
}

// isLoadAcceptable checks if current load is within acceptable thresholds
func (lbs *LoadBasedScheduler) isLoadAcceptable() bool {
	load := lbs.currentLoad

	if load.CPUPercent > lbs.cpuThreshold {
		return false
	}

	if load.MemoryPercent > lbs.memoryThreshold {
		return false
	}

	if load.GoroutineCount > lbs.goroutineThreshold {
		return false
	}

	return true
}

// GetAverageLoad returns average load over recent history
func (lbs *LoadBasedScheduler) GetAverageLoad() SystemLoad {
	lbs.mu.RLock()
	defer lbs.mu.RUnlock()

	if len(lbs.loadHistory) == 0 {
		return lbs.currentLoad
	}

	var avgCPU, avgMem float64
	var avgGoroutines int

	for _, load := range lbs.loadHistory {
		avgCPU += load.CPUPercent
		avgMem += load.MemoryPercent
		avgGoroutines += load.GoroutineCount
	}

	count := float64(len(lbs.loadHistory))
	return SystemLoad{
		CPUPercent:     avgCPU / count,
		MemoryPercent:  avgMem / count,
		GoroutineCount: int(float64(avgGoroutines) / count),
		Timestamp:      time.Now(),
	}
}

// SetThresholds configures load thresholds
func (lbs *LoadBasedScheduler) SetThresholds(cpu, memory float64, goroutines int) {
	lbs.mu.Lock()
	defer lbs.mu.Unlock()

	lbs.cpuThreshold = cpu
	lbs.memoryThreshold = memory
	lbs.goroutineThreshold = goroutines
}

// NewPriorityScheduler creates a new priority-based scheduler
func NewPriorityScheduler() *PriorityScheduler {
	ps := &PriorityScheduler{
		priorityQueues: make(map[ProcessorPriority]*PriorityQueue),
		enabled:        true,
	}

	// Initialize priority queues
	for priority := PriorityIdle; priority <= PriorityCritical; priority++ {
		ps.priorityQueues[priority] = &PriorityQueue{
			tasks:   make([]*PriorityTask, 0),
			maxSize: 1000, // Max 1000 tasks per priority level
		}
	}

	return ps
}

// Enqueue adds a task to the appropriate priority queue
func (ps *PriorityScheduler) Enqueue(task *ScheduledTask, priority ProcessorPriority) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.enabled {
		return nil
	}

	queue, exists := ps.priorityQueues[priority]
	if !exists {
		return fmt.Errorf("invalid priority level: %d", priority)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.tasks) >= queue.maxSize {
		return fmt.Errorf("priority queue full for priority %d", priority)
	}

	priorityTask := &PriorityTask{
		Task:      task,
		Priority:  priority,
		QueueTime: time.Now(),
	}

	queue.tasks = append(queue.tasks, priorityTask)
	return nil
}

// Dequeue retrieves the highest priority task
func (ps *PriorityScheduler) Dequeue() *PriorityTask {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.enabled {
		return nil
	}

	// Check queues from highest to lowest priority
	for priority := PriorityCritical; priority >= PriorityIdle; priority-- {
		queue := ps.priorityQueues[priority]
		queue.mu.Lock()

		if len(queue.tasks) > 0 {
			task := queue.tasks[0]
			queue.tasks = queue.tasks[1:]
			queue.mu.Unlock()
			return task
		}

		queue.mu.Unlock()
	}

	return nil
}

// GetQueueDepths returns the depth of each priority queue
func (ps *PriorityScheduler) GetQueueDepths() map[ProcessorPriority]int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	depths := make(map[ProcessorPriority]int)
	for priority, queue := range ps.priorityQueues {
		queue.mu.RLock()
		depths[priority] = len(queue.tasks)
		queue.mu.RUnlock()
	}

	return depths
}

// NewConditionalScheduler creates a new conditional scheduler
func NewConditionalScheduler(flowController *FlowController) *ConditionalScheduler {
	return &ConditionalScheduler{
		rules: make(map[uuid.UUID]*ExecutionRule),
		evaluator: &RuleEvaluator{
			flowController: flowController,
		},
		enabled: true,
	}
}

// AddRule adds an execution rule
func (cs *ConditionalScheduler) AddRule(rule *ExecutionRule) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}

	cs.rules[rule.ID] = rule
	return nil
}

// RemoveRule removes an execution rule
func (cs *ConditionalScheduler) RemoveRule(ruleID uuid.UUID) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	delete(cs.rules, ruleID)
}

// ShouldExecute evaluates if a processor should execute based on rules
func (cs *ConditionalScheduler) ShouldExecute(processorID uuid.UUID, task *ScheduledTask) (bool, RuleAction) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if !cs.enabled {
		return true, ActionExecute
	}

	// Find rules for this processor
	var applicableRules []*ExecutionRule
	for _, rule := range cs.rules {
		if rule.ProcessorID == processorID && rule.Enabled {
			applicableRules = append(applicableRules, rule)
		}
	}

	if len(applicableRules) == 0 {
		return true, ActionExecute // No rules, allow execution
	}

	// Evaluate rules (use first matching rule for now)
	for _, rule := range applicableRules {
		if cs.evaluator.Evaluate(rule, task) {
			switch rule.Action {
			case ActionSkip:
				return false, ActionSkip
			case ActionDelay:
				return false, ActionDelay
			case ActionExecute:
				return true, ActionExecute
			default:
				return true, rule.Action
			}
		}
	}

	return true, ActionExecute
}

// Evaluate evaluates a rule against current state
func (re *RuleEvaluator) Evaluate(rule *ExecutionRule, task *ScheduledTask) bool {
	if len(rule.Conditions) == 0 {
		return true
	}

	results := make([]bool, len(rule.Conditions))

	for i, condition := range rule.Conditions {
		results[i] = re.evaluateCondition(condition, task)
	}

	// Apply evaluation mode
	switch rule.EvaluateMode {
	case EvaluateModeAll:
		for _, result := range results {
			if !result {
				return false
			}
		}
		return true

	case EvaluateModeAny:
		for _, result := range results {
			if result {
				return true
			}
		}
		return false

	case EvaluateModeNone:
		for _, result := range results {
			if result {
				return false
			}
		}
		return true

	default:
		return false
	}
}

// evaluateCondition evaluates a single condition
func (re *RuleEvaluator) evaluateCondition(condition Condition, task *ScheduledTask) bool {
	switch condition.Type {
	case ConditionQueueDepth:
		return re.evaluateQueueDepth(condition, task)
	case ConditionFlowFileCount:
		return re.evaluateFlowFileCount(condition, task)
	case ConditionTimeOfDay:
		return re.evaluateTimeOfDay(condition)
	case ConditionDayOfWeek:
		return re.evaluateDayOfWeek(condition)
	default:
		return true // Unknown condition type, default to true
	}
}

func (re *RuleEvaluator) evaluateQueueDepth(condition Condition, task *ScheduledTask) bool {
	if task == nil || task.ProcessorNode == nil {
		return false
	}

	// Get total queue depth for processor's input queues
	var totalDepth int64
	for _, conn := range task.ProcessorNode.Connections {
		conn.RLock()
		totalDepth += conn.Queue.Size()
		conn.RUnlock()
	}

	return re.compareValues(totalDepth, condition.Operator, condition.Value)
}

func (re *RuleEvaluator) evaluateFlowFileCount(condition Condition, task *ScheduledTask) bool {
	if task == nil || task.ProcessorNode == nil {
		return false
	}

	task.ProcessorNode.mu.RLock()
	count := task.ProcessorNode.Status.FlowFilesIn
	task.ProcessorNode.mu.RUnlock()

	return re.compareValues(count, condition.Operator, condition.Value)
}

func (re *RuleEvaluator) evaluateTimeOfDay(condition Condition) bool {
	now := time.Now()
	hour := now.Hour()
	return re.compareValues(hour, condition.Operator, condition.Value)
}

func (re *RuleEvaluator) evaluateDayOfWeek(condition Condition) bool {
	now := time.Now()
	dayOfWeek := int(now.Weekday())
	return re.compareValues(dayOfWeek, condition.Operator, condition.Value)
}

func (re *RuleEvaluator) compareValues(actual interface{}, operator Operator, expected interface{}) bool {
	switch operator {
	case OperatorEquals:
		return actual == expected
	case OperatorNotEquals:
		return actual != expected
	case OperatorGreaterThan:
		return re.compareNumeric(actual, expected, func(a, b float64) bool { return a > b })
	case OperatorLessThan:
		return re.compareNumeric(actual, expected, func(a, b float64) bool { return a < b })
	case OperatorGreaterThanEquals:
		return re.compareNumeric(actual, expected, func(a, b float64) bool { return a >= b })
	case OperatorLessThanEquals:
		return re.compareNumeric(actual, expected, func(a, b float64) bool { return a <= b })
	default:
		return false
	}
}

func (re *RuleEvaluator) compareNumeric(actual, expected interface{}, compareFn func(float64, float64) bool) bool {
	var actualVal, expectedVal float64

	switch v := actual.(type) {
	case int:
		actualVal = float64(v)
	case int64:
		actualVal = float64(v)
	case float64:
		actualVal = v
	default:
		return false
	}

	switch v := expected.(type) {
	case int:
		expectedVal = float64(v)
	case int64:
		expectedVal = float64(v)
	case float64:
		expectedVal = v
	default:
		return false
	}

	return compareFn(actualVal, expectedVal)
}

// GetRules returns all rules for a processor
func (cs *ConditionalScheduler) GetRules(processorID uuid.UUID) []*ExecutionRule {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var rules []*ExecutionRule
	for _, rule := range cs.rules {
		if rule.ProcessorID == processorID {
			rules = append(rules, rule)
		}
	}

	return rules
}
