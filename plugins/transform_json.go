package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/pkg/types"
)

// TransformJSONProcessor transforms JSON using path expressions
type TransformJSONProcessor struct {
	*types.BaseProcessor
}

// NewTransformJSONProcessor creates a new TransformJSON processor
func NewTransformJSONProcessor() *TransformJSONProcessor {
	info := types.ProcessorInfo{
		Name:        "TransformJSON",
		Description: "Extracts and transforms JSON data using simplified JSONPath expressions",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"json", "transform", "extract", "parse"},
		Properties: []types.PropertySpec{
			{
				Name:         "JSON Path Expression",
				Description:  "Simplified JSONPath expression (e.g., .data.items[0].name, .users[*].email)",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:         "Destination",
				Description:  "Where to put the result",
				Required:     false,
				DefaultValue: "flowfile-content",
				AllowedValues: []string{"flowfile-content", "flowfile-attribute"},
			},
			{
				Name:         "Destination Attribute",
				Description:  "Attribute name when destination is flowfile-attribute",
				Required:     false,
				DefaultValue: "json.result",
			},
			{
				Name:         "Return Type",
				Description:  "How to format the result",
				Required:     false,
				DefaultValue: "auto",
				AllowedValues: []string{"auto", "json", "string"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &TransformJSONProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *TransformJSONProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing TransformJSON processor")

	// Validate required properties
	if !ctx.HasProperty("JSON Path Expression") {
		return fmt.Errorf("JSON Path Expression property is required")
	}

	pathExpr := ctx.GetPropertyValue("JSON Path Expression")
	if pathExpr == "" {
		return fmt.Errorf("JSON Path Expression cannot be empty")
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *TransformJSONProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	pathExpr := processorCtx.GetPropertyValue("JSON Path Expression")
	destination := processorCtx.GetPropertyValue("Destination")
	if destination == "" {
		destination = "flowfile-content"
	}
	destAttribute := processorCtx.GetPropertyValue("Destination Attribute")
	if destAttribute == "" {
		destAttribute = "json.result"
	}
	returnType := processorCtx.GetPropertyValue("Return Type")
	if returnType == "" {
		returnType = "auto"
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Parse JSON
	var data interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		logger.Error("Failed to parse JSON", "error", err)
		session.PutAttribute(flowFile, "json.error", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Apply JSONPath expression
	result, err := p.applyJSONPath(data, pathExpr)
	if err != nil {
		logger.Error("Failed to apply JSONPath", "error", err, "path", pathExpr)
		session.PutAttribute(flowFile, "json.error", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Format result based on return type
	formattedResult, err := p.formatResult(result, returnType)
	if err != nil {
		logger.Error("Failed to format result", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Write result to destination
	if destination == "flowfile-content" {
		if err := session.Write(flowFile, []byte(formattedResult)); err != nil {
			logger.Error("Failed to write result to FlowFile", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return nil
		}
	} else {
		session.PutAttribute(flowFile, destAttribute, formattedResult)
	}

	// Add metadata attributes
	session.PutAttribute(flowFile, "json.path", pathExpr)
	session.PutAttribute(flowFile, "json.destination", destination)

	// Transfer to success
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Successfully transformed JSON",
		"path", pathExpr,
		"destination", destination,
		"flowFileId", flowFile.ID)

	return nil
}

// applyJSONPath applies a simplified JSONPath expression
func (p *TransformJSONProcessor) applyJSONPath(data interface{}, path string) (interface{}, error) {
	// Remove leading dot if present
	if strings.HasPrefix(path, ".") {
		path = path[1:]
	}

	if path == "" {
		return data, nil
	}

	// Split path into segments
	segments := p.parsePathSegments(path)

	current := data
	for _, segment := range segments {
		var err error
		current, err = p.navigateSegment(current, segment)
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

// parsePathSegments splits a path into segments
func (p *TransformJSONProcessor) parsePathSegments(path string) []string {
	var segments []string
	var current strings.Builder
	inBracket := false

	for i := 0; i < len(path); i++ {
		ch := path[i]

		if ch == '[' {
			if current.Len() > 0 {
				segments = append(segments, current.String())
				current.Reset()
			}
			inBracket = true
			current.WriteByte(ch)
		} else if ch == ']' {
			current.WriteByte(ch)
			if inBracket {
				segments = append(segments, current.String())
				current.Reset()
				inBracket = false
			}
		} else if ch == '.' && !inBracket {
			if current.Len() > 0 {
				segments = append(segments, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments
}

// navigateSegment navigates through one segment of the path
func (p *TransformJSONProcessor) navigateSegment(data interface{}, segment string) (interface{}, error) {
	// Handle array access: [0], [*]
	if strings.HasPrefix(segment, "[") && strings.HasSuffix(segment, "]") {
		inner := segment[1 : len(segment)-1]

		// Handle wildcard [*]
		if inner == "*" {
			arr, ok := data.([]interface{})
			if !ok {
				return nil, fmt.Errorf("cannot use [*] on non-array")
			}
			return arr, nil
		}

		// Handle numeric index
		index, err := strconv.Atoi(inner)
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %s", inner)
		}

		arr, ok := data.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot index non-array with [%d]", index)
		}

		if index < 0 || index >= len(arr) {
			return nil, fmt.Errorf("array index out of bounds: %d (length: %d)", index, len(arr))
		}

		return arr[index], nil
	}

	// Handle object property access
	obj, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("cannot access property '%s' on non-object", segment)
	}

	value, exists := obj[segment]
	if !exists {
		return nil, fmt.Errorf("property '%s' not found", segment)
	}

	return value, nil
}

// formatResult formats the result based on return type
func (p *TransformJSONProcessor) formatResult(result interface{}, returnType string) (string, error) {
	switch returnType {
	case "string":
		// Convert to string representation
		return fmt.Sprintf("%v", result), nil

	case "json":
		// Convert to JSON
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to marshal to JSON: %w", err)
		}
		return string(jsonBytes), nil

	case "auto":
		// Auto-detect: if it's a string, return as-is; otherwise JSON
		if str, ok := result.(string); ok {
			return str, nil
		}
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to marshal to JSON: %w", err)
		}
		return string(jsonBytes), nil

	default:
		return "", fmt.Errorf("unknown return type: %s", returnType)
	}
}

// Validate validates the processor configuration
func (p *TransformJSONProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate JSON Path Expression
	if pathExpr, exists := config.Properties["JSON Path Expression"]; exists && pathExpr != "" {
		// Basic validation - check for balanced brackets
		openBrackets := strings.Count(pathExpr, "[")
		closeBrackets := strings.Count(pathExpr, "]")
		if openBrackets != closeBrackets {
			results = append(results, types.ValidationResult{
				Property: "JSON Path Expression",
				Valid:    false,
				Message:  "Unbalanced brackets in path expression",
			})
		}
	}

	// Validate Destination
	if dest, exists := config.Properties["Destination"]; exists {
		validDests := []string{"flowfile-content", "flowfile-attribute"}
		valid := false
		for _, vd := range validDests {
			if dest == vd {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "Destination",
				Valid:    false,
				Message:  "Destination must be flowfile-content or flowfile-attribute",
			})
		}
	}

	// Validate Return Type
	if returnType, exists := config.Properties["Return Type"]; exists {
		validTypes := []string{"auto", "json", "string"}
		valid := false
		for _, vt := range validTypes {
			if returnType == vt {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "Return Type",
				Valid:    false,
				Message:  "Return Type must be auto, json, or string",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *TransformJSONProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed for this processor
}
