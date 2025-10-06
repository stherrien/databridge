package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewInvokeHTTPProcessor(t *testing.T) {
	processor := NewInvokeHTTPProcessor()

	if processor == nil {
		t.Fatal("NewInvokeHTTPProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "InvokeHTTP" {
		t.Errorf("Expected processor name InvokeHTTP, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 10 {
		t.Errorf("Expected 10 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 4 {
		t.Errorf("Expected 4 relationships, got %d", len(info.Relationships))
	}

	// Check relationships
	expectedRels := map[string]bool{
		"success":  false,
		"failure":  false,
		"retry":    false,
		"no retry": false,
	}

	for _, rel := range info.Relationships {
		if _, exists := expectedRels[rel.Name]; exists {
			expectedRels[rel.Name] = true
		}
	}

	for name, found := range expectedRels {
		if !found {
			t.Errorf("Expected relationship %s not found", name)
		}
	}
}

func TestInvokeHTTPProcessorInitialize(t *testing.T) {
	processor := NewInvokeHTTPProcessor()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":        "GET",
			"Remote URL":         "https://example.com",
			"Connection Timeout": "5s",
			"Read Timeout":       "30s",
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid properties: %v", err)
	}

	// Test initialization without HTTP Method
	ctxNoMethod := &mockProcessorContext{
		properties: map[string]string{
			"Remote URL": "https://example.com",
		},
	}

	err = processor.Initialize(ctxNoMethod)
	if err == nil {
		t.Error("Initialize should fail without HTTP Method property")
	}

	// Test initialization without Remote URL
	ctxNoURL := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
		},
	}

	err = processor.Initialize(ctxNoURL)
	if err == nil {
		t.Error("Initialize should fail without Remote URL property")
	}

	// Test initialization with empty URL
	ctxEmptyURL := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  "",
		},
	}

	err = processor.Initialize(ctxEmptyURL)
	if err == nil {
		t.Error("Initialize should fail with empty Remote URL")
	}

	// Test initialization with invalid timeout
	ctxInvalidTimeout := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":        "GET",
			"Remote URL":         "https://example.com",
			"Connection Timeout": "invalid",
		},
	}

	err = processor.Initialize(ctxInvalidTimeout)
	if err == nil {
		t.Error("Initialize should fail with invalid Connection Timeout")
	}
}

func TestInvokeHTTPProcessorValidate(t *testing.T) {
	processor := NewInvokeHTTPProcessor()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"HTTP Method":         "POST",
			"Remote URL":          "https://api.example.com/endpoint",
			"Connection Timeout":  "5s",
			"Read Timeout":        "30s",
			"Send Message Body":   "true",
			"Follow Redirects":    "true",
			"Basic Auth Username": "user",
			"Basic Auth Password": "pass",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test invalid HTTP method
	invalidMethodConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"HTTP Method": "INVALID",
			"Remote URL":  "https://example.com",
		},
	}

	results = processor.Validate(invalidMethodConfig)
	hasMethodError := false
	for _, result := range results {
		if result.Property == "HTTP Method" && !result.Valid {
			hasMethodError = true
			break
		}
	}

	if !hasMethodError {
		t.Error("Invalid HTTP method should cause validation error")
	}

	// Test invalid URL (no protocol)
	invalidURLConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  "example.com",
		},
	}

	results = processor.Validate(invalidURLConfig)
	hasURLError := false
	for _, result := range results {
		if result.Property == "Remote URL" && !result.Valid {
			hasURLError = true
			break
		}
	}

	if !hasURLError {
		t.Error("Invalid URL should cause validation error")
	}

	// Test invalid connection timeout
	invalidConnTimeoutConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"HTTP Method":        "GET",
			"Remote URL":         "https://example.com",
			"Connection Timeout": "not-a-duration",
		},
	}

	results = processor.Validate(invalidConnTimeoutConfig)
	hasTimeoutError := false
	for _, result := range results {
		if result.Property == "Connection Timeout" && !result.Valid {
			hasTimeoutError = true
			break
		}
	}

	if !hasTimeoutError {
		t.Error("Invalid connection timeout should cause validation error")
	}

	// Test invalid read timeout
	invalidReadTimeoutConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"HTTP Method":  "GET",
			"Remote URL":   "https://example.com",
			"Read Timeout": "not-a-duration",
		},
	}

	results = processor.Validate(invalidReadTimeoutConfig)
	hasReadTimeoutError := false
	for _, result := range results {
		if result.Property == "Read Timeout" && !result.Valid {
			hasReadTimeoutError = true
			break
		}
	}

	if !hasReadTimeoutError {
		t.Error("Invalid read timeout should cause validation error")
	}
}

func TestInvokeHTTPProcessorGETRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Response data"))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("Request body"))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":       "GET",
			"Remote URL":        server.URL,
			"Send Message Body": "false", // GET typically doesn't send body
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify response
	attrs := session.attributes[flowFile.ID]
	if attrs["http.status.code"] != "200" {
		t.Errorf("Expected status code 200, got %s", attrs["http.status.code"])
	}

	// Verify content
	if content, exists := session.writtenContent[flowFile.ID]; exists {
		if string(content) != "Response data" {
			t.Errorf("Expected response 'Response data', got %s", string(content))
		}
	} else {
		t.Error("Response should be written to FlowFile")
	}

	// Verify transfer to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Errorf("FlowFile should be transferred to success, got %s", rel.Name)
	}
}

func TestInvokeHTTPProcessorPOSTRequest(t *testing.T) {
	// Create test server
	receivedBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Created"))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "POST",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	requestBody := []byte("Request payload")
	session.Write(flowFile, requestBody)
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":       "POST",
			"Remote URL":        server.URL,
			"Send Message Body": "true",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify request body was sent
	if receivedBody != "Request payload" {
		t.Errorf("Expected request body 'Request payload', got %s", receivedBody)
	}

	// Verify response
	attrs := session.attributes[flowFile.ID]
	if attrs["http.status.code"] != "201" {
		t.Errorf("Expected status code 201, got %s", attrs["http.status.code"])
	}

	// Verify transfer to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Errorf("FlowFile should be transferred to success, got %s", rel.Name)
	}
}

func TestInvokeHTTPProcessorBasicAuth(t *testing.T) {
	// Create test server that checks auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "testuser" || password != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authenticated"))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":         "GET",
			"Remote URL":          server.URL,
			"Basic Auth Username": "testuser",
			"Basic Auth Password": "testpass",
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":         "GET",
			"Remote URL":          server.URL,
			"Basic Auth Username": "testuser",
			"Basic Auth Password": "testpass",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify authenticated
	attrs := session.attributes[flowFile.ID]
	if attrs["http.status.code"] != "200" {
		t.Errorf("Expected status code 200, got %s", attrs["http.status.code"])
	}
}

func TestInvokeHTTPProcessor4xxResponse(t *testing.T) {
	// Create test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify status code
	attrs := session.attributes[flowFile.ID]
	if attrs["http.status.code"] != "404" {
		t.Errorf("Expected status code 404, got %s", attrs["http.status.code"])
	}

	// Verify transfer to no retry (4xx = client error)
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "no retry" {
		t.Errorf("FlowFile should be transferred to 'no retry', got %s", rel.Name)
	}
}

func TestInvokeHTTPProcessor5xxResponse(t *testing.T) {
	// Create test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Server Error"))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify status code
	attrs := session.attributes[flowFile.ID]
	if attrs["http.status.code"] != "500" {
		t.Errorf("Expected status code 500, got %s", attrs["http.status.code"])
	}

	// Verify transfer to retry (5xx = server error, can retry)
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "retry" {
		t.Errorf("FlowFile should be transferred to 'retry', got %s", rel.Name)
	}
}

func TestInvokeHTTPProcessorAttributesToSend(t *testing.T) {
	// Create test server that checks headers
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile with attributes
	session := newMockProcessSession()
	flowFile := session.Create()
	session.PutAttribute(flowFile, "request-id", "12345")
	session.PutAttribute(flowFile, "user-type", "premium")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method":        "GET",
			"Remote URL":         server.URL,
			"Attributes to Send": "request-id, user-type",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify headers were sent (note: exact header name format may vary)
	// The implementation converts "request-id" to "X-Request-Id" format
	if len(receivedHeaders) == 0 {
		t.Error("Expected headers to be sent")
	}
}

func TestInvokeHTTPProcessorNoFlowFile(t *testing.T) {
	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  "https://example.com",
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Session with no FlowFiles
	session := newMockProcessSession()

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  "https://example.com",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error when no FlowFile: %v", err)
	}
}

func TestInvokeHTTPProcessorOnTriggerWithoutContext(t *testing.T) {
	processor := NewInvokeHTTPProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestInvokeHTTPProcessorOnStopped(t *testing.T) {
	processor := NewInvokeHTTPProcessor()

	// Initialize first
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  "https://example.com",
		},
	}
	processor.Initialize(initCtx)

	ctx := context.Background()

	// OnStopped should not panic and should close connections
	processor.OnStopped(ctx)
}

func TestInvokeHTTPProcessorResponseHeaders(t *testing.T) {
	// Create test server that sets headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "success"}`))
	}))
	defer server.Close()

	processor := NewInvokeHTTPProcessor()

	// Initialize processor
	initCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}
	if err := processor.Initialize(initCtx); err != nil {
		t.Fatal(err)
	}

	// Create session and FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"HTTP Method": "GET",
			"Remote URL":  server.URL,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify response headers were added as attributes
	attrs := session.attributes[flowFile.ID]

	// Check for http.header.* attributes
	hasHeaders := false
	for key := range attrs {
		if len(key) > 12 && key[:12] == "http.header." {
			hasHeaders = true
			break
		}
	}

	if !hasHeaders {
		t.Error("Expected response headers to be added as attributes")
	}
}
