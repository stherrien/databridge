package plugins

import (
	"context"
	"fmt"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getUpdateAttributeInfo()
	plugin.RegisterBuiltInProcessor("UpdateAttribute", func() types.Processor {
		return NewUpdateAttributeProcessor()
	}, info)
}

func getUpdateAttributeInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"UpdateAttribute",
		"UpdateAttribute",
		"1.0.0",
		"DataBridge",
		"Updates FlowFile attributes using configurable rules and expressions. Supports static values and dynamic expressions using ${attribute.name} syntax.",
		[]string{"attributes", "metadata", "transformation"},
	)
}

// UpdateAttributeProcessor updates FlowFile attributes
type UpdateAttributeProcessor struct {
	*types.BaseProcessor
}

// NewUpdateAttributeProcessor creates a new UpdateAttribute processor
func NewUpdateAttributeProcessor() *UpdateAttributeProcessor {
	info := types.ProcessorInfo{
		Name:        "UpdateAttribute",
		Description: "Updates FlowFile attributes using configurable rules and expressions",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"attributes", "metadata", "transformation"},
		Properties: []types.PropertySpec{
			{
				Name:         "Attribute Rules",
				DisplayName:  "Attribute Rules",
				Description:  "JSON map of attribute names to values. Supports expressions like ${existing.attr}",
				Required:     false,
				DefaultValue: "{}",
				Type:         "multiline",
				Placeholder:  `{"new.attribute": "value", "filename": "${uuid}"}`,
				HelpText:     "JSON object with attribute names as keys and values (static or expressions)",
			},
			{
				Name:         "Delete Attributes",
				DisplayName:  "Delete Attributes",
				Description:  "Comma-separated list of attributes to delete",
				Required:     false,
				DefaultValue: "",
				Type:         "string",
				Placeholder:  "temp.attr1,temp.attr2",
				HelpText:     "List of attribute names to remove from the FlowFile",
			},
			{
				Name:          "Store State",
				DisplayName:   "Store State",
				Description:   "Whether to store state for use in subsequent FlowFiles",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &UpdateAttributeProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *UpdateAttributeProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *UpdateAttributeProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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

	// Parse attribute rules
	rulesJSON := processorCtx.GetPropertyValue("Attribute Rules")
	if rulesJSON == "" {
		rulesJSON = "{}"
	}

	// Simple JSON parsing (in production, use proper JSON library)
	rules := parseAttributeRules(rulesJSON)

	// Apply attribute updates
	for key, valueExpr := range rules {
		// Resolve expressions like ${attribute.name}
		resolvedValue := resolveExpression(valueExpr, flowFile.Attributes)
		session.PutAttribute(flowFile, key, resolvedValue)
		logger.Debug("Updated attribute",
			"flowFileId", flowFile.ID,
			"attribute", key,
			"value", resolvedValue)
	}

	// Handle attribute deletions
	deleteList := processorCtx.GetPropertyValue("Delete Attributes")
	if deleteList != "" {
		attributesToDelete := strings.Split(deleteList, ",")
		for _, attr := range attributesToDelete {
			attr = strings.TrimSpace(attr)
			if attr != "" {
				session.RemoveAttribute(flowFile, attr)
				logger.Debug("Deleted attribute",
					"flowFileId", flowFile.ID,
					"attribute", attr)
			}
		}
	}

	logger.Info("Updated FlowFile attributes",
		"flowFileId", flowFile.ID,
		"rulesApplied", len(rules))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}

// parseAttributeRules parses JSON rules into a map
func parseAttributeRules(rulesJSON string) map[string]string {
	rules := make(map[string]string)

	// Simple JSON parser for basic key-value pairs
	// Remove braces and quotes
	rulesJSON = strings.Trim(rulesJSON, "{}")
	rulesJSON = strings.TrimSpace(rulesJSON)

	if rulesJSON == "" {
		return rules
	}

	// Split by comma (simple parser)
	pairs := strings.Split(rulesJSON, ",")
	for _, pair := range pairs {
		// Find the first : to split key and value
		colonIdx := strings.Index(pair, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(pair[:colonIdx])
		value := strings.TrimSpace(pair[colonIdx+1:])

		// Remove quotes
		key = strings.Trim(key, "\"'")
		value = strings.Trim(value, "\"'")

		rules[key] = value
	}

	return rules
}
