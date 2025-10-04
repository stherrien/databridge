package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawntherrien/databridge/internal/api/models"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/pkg/types"
)

// TestHelper provides common test utilities
type TestHelper struct {
	flowController *core.FlowController
	router         *gin.Engine
	logger         *logrus.Logger
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T) *TestHelper {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// Create in-memory repositories for testing
	flowFileRepo := core.NewInMemoryFlowFileRepository()
	contentRepo := core.NewInMemoryContentRepository()
	provenanceRepo := core.NewInMemoryProvenanceRepository()

	// Create flow controller
	flowController := core.NewFlowController(
		flowFileRepo,
		contentRepo,
		provenanceRepo,
		logger,
	)

	// Start flow controller
	err := flowController.Start()
	require.NoError(t, err)

	// Create router
	router := gin.New()

	helper := &TestHelper{
		flowController: flowController,
		router:         router,
		logger:         logger,
	}

	helper.setupRoutes()

	return helper
}

// setupRoutes sets up test routes
func (h *TestHelper) setupRoutes() {
	flowHandler := NewFlowHandler(h.flowController)
	processorHandler := NewProcessorHandler(h.flowController)
	connectionHandler := NewConnectionHandler(h.flowController)

	api := h.router.Group("/api")
	{
		// Flow routes
		flows := api.Group("/flows")
		{
			flows.GET("", flowHandler.ListFlows)
			flows.POST("", flowHandler.CreateFlow)
			flows.GET("/:id", flowHandler.GetFlow)
			flows.PUT("/:id", flowHandler.UpdateFlow)
			flows.DELETE("/:id", flowHandler.DeleteFlow)
			flows.GET("/:id/status", flowHandler.GetFlowStatus)
		}

		// Processor routes
		processors := api.Group("/processors")
		{
			processors.GET("", processorHandler.ListProcessors)
			processors.GET("/:id", processorHandler.GetProcessor)
			processors.PUT("/:id", processorHandler.UpdateProcessor)
			processors.DELETE("/:id", processorHandler.DeleteProcessor)
			processors.PUT("/:id/start", processorHandler.StartProcessor)
			processors.PUT("/:id/stop", processorHandler.StopProcessor)
			processors.GET("/:id/status", processorHandler.GetProcessorStatus)
		}

		// Connection routes
		connections := api.Group("/connections")
		{
			connections.GET("", connectionHandler.ListConnections)
			connections.POST("", connectionHandler.CreateConnection)
			connections.GET("/:id", connectionHandler.GetConnection)
			connections.PUT("/:id", connectionHandler.UpdateConnection)
			connections.DELETE("/:id", connectionHandler.DeleteConnection)
		}
	}
}

// Cleanup stops the flow controller
func (h *TestHelper) Cleanup() {
	h.flowController.Stop()
}

// makeRequest makes an HTTP request and returns the response
func (h *TestHelper) makeRequest(method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)

	return w
}

