package plugin

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ResourceLimits restricts plugin resource usage
type ResourceLimits struct {
	MaxMemoryMB    int64   `json:"maxMemoryMB"`
	MaxCPUPercent  float64 `json:"maxCpuPercent"`
	MaxGoroutines  int     `json:"maxGoroutines"`
	MaxFileHandles int     `json:"maxFileHandles"`
	MaxExecutionMS int64   `json:"maxExecutionMs"`
}

// DefaultResourceLimits returns default resource limits
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxMemoryMB:    512,    // 512MB default
		MaxCPUPercent:  50.0,   // 50% CPU
		MaxGoroutines:  100,    // 100 goroutines
		MaxFileHandles: 100,    // 100 file handles
		MaxExecutionMS: 30000,  // 30 seconds
	}
}

// PluginNamespace isolates plugin resources
type PluginNamespace struct {
	ID             string
	Name           string
	Processors     map[string]types.Processor
	Config         map[string]interface{}
	ResourceLimits ResourceLimits
	metrics        *ResourceMetrics
	mu             sync.RWMutex
	logger         *logrus.Logger
}

// ResourceMetrics tracks resource usage
type ResourceMetrics struct {
	MemoryUsageMB     int64
	GoroutineCount    int32
	FileHandleCount   int32
	ExecutionTimeMS   int64
	CPUUsagePercent   float64
	ViolationCount    int64
	LastViolationTime time.Time
	mu                sync.RWMutex
}

// NamespaceManager manages plugin namespaces
type NamespaceManager struct {
	namespaces map[string]*PluginNamespace
	mu         sync.RWMutex
	logger     *logrus.Logger
}

// NewNamespaceManager creates a new namespace manager
func NewNamespaceManager(logger *logrus.Logger) *NamespaceManager {
	return &NamespaceManager{
		namespaces: make(map[string]*PluginNamespace),
		logger:     logger,
	}
}

// CreateNamespace creates a new plugin namespace
func (nm *NamespaceManager) CreateNamespace(id, name string, limits ResourceLimits) (*PluginNamespace, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if _, exists := nm.namespaces[id]; exists {
		return nil, fmt.Errorf("namespace %s already exists", id)
	}

	ns := &PluginNamespace{
		ID:             id,
		Name:           name,
		Processors:     make(map[string]types.Processor),
		Config:         make(map[string]interface{}),
		ResourceLimits: limits,
		metrics:        &ResourceMetrics{},
		logger:         nm.logger,
	}

	nm.namespaces[id] = ns

	nm.logger.WithFields(logrus.Fields{
		"namespaceId":   id,
		"namespaceName": name,
	}).Info("Created plugin namespace")

	return ns, nil
}

// DeleteNamespace removes a namespace
func (nm *NamespaceManager) DeleteNamespace(id string) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if _, exists := nm.namespaces[id]; !exists {
		return fmt.Errorf("namespace %s not found", id)
	}

	delete(nm.namespaces, id)

	nm.logger.WithField("namespaceId", id).Info("Deleted plugin namespace")
	return nil
}

// GetNamespace retrieves a namespace
func (nm *NamespaceManager) GetNamespace(id string) (*PluginNamespace, error) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	ns, exists := nm.namespaces[id]
	if !exists {
		return nil, fmt.Errorf("namespace %s not found", id)
	}

	return ns, nil
}

