package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getRouteJSONInfo()
	plugin.RegisterBuiltInProcessor("RouteJSON", func() types.Processor {
		return NewRouteJSONProcessor()
	}, info)
}

func getRouteJSONInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"RouteJSON",
		"RouteJSON",
		"1.0.0",
		"DataBridge",
		"Routes FlowFiles based on JSON structure and values using JSONPath expressions. FlowFiles are routed to dynamically created relationships based on matching rules.",
		[]string{"routing", "json", "jsonpath", "conditional"},
	)
}

// RouteJSONProcessor routes FlowFiles based on JSON content
type RouteJSONProcessor struct {
	*types.BaseProcessor
}

// NewRouteJSONProcessor creates a new RouteJSON processor
func NewRouteJSONProcessor() *RouteJSONProcessor {
	info := types.ProcessorInfo{
		Name:        "RouteJSON",
		Description: "Routes FlowFiles based on JSON structure and values",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"routing", "json", "jsonpath", "conditional"},
		Properties: []types.PropertySpec{
			{
				Name:        "Route Path 1",
				DisplayName: "Route Path 1",
				Description: "JSON path to evaluate for route 1",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'status' or 'data.level'",
				HelpText:    "Dot notation path to JSON field (e.g., 'user.name', 'items[0].id')",
			},
			{
				Name:        "Route Value 1",
				DisplayName: "Route Value 1",
				Description: "Expected value for route 1",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'active'",
				HelpText:    "FlowFiles where path equals this value will be routed to 'match_1'",
			},
			{
				Name:         "Route Name 1",
				DisplayName:  "Route Name 1",
				Description:  "Name for route 1 relationship",
				Required:     false,
				DefaultValue: "match_1",
				Type:         "string",
				Placeholder:  "match_1",
				HelpText:     "Relationship name for FlowFiles matching condition 1",
			},
			{
				Name:        "Route Path 2",
				DisplayName: "Route Path 2",
				Description: "JSON path to evaluate for route 2",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'type'",
				HelpText:    "Dot notation path to JSON field",
			},
			{
				Name:        "Route Value 2",
				DisplayName: "Route Value 2",
				Description: "Expected value for route 2",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'error'",
				HelpText:    "FlowFiles where path equals this value will be routed to 'match_2'",
			},
			{
				Name:         "Route Name 2",
				DisplayName:  "Route Name 2",
				Description:  "Name for route 2 relationship",
				Required:     false,
				DefaultValue: "match_2",
				Type:         "string",
				Placeholder:  "match_2",
				HelpText:     "Relationship name for FlowFiles matching condition 2",
			},
			{
				Name:        "Route Path 3",
				DisplayName: "Route Path 3",
				Description: "JSON path to evaluate for route 3",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'priority'",
				HelpText:    "Dot notation path to JSON field",
			},
			{
				Name:        "Route Value 3",
				DisplayName: "Route Value 3",
				Description: "Expected value for route 3",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'high'",
				HelpText:    "FlowFiles where path equals this value will be routed to 'match_3'",
			},
			{
				Name:         "Route Name 3",
				DisplayName:  "Route Name 3",
				Description:  "Name for route 3 relationship",
				Required:     false,
				DefaultValue: "match_3",
				Type:         "string",
				Placeholder:  "match_3",
				HelpText:     "Relationship name for FlowFiles matching condition 3",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "unmatched",
				Description: "FlowFiles that don't match any condition",
			},
			types.RelationshipFailure,
		},
	}

	return &RouteJSONProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *RouteJSONProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *RouteJSONProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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

	// Parse JSON
	var jsonData interface{}
	if err := json.Unmarshal(content, &jsonData); err != nil {
		logger.Error("Failed to parse JSON",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Check each route condition
	matched := false
	for i := 1; i <= 3; i++ {
		pathKey := fmt.Sprintf("Route Path %d", i)
		valueKey := fmt.Sprintf("Route Value %d", i)
		nameKey := fmt.Sprintf("Route Name %d", i)

		path := processorCtx.GetPropertyValue(pathKey)
		if path == "" {
			continue
		}

		expectedValue := processorCtx.GetPropertyValue(valueKey)
		routeName := processorCtx.GetPropertyValue(nameKey)
		if routeName == "" {
			routeName = fmt.Sprintf("match_%d", i)
		}

		// Extract value from JSON using path
		actualValue, err := getJSONValue(jsonData, path)
		if err != nil {
			logger.Debug("Failed to extract JSON value",
				"flowFileId", flowFile.ID,
				"path", path,
				"error", err)
			continue
		}

		// Convert actual value to string for comparison
		actualStr := fmt.Sprintf("%v", actualValue)

		if actualStr == expectedValue {
			session.PutAttribute(flowFile, "route.matched", routeName)
			session.PutAttribute(flowFile, "route.path", path)
			session.PutAttribute(flowFile, "route.value", actualStr)
			session.Transfer(flowFile, types.Relationship{Name: routeName})
			matched = true
			logger.Info("FlowFile matched JSON condition",
				"flowFileId", flowFile.ID,
				"route", routeName,
				"path", path,
				"value", actualStr)
			break
		}
	}

	if !matched {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched"})
		logger.Debug("FlowFile did not match any condition",
			"flowFileId", flowFile.ID)
	}

	return nil
}

// getJSONValue extracts a value from JSON using a simple dot notation path
func getJSONValue(data interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil, fmt.Errorf("path not found: %s", part)
			}
			current = val
		case []interface{}:
			// Simple array index support (e.g., "items[0]")
			// For now, we'll just return error for arrays
			return nil, fmt.Errorf("array indexing not yet supported")
		default:
			return nil, fmt.Errorf("cannot traverse through %T", current)
		}
	}

	return current, nil
}
