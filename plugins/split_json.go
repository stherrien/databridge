package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getSplitJSONInfo()
	plugin.RegisterBuiltInProcessor("SplitJSON", func() types.Processor {
		return NewSplitJSONProcessor()
	}, info)
}

func getSplitJSONInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"SplitJSON",
		"SplitJSON",
		"1.0.0",
		"DataBridge",
		"Splits a JSON array into individual FlowFiles, one for each array element. Creates a new FlowFile for each JSON object in the array.",
		[]string{"json", "split", "array", "segmentation"},
	)
}

// SplitJSONProcessor splits JSON arrays into individual FlowFiles
type SplitJSONProcessor struct {
	*types.BaseProcessor
}

// NewSplitJSONProcessor creates a new SplitJSON processor
func NewSplitJSONProcessor() *SplitJSONProcessor {
	info := types.ProcessorInfo{
		Name:        "SplitJSON",
		Description: "Splits a JSON array into individual FlowFiles, one for each array element",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"json", "split", "array", "segmentation"},
		Properties: []types.PropertySpec{
			{
				Name:         "JSON Path Expression",
				DisplayName:  "JSON Path Expression",
				Description:  "JSONPath expression to locate the array to split",
				Required:     false,
				DefaultValue: "$",
				Type:         "string",
				Placeholder:  "$.data.items",
				HelpText:     "JSONPath to the array. Default '$' means root array. Use $.field for nested arrays.",
			},
			{
				Name:         "Null Value Representation",
				DisplayName:  "Null Value Representation",
				Description:  "How to handle JSON null values",
				Required:     false,
				DefaultValue: "empty",
				AllowedValues: []string{"empty", "null"},
				Type:         "select",
				HelpText:     "empty: Convert to empty string; null: Keep as JSON null",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "splits",
				Description: "Individual JSON elements from the array",
			},
			{
				Name:        "original",
				Description: "The original FlowFile",
			},
			types.RelationshipFailure,
		},
	}

	return &SplitJSONProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *SplitJSONProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *SplitJSONProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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

	// Get configuration
	jsonPath := processorCtx.GetPropertyValue("JSON Path Expression")
	if jsonPath == "" {
		jsonPath = "$"
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
		logger.Error("Failed to parse JSON content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Navigate to array using JSONPath
	arrayData, err := p.navigateToArray(jsonData, jsonPath)
	if err != nil {
		logger.Error("Failed to navigate to array",
			"flowFileId", flowFile.ID,
			"jsonPath", jsonPath,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	if len(arrayData) == 0 {
		logger.Warn("Empty array, routing to original",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.Relationship{Name: "original", Description: "Original FlowFile"})
		return nil
	}

	// Create split FlowFiles for each array element
	splitRelationship := types.Relationship{Name: "splits", Description: "Split FlowFiles"}

	for i, element := range arrayData {
		// Serialize element to JSON
		elementJSON, err := json.MarshalIndent(element, "", "  ")
		if err != nil {
			logger.Error("Failed to serialize JSON element",
				"flowFileId", flowFile.ID,
				"index", i,
				"error", err)
			continue
		}

		// Create new FlowFile for split
		splitFlowFile := session.Create()

		// Copy attributes from original
		for key, value := range flowFile.Attributes {
			session.PutAttribute(splitFlowFile, key, value)
		}

		// Add split metadata
		session.PutAttribute(splitFlowFile, "split.index", fmt.Sprintf("%d", i))
		session.PutAttribute(splitFlowFile, "split.count", fmt.Sprintf("%d", len(arrayData)))
		session.PutAttribute(splitFlowFile, "split.parent.uuid", flowFile.ID.String())
		session.PutAttribute(splitFlowFile, "fragment.index", fmt.Sprintf("%d", i))
		session.PutAttribute(splitFlowFile, "fragment.count", fmt.Sprintf("%d", len(arrayData)))
		session.PutAttribute(splitFlowFile, "mime.type", "application/json")

		// Write split content
		if err := session.Write(splitFlowFile, elementJSON); err != nil {
			logger.Error("Failed to write split content",
				"flowFileId", flowFile.ID,
				"splitIndex", i,
				"error", err)
			continue
		}

		session.Transfer(splitFlowFile, splitRelationship)
	}

	// Route original to original relationship
	session.Transfer(flowFile, types.Relationship{Name: "original", Description: "Original FlowFile"})

	logger.Info("Successfully split JSON array",
		"flowFileId", flowFile.ID,
		"splitCount", len(arrayData))

	return nil
}

// navigateToArray navigates to an array using a simple JSONPath expression
func (p *SplitJSONProcessor) navigateToArray(data interface{}, path string) ([]interface{}, error) {
	// Handle root array
	if path == "$" || path == "" {
		if arr, ok := data.([]interface{}); ok {
			return arr, nil
		}
		return nil, fmt.Errorf("root is not an array")
	}

	// Simple path navigation ($.field or $.field.nested)
	current := data
	path = path[1:] // Remove leading $

	if path[0] == '.' {
		path = path[1:] // Remove leading dot
	}

	fields := splitPath(path)
	for _, field := range fields {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot navigate through non-object at field: %s", field)
		}

		next, exists := obj[field]
		if !exists {
			return nil, fmt.Errorf("field not found: %s", field)
		}

		current = next
	}

	// Verify final value is an array
	arr, ok := current.([]interface{})
	if !ok {
		return nil, fmt.Errorf("path does not point to an array")
	}

	return arr, nil
}

// splitPath splits a path like "data.items" into ["data", "items"]
func splitPath(path string) []string {
	if path == "" {
		return nil
	}

	var parts []string
	current := ""
	inBracket := false

	for i := 0; i < len(path); i++ {
		ch := path[i]

		if ch == '[' {
			inBracket = true
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else if ch == ']' {
			inBracket = false
		} else if ch == '.' && !inBracket {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}
