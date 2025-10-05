package plugins

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewTransformJSONProcessor(t *testing.T) {
	processor := NewTransformJSONProcessor()

	if processor == nil {
		t.Fatal("NewTransformJSONProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "TransformJSON" {
		t.Errorf("Expected processor name TransformJSON, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 4 {
		t.Errorf("Expected 4 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(info.Relationships))
	}
}

func TestTransformJSONProcessorInitialize(t *testing.T) {
	processor := NewTransformJSONProcessor()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".data.items[0]",
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid properties: %v", err)
	}

	// Test initialization without JSON Path Expression
	ctxNoPath := &mockProcessorContext{
		properties: map[string]string{},
	}

	err = processor.Initialize(ctxNoPath)
	if err == nil {
		t.Error("Initialize should fail without JSON Path Expression property")
	}

	// Test initialization with empty path
	ctxEmptyPath := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": "",
		},
	}

	err = processor.Initialize(ctxEmptyPath)
	if err == nil {
		t.Error("Initialize should fail with empty JSON Path Expression")
	}
}

func TestTransformJSONProcessorValidate(t *testing.T) {
	processor := NewTransformJSONProcessor()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"JSON Path Expression": ".data.items[0].name",
			"Destination":          "flowfile-content",
			"Return Type":          "json",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test unbalanced brackets
	invalidBracketsConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"JSON Path Expression": ".data.items[0",
		},
	}

	results = processor.Validate(invalidBracketsConfig)
	hasBracketError := false
	for _, result := range results {
		if result.Property == "JSON Path Expression" && !result.Valid {
			hasBracketError = true
			break
		}
	}

	if !hasBracketError {
		t.Error("Unbalanced brackets should cause validation error")
	}

	// Test invalid destination
	invalidDestConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"JSON Path Expression": ".data",
			"Destination":          "invalid",
		},
	}

	results = processor.Validate(invalidDestConfig)
	hasDestError := false
	for _, result := range results {
		if result.Property == "Destination" && !result.Valid {
			hasDestError = true
			break
		}
	}

	if !hasDestError {
		t.Error("Invalid destination should cause validation error")
	}

	// Test invalid return type
	invalidTypeConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"JSON Path Expression": ".data",
			"Return Type":          "invalid",
		},
	}

	results = processor.Validate(invalidTypeConfig)
	hasTypeError := false
	for _, result := range results {
		if result.Property == "Return Type" && !result.Valid {
			hasTypeError = true
			break
		}
	}

	if !hasTypeError {
		t.Error("Invalid return type should cause validation error")
	}
}

func TestTransformJSONProcessorSimplePath(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"name": "John", "age": 30}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".name",
			"Destination":          "flowfile-content",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result
	content := session.writtenContent[flowFile.ID]
	if string(content) != "John" {
		t.Errorf("Expected 'John', got %s", string(content))
	}

	// Verify transfer to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success")
	}
}

func TestTransformJSONProcessorArrayAccess(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"items": ["apple", "banana", "cherry"]}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".items[1]",
			"Destination":          "flowfile-content",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result
	content := session.writtenContent[flowFile.ID]
	if string(content) != "banana" {
		t.Errorf("Expected 'banana', got %s", string(content))
	}
}

func TestTransformJSONProcessorNestedPath(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"data": {"user": {"name": "Alice", "email": "alice@example.com"}}}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".data.user.email",
			"Destination":          "flowfile-content",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result
	content := session.writtenContent[flowFile.ID]
	if string(content) != "alice@example.com" {
		t.Errorf("Expected 'alice@example.com', got %s", string(content))
	}
}

func TestTransformJSONProcessorArrayOfObjects(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"users": [{"name": "Alice", "age": 25}, {"name": "Bob", "age": 30}]}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".users[0].name",
			"Destination":          "flowfile-content",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result
	content := session.writtenContent[flowFile.ID]
	if string(content) != "Alice" {
		t.Errorf("Expected 'Alice', got %s", string(content))
	}
}

func TestTransformJSONProcessorReturnJSON(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"user": {"name": "Alice", "age": 25}}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".user",
			"Destination":          "flowfile-content",
			"Return Type":          "json",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result is valid JSON
	content := session.writtenContent[flowFile.ID]
	var result map[string]interface{}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Errorf("Result should be valid JSON: %v", err)
	}

	if result["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", result["name"])
	}
}

