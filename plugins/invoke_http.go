package plugins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func init() {
	info := getInvokeHTTPInfo()
	plugin.RegisterBuiltInProcessor("InvokeHTTP", func() types.Processor {
		return NewInvokeHTTPProcessor()
	}, info)
}

func getInvokeHTTPInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"InvokeHTTP",
		"InvokeHTTP",
		"1.0.0",
		"DataBridge",
		"Makes HTTP requests to interact with REST APIs",
		[]string{"http", "rest", "api", "client"},
	)
}

// InvokeHTTPProcessor makes HTTP requests
type InvokeHTTPProcessor struct {
	*types.BaseProcessor
	client *http.Client
}

// Define additional relationships
var (
	RelationshipRetry = types.Relationship{
		Name:        "retry",
		Description: "FlowFiles that should be retried (5xx server errors)",
	}
	RelationshipNoRetry = types.Relationship{
		Name:        "no retry",
		Description: "FlowFiles that should not be retried (4xx client errors)",
	}
)

// NewInvokeHTTPProcessor creates a new InvokeHTTP processor
func NewInvokeHTTPProcessor() *InvokeHTTPProcessor {
	info := types.ProcessorInfo{
		Name:        "InvokeHTTP",
		Description: "Makes HTTP requests to interact with REST APIs",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"http", "rest", "api", "client"},
		Properties: []types.PropertySpec{
			{
				Name:          "HTTP Method",
				Description:   "HTTP method to use",
				Required:      true,
				DefaultValue:  "GET",
				AllowedValues: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"},
			},
			{
				Name:         "Remote URL",
				Description:  "Target URL for the request",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:         "Connection Timeout",
				Description:  "Timeout for establishing connection (e.g., 5s, 1m)",
				Required:     false,
				DefaultValue: "5s",
			},
			{
				Name:         "Read Timeout",
				Description:  "Timeout for reading response (e.g., 30s, 1m)",
				Required:     false,
				DefaultValue: "30s",
			},
			{
				Name:          "Send Message Body",
				Description:   "Whether to send FlowFile content as request body",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Attributes to Send",
				Description:  "Comma-separated list of attribute names to send as headers (format: attr-name will become X-Attr-Name header)",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:          "Follow Redirects",
				Description:   "Whether to automatically follow HTTP redirects",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Basic Auth Username",
				Description:  "Username for basic authentication",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:         "Basic Auth Password",
				Description:  "Password for basic authentication",
				Required:     false,
				DefaultValue: "",
				Sensitive:    true,
			},
			{
				Name:         "Content-Type",
				Description:  "Content-Type header for request",
				Required:     false,
				DefaultValue: "application/octet-stream",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
			RelationshipRetry,
			RelationshipNoRetry,
		},
	}

	return &InvokeHTTPProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *InvokeHTTPProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing InvokeHTTP processor")

	// Validate required properties
	if !ctx.HasProperty("HTTP Method") {
		return fmt.Errorf("HTTP Method property is required")
	}

	if !ctx.HasProperty("Remote URL") {
		return fmt.Errorf("Remote URL property is required")
	}

	url := ctx.GetPropertyValue("Remote URL")
	if url == "" {
		return fmt.Errorf("Remote URL cannot be empty")
	}

	// Parse timeouts
	connTimeoutStr := ctx.GetPropertyValue("Connection Timeout")
	if connTimeoutStr == "" {
		connTimeoutStr = "5s"
	}
	connTimeout, err := time.ParseDuration(connTimeoutStr)
	if err != nil {
		return fmt.Errorf("invalid Connection Timeout: %w", err)
	}

	readTimeoutStr := ctx.GetPropertyValue("Read Timeout")
	if readTimeoutStr == "" {
		readTimeoutStr = "30s"
	}
	readTimeout, err := time.ParseDuration(readTimeoutStr)
	if err != nil {
		return fmt.Errorf("invalid Read Timeout: %w", err)
	}

	// Create HTTP client
	followRedirects := ctx.GetPropertyValue("Follow Redirects") != "false"

	p.client = &http.Client{
		Timeout: readTimeout,
		Transport: &http.Transport{
			DialContext:           (&http.Transport{}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   connTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	if !followRedirects {
		p.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *InvokeHTTPProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get processor context to access properties
	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		return fmt.Errorf("processor context not available")
	}

	// Get a FlowFile to process
	flowFile := session.Get()
	if flowFile == nil {
		logger.Debug("No FlowFile available to process")
		return nil
	}

	// Build and execute HTTP request
	config := p.buildRequestConfig(processorCtx)
	req, err := p.buildHTTPRequest(ctx, session, flowFile, config)
	if err != nil {
		logger.Error("Failed to build HTTP request", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Execute request and handle response
	p.executeRequestAndHandleResponse(ctx, session, flowFile, req, config, logger)

	return nil
}

// httpRequestConfig holds configuration for HTTP requests
type httpRequestConfig struct {
	method      string
	url         string
	sendBody    bool
	attrsToSend string
	username    string
	password    string
	contentType string
}

// buildRequestConfig extracts HTTP request configuration from processor context
func (p *InvokeHTTPProcessor) buildRequestConfig(ctx types.ProcessorContext) *httpRequestConfig {
	method := ctx.GetPropertyValue("HTTP Method")
	if method == "" {
		method = "GET"
	}

	contentType := ctx.GetPropertyValue("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &httpRequestConfig{
		method:      method,
		url:         ctx.GetPropertyValue("Remote URL"),
		sendBody:    ctx.GetPropertyValue("Send Message Body") != "false",
		attrsToSend: ctx.GetPropertyValue("Attributes to Send"),
		username:    ctx.GetPropertyValue("Basic Auth Username"),
		password:    ctx.GetPropertyValue("Basic Auth Password"),
		contentType: contentType,
	}
}

// buildHTTPRequest builds an HTTP request from FlowFile and config
func (p *InvokeHTTPProcessor) buildHTTPRequest(ctx context.Context, session types.ProcessSession, flowFile *types.FlowFile, config *httpRequestConfig) (*http.Request, error) {
	// Prepare request body
	body, err := p.prepareRequestBody(session, flowFile, config)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, config.method, config.url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Configure request headers
	p.configureRequestHeaders(req, flowFile, config)

	return req, nil
}

// prepareRequestBody prepares the request body if needed
func (p *InvokeHTTPProcessor) prepareRequestBody(session types.ProcessSession, flowFile *types.FlowFile, config *httpRequestConfig) (io.Reader, error) {
	if !config.sendBody || (config.method != "POST" && config.method != "PUT" && config.method != "PATCH") {
		return nil, nil
	}

	content, err := session.Read(flowFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read FlowFile content: %w", err)
	}

	return strings.NewReader(string(content)), nil
}

// configureRequestHeaders configures HTTP request headers
func (p *InvokeHTTPProcessor) configureRequestHeaders(req *http.Request, flowFile *types.FlowFile, config *httpRequestConfig) {
	// Set Content-Type header if sending body
	if config.sendBody && req.Body != nil {
		req.Header.Set("Content-Type", config.contentType)
	}

	// Add basic auth if configured
	if config.username != "" {
		req.SetBasicAuth(config.username, config.password)
	}

	// Add attributes as headers
	if config.attrsToSend != "" {
		p.addAttributesAsHeaders(req, flowFile, config.attrsToSend)
	}
}

// addAttributesAsHeaders adds FlowFile attributes as HTTP headers
func (p *InvokeHTTPProcessor) addAttributesAsHeaders(req *http.Request, flowFile *types.FlowFile, attrsToSend string) {
	attrNames := strings.Split(attrsToSend, ",")
	caser := cases.Title(language.English)

	for _, attrName := range attrNames {
		attrName = strings.TrimSpace(attrName)
		if value, exists := flowFile.GetAttribute(attrName); exists {
			// Convert attribute name to header name (e.g., "my-attr" -> "X-My-Attr")
			headerName := "X-" + strings.ReplaceAll(caser.String(strings.ReplaceAll(attrName, "-", " ")), " ", "-")
			req.Header.Set(headerName, value)
		}
	}
}

// executeRequestAndHandleResponse executes the HTTP request and handles the response
func (p *InvokeHTTPProcessor) executeRequestAndHandleResponse(ctx context.Context, session types.ProcessSession, flowFile *types.FlowFile, req *http.Request, config *httpRequestConfig, logger types.Logger) {
	// Make request
	startTime := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("HTTP request failed", "error", err, "url", config.url)
		session.PutAttribute(flowFile, "http.error.message", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Read and process response
	if !p.processResponse(session, flowFile, resp, duration, config, logger) {
		return
	}

	logger.Info("HTTP request completed",
		"url", config.url,
		"method", config.method,
		"statusCode", resp.StatusCode,
		"duration", duration,
		"flowFileId", flowFile.ID)
}

// processResponse processes the HTTP response
func (p *InvokeHTTPProcessor) processResponse(session types.ProcessSession, flowFile *types.FlowFile, resp *http.Response, duration time.Duration, config *httpRequestConfig, logger types.Logger) bool {
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		session.PutAttribute(flowFile, "http.error.message", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return false
	}

	// Update FlowFile with response
	err = session.Write(flowFile, respBody)
	if err != nil {
		logger.Error("Failed to write response to FlowFile", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return false
	}

	// Add response metadata
	p.addResponseAttributes(session, flowFile, resp, duration, config)

	// Route based on status code
	relationship := p.determineRelationship(resp.StatusCode)
	session.Transfer(flowFile, relationship)

	return true
}

// addResponseAttributes adds response metadata as FlowFile attributes
func (p *InvokeHTTPProcessor) addResponseAttributes(session types.ProcessSession, flowFile *types.FlowFile, resp *http.Response, duration time.Duration, config *httpRequestConfig) {
	session.PutAttribute(flowFile, "http.status.code", strconv.Itoa(resp.StatusCode))
	session.PutAttribute(flowFile, "http.status.message", resp.Status)
	session.PutAttribute(flowFile, "http.response.time", duration.String())
	session.PutAttribute(flowFile, "http.remote.url", config.url)
	session.PutAttribute(flowFile, "http.method", config.method)

	// Add response headers as attributes
	for key, values := range resp.Header {
		if len(values) > 0 {
			session.PutAttribute(flowFile, "http.header."+strings.ToLower(key), values[0])
		}
	}
}

// determineRelationship determines the routing relationship based on status code
func (p *InvokeHTTPProcessor) determineRelationship(statusCode int) types.Relationship {
	if statusCode >= 200 && statusCode < 300 {
		return types.RelationshipSuccess
	} else if statusCode >= 400 && statusCode < 500 {
		return RelationshipNoRetry // Client errors - don't retry
	} else if statusCode >= 500 {
		return RelationshipRetry // Server errors - can retry
	}
	return types.RelationshipFailure
}

// Validate validates the processor configuration
func (p *InvokeHTTPProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate HTTP Method
	if method, exists := config.Properties["HTTP Method"]; exists {
		validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
		valid := false
		for _, vm := range validMethods {
			if method == vm {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "HTTP Method",
				Valid:    false,
				Message:  "Invalid HTTP method",
			})
		}
	}

	// Validate Remote URL
	if url, exists := config.Properties["Remote URL"]; exists && url != "" {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			results = append(results, types.ValidationResult{
				Property: "Remote URL",
				Valid:    false,
				Message:  "URL must start with http:// or https://",
			})
		}
	}

	// Validate Connection Timeout
	if timeoutStr, exists := config.Properties["Connection Timeout"]; exists && timeoutStr != "" {
		var err error
		_, err = time.ParseDuration(timeoutStr)
		if err != nil {
			results = append(results, types.ValidationResult{
				Property: "Connection Timeout",
				Valid:    false,
				Message:  "Invalid duration format (use: 5s, 1m, etc.)",
			})
		}
	}

	// Validate Read Timeout
	if timeoutStr, exists := config.Properties["Read Timeout"]; exists && timeoutStr != "" {
		var err error
		_, err = time.ParseDuration(timeoutStr)
		if err != nil {
			results = append(results, types.ValidationResult{
				Property: "Read Timeout",
				Valid:    false,
				Message:  "Invalid duration format (use: 30s, 1m, etc.)",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *InvokeHTTPProcessor) OnStopped(ctx context.Context) {
	// Close idle connections
	if p.client != nil && p.client.Transport != nil {
		if transport, ok := p.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}
