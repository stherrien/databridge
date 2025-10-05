package core

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/plugin"
	_ "github.com/shawntherrien/databridge/plugins" // Import to register built-in processors
	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestFC() *FlowController {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	flowFileRepo := NewInMemoryFlowFileRepository()
	contentRepo := NewInMemoryContentRepository()
	provenanceRepo := NewInMemoryProvenanceRepository()

	config := plugin.PluginManagerConfig{
		PluginDir:       "",
		AutoLoad:        false,
		MonitorInterval: 1 * time.Minute, // Set non-zero interval for resource monitor
	}
	pluginManager, err := plugin.NewPluginManager(config, logger)
	if err != nil {
		panic(err)
	}

	// Initialize plugin manager to register built-in processors
	if err := pluginManager.Initialize(); err != nil {
		panic(err)
	}

	return NewFlowControllerWithPlugins(
		flowFileRepo,
		contentRepo,
		provenanceRepo,
		pluginManager,
		logger,
	)
}

// TestCreateProcessGroup tests creating process groups
func TestCreateProcessGroup(t *testing.T) {
	fc := setupTestFC()

	// Test creating a process group
	pg, err := fc.CreateProcessGroup("Test Group", nil)
	require.NoError(t, err)
	assert.NotNil(t, pg)
	assert.Equal(t, "Test Group", pg.Name)
	assert.NotEqual(t, uuid.Nil, pg.ID)

	// Verify it's in the controller
	retrieved, exists := fc.GetProcessGroup(pg.ID)
	assert.True(t, exists)
	assert.Equal(t, pg.ID, retrieved.ID)
}

// TestCreateProcessGroupWithParent tests creating nested process groups
func TestCreateProcessGroupWithParent(t *testing.T) {
	fc := setupTestFC()

	// Create parent
	parent, err := fc.CreateProcessGroup("Parent Group", nil)
	require.NoError(t, err)

	// Create child
	child, err := fc.CreateProcessGroup("Child Group", &parent.ID)
	require.NoError(t, err)
	assert.NotNil(t, child)
	assert.Equal(t, parent.ID, child.Parent.ID)

	// Verify parent has child
	parent.RLock()
	_, hasChild := parent.Children[child.ID]
	parent.RUnlock()
	assert.True(t, hasChild)
}

// TestCreateProcessGroupInvalidParent tests error handling for invalid parent
func TestCreateProcessGroupInvalidParent(t *testing.T) {
	fc := setupTestFC()

	nonExistentID := uuid.New()
	_, err := fc.CreateProcessGroup("Test Group", &nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parent process group not found")
}

// TestUpdateProcessGroup tests updating process group names
func TestUpdateProcessGroup(t *testing.T) {
	fc := setupTestFC()

	pg, err := fc.CreateProcessGroup("Original Name", nil)
	require.NoError(t, err)

	// Update name
	updated, err := fc.UpdateProcessGroup(pg.ID, "New Name")
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.Name)

	// Verify change persisted
	retrieved, exists := fc.GetProcessGroup(pg.ID)
	require.True(t, exists)
	assert.Equal(t, "New Name", retrieved.Name)
}

