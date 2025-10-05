package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getEvaluateJSONPathInfo()
	plugin.RegisterBuiltInProcessor("EvaluateJSONPath", func() types.Processor {
		return NewEvaluateJSONPathProcessor()
	}, info)
}

func getEvaluateJSONPathInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"EvaluateJSONPath",
		"EvaluateJSONPath",
		"1.0.0",
		"DataBridge",
		"Evaluates JSONPath expressions against FlowFile JSON content and stores results in FlowFile attributes. Supports simple path expressions for extracting values from JSON documents.",
		[]string{"json", "attributes", "extraction", "jsonpath"},
	)
}

// EvaluateJSONPathProcessor extracts values from JSON using JSONPath expressions
type EvaluateJSONPathProcessor struct {
	*types.BaseProcessor
}

// NewEvaluateJSONPathProcessor creates a new EvaluateJSONPath processor
func NewEvaluateJSONPathProcessor() *EvaluateJSONPathProcessor {
	info := types.ProcessorInfo{
		Name:        "EvaluateJSONPath",
		Description: "Evaluates JSONPath expressions against JSON content and stores results in attributes",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"json", "attributes", "extraction", "jsonpath"},
		Properties: []types.PropertySpec{
			{
				Name:          "Destination",
				DisplayName:   "Destination",
				Description:   "Whether to write results to FlowFile attributes or content",
				Required:      true,
				DefaultValue:  "flowfile-attribute",
				AllowedValues: []string{"flowfile-attribute", "flowfile-content"},
				Type:          "select",
				HelpText:      "flowfile-attribute: Store as attributes; flowfile-content: Replace content with extracted value",
			},
			{
				Name:          "Return Type",
				DisplayName:   "Return Type",
				Description:   "Type of return value from JSONPath expressions",
				Required:      false,
				DefaultValue:  "auto-detect",
				AllowedValues: []string{"auto-detect", "json", "scalar"},
				Type:          "select",
				HelpText:      "auto-detect: Determine type automatically; json: Return as JSON string; scalar: Return as plain string",
			},
			{
				Name:          "Path Not Found Behavior",
				DisplayName:   "Path Not Found Behavior",
				Description:   "What to do when JSONPath doesn't match",
				Required:      false,
				DefaultValue:  "ignore",
				AllowedValues: []string{"ignore", "warn"},
				Type:          "select",
				HelpText:      "ignore: Skip silently; warn: Log warning message",
			},
			{
				Name:          "Null Value Representation",
				DisplayName:   "Null Value Representation",
				Description:   "How to represent JSON null values in attributes",
				Required:      false,
				DefaultValue:  "empty",
				AllowedValues: []string{"empty", "null"},
				Type:          "select",
				HelpText:      "empty: Use empty string; null: Use string 'null'",
			},
			{
				Name:         "JSONPath Expressions",
				DisplayName:  "JSONPath Expressions",
				Description:  "JSON map of attribute names to JSONPath expressions",
				Required:     true,
				DefaultValue: "{}",
				Type:         "multiline",
				Placeholder:  `{"user.name": "$.user.name", "user.email": "$.user.email"}`,
				HelpText:     "JSON object mapping attribute names to JSONPath expressions. Supports basic paths like $.field, $.nested.field, $.array[0]",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
			{
				Name:        "unmatched",
				Description: "FlowFiles that have no JSONPath matches",
			},
		},
	}

	return &EvaluateJSONPathProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *EvaluateJSONPathProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *EvaluateJSONPathProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get FlowFile from input
	flowFile := session.Get()
	if flowFile == nil {
		return nil
	}

	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("failed to get processor context")
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Parse JSON content
	var jsonData interface{}
	if err := json.Unmarshal(content, &jsonData); err != nil {
		logger.Error("Failed to parse JSON content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Get JSONPath expressions
	expressionsJSON := processorCtx.GetPropertyValue("JSONPath Expressions")
	if expressionsJSON == "" || expressionsJSON == "{}" {
		logger.Warn("No JSONPath expressions configured",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.RelationshipSuccess)
		return nil
	}

	// Parse expressions
	expressions, err := parseJSONPathExpressions(expressionsJSON)
	if err != nil {
		logger.Error("Failed to parse JSONPath expressions",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Get configuration
	destination := processorCtx.GetPropertyValue("Destination")
	returnType := processorCtx.GetPropertyValue("Return Type")
	pathNotFoundBehavior := processorCtx.GetPropertyValue("Path Not Found Behavior")
	nullValueRep := processorCtx.GetPropertyValue("Null Value Representation")

	// Evaluate expressions
	matchCount := 0
	results := make(map[string]string)

	for attrName, path := range expressions {
		value, found := evaluateJSONPath(jsonData, path)
		if !found {
			if pathNotFoundBehavior == "warn" {
				logger.Warn("JSONPath expression did not match",
					"flowFileId", flowFile.ID,
					"attribute", attrName,
					"path", path)
			}
			continue
		}

		matchCount++

		// Convert value to string based on return type
		strValue := formatJSONValue(value, returnType, nullValueRep)
		results[attrName] = strValue

		logger.Debug("JSONPath match found",
			"flowFileId", flowFile.ID,
			"attribute", attrName,
			"path", path,
			"value", strValue)
	}

	// Handle results based on destination
	if destination == "flowfile-attribute" {
		// Store results as attributes
		for attrName, value := range results {
			session.PutAttribute(flowFile, attrName, value)
		}
	} else {
		// Replace content with first result (or JSON map of all results)
		var outputContent []byte
		if len(results) == 1 {
			// Single result - just output the value
			for _, value := range results {
				outputContent = []byte(value)
			}
		} else {
			// Multiple results - output as JSON map
			outputContent, err = json.MarshalIndent(results, "", "  ")
			if err != nil {
				logger.Error("Failed to marshal results to JSON",
					"flowFileId", flowFile.ID,
					"error", err)
				session.Transfer(flowFile, types.RelationshipFailure)
				return err
			}
		}

		if err := session.Write(flowFile, outputContent); err != nil {
			logger.Error("Failed to write results to content",
				"flowFileId", flowFile.ID,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}
	}

	logger.Info("Successfully evaluated JSONPath expressions",
		"flowFileId", flowFile.ID,
		"matchCount", matchCount,
		"totalExpressions", len(expressions))

	// Route based on matches
	if matchCount == 0 {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched", Description: "No JSONPath matches"})
	} else {
		session.Transfer(flowFile, types.RelationshipSuccess)
	}

	return nil
}

// parseJSONPathExpressions parses JSON map of attribute names to paths
func parseJSONPathExpressions(expressionsJSON string) (map[string]string, error) {
	expressions := make(map[string]string)

	// Try proper JSON parsing first
	if err := json.Unmarshal([]byte(expressionsJSON), &expressions); err == nil {
		return expressions, nil
	}

	// Fallback to simple parser (same as UpdateAttribute)
	expressionsJSON = strings.Trim(expressionsJSON, "{}")
	expressionsJSON = strings.TrimSpace(expressionsJSON)

	if expressionsJSON == "" {
		return expressions, nil
	}

	pairs := strings.Split(expressionsJSON, ",")
	for _, pair := range pairs {
		colonIdx := strings.Index(pair, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(pair[:colonIdx])
		value := strings.TrimSpace(pair[colonIdx+1:])

		key = strings.Trim(key, "\"'")
		value = strings.Trim(value, "\"'")

		expressions[key] = value
	}

	return expressions, nil
}

// evaluateJSONPath evaluates a simple JSONPath expression
// Supports: $.field, $.nested.field, $.array[0], $.nested.array[0].field
func evaluateJSONPath(data interface{}, path string) (interface{}, bool) {
	// Remove leading $. or $
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")

	if path == "" {
		return data, true
	}

	// Split path into segments
	segments := parsePathSegments(path)

	current := data
	for _, segment := range segments {
		if segment.isArray {
			// Array access: field[index]
			arr, ok := getField(current, segment.field)
			if !ok {
				return nil, false
			}

			arrSlice, ok := arr.([]interface{})
			if !ok {
				return nil, false
			}

			if segment.index < 0 || segment.index >= len(arrSlice) {
				return nil, false
			}

			current = arrSlice[segment.index]
		} else {
			// Simple field access
			value, ok := getField(current, segment.field)
			if !ok {
				return nil, false
			}
			current = value
		}
	}

	return current, true
}

// pathSegment represents a segment of a JSONPath
type pathSegment struct {
	field   string
	isArray bool
	index   int
}

// parsePathSegments parses a JSONPath into segments
func parsePathSegments(path string) []pathSegment {
	segments := make([]pathSegment, 0)

	// Pattern to match field[index] or just field
	re := regexp.MustCompile(`([^.\[]+)(?:\[(\d+)\])?`)
	matches := re.FindAllStringSubmatch(path, -1)

	for _, match := range matches {
		if match[1] == "" {
			continue
		}

		segment := pathSegment{
			field: match[1],
		}

		if match[2] != "" {
			// Has array index
			segment.isArray = true
			fmt.Sscanf(match[2], "%d", &segment.index)
		}

		segments = append(segments, segment)
	}

	return segments
}

// getField gets a field from a JSON object
func getField(data interface{}, field string) (interface{}, bool) {
	switch v := data.(type) {
	case map[string]interface{}:
		value, ok := v[field]
		return value, ok
	default:
		return nil, false
	}
}

// formatJSONValue formats a JSON value as a string
func formatJSONValue(value interface{}, returnType string, nullValueRep string) string {
	if value == nil {
		if nullValueRep == "null" {
			return "null"
		}
		return ""
	}

	switch returnType {
	case "json":
		// Return as JSON string
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprintf("%v", value)
		}
		return string(jsonBytes)

	case "scalar":
		// Return as plain string
		return fmt.Sprintf("%v", value)

	case "auto-detect":
		fallthrough
	default:
		// Auto-detect: if it's a simple type, return as string; otherwise JSON
		switch value.(type) {
		case string, int, int64, float64, bool:
			return fmt.Sprintf("%v", value)
		default:
			jsonBytes, err := json.Marshal(value)
			if err != nil {
				return fmt.Sprintf("%v", value)
			}
			return string(jsonBytes)
		}
	}
}