func TestTransformJSONProcessorDestinationAttribute(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"status": "active"}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression":  ".status",
			"Destination":           "flowfile-attribute",
			"Destination Attribute": "user.status",
			"Return Type":           "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify attribute was set
	attrs := session.attributes[flowFile.ID]
	if attrs["user.status"] != "active" {
		t.Errorf("Expected user.status=active, got %s", attrs["user.status"])
	}

	// Original content should be unchanged
	originalContent := session.writtenContent[flowFile.ID]
	if string(originalContent) != jsonData {
		t.Error("Original content should be unchanged when destination is attribute")
	}
}

func TestTransformJSONProcessorInvalidJSON(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	invalidJSON := `{invalid json`
	session.Write(flowFile, []byte(invalidJSON))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".data",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify transfer to failure
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "failure" {
		t.Error("FlowFile should be transferred to failure on invalid JSON")
	}

	// Verify error attribute
	attrs := session.attributes[flowFile.ID]
	if _, exists := attrs["json.error"]; !exists {
		t.Error("Expected json.error attribute on failure")
	}
}

func TestTransformJSONProcessorInvalidPath(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"name": "John"}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".nonexistent.field",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify transfer to failure
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "failure" {
		t.Error("FlowFile should be transferred to failure on invalid path")
	}
}

func TestTransformJSONProcessorArrayOutOfBounds(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"items": ["a", "b"]}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".items[5]",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify transfer to failure
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "failure" {
		t.Error("FlowFile should be transferred to failure on array out of bounds")
	}
}

func TestTransformJSONProcessorRootPath(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"name": "John", "age": 30}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".",
			"Return Type":          "json",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result is the full JSON
	content := session.writtenContent[flowFile.ID]
	var result map[string]interface{}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Errorf("Result should be valid JSON: %v", err)
	}

	if result["name"] != "John" {
		t.Error("Root path should return entire JSON object")
	}
}

func TestTransformJSONProcessorAutoReturnType(t *testing.T) {
	processor := NewTransformJSONProcessor()

	// Test with string value
	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"message": "hello"}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".message",
			"Return Type":          "auto",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// For string values, auto should return the string as-is
	content := session.writtenContent[flowFile.ID]
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got %s", string(content))
	}

	// Test with object value
	session2 := newMockProcessSession()
	flowFile2 := session2.Create()

	jsonData2 := `{"data": {"key": "value"}}`
	session2.Write(flowFile2, []byte(jsonData2))
	session2.createdFlowFiles = []*types.FlowFile{flowFile2}

	ctx2 := context.WithValue(context.Background(), "processorContext", processorCtx)

	err = processor.OnTrigger(ctx2, session2)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}
}

func TestTransformJSONProcessorComplexPath(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{
		"response": {
			"data": {
				"users": [
					{"id": 1, "name": "Alice"},
					{"id": 2, "name": "Bob"}
				]
			}
		}
	}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".response.data.users[1].name",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify result
	content := session.writtenContent[flowFile.ID]
	if string(content) != "Bob" {
		t.Errorf("Expected 'Bob', got %s", string(content))
	}
}

func TestTransformJSONProcessorNoFlowFile(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".data",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error when no FlowFile: %v", err)
	}
}

func TestTransformJSONProcessorOnTriggerWithoutContext(t *testing.T) {
	processor := NewTransformJSONProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestTransformJSONProcessorOnStopped(t *testing.T) {
	processor := NewTransformJSONProcessor()
	ctx := context.Background()

	// OnStopped should not panic
	processor.OnStopped(ctx)
}

func TestTransformJSONProcessorNumericValues(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"count": 42, "price": 19.99}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".count",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify numeric value is converted to string
	content := session.writtenContent[flowFile.ID]
	if string(content) != "42" {
		t.Errorf("Expected '42', got %s", string(content))
	}
}

func TestTransformJSONProcessorBooleanValues(t *testing.T) {
	processor := NewTransformJSONProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	jsonData := `{"active": true, "deleted": false}`
	session.Write(flowFile, []byte(jsonData))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"JSON Path Expression": ".active",
			"Return Type":          "string",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify boolean value is converted to string
	content := session.writtenContent[flowFile.ID]
	if string(content) != "true" {
		t.Errorf("Expected 'true', got %s", string(content))
	}
}
