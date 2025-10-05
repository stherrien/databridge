package customtransform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Register this processor as a built-in plugin
func init() {
	info := plugin.NewProcessorInfo(
		"CustomTransform",
		"Custom Transform Processor",
		"1.0.0",
		"DataBridge Examples",
		"Transforms FlowFile content using configurable rules",
		[]string{"transform", "example", "custom"},
	)

	plugin.RegisterBuiltInProcessor("CustomTransform", func() types.Processor {
		return NewCustomTransformProcessor()
	}, info)
}

// CustomTransformProcessor transforms FlowFile content
type CustomTransformProcessor struct {
	*types.BaseProcessor
}

// TransformRule defines a transformation rule
type TransformRule struct {
	Type       string `json:"type"`       // uppercase, lowercase, replace, prefix, suffix
	Target     string `json:"target"`     // what to replace (for replace type)
	Value      string `json:"value"`      // replacement value
	ApplyToKey bool   `json:"applyToKey"` // apply to attribute keys vs values
}

// NewCustomTransformProcessor creates a new transform processor
func NewCustomTransformProcessor() *CustomTransformProcessor {
	info := types.ProcessorInfo{
		Name:        "CustomTransform",
		Description: "Transforms FlowFile content and attributes using configurable rules",
		Version:     "1.0.0",
		Author:      "DataBridge Examples",
		Tags:        []string{"transform", "example", "custom"},
		Properties: []types.PropertySpec{
			{
				Name:         "Transform Type",
				Description:  "Type of transformation to apply",
				Required:     true,
				DefaultValue: "uppercase",
				AllowedValues: []string{
					"uppercase",
					"lowercase",
					"replace",
					"prefix",
					"suffix",
					"json-pretty",
				},
			},
			{
				Name:         "Replace Target",
				Description:  "String to replace (for 'replace' transform type)",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:         "Replace Value",
				Description:  "Replacement string (for 'replace', 'prefix', or 'suffix' types)",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:          "Transform Content",
				Description:   "Whether to transform FlowFile content",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:          "Transform Attributes",
				Description:   "Whether to transform FlowFile attributes",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Attribute Pattern",
				Description:  "Only transform attributes matching this pattern (empty = all)",
				Required:     false,
				DefaultValue: "",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
			{
				Name:        "invalid",
				Description: "FlowFiles with invalid content",
			},
		},
	}

	return &CustomTransformProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *CustomTransformProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing CustomTransform processor")

	// Validate configuration
	if !ctx.HasProperty("Transform Type") {
		return fmt.Errorf("Transform Type property is required")
	}

	transformType := ctx.GetPropertyValue("Transform Type")
	if transformType == "replace" {
		if !ctx.HasProperty("Replace Target") {
			return fmt.Errorf("Replace Target is required for 'replace' transform type")
		}
	}

	return nil
}

// OnTrigger processes FlowFiles
func (p *CustomTransformProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get processor context
	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		return fmt.Errorf("processor context not available")
	}

	// Get FlowFile
	flowFile := session.Get()
	if flowFile == nil {
		return nil
	}

	// Read configuration
	transformType := processorCtx.GetPropertyValue("Transform Type")
	replaceTarget := processorCtx.GetPropertyValue("Replace Target")
	replaceValue := processorCtx.GetPropertyValue("Replace Value")
	transformContent := processorCtx.GetPropertyValue("Transform Content") == "true"
	transformAttrs := processorCtx.GetPropertyValue("Transform Attributes") == "true"
	attrPattern := processorCtx.GetPropertyValue("Attribute Pattern")

	logger.Info("Transforming FlowFile",
		"flowFileId", flowFile.ID,
		"transformType", transformType,
		"transformContent", transformContent,
		"transformAttrs", transformAttrs)

	// Transform content if requested
	if transformContent {
		content, err := session.Read(flowFile)
		if err != nil {
			logger.Error("Failed to read FlowFile content", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		transformedContent, err := p.transformData(content, transformType, replaceTarget, replaceValue)
		if err != nil {
			logger.Error("Failed to transform content", "error", err)
			session.PutAttribute(flowFile, "transform.error", err.Error())
			session.Transfer(flowFile, types.Relationship{Name: "invalid"})
			return nil
		}

		if err := session.Write(flowFile, transformedContent); err != nil {
			logger.Error("Failed to write transformed content", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}
	}

	// Transform attributes if requested
	if transformAttrs {
		p.transformAttributes(flowFile, session, transformType, attrPattern, replaceTarget, replaceValue)
	}

	// Add transformation metadata
	session.PutAttribute(flowFile, "transform.type", transformType)
	session.PutAttribute(flowFile, "transform.timestamp", time.Now().Format(time.RFC3339))
	session.PutAttribute(flowFile, "transform.processor", "CustomTransform")

	// Transfer to success
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Successfully transformed FlowFile", "flowFileId", flowFile.ID)
	return nil
}

// Validate validates the processor configuration
func (p *CustomTransformProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate transform type
	transformType, exists := config.Properties["Transform Type"]
	if exists {
		validTypes := []string{"uppercase", "lowercase", "replace", "prefix", "suffix", "json-pretty"}
		valid := false
		for _, t := range validTypes {
			if transformType == t {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "Transform Type",
				Valid:    false,
				Message:  fmt.Sprintf("Invalid transform type: %s", transformType),
			})
		}

		// Validate replace-specific properties
		if transformType == "replace" {
			if target, exists := config.Properties["Replace Target"]; !exists || target == "" {
				results = append(results, types.ValidationResult{
					Property: "Replace Target",
					Valid:    false,
					Message:  "Replace Target is required for 'replace' transform type",
				})
			}
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *CustomTransformProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed
}

// Private helper methods

func (p *CustomTransformProcessor) transformData(data []byte, transformType, target, value string) ([]byte, error) {
	content := string(data)

	switch transformType {
	case "uppercase":
		return []byte(strings.ToUpper(content)), nil

	case "lowercase":
		return []byte(strings.ToLower(content)), nil

	case "replace":
		return []byte(strings.ReplaceAll(content, target, value)), nil

	case "prefix":
		return []byte(value + content), nil

	case "suffix":
		return []byte(content + value), nil

	case "json-pretty":
		// Try to parse as JSON and pretty print
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to format JSON: %w", err)
		}
		return prettyJSON, nil

	default:
		return nil, fmt.Errorf("unknown transform type: %s", transformType)
	}
}

func (p *CustomTransformProcessor) transformAttributes(
	flowFile *types.FlowFile,
	session types.ProcessSession,
	transformType, pattern, target, value string,
) {
	for key, val := range flowFile.Attributes {
		// Check pattern if specified
		if pattern != "" && !strings.Contains(key, pattern) {
			continue
		}

		// Transform attribute value
		var transformedValue string
		switch transformType {
		case "uppercase":
			transformedValue = strings.ToUpper(val)
		case "lowercase":
			transformedValue = strings.ToLower(val)
		case "replace":
			transformedValue = strings.ReplaceAll(val, target, value)
		case "prefix":
			transformedValue = value + val
		case "suffix":
			transformedValue = val + value
		default:
			transformedValue = val
		}

		session.PutAttribute(flowFile, key, transformedValue)
	}
}