// TestFlowHandlers tests flow-related endpoints
func TestFlowHandlers(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	t.Run("ListFlows", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/flows", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response models.ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response.Items)
	})

	t.Run("CreateFlow", func(t *testing.T) {
		req := models.CreateProcessGroupRequest{
			Name: "Test Flow",
		}

		w := helper.makeRequest("POST", "/api/flows", req)
		assert.Equal(t, http.StatusCreated, w.Code)

		var flow models.ProcessGroupDTO
		err := json.Unmarshal(w.Body.Bytes(), &flow)
		require.NoError(t, err)
		assert.Equal(t, "Test Flow", flow.Name)
		assert.NotEqual(t, uuid.Nil, flow.ID)
	})

	t.Run("GetFlow", func(t *testing.T) {
		// Create a flow first
		flow, err := helper.flowController.CreateProcessGroup("Test Flow", nil)
		require.NoError(t, err)

		// Get the flow
		w := helper.makeRequest("GET", "/api/flows/"+flow.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var responseFlow models.ProcessGroupDTO
		err = json.Unmarshal(w.Body.Bytes(), &responseFlow)
		require.NoError(t, err)
		assert.Equal(t, flow.ID, responseFlow.ID)
		assert.Equal(t, "Test Flow", responseFlow.Name)
	})

	t.Run("GetFlow_NotFound", func(t *testing.T) {
		randomID := uuid.New()
		w := helper.makeRequest("GET", "/api/flows/"+randomID.String(), nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("UpdateFlow", func(t *testing.T) {
		// Create a flow first
		flow, err := helper.flowController.CreateProcessGroup("Test Flow", nil)
		require.NoError(t, err)

		// Update the flow
		req := models.UpdateProcessGroupRequest{
			Name: "Updated Flow",
		}

		w := helper.makeRequest("PUT", "/api/flows/"+flow.ID.String(), req)
		assert.Equal(t, http.StatusOK, w.Code)

		var updatedFlow models.ProcessGroupDTO
		err = json.Unmarshal(w.Body.Bytes(), &updatedFlow)
		require.NoError(t, err)
		assert.Equal(t, "Updated Flow", updatedFlow.Name)
	})

	t.Run("DeleteFlow", func(t *testing.T) {
		// Create a parent flow first (to avoid root group deletion restriction)
		parentFlow, err := helper.flowController.CreateProcessGroup("Parent Flow", nil)
		require.NoError(t, err)

		// Create a child flow
		childFlow, err := helper.flowController.CreateProcessGroup("Child Flow", &parentFlow.ID)
		require.NoError(t, err)

		// Delete the child flow (non-root, no processors)
		w := helper.makeRequest("DELETE", "/api/flows/"+childFlow.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify it's deleted
		_, exists := helper.flowController.GetProcessGroup(childFlow.ID)
		assert.False(t, exists)
	})

	t.Run("GetFlowStatus", func(t *testing.T) {
		// Create a flow first
		flow, err := helper.flowController.CreateProcessGroup("Test Flow", nil)
		require.NoError(t, err)

		// Get status
		w := helper.makeRequest("GET", "/api/flows/"+flow.ID.String()+"/status", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var status models.ProcessGroupStatusDTO
		err = json.Unmarshal(w.Body.Bytes(), &status)
		require.NoError(t, err)
		assert.Equal(t, flow.ID, status.ID)
	})
}

// TestProcessorHandlers tests processor-related endpoints
func TestProcessorHandlers(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Create a test processor
	testProcessor := &TestProcessor{}
	config := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "Test Processor",
		Type:          "TestProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
		Properties:    map[string]string{},
		AutoTerminate: map[string]bool{},
	}

	processor, err := helper.flowController.AddProcessor(testProcessor, config)
	require.NoError(t, err)

	t.Run("ListProcessors", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/processors", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response models.ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response.Items)
	})

	t.Run("GetProcessor", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/processors/"+processor.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var processorDTO models.ProcessorDTO
		err := json.Unmarshal(w.Body.Bytes(), &processorDTO)
		require.NoError(t, err)
		assert.Equal(t, processor.ID, processorDTO.ID)
		assert.Equal(t, "Test Processor", processorDTO.Name)
	})

	t.Run("UpdateProcessor", func(t *testing.T) {
		newName := "Updated Processor"
		req := models.UpdateProcessorRequest{
			Name: &newName,
		}

		w := helper.makeRequest("PUT", "/api/processors/"+processor.ID.String(), req)
		assert.Equal(t, http.StatusOK, w.Code)

		var updated models.ProcessorDTO
		err := json.Unmarshal(w.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, newName, updated.Name)
	})

	t.Run("StartProcessor", func(t *testing.T) {
		w := helper.makeRequest("PUT", "/api/processors/"+processor.ID.String()+"/start", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify processor is running
		p, _ := helper.flowController.GetProcessor(processor.ID)
		assert.Equal(t, types.ProcessorStateRunning, p.Status.State)
	})

	t.Run("StopProcessor", func(t *testing.T) {
		w := helper.makeRequest("PUT", "/api/processors/"+processor.ID.String()+"/stop", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify processor is stopped
		p, _ := helper.flowController.GetProcessor(processor.ID)
		assert.Equal(t, types.ProcessorStateStopped, p.Status.State)
	})

	t.Run("GetProcessorStatus", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/processors/"+processor.ID.String()+"/status", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var status models.ProcessorStatusDTO
		err := json.Unmarshal(w.Body.Bytes(), &status)
		require.NoError(t, err)
		assert.Equal(t, processor.ID, status.ID)
	})

	t.Run("DeleteProcessor", func(t *testing.T) {
		w := helper.makeRequest("DELETE", "/api/processors/"+processor.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify it's deleted
		_, exists := helper.flowController.GetProcessor(processor.ID)
		assert.False(t, exists)
	})
}

// TestConnectionHandlers tests connection-related endpoints
func TestConnectionHandlers(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Create two test processors
	testProcessor1 := &TestProcessor{}
	config1 := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "Source Processor",
		Type:          "TestProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
		Properties:    map[string]string{},
		AutoTerminate: map[string]bool{},
	}

	processor1, err := helper.flowController.AddProcessor(testProcessor1, config1)
	require.NoError(t, err)

	testProcessor2 := &TestProcessor{}
	config2 := types.ProcessorConfig{
		ID:            uuid.New(),
		Name:          "Destination Processor",
		Type:          "TestProcessor",
		ScheduleType:  types.ScheduleTypeTimer,
		ScheduleValue: "1s",
		Concurrency:   1,
		Properties:    map[string]string{},
		AutoTerminate: map[string]bool{},
	}

	processor2, err := helper.flowController.AddProcessor(testProcessor2, config2)
	require.NoError(t, err)

	t.Run("CreateConnection", func(t *testing.T) {
		req := models.CreateConnectionRequest{
			Name:          "Test Connection",
			SourceID:      processor1.ID,
			DestinationID: processor2.ID,
			Relationship:  "success",
		}

		w := helper.makeRequest("POST", "/api/connections", req)
		assert.Equal(t, http.StatusCreated, w.Code)

		var connection models.ConnectionDTO
		err := json.Unmarshal(w.Body.Bytes(), &connection)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, connection.ID)
		assert.Equal(t, processor1.ID, connection.SourceID)
		assert.Equal(t, processor2.ID, connection.DestinationID)
	})

	t.Run("ListConnections", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/connections", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response models.ListResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotNil(t, response.Items)
	})

	// Create a connection for remaining tests
	connection, err := helper.flowController.AddConnection(
		processor1.ID,
		processor2.ID,
		types.RelationshipSuccess,
	)
	require.NoError(t, err)

	t.Run("GetConnection", func(t *testing.T) {
		w := helper.makeRequest("GET", "/api/connections/"+connection.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var connDTO models.ConnectionDTO
		err := json.Unmarshal(w.Body.Bytes(), &connDTO)
		require.NoError(t, err)
		assert.Equal(t, connection.ID, connDTO.ID)
	})

	t.Run("UpdateConnection", func(t *testing.T) {
		newName := "Updated Connection"
		backPressure := int64(5000)
		req := models.UpdateConnectionRequest{
			Name:             &newName,
			BackPressureSize: &backPressure,
		}

		w := helper.makeRequest("PUT", "/api/connections/"+connection.ID.String(), req)
		assert.Equal(t, http.StatusOK, w.Code)

		var updated models.ConnectionDTO
		err := json.Unmarshal(w.Body.Bytes(), &updated)
		require.NoError(t, err)
		assert.Equal(t, newName, updated.Name)
		assert.Equal(t, backPressure, updated.BackPressureSize)
	})

	t.Run("DeleteConnection", func(t *testing.T) {
		w := helper.makeRequest("DELETE", "/api/connections/"+connection.ID.String(), nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify it's deleted
		_, exists := helper.flowController.GetConnection(connection.ID)
		assert.False(t, exists)
	})
}

// TestProcessor is a simple test processor implementation
type TestProcessor struct {
	types.BaseProcessor
}

func (p *TestProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

func (p *TestProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	return nil
}

func (p *TestProcessor) GetInfo() types.ProcessorInfo {
	return types.ProcessorInfo{
		Name:        "TestProcessor",
		Description: "A test processor",
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

func (p *TestProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	return []types.ValidationResult{}
}

func (p *TestProcessor) OnStopped(ctx context.Context) {}
