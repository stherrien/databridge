package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/internal/plugin"
	_ "github.com/shawntherrien/databridge/plugins" // Import to register built-in processors
	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestFlowController(t *testing.T) *core.FlowController {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	flowFileRepo := core.NewInMemoryFlowFileRepository()
	contentRepo := core.NewInMemoryContentRepository()
	provenanceRepo := core.NewInMemoryProvenanceRepository()

	pluginConfig := plugin.PluginManagerConfig{
		PluginDir:       "",
		AutoLoad:        false,
		MonitorInterval: 1 * time.Minute, // Set non-zero interval for resource monitor
	}
	pluginManager, err := plugin.NewPluginManager(pluginConfig, logger)
	if err != nil {
		t.Fatalf("Failed to create plugin manager: %v", err)
	}

	// Initialize plugin manager to register built-in processors
	if err := pluginManager.Initialize(); err != nil {
		t.Fatalf("Failed to initialize plugin manager: %v", err)
	}

	return core.NewFlowControllerWithPlugins(
		flowFileRepo,
		contentRepo,
		provenanceRepo,
		pluginManager,
		logger,
	)
}

func TestHandleListFlows(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	// Create test flows
	flow1, err := fc.CreateProcessGroup("Test Flow 1", nil)
	require.NoError(t, err)
	flow2, err := fc.CreateProcessGroup("Test Flow 2", nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/flows", nil)
	w := httptest.NewRecorder()

	handlers.HandleListFlows(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	flows, ok := response["flows"].([]interface{})
	require.True(t, ok)
	// We expect at least 3 flows (root + 2 created)
	assert.GreaterOrEqual(t, len(flows), 2)

	// Verify flow IDs exist
	flowIDs := []string{flow1.ID.String(), flow2.ID.String()}
	foundCount := 0
	for _, f := range flows {
		flowMap := f.(map[string]interface{})
		flowID := flowMap["id"].(string)
		for _, expectedID := range flowIDs {
			if flowID == expectedID {
				foundCount++
			}
		}
	}
	assert.Equal(t, 2, foundCount)
}

func TestHandleGetFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	// Create a test flow with processors and connections
	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	// Create processors
	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "TestProcessor1",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		Position:       &types.Position{X: 100, Y: 100},
		ProcessGroupID: &flow.ID,
	}

	node1, err := fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node1)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/flows/"+flow.ID.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleGetFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, flow.ID.String(), response["id"])
	assert.Equal(t, "Test Flow", response["name"])

	processors := response["processors"].([]interface{})
	assert.GreaterOrEqual(t, len(processors), 1)
}

