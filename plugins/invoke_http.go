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

	// Read configuration properties
	method := processorCtx.GetPropertyValue("HTTP Method")
	if method == "" {
		method = "GET"
	}

	url := processorCtx.GetPropertyValue("Remote URL")
	sendBody := processorCtx.GetPropertyValue("Send Message Body") != "false"
	attrsToSend := processorCtx.GetPropertyValue("Attributes to Send")
	username := processorCtx.GetPropertyValue("Basic Auth Username")
	password := processorCtx.GetPropertyValue("Basic Auth Password")
	contentType := processorCtx.GetPropertyValue("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Prepare request body
	var body io.Reader
	var err error
	if sendBody && (method == "POST" || method == "PUT" || method == "PATCH") {
		var content []byte
		content, err = session.Read(flowFile)
		if err != nil {
			logger.Error("Failed to read FlowFile content", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return nil
		}
		body = strings.NewReader(string(content))
	}

	// Create request
	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		logger.Error("Failed to create HTTP request", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Set Content-Type header if sending body
	if sendBody && body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	// Add basic auth if configured
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	// Add attributes as headers
	if attrsToSend != "" {
		attrNames := strings.Split(attrsToSend, ",")
		for _, attrName := range attrNames {
			attrName = strings.TrimSpace(attrName)
			if value, exists := flowFile.GetAttribute(attrName); exists {
				// Convert attribute name to header name (e.g., "my-attr" -> "X-My-Attr")
				caser := cases.Title(language.English)
				headerName := "X-" + strings.ReplaceAll(caser.String(strings.ReplaceAll(attrName, "-", " ")), " ", "-")
				req.Header.Set(headerName, value)
			}
		}
	}

	// Make request
	startTime := time.Now()
	var resp *http.Response
	resp, err = p.client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("HTTP request failed", "error", err, "url", url)
		session.PutAttribute(flowFile, "http.error.message", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}
	defer resp.Body.Close()

	// Read response body
	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		session.PutAttribute(flowFile, "http.error.message", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Update FlowFile with response
	err = session.Write(flowFile, respBody)
	if err != nil {
		logger.Error("Failed to write response to FlowFile", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Add response attributes
	session.PutAttribute(flowFile, "http.status.code", strconv.Itoa(resp.StatusCode))
	session.PutAttribute(flowFile, "http.status.message", resp.Status)
	session.PutAttribute(flowFile, "http.response.time", duration.String())
	session.PutAttribute(flowFile, "http.remote.url", url)
	session.PutAttribute(flowFile, "http.method", method)

	// Add response headers as attributes
	for key, values := range resp.Header {
		if len(values) > 0 {
			session.PutAttribute(flowFile, "http.header."+strings.ToLower(key), values[0])
		}
	}

	// Route based on status code
	statusCode := resp.StatusCode
	var relationship types.Relationship

	if statusCode >= 200 && statusCode < 300 {
		relationship = types.RelationshipSuccess
	} else if statusCode >= 400 && statusCode < 500 {
		relationship = RelationshipNoRetry // Client errors - don't retry
	} else if statusCode >= 500 {
		relationship = RelationshipRetry // Server errors - can retry
	} else {
		relationship = types.RelationshipFailure
	}

	session.Transfer(flowFile, relationship)

	logger.Info("HTTP request completed",
		"url", url,
		"method", method,
		"statusCode", statusCode,
		"duration", duration,
		"flowFileId", flowFile.ID)

	return nil
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
