package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

func setupTestEnvironment(t *testing.T) (*core.FlowController, *core.ProcessScheduler, *logrus.Logger) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// Create temporary repositories
	flowFileRepo := core.NewInMemoryFlowFileRepository()
	contentRepo := core.NewInMemoryContentRepository()
	provenanceRepo := core.NewInMemoryProvenanceRepository()

	// Create FlowController
	flowController := core.NewFlowController(
		flowFileRepo,
		contentRepo,
		provenanceRepo,
		logger,
	)

	// Start FlowController
	err := flowController.Start()
	require.NoError(t, err)

	// Get scheduler (it's created by FlowController)
	scheduler := flowController.GetScheduler()

	return flowController, scheduler, logger
}

func createTestProcessor(fc *core.FlowController, name string) (*core.ProcessorNode, error) {
	processor := &MockProcessor{}
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          name,
		Type:          "MockProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
		Properties:    map[string]string{},
		AutoTerminate: map[string]bool{"success": true},
	}

	return fc.AddProcessor(processor, config)
}

func TestHandleSystemStatus(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors
	_, err := createTestProcessor(fc, "Test Processor 1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	w := httptest.NewRecorder()

	handlers.HandleSystemStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var status SystemStatus
	err = json.NewDecoder(w.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "running", status.Status)
	assert.Equal(t, 1, status.TotalProcessors)
	assert.True(t, status.FlowController.Running)
	assert.NotZero(t, status.Timestamp)

	logger.Info("TestHandleSystemStatus passed")
}

func TestHandleHealth(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	req := httptest.NewRequest(http.MethodGet, "/api/system/health", nil)
	w := httptest.NewRecorder()

	handlers.HandleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var health HealthStatus
	err := json.NewDecoder(w.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "healthy", health.Status)
	assert.NotEmpty(t, health.Components)
	assert.NotZero(t, health.Timestamp)

	logger.Info("TestHandleHealth passed")
}

func TestHandleSystemMetrics(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	req := httptest.NewRequest(http.MethodGet, "/api/system/metrics", nil)
	w := httptest.NewRecorder()

	handlers.HandleSystemMetrics(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics SystemMetrics
	err := json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.NotZero(t, metrics.Memory.AllocMB)
	assert.NotZero(t, metrics.CPU.NumCPU)
	assert.NotZero(t, metrics.Timestamp)

	logger.Info("TestHandleSystemMetrics passed")
}

func TestHandleSystemStats(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors
	proc1, err := createTestProcessor(fc, "Test Processor 1")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Test Processor 2")
	require.NoError(t, err)

	// Create connection
	_, err = fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/system/stats", nil)
	w := httptest.NewRecorder()

	handlers.HandleSystemStats(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var stats StatsSummary
	err = json.NewDecoder(w.Body).Decode(&stats)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.TotalProcessors)
	assert.Equal(t, 1, stats.TotalConnections)
	assert.Len(t, stats.ProcessorStats, 2)
	assert.Len(t, stats.ConnectionStats, 1)

	logger.Info("TestHandleSystemStats passed")
}

func TestHandleProcessors(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors
	_, err := createTestProcessor(fc, "Test Processor 1")
	require.NoError(t, err)
	_, err = createTestProcessor(fc, "Test Processor 2")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/processors", nil)
	w := httptest.NewRecorder()

	handlers.HandleProcessors(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics []ProcessorMetrics
	err = json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.Len(t, metrics, 2)
	// Just check that both processors are returned (order not guaranteed)
	names := []string{metrics[0].Name, metrics[1].Name}
	assert.Contains(t, names, "Test Processor 1")
	assert.Contains(t, names, "Test Processor 2")

	logger.Info("TestHandleProcessors passed")
}

func TestHandleProcessor(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processor
	proc, err := createTestProcessor(fc, "Test Processor")
	require.NoError(t, err)

	// Test valid processor
	router := mux.NewRouter()
	router.HandleFunc("/api/monitoring/processors/{id}", handlers.HandleProcessor)

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/processors/"+proc.ID.String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics ProcessorMetrics
	err = json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.Equal(t, proc.ID, metrics.ID)
	assert.Equal(t, "Test Processor", metrics.Name)

	// Test invalid processor ID
	req = httptest.NewRequest(http.MethodGet, "/api/monitoring/processors/invalid-id", nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test non-existent processor
	randomID := uuid.New()
	req = httptest.NewRequest(http.MethodGet, "/api/monitoring/processors/"+randomID.String(), nil)
	w = httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	logger.Info("TestHandleProcessor passed")
}

func TestHandleConnections(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors and connections
	proc1, err := createTestProcessor(fc, "Source Processor")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Destination Processor")
	require.NoError(t, err)

	_, err = fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/connections", nil)
	w := httptest.NewRecorder()

	handlers.HandleConnections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics []ConnectionMetrics
	err = json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.Len(t, metrics, 1)
	assert.Equal(t, "Source Processor", metrics[0].SourceName)
	assert.Equal(t, "Destination Processor", metrics[0].DestinationName)

	logger.Info("TestHandleConnections passed")
}

func TestHandleConnection(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors and connection
	proc1, err := createTestProcessor(fc, "Source Processor")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Destination Processor")
	require.NoError(t, err)

	conn, err := fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	// Test valid connection
	router := mux.NewRouter()
	router.HandleFunc("/api/monitoring/connections/{id}", handlers.HandleConnection)

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/connections/"+conn.ID.String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics ConnectionMetrics
	err = json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.Equal(t, conn.ID, metrics.ID)

	logger.Info("TestHandleConnection passed")
}

func TestHandleQueues(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create test processors and connections
	proc1, err := createTestProcessor(fc, "Processor 1")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Processor 2")
	require.NoError(t, err)

	_, err = fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/queues", nil)
	w := httptest.NewRecorder()

	handlers.HandleQueues(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metrics QueueMetrics
	err = json.NewDecoder(w.Body).Decode(&metrics)
	require.NoError(t, err)

	assert.NotZero(t, metrics.TotalCapacity)
	assert.Len(t, metrics.Queues, 1)

	logger.Info("TestHandleQueues passed")
}

// MockProcessor is a simple mock processor for testing
type MockProcessor struct{}

func (p *MockProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

func (p *MockProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	// Simple mock implementation
	return nil
}

func (p *MockProcessor) GetInfo() types.ProcessorInfo {
	return types.ProcessorInfo{
		Name:        "MockProcessor",
		Description: "Mock processor for testing",
		Version:     "1.0.0",
		Author:      "Test",
		Tags:        []string{"test"},
		Properties:  []types.PropertySpec{},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}
}

func (p *MockProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	return []types.ValidationResult{}
}

func (p *MockProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed
}
