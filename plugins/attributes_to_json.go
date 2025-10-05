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
	info := getAttributesToJSONInfo()
	plugin.RegisterBuiltInProcessor("AttributesToJSON", func() types.Processor {
		return NewAttributesToJSONProcessor()
	}, info)
}

func getAttributesToJSONInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"AttributesToJSON",
		"AttributesToJSON",
		"1.0.0",
		"DataBridge",
		"Converts FlowFile attributes into JSON format. Can output as content or a single attribute. Supports filtering attributes by name pattern.",
		[]string{"attributes", "json", "conversion", "metadata"},
	)
}

// AttributesToJSONProcessor converts attributes to JSON
type AttributesToJSONProcessor struct {
	*types.BaseProcessor
}

// NewAttributesToJSONProcessor creates a new AttributesToJSON processor
func NewAttributesToJSONProcessor() *AttributesToJSONProcessor {
	info := types.ProcessorInfo{
		Name:        "AttributesToJSON",
		Description: "Converts FlowFile attributes into JSON format",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"attributes", "json", "conversion", "metadata"},
		Properties: []types.PropertySpec{
			{
				Name:         "Destination",
				DisplayName:  "Destination",
				Description:  "Where to write the JSON output",
				Required:     true,
				DefaultValue: "flowfile-content",
				AllowedValues: []string{"flowfile-attribute", "flowfile-content"},
				Type:         "select",
				HelpText:     "flowfile-attribute: Write to a single attribute; flowfile-content: Replace FlowFile content",
			},
			{
				Name:         "Attributes List",
				DisplayName:  "Attributes List",
				Description:  "Comma-separated list of attributes to include (empty = all)",
				Required:     false,
				DefaultValue: "",
				Type:         "string",
				Placeholder:  "filename,path,size",
				HelpText:     "Leave empty to include all attributes, or specify comma-separated list",
			},
			{
				Name:         "Attributes Regular Expression",
				DisplayName:  "Attributes Regular Expression",
				Description:  "Regular expression to filter attribute names",
				Required:     false,
				DefaultValue: "",
				Type:         "string",
				Placeholder:  "^(filename|path).*",
				HelpText:     "Only attributes matching this regex will be included",
			},
			{
				Name:         "Destination Attribute Name",
				DisplayName:  "Destination Attribute Name",
				Description:  "Name of attribute to write JSON to (when destination is flowfile-attribute)",
				Required:     false,
				DefaultValue: "JSONAttributes",
				Type:         "string",
				Placeholder:  "JSONAttributes",
				HelpText:     "Attribute name for JSON output when using flowfile-attribute destination",
			},
			{
				Name:         "Include Core Attributes",
				DisplayName:  "Include Core Attributes",
				Description:  "Whether to include core attributes like uuid, filename",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "When false, filters out standard FlowFile attributes",
			},
			{
				Name:         "Null Value for Empty Attribute",
				DisplayName:  "Null Value for Empty Attribute",
				Description:  "Whether to use JSON null for empty string attributes",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "When true, empty strings become JSON null values",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &AttributesToJSONProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *AttributesToJSONProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *AttributesToJSONProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	destination := processorCtx.GetPropertyValue("Destination")
	attributesList := processorCtx.GetPropertyValue("Attributes List")
	destinationAttr := processorCtx.GetPropertyValue("Destination Attribute Name")
	includeCoreAttrs := processorCtx.GetPropertyValue("Include Core Attributes") == "true"
	nullForEmpty := processorCtx.GetPropertyValue("Null Value for Empty Attribute") == "true"

	if destinationAttr == "" {
		destinationAttr = "JSONAttributes"
	}

	// Build attribute filter
	var attributeNames []string
	if attributesList != "" {
		parts := strings.Split(attributesList, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				attributeNames = append(attributeNames, trimmed)
			}
		}
	}

	// Filter attributes
	jsonMap := make(map[string]interface{})
	coreAttributes := map[string]bool{
		"uuid": true, "filename": true, "path": true,
		"mime.type": true, "fragment.index": true, "fragment.count": true,
	}

	for key, value := range flowFile.Attributes {
		// Check if attribute should be included
		shouldInclude := false

		if len(attributeNames) > 0 {
			// Check if in explicit list
			for _, attrName := range attributeNames {
				if key == attrName {
					shouldInclude = true
					break
				}
			}
		} else {
			// Include all by default
			shouldInclude = true
		}

		// Filter out core attributes if requested
		if !includeCoreAttrs && coreAttributes[key] {
			shouldInclude = false
		}

		if shouldInclude {
			// Handle empty values
			if value == "" && nullForEmpty {
				jsonMap[key] = nil
			} else {
				jsonMap[key] = value
			}
		}
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(jsonMap, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal attributes to JSON",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Write based on destination
	if destination == "flowfile-content" {
		// Replace content with JSON
		if err := session.Write(flowFile, jsonBytes); err != nil {
			logger.Error("Failed to write JSON content",
				"flowFileId", flowFile.ID,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}
		session.PutAttribute(flowFile, "mime.type", "application/json")
	} else {
		// Write to attribute
		session.PutAttribute(flowFile, destinationAttr, string(jsonBytes))
	}

	logger.Info("Successfully converted attributes to JSON",
		"flowFileId", flowFile.ID,
		"destination", destination,
		"attributeCount", len(jsonMap))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}