// ListNamespaces returns all namespaces
func (nm *NamespaceManager) ListNamespaces() []*PluginNamespace {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	namespaces := make([]*PluginNamespace, 0, len(nm.namespaces))
	for _, ns := range nm.namespaces {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

// PluginNamespace methods

// AddProcessor adds a processor to the namespace
func (ns *PluginNamespace) AddProcessor(name string, processor types.Processor) error {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if _, exists := ns.Processors[name]; exists {
		return fmt.Errorf("processor %s already exists in namespace", name)
	}

	ns.Processors[name] = processor
	return nil
}

// RemoveProcessor removes a processor from the namespace
func (ns *PluginNamespace) RemoveProcessor(name string) error {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if _, exists := ns.Processors[name]; !exists {
		return fmt.Errorf("processor %s not found in namespace", name)
	}

	delete(ns.Processors, name)
	return nil
}

// GetProcessor retrieves a processor from the namespace
func (ns *PluginNamespace) GetProcessor(name string) (types.Processor, error) {
	ns.mu.RLock()
	defer ns.mu.RUnlock()

	processor, exists := ns.Processors[name]
	if !exists {
		return nil, fmt.Errorf("processor %s not found in namespace", name)
	}

	return processor, nil
}

// SetConfig sets a configuration value
func (ns *PluginNamespace) SetConfig(key string, value interface{}) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.Config[key] = value
}

// GetConfig retrieves a configuration value
func (ns *PluginNamespace) GetConfig(key string) (interface{}, bool) {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	value, exists := ns.Config[key]
	return value, exists
}

// GetMetrics returns current resource metrics
func (ns *PluginNamespace) GetMetrics() ResourceMetrics {
	ns.metrics.mu.RLock()
	defer ns.metrics.mu.RUnlock()

	return *ns.metrics
}

// CheckResourceLimits checks if resource limits are exceeded
func (ns *PluginNamespace) CheckResourceLimits() []string {
	var violations []string

	metrics := ns.GetMetrics()

	// Check memory
	if ns.ResourceLimits.MaxMemoryMB > 0 && metrics.MemoryUsageMB > ns.ResourceLimits.MaxMemoryMB {
		violations = append(violations, fmt.Sprintf(
			"Memory limit exceeded: %d MB (limit: %d MB)",
			metrics.MemoryUsageMB, ns.ResourceLimits.MaxMemoryMB,
		))
	}

	// Check goroutines
	if ns.ResourceLimits.MaxGoroutines > 0 && int(metrics.GoroutineCount) > ns.ResourceLimits.MaxGoroutines {
		violations = append(violations, fmt.Sprintf(
			"Goroutine limit exceeded: %d (limit: %d)",
			metrics.GoroutineCount, ns.ResourceLimits.MaxGoroutines,
		))
	}

	// Check CPU
	if ns.ResourceLimits.MaxCPUPercent > 0 && metrics.CPUUsagePercent > ns.ResourceLimits.MaxCPUPercent {
		violations = append(violations, fmt.Sprintf(
			"CPU limit exceeded: %.2f%% (limit: %.2f%%)",
			metrics.CPUUsagePercent, ns.ResourceLimits.MaxCPUPercent,
		))
	}

	// Check file handles
	if ns.ResourceLimits.MaxFileHandles > 0 && int(metrics.FileHandleCount) > ns.ResourceLimits.MaxFileHandles {
		violations = append(violations, fmt.Sprintf(
			"File handle limit exceeded: %d (limit: %d)",
			metrics.FileHandleCount, ns.ResourceLimits.MaxFileHandles,
		))
	}

	// Check execution time
	if ns.ResourceLimits.MaxExecutionMS > 0 && metrics.ExecutionTimeMS > ns.ResourceLimits.MaxExecutionMS {
		violations = append(violations, fmt.Sprintf(
			"Execution time limit exceeded: %d ms (limit: %d ms)",
			metrics.ExecutionTimeMS, ns.ResourceLimits.MaxExecutionMS,
		))
	}

	// Record violations
	if len(violations) > 0 {
		ns.metrics.mu.Lock()
		ns.metrics.ViolationCount++
		ns.metrics.LastViolationTime = time.Now()
		ns.metrics.mu.Unlock()
	}

	return violations
}

// UpdateMetrics updates resource metrics
func (ns *PluginNamespace) UpdateMetrics() {
	ns.metrics.mu.Lock()
	defer ns.metrics.mu.Unlock()

	// Update goroutine count
	ns.metrics.GoroutineCount = int32(runtime.NumGoroutine())

	// Update memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	ns.metrics.MemoryUsageMB = int64(m.Alloc / 1024 / 1024)

	// CPU usage would require more sophisticated tracking
	// This is a simplified placeholder
	ns.metrics.CPUUsagePercent = 0.0
}

// ResourceGuard provides context-based resource tracking
type ResourceGuard struct {
	namespace   *PluginNamespace
	startTime   time.Time
	goroutines  int32
	fileHandles int32
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewResourceGuard creates a resource guard for a namespace
func (ns *PluginNamespace) NewResourceGuard(parentCtx context.Context) *ResourceGuard {
	ctx, cancel := context.WithCancel(parentCtx)

	// Apply timeout if configured
	if ns.ResourceLimits.MaxExecutionMS > 0 {
		timeout := time.Duration(ns.ResourceLimits.MaxExecutionMS) * time.Millisecond
		ctx, cancel = context.WithTimeout(parentCtx, timeout)
	}

	return &ResourceGuard{
		namespace:   ns,
		startTime:   time.Now(),
		goroutines:  int32(runtime.NumGoroutine()),
		fileHandles: 0,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Context returns the guard's context
func (rg *ResourceGuard) Context() context.Context {
	return rg.ctx
}

// Release releases the resource guard
func (rg *ResourceGuard) Release() {
	rg.cancel()

	// Update execution time metric
	executionTime := time.Since(rg.startTime).Milliseconds()
	rg.namespace.metrics.mu.Lock()
	rg.namespace.metrics.ExecutionTimeMS = executionTime
	rg.namespace.metrics.mu.Unlock()
}

// TrackGoroutine increments the goroutine counter
func (rg *ResourceGuard) TrackGoroutine() {
	atomic.AddInt32(&rg.namespace.metrics.GoroutineCount, 1)
}

// UntrackGoroutine decrements the goroutine counter
func (rg *ResourceGuard) UntrackGoroutine() {
	atomic.AddInt32(&rg.namespace.metrics.GoroutineCount, -1)
}

// TrackFileHandle increments the file handle counter
func (rg *ResourceGuard) TrackFileHandle() {
	atomic.AddInt32(&rg.namespace.metrics.FileHandleCount, 1)
}

// UntrackFileHandle decrements the file handle counter
func (rg *ResourceGuard) UntrackFileHandle() {
	atomic.AddInt32(&rg.namespace.metrics.FileHandleCount, -1)
}

// CheckLimits checks if any limits are being exceeded
func (rg *ResourceGuard) CheckLimits() error {
	violations := rg.namespace.CheckResourceLimits()
	if len(violations) > 0 {
		return fmt.Errorf("resource limits exceeded: %v", violations)
	}
	return nil
}

// ResourceMonitor monitors resource usage for all namespaces
type ResourceMonitor struct {
	manager       *NamespaceManager
	logger        *logrus.Logger
	interval      time.Duration
	stopChan      chan struct{}
	running       bool
	mu            sync.Mutex
	alertCallback func(string, []string)
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(manager *NamespaceManager, logger *logrus.Logger, interval time.Duration) *ResourceMonitor {
	return &ResourceMonitor{
		manager:  manager,
		logger:   logger,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// SetAlertCallback sets a callback for resource violations
func (rm *ResourceMonitor) SetAlertCallback(callback func(string, []string)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.alertCallback = callback
}

// Start starts monitoring
func (rm *ResourceMonitor) Start() {
	rm.mu.Lock()
	if rm.running {
		rm.mu.Unlock()
		return
	}
	rm.running = true
	rm.mu.Unlock()

	go rm.monitorLoop()
}

// Stop stops monitoring
func (rm *ResourceMonitor) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.running {
		return
	}

	rm.running = false
	close(rm.stopChan)
}

// monitorLoop is the main monitoring loop
func (rm *ResourceMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.checkAllNamespaces()
		case <-rm.stopChan:
			return
		}
	}
}

// checkAllNamespaces checks resource usage for all namespaces
func (rm *ResourceMonitor) checkAllNamespaces() {
	namespaces := rm.manager.ListNamespaces()

	for _, ns := range namespaces {
		// Update metrics
		ns.UpdateMetrics()

		// Check limits
		violations := ns.CheckResourceLimits()
		if len(violations) > 0 {
			rm.logger.WithFields(logrus.Fields{
				"namespace":  ns.Name,
				"violations": violations,
			}).Warn("Resource limit violations detected")

			// Call alert callback if set
			rm.mu.Lock()
			callback := rm.alertCallback
			rm.mu.Unlock()

			if callback != nil {
				callback(ns.ID, violations)
			}
		}
	}
}

// GetAllMetrics returns metrics for all namespaces
func (rm *ResourceMonitor) GetAllMetrics() map[string]ResourceMetrics {
	namespaces := rm.manager.ListNamespaces()
	metrics := make(map[string]ResourceMetrics)

	for _, ns := range namespaces {
		metrics[ns.ID] = ns.GetMetrics()
	}

	return metrics
}