func TestHandleGetFlow_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	nonExistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/flows/"+nonExistentID.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleGetFlow(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetFlow_InvalidID(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodGet, "/api/flows/invalid-id", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid-id"})
	w := httptest.NewRecorder()

	handlers.HandleGetFlow(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	body := map[string]interface{}{
		"name": "New Test Flow",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handlers.HandleCreateFlow(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "New Test Flow", response["name"])
}

func TestHandleCreateFlow_MissingName(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	body := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handlers.HandleCreateFlow(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateFlow_InvalidJSON(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodPost, "/api/flows", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handlers.HandleCreateFlow(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUpdateFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Original Name", nil)
	require.NoError(t, err)

	body := map[string]interface{}{
		"name": "Updated Name",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/flows/"+flow.ID.String(), bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleUpdateFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "Updated Name", response["name"])
}

func TestHandleUpdateFlow_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	nonExistentID := uuid.New()
	body := map[string]interface{}{
		"name": "Updated Name",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/flows/"+nonExistentID.String(), bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleUpdateFlow(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleDeleteFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	// Create a parent flow first (root cannot be deleted)
	parent, err := fc.CreateProcessGroup("Parent", nil)
	require.NoError(t, err)

	// Create child flow to delete
	flow, err := fc.CreateProcessGroup("To Delete", &parent.ID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/flows/"+flow.ID.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleDeleteFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify flow is deleted
	_, exists := fc.GetProcessGroup(flow.ID)
	assert.False(t, exists)
}

func TestHandleStartFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/start", nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleStartFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleStopFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/stop", nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleStopFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandleGetProcessorTypes(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodGet, "/api/processors/types", nil)
	w := httptest.NewRecorder()

	handlers.HandleGetProcessorTypes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	types, ok := response["types"].([]interface{})
	require.True(t, ok)
	assert.Greater(t, len(types), 0)
}

func TestHandleGetProcessorMetadata(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodGet, "/api/processors/types/GenerateFlowFile", nil)
	req = mux.SetURLVars(req, map[string]string{"type": "GenerateFlowFile"})
	w := httptest.NewRecorder()

	handlers.HandleGetProcessorMetadata(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "GenerateFlowFile", response["type"])
	assert.NotEmpty(t, response["description"])
}

func TestHandleGetProcessorMetadata_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodGet, "/api/processors/types/NonExistentProcessor", nil)
	req = mux.SetURLVars(req, map[string]string{"type": "NonExistentProcessor"})
	w := httptest.NewRecorder()

	handlers.HandleGetProcessorMetadata(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleCreateProcessor(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	body := map[string]interface{}{
		"type": "GenerateFlowFile",
		"config": map[string]interface{}{
			"name": "TestProcessor",
		},
		"position": map[string]interface{}{
			"x": 100.0,
			"y": 200.0,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/processors", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateProcessor(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "GenerateFlowFile", response["type"])
}

func TestHandleCreateProcessor_InvalidFlowID(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	body := map[string]interface{}{
		"type": "GenerateFlowFile",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/invalid-id/processors", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": "invalid-id"})
	w := httptest.NewRecorder()

	handlers.HandleCreateProcessor(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateProcessor_FlowNotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	nonExistentID := uuid.New()
	body := map[string]interface{}{
		"type": "GenerateFlowFile",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+nonExistentID.String()+"/processors", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateProcessor(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleCreateProcessor_MissingType(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	body := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/processors", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateProcessor(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleUpdateProcessor(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "OriginalName",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		Position:       &types.Position{X: 100, Y: 100},
		ProcessGroupID: &flow.ID,
	}

	node, err := fc.CreateProcessorByType("GenerateFlowFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node)
	require.NoError(t, err)

	body := map[string]interface{}{
		"config": map[string]interface{}{
			"name": "UpdatedName",
			"properties": map[string]interface{}{
				"testProperty": "testValue",
			},
		},
		"position": map[string]interface{}{
			"x": 200.0,
			"y": 300.0,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/flows/"+flow.ID.String()+"/processors/"+node.ID.String(), bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"flowId": flow.ID.String(), "processorId": node.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleUpdateProcessor(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify updates
	updatedNode, exists := fc.GetProcessor(node.ID)
	require.True(t, exists)
	assert.Equal(t, "UpdatedName", updatedNode.Name)
	assert.Equal(t, 200.0, updatedNode.Config.Position.X)
	assert.Equal(t, 300.0, updatedNode.Config.Position.Y)
}

func TestHandleUpdateProcessor_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	nonExistentID := uuid.New()
	body := map[string]interface{}{
		"config": map[string]interface{}{
			"name": "UpdatedName",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/flows/"+flow.ID.String()+"/processors/"+nonExistentID.String(), bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"flowId": flow.ID.String(), "processorId": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleUpdateProcessor(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDeleteProcessor(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "ToDelete",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	node, err := fc.CreateProcessorByType("GenerateFlowFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/flows/"+flow.ID.String()+"/processors/"+node.ID.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"flowId": flow.ID.String(), "processorId": node.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleDeleteProcessor(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify processor is deleted
	_, exists := fc.GetProcessor(node.ID)
	assert.False(t, exists)
}

func TestHandleCreateConnection(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	// Create source and destination processors
	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Source",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	source, err := fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, source)
	require.NoError(t, err)

	config2 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Destination",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	dest, err := fc.CreateProcessorByType("PutFile", config2)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, dest)
	require.NoError(t, err)

	body := map[string]interface{}{
		"sourceId":     source.ID.String(),
		"targetId":     dest.ID.String(),
		"relationship": "success",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/connections", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateConnection(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["id"])
	assert.Equal(t, source.ID.String(), response["sourceId"])
	assert.Equal(t, dest.ID.String(), response["targetId"])
}

func TestHandleCreateConnection_InvalidSourceID(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	body := map[string]interface{}{
		"sourceId":     "invalid-id",
		"targetId":     uuid.New().String(),
		"relationship": "success",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/connections", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateConnection(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateConnection_SourceNotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	body := map[string]interface{}{
		"sourceId":     uuid.New().String(),
		"targetId":     uuid.New().String(),
		"relationship": "success",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/connections", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleCreateConnection(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleDeleteConnection(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	flow, err := fc.CreateProcessGroup("Test Flow", nil)
	require.NoError(t, err)

	// Create source and destination processors
	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Source",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	source, err := fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)

	config2 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Destination",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	dest, err := fc.CreateProcessorByType("PutFile", config2)
	require.NoError(t, err)

	// Create connection
	conn, err := fc.AddConnection(source.ID, dest.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/flows/"+flow.ID.String()+"/connections/"+conn.ID.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"flowId": flow.ID.String(), "connectionId": conn.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleDeleteConnection(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify connection is deleted
	_, exists := fc.GetConnection(conn.ID)
	assert.False(t, exists)
}

func TestHandleExportFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	// Create a flow with processors
	flow, err := fc.CreateProcessGroup("Export Test Flow", nil)
	require.NoError(t, err)

	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "TestProcessor",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     map[string]string{"test": "value"},
		AutoTerminate:  make(map[string]bool),
		Position:       &types.Position{X: 150, Y: 250},
		ProcessGroupID: &flow.ID,
	}

	node, err := fc.CreateProcessorByType("GenerateFlowFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/flows/"+flow.ID.String()+"/export", nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleExportFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, flow.ID.String(), response["id"])
	assert.Equal(t, "Export Test Flow", response["name"])
	assert.Equal(t, "1.0", response["version"])
	assert.NotEmpty(t, response["exportedAt"])

	processors := response["processors"].([]interface{})
	assert.Len(t, processors, 1)

	proc := processors[0].(map[string]interface{})
	assert.Equal(t, "TestProcessor", proc["name"])
	assert.Equal(t, "GenerateFlowFile", proc["type"])
}

func TestHandleExportFlow_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	nonExistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/flows/"+nonExistentID.String()+"/export", nil)
	req = mux.SetURLVars(req, map[string]string{"id": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleExportFlow(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleImportFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	exportData := map[string]interface{}{
		"name":    "Imported Flow",
		"version": "1.0",
		"processors": []interface{}{
			map[string]interface{}{
				"id":            uuid.New().String(),
				"name":          "ImportedProcessor",
				"type":          "GenerateFlowFile",
				"properties":    map[string]interface{}{"test": "value"},
				"scheduleType":  "TIMER",
				"scheduleValue": "2s",
				"concurrency":   2,
				"autoTerminate": map[string]interface{}{},
				"position":      map[string]interface{}{"x": 100.0, "y": 200.0},
			},
		},
		"connections": []interface{}{},
	}
	bodyBytes, _ := json.Marshal(exportData)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/import", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handlers.HandleImportFlow(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "Imported Flow", response["name"])

	// Verify the flow was actually imported
	flowID, err := uuid.Parse(response["id"].(string))
	require.NoError(t, err)

	importedFlow, exists := fc.GetProcessGroup(flowID)
	require.True(t, exists)
	assert.Equal(t, "Imported Flow", importedFlow.Name)
}

func TestHandleImportFlow_InvalidJSON(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/import", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handlers.HandleImportFlow(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleImportFlow_MissingName(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	exportData := map[string]interface{}{
		"version": "1.0",
	}
	bodyBytes, _ := json.Marshal(exportData)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/import", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handlers.HandleImportFlow(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleValidateFlow(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	// Create a valid flow
	flow, err := fc.CreateProcessGroup("Validation Test Flow", nil)
	require.NoError(t, err)

	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Source",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  map[string]bool{"success": true},
		ProcessGroupID: &flow.ID,
	}

	_, err = fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+flow.ID.String()+"/validate", nil)
	req = mux.SetURLVars(req, map[string]string{"id": flow.ID.String()})
	w := httptest.NewRecorder()

	handlers.HandleValidateFlow(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, flow.ID.String(), response["flowId"])
	assert.NotNil(t, response["valid"])
	assert.NotNil(t, response["errors"])
	assert.NotNil(t, response["warnings"])
}

func TestHandleValidateFlow_NotFound(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	nonExistentID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/flows/"+nonExistentID.String()+"/validate", nil)
	req = mux.SetURLVars(req, map[string]string{"id": nonExistentID.String()})
	w := httptest.NewRecorder()

	handlers.HandleValidateFlow(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleValidateFlow_InvalidID(t *testing.T) {
	fc := setupTestFlowController(t)
	handlers := NewFlowHandlers(fc)

	req := httptest.NewRequest(http.MethodPost, "/api/flows/invalid-id/validate", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "invalid-id"})
	w := httptest.NewRecorder()

	handlers.HandleValidateFlow(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