// TestUpdateProcessGroupNotFound tests error handling for non-existent groups
func TestUpdateProcessGroupNotFound(t *testing.T) {
	fc := setupTestFC()

	nonExistentID := uuid.New()
	_, err := fc.UpdateProcessGroup(nonExistentID, "New Name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "process group not found")
}

// TestDeleteProcessGroup tests deleting empty process groups
func TestDeleteProcessGroup(t *testing.T) {
	fc := setupTestFC()

	// Create parent and child
	parent, err := fc.CreateProcessGroup("Parent", nil)
	require.NoError(t, err)

	child, err := fc.CreateProcessGroup("Child", &parent.ID)
	require.NoError(t, err)

	// Delete child
	err = fc.DeleteProcessGroup(child.ID)
	require.NoError(t, err)

	// Verify child is deleted
	_, exists := fc.GetProcessGroup(child.ID)
	assert.False(t, exists)

	// Verify parent no longer has child
	parent.RLock()
	_, hasChild := parent.Children[child.ID]
	parent.RUnlock()
	assert.False(t, hasChild)
}

// TestDeleteProcessGroupWithProcessors tests error when deleting groups with processors
func TestDeleteProcessGroupWithProcessors(t *testing.T) {
	fc := setupTestFC()

	// Create parent first (root cannot be deleted)
	parent, err := fc.CreateProcessGroup("Parent", nil)
	require.NoError(t, err)

	pg, err := fc.CreateProcessGroup("Test Group", &parent.ID)
	require.NoError(t, err)

	// Add a processor to the group
	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Test Processor",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &pg.ID,
	}

	node, err := fc.CreateProcessorByType("GenerateFlowFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(pg.ID, node)
	require.NoError(t, err)

	// Try to delete - should fail
	err = fc.DeleteProcessGroup(pg.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete process group with processors")
}

// TestDeleteProcessGroupWithConnections tests error when deleting groups with connections
func TestDeleteProcessGroupWithConnections(t *testing.T) {
	fc := setupTestFC()

	// Create parent first (root cannot be deleted)
	parent, err := fc.CreateProcessGroup("Parent", nil)
	require.NoError(t, err)

	pg, err := fc.CreateProcessGroup("Test Group", &parent.ID)
	require.NoError(t, err)

	// Create processors
	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Source",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &pg.ID,
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
		ProcessGroupID: &pg.ID,
	}

	dest, err := fc.CreateProcessorByType("PutFile", config2)
	require.NoError(t, err)

	// Create connection
	conn, err := fc.AddConnection(source.ID, dest.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	// Add connection to process group
	pg.mu.Lock()
	pg.Connections[conn.ID] = conn
	pg.mu.Unlock()

	// Try to delete - should fail
	err = fc.DeleteProcessGroup(pg.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete process group with connections")
}

// TestDeleteRootProcessGroup tests that root group cannot be deleted
func TestDeleteRootProcessGroup(t *testing.T) {
	fc := setupTestFC()

	// Find root group (one with no parent)
	var rootID uuid.UUID
	for id, pg := range fc.GetProcessGroups() {
		if pg.Parent == nil {
			rootID = id
			break
		}
	}

	// Try to delete root - should fail
	err := fc.DeleteProcessGroup(rootID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete root process group")
}

// TestExportFlow tests exporting flow configuration
func TestExportFlow(t *testing.T) {
	fc := setupTestFC()

	// Create a flow with processors and connections
	flow, err := fc.CreateProcessGroup("Export Test", nil)
	require.NoError(t, err)

	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "TestProcessor",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "2s",
		Concurrency:    2,
		Properties:     map[string]string{"prop1": "value1"},
		AutoTerminate:  map[string]bool{"failure": true},
		Position:       &types.Position{X: 100, Y: 200},
		ProcessGroupID: &flow.ID,
	}

	node1, err := fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node1)
	require.NoError(t, err)

	config2 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "TestProcessor2",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeEvent,
		ScheduleValue:  "1",
		Concurrency:    1,
		Properties:     map[string]string{"directory": "/tmp"},
		AutoTerminate:  make(map[string]bool),
		Position:       &types.Position{X: 300, Y: 200},
		ProcessGroupID: &flow.ID,
	}

	node2, err := fc.CreateProcessorByType("PutFile", config2)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node2)
	require.NoError(t, err)

	// Create connection
	conn, err := fc.AddConnection(node1.ID, node2.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	// Add connection to flow
	flow.mu.Lock()
	flow.Connections[conn.ID] = conn
	flow.mu.Unlock()

	// Export flow
	exportData, err := fc.ExportFlow(flow.ID)
	require.NoError(t, err)

	// Verify export data
	exportID := exportData["id"]
	if id, ok := exportID.(uuid.UUID); ok {
		exportID = id.String()
	}
	assert.Equal(t, flow.ID.String(), exportID)
	assert.Equal(t, "Export Test", exportData["name"])
	assert.Equal(t, "1.0", exportData["version"])
	assert.NotEmpty(t, exportData["exportedAt"])

	processors := exportData["processors"].([]map[string]interface{})
	assert.Len(t, processors, 2)

	// Verify processor details
	proc1 := processors[0]
	assert.Equal(t, "TestProcessor", proc1["name"])
	assert.Equal(t, "GenerateFlowFile", proc1["type"])

	// ScheduleType is an enum type, convert to string
	scheduleType := proc1["scheduleType"]
	if st, ok := scheduleType.(types.ScheduleType); ok {
		scheduleType = string(st)
	}
	assert.Equal(t, string(types.ScheduleTypeTimer), scheduleType)
	assert.Equal(t, "2s", proc1["scheduleValue"])
	assert.Equal(t, 2, proc1["concurrency"])

	props := proc1["properties"].(map[string]string)
	assert.Equal(t, "value1", props["prop1"])

	position := proc1["position"].(map[string]float64)
	assert.Equal(t, 100.0, position["x"])
	assert.Equal(t, 200.0, position["y"])

	// Verify connections
	connections := exportData["connections"].([]map[string]interface{})
	assert.Len(t, connections, 1)

	conn1 := connections[0]
	// Convert UUID fields to strings for comparison
	connID := conn1["id"]
	if id, ok := connID.(uuid.UUID); ok {
		connID = id.String()
	}
	sourceID := conn1["sourceId"]
	if id, ok := sourceID.(uuid.UUID); ok {
		sourceID = id.String()
	}
	destID := conn1["destinationId"]
	if id, ok := destID.(uuid.UUID); ok {
		destID = id.String()
	}

	assert.Equal(t, conn.ID.String(), connID)
	assert.Equal(t, node1.ID.String(), sourceID)
	assert.Equal(t, node2.ID.String(), destID)
	assert.Equal(t, "success", conn1["relationship"])
}

// TestExportFlowNotFound tests exporting non-existent flow
func TestExportFlowNotFound(t *testing.T) {
	fc := setupTestFC()

	nonExistentID := uuid.New()
	_, err := fc.ExportFlow(nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flow not found")
}

// TestImportFlow tests importing flow configuration
func TestImportFlow(t *testing.T) {
	fc := setupTestFC()

	sourceID := uuid.New().String()
	destID := uuid.New().String()

	importData := map[string]interface{}{
		"name":    "Imported Flow",
		"version": "1.0",
		"processors": []interface{}{
			map[string]interface{}{
				"id":            sourceID,
				"name":          "ImportedSource",
				"type":          "GenerateFlowFile",
				"properties":    map[string]interface{}{"data": "test"},
				"scheduleType":  string(types.ScheduleTypeTimer),
				"scheduleValue": "3s",
				"concurrency":   float64(3),
				"autoTerminate": map[string]interface{}{"failure": true},
				"position":      map[string]interface{}{"x": 150.0, "y": 250.0},
			},
			map[string]interface{}{
				"id":            destID,
				"name":          "ImportedDest",
				"type":          "PutFile",
				"properties":    map[string]interface{}{"directory": "/tmp"},
				"scheduleType":  string(types.ScheduleTypeEvent),
				"scheduleValue": "1",
				"concurrency":   float64(1),
				"autoTerminate": map[string]interface{}{},
				"position":      map[string]interface{}{"x": 350.0, "y": 250.0},
			},
		},
		"connections": []interface{}{
			map[string]interface{}{
				"sourceId":      sourceID,
				"destinationId": destID,
				"relationship":  "success",
			},
		},
	}

	// Import flow
	flow, err := fc.ImportFlow(importData)
	require.NoError(t, err)
	assert.NotNil(t, flow)
	assert.Equal(t, "Imported Flow", flow.Name)

	// Verify processors were imported
	flow.RLock()
	assert.Equal(t, 2, len(flow.Processors))
	assert.Equal(t, 1, len(flow.Connections))
	flow.RUnlock()

	// Find imported processors (IDs will be different)
	var importedSource, importedDest *ProcessorNode
	for _, proc := range flow.Processors {
		if proc.Name == "ImportedSource" {
			importedSource = proc
		}
		if proc.Name == "ImportedDest" {
			importedDest = proc
		}
	}

	require.NotNil(t, importedSource)
	require.NotNil(t, importedDest)

	// Verify processor properties
	assert.Equal(t, "GenerateFlowFile", importedSource.Type)
	assert.Equal(t, "test", importedSource.Config.Properties["data"])
	assert.Equal(t, types.ScheduleTypeTimer, importedSource.Config.ScheduleType)
	assert.Equal(t, "3s", importedSource.Config.ScheduleValue)
	assert.Equal(t, 3, importedSource.Config.Concurrency)
	assert.Equal(t, 150.0, importedSource.Config.Position.X)
	assert.Equal(t, 250.0, importedSource.Config.Position.Y)

	assert.Equal(t, "PutFile", importedDest.Type)
	assert.Equal(t, "/tmp", importedDest.Config.Properties["directory"])

	// Verify connection
	var conn *Connection
	for _, c := range flow.Connections {
		conn = c
		break
	}
	require.NotNil(t, conn)
	assert.Equal(t, importedSource.ID, conn.Source.ID)
	assert.Equal(t, importedDest.ID, conn.Destination.ID)
	assert.Equal(t, "success", conn.Relationship.Name)
}

// TestImportFlowMissingName tests import validation
func TestImportFlowMissingName(t *testing.T) {
	fc := setupTestFC()

	importData := map[string]interface{}{
		"version": "1.0",
	}

	_, err := fc.ImportFlow(importData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flow name is required")
}

// TestValidateFlow tests flow validation
func TestValidateFlow(t *testing.T) {
	fc := setupTestFC()

	// Create a valid flow
	flow, err := fc.CreateProcessGroup("Valid Flow", nil)
	require.NoError(t, err)

	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "TestProcessor",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     map[string]string{"File Size": "1024"},
		AutoTerminate:  map[string]bool{"success": true, "failure": true},
		ProcessGroupID: &flow.ID,
	}

	node, err := fc.CreateProcessorByType("GenerateFlowFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node)
	require.NoError(t, err)

	// Add to flow
	flow.mu.Lock()
	flow.Processors[node.ID] = node
	flow.mu.Unlock()

	// Validate
	result, err := fc.ValidateFlow(flow.ID)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

// TestValidateFlowWithWarnings tests validation warnings for disconnected processors
func TestValidateFlowWithWarnings(t *testing.T) {
	fc := setupTestFC()

	flow, err := fc.CreateProcessGroup("Flow With Warnings", nil)
	require.NoError(t, err)

	// Create a non-source processor with no incoming connections
	config := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "DisconnectedProcessor",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     map[string]string{"Directory": "/tmp", "Conflict Resolution Strategy": "replace"},
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	node, err := fc.CreateProcessorByType("PutFile", config)
	require.NoError(t, err)
	err = fc.AddProcessorToGroup(flow.ID, node)
	require.NoError(t, err)

	// Add to flow
	flow.mu.Lock()
	flow.Processors[node.ID] = node
	flow.mu.Unlock()

	// Validate
	result, err := fc.ValidateFlow(flow.ID)
	require.NoError(t, err)
	assert.True(t, result.Valid) // Still valid, just warnings
	assert.NotEmpty(t, result.Warnings)
	assert.Contains(t, result.Warnings[0], "no incoming connections")
	assert.Contains(t, result.Warnings[1], "no outgoing connections")
}

// TestValidateFlowWithCycle tests cycle detection
func TestValidateFlowWithCycle(t *testing.T) {
	fc := setupTestFC()

	flow, err := fc.CreateProcessGroup("Flow With Cycle", nil)
	require.NoError(t, err)

	// Create three processors in a cycle
	config1 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Proc1",
		Type:           "GenerateFlowFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     map[string]string{"File Size": "1024"},
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	node1, err := fc.CreateProcessorByType("GenerateFlowFile", config1)
	require.NoError(t, err)

	config2 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Proc2",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	node2, err := fc.CreateProcessorByType("PutFile", config2)
	require.NoError(t, err)

	config3 := types.ProcessorConfig{
		ID:             uuid.New(),
		Name:           "Proc3",
		Type:           "PutFile",
		ScheduleType:   types.ScheduleTypeTimer,
		ScheduleValue:  "1s",
		Concurrency:    1,
		Properties:     make(map[string]string),
		AutoTerminate:  make(map[string]bool),
		ProcessGroupID: &flow.ID,
	}

	node3, err := fc.CreateProcessorByType("PutFile", config3)
	require.NoError(t, err)

	// Create cycle: 1 -> 2 -> 3 -> 1
	conn1, err := fc.AddConnection(node1.ID, node2.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	conn2, err := fc.AddConnection(node2.ID, node3.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	conn3, err := fc.AddConnection(node3.ID, node1.ID, types.Relationship{Name: "success"})
	require.NoError(t, err)

	// Add to flow
	flow.mu.Lock()
	flow.Processors[node1.ID] = node1
	flow.Processors[node2.ID] = node2
	flow.Processors[node3.ID] = node3
	flow.Connections[conn1.ID] = conn1
	flow.Connections[conn2.ID] = conn2
	flow.Connections[conn3.ID] = conn3
	flow.mu.Unlock()

	// Validate
	result, err := fc.ValidateFlow(flow.ID)
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
	assert.Contains(t, result.Errors[0], "contains cycles")
}

// TestValidateFlowNotFound tests validation of non-existent flow
func TestValidateFlowNotFound(t *testing.T) {
	fc := setupTestFC()

	nonExistentID := uuid.New()
	_, err := fc.ValidateFlow(nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "flow not found")
}

// TestGetProcessGroups tests retrieving all process groups
func TestGetProcessGroups(t *testing.T) {
	fc := setupTestFC()

	// Should have at least the root group
	groups := fc.GetProcessGroups()
	assert.GreaterOrEqual(t, len(groups), 1)

	// Create some groups
	pg1, err := fc.CreateProcessGroup("Group 1", nil)
	require.NoError(t, err)

	pg2, err := fc.CreateProcessGroup("Group 2", nil)
	require.NoError(t, err)

	// Verify they're all returned
	groups = fc.GetProcessGroups()
	assert.Contains(t, groups, pg1.ID)
	assert.Contains(t, groups, pg2.ID)
}
