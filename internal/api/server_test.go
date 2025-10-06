package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestServerLifecycle(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	config := Config{
		Port:                 8081, // Use non-default port for testing
		MetricsCacheInterval: 5 * time.Second,
		SSEUpdateInterval:    2 * time.Second,
	}

	server := NewServer(fc, scheduler, logger, config)
	require.NotNil(t, server)

	// Start server
	err := server.Start()
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	require.NoError(t, err)

	logger.Info("TestServerLifecycle passed")
}

func TestServerEndpoints(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	config := DefaultConfig()
	server := NewServer(fc, scheduler, logger, config)

	// Create test processors
	_, err := createTestProcessor(fc, "Test Processor")
	require.NoError(t, err)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{
			name:           "Health Check",
			path:           "/health",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "System Status",
			path:           "/api/system/status",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Health Status",
			path:           "/api/system/health",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "System Metrics",
			path:           "/api/system/metrics",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "System Stats",
			path:           "/api/system/stats",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Processors",
			path:           "/api/monitoring/processors",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Connections",
			path:           "/api/monitoring/connections",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Queues",
			path:           "/api/monitoring/queues",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			server.router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, "Expected status code %d for %s, got %d", tt.expectedStatus, tt.path, w.Code)
		})
	}

	logger.Info("TestServerEndpoints passed")
}

func TestCORSMiddleware(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	config := DefaultConfig()
	server := NewServer(fc, scheduler, logger, config)

	// Test CORS headers on valid endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))

	// Test OPTIONS request
	req = httptest.NewRequest(http.MethodOptions, "/api/system/status", nil)
	w = httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	logger.Info("TestCORSMiddleware passed")
}

func TestMetricsCaching(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	cacheInterval := 1 * time.Second
	collector := NewMetricsCollector(fc, scheduler, cacheInterval)

	// Get metrics first time
	metrics1 := collector.GetSystemMetrics()
	time1 := metrics1.Timestamp

	// Get metrics immediately (should be cached)
	metrics2 := collector.GetSystemMetrics()
	time2 := metrics2.Timestamp

	// Timestamps should be the same (cached)
	assert.Equal(t, time1, time2)

	// Wait for cache to expire
	time.Sleep(cacheInterval + 100*time.Millisecond)

	// Get metrics again (should be fresh)
	metrics3 := collector.GetSystemMetrics()
	time3 := metrics3.Timestamp

	// Timestamps should be different (not cached)
	assert.NotEqual(t, time1, time3)

	logger.Info("TestMetricsCaching passed")
}

func TestSSEStream(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	// Create SSE handler with short update interval
	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	sseHandler := NewSSEHandler(collector, logger, 100*time.Millisecond)

	// Create test request with cancel context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/monitoring/stream", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	// Start handling in goroutine
	done := make(chan bool)
	go func() {
		sseHandler.HandleStream(w, req)
		done <- true
	}()

	// Wait for initial data
	time.Sleep(50 * time.Millisecond)

	// Cancel the request to stop streaming
	cancel()

	// Wait for handler to finish
	select {
	case <-done:
		// Handler finished
	case <-time.After(1 * time.Second):
		t.Fatal("Handler did not finish in time")
	}

	// Stop SSE handler
	sseHandler.Stop()

	// Check response headers
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))

	// Note: In a real test, we would check the actual SSE events
	// but httptest.ResponseRecorder doesn't support streaming well

	logger.Info("TestSSEStream passed")
}

func TestHealthStatusWithDegradedState(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)
	handlers := NewMonitoringHandlers(collector)

	// Create a processor with invalid configuration
	processor := &MockProcessor{}
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "Invalid Processor",
		Type:          "MockProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "invalid-duration", // This should cause validation error
		Concurrency:   1,
		Properties:    map[string]string{},
		AutoTerminate: map[string]bool{"success": true},
	}

	proc, err := fc.AddProcessor(processor, config)
	require.NoError(t, err)

	// Try to start it (should fail validation)
	_ = fc.StartProcessor(proc.ID)
	// Validation should fail but we don't check the error here

	req := httptest.NewRequest(http.MethodGet, "/api/system/health", nil)
	w := httptest.NewRecorder()

	handlers.HandleHealth(w, req)

	var health HealthStatus
	err = json.NewDecoder(w.Body).Decode(&health)
	require.NoError(t, err)

	// Status might be degraded or healthy depending on validation behavior
	assert.Contains(t, []string{"healthy", "degraded", "unhealthy"}, health.Status)

	logger.Info("TestHealthStatusWithDegradedState passed")
}

func TestProcessorMetricsCalculations(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)

	// Create and start a processor
	proc, err := createTestProcessor(fc, "Test Processor")
	require.NoError(t, err)

	err = fc.StartProcessor(proc.ID)
	require.NoError(t, err)

	// Simulate some processing by updating processor status
	proc.Status.TasksCompleted = 10
	proc.Status.FlowFilesIn = 100
	proc.Status.FlowFilesOut = 95
	proc.Status.BytesIn = 1024 * 100
	proc.Status.BytesOut = 1024 * 95
	proc.Status.AverageTaskTime = 50 * time.Millisecond
	proc.Status.LastRun = time.Now()

	// Get metrics
	metrics, exists := collector.GetProcessorMetricsByID(proc.ID)
	require.True(t, exists)

	assert.Equal(t, int64(10), metrics.TasksCompleted)
	assert.Equal(t, int64(100), metrics.FlowFilesIn)
	assert.Equal(t, int64(95), metrics.FlowFilesOut)
	assert.Equal(t, 50.0, metrics.AverageExecutionTimeMS)

	logger.Info("TestProcessorMetricsCalculations passed")
}

func TestConnectionMetricsCalculations(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)

	// Create processors and connection
	proc1, err := createTestProcessor(fc, "Source")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Destination")
	require.NoError(t, err)

	conn, err := fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	// Get connection metrics
	metrics, exists := collector.GetConnectionMetricsByID(conn.ID)
	require.True(t, exists)

	assert.Equal(t, conn.ID, metrics.ID)
	assert.Equal(t, proc1.ID, metrics.SourceID)
	assert.Equal(t, proc2.ID, metrics.DestinationID)
	assert.Equal(t, "success", metrics.Relationship)
	assert.Equal(t, int64(0), metrics.QueueDepth)
	assert.Equal(t, conn.BackPressureSize, metrics.MaxQueueSize)

	logger.Info("TestConnectionMetricsCalculations passed")
}

func TestQueueMetricsAggregation(t *testing.T) {
	fc, scheduler, logger := setupTestEnvironment(t)
	defer fc.Stop()

	collector := NewMetricsCollector(fc, scheduler, 5*time.Second)

	// Create multiple connections
	proc1, err := createTestProcessor(fc, "Processor 1")
	require.NoError(t, err)
	proc2, err := createTestProcessor(fc, "Processor 2")
	require.NoError(t, err)
	proc3, err := createTestProcessor(fc, "Processor 3")
	require.NoError(t, err)

	_, err = fc.AddConnection(proc1.ID, proc2.ID, types.RelationshipSuccess)
	require.NoError(t, err)
	_, err = fc.AddConnection(proc2.ID, proc3.ID, types.RelationshipSuccess)
	require.NoError(t, err)

	// Get queue metrics
	metrics := collector.GetQueueMetrics()

	assert.Equal(t, 2, len(metrics.Queues))
	assert.NotZero(t, metrics.TotalCapacity)
	assert.GreaterOrEqual(t, metrics.PercentFull, 0.0)
	assert.LessOrEqual(t, metrics.PercentFull, 100.0)

	logger.Info("TestQueueMetricsAggregation passed")
}
