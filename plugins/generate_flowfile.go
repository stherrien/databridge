package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	// Register this processor as a built-in
	info := getGenerateFlowFileInfo()
	plugin.RegisterBuiltInProcessor("GenerateFlowFile", func() types.Processor {
		return NewGenerateFlowFileProcessor()
	}, info)
}

func getGenerateFlowFileInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"GenerateFlowFile",
		"GenerateFlowFile",
		"1.0.0",
		"DataBridge",
		"Generates FlowFiles with configurable content at regular intervals",
		[]string{"generate", "source", "testing"},
	)
}

// GenerateFlowFileProcessor creates FlowFiles with configurable content
type GenerateFlowFileProcessor struct {
	*types.BaseProcessor
}

// NewGenerateFlowFileProcessor creates a new GenerateFlowFile processor
func NewGenerateFlowFileProcessor() *GenerateFlowFileProcessor {
	info := types.ProcessorInfo{
		Name:        "GenerateFlowFile",
		Description: "Generates FlowFiles with configurable content at regular intervals",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"generate", "source", "testing"},
		Properties: []types.PropertySpec{
			{
				Name:         "File Size",
				Description:  "Size of the generated content in bytes",
				Required:     true,
				DefaultValue: "1024",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Content",
				Description:  "Content to generate (will be repeated to reach file size)",
				Required:     false,
				DefaultValue: "Hello, DataBridge!",
			},
			{
				Name:         "Unique FlowFiles",
				Description:  "Whether to generate unique content for each FlowFile",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Custom Text",
				Description:  "Custom text to include in generated content",
				Required:     false,
				DefaultValue: "",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
		},
	}

	return &GenerateFlowFileProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *GenerateFlowFileProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing GenerateFlowFile processor")

	// Validate required properties
	if !ctx.HasProperty("File Size") {
		return fmt.Errorf("File Size property is required")
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *GenerateFlowFileProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get processor context to access properties
	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		return fmt.Errorf("processor context not available")
	}

	// Read configuration properties
	fileSizeStr := processorCtx.GetPropertyValue("File Size")
	content := processorCtx.GetPropertyValue("Content")
	uniqueStr := processorCtx.GetPropertyValue("Unique FlowFiles")
	customText := processorCtx.GetPropertyValue("Custom Text")

	// Parse file size
	var fileSize int64 = 1024 // default
	if fileSizeStr != "" {
		if _, err := fmt.Sscanf(fileSizeStr, "%d", &fileSize); err != nil {
			logger.Warn("Invalid File Size, using default", "fileSize", fileSizeStr, "error", err)
		}
	}

	// Create new FlowFile
	flowFile := session.Create()

	// Generate content
	generatedContent := p.generateContent(content, customText, uniqueStr == "true", fileSize)

	// Write content to FlowFile
	if err := session.Write(flowFile, []byte(generatedContent)); err != nil {
		logger.Error("Failed to write content to FlowFile", "error", err)
		session.Remove(flowFile)
		return fmt.Errorf("failed to write content: %w", err)
	}

	// Set standard attributes
	session.PutAttribute(flowFile, "filename", fmt.Sprintf("generated_%s.txt", flowFile.ID.String()[:8]))
	session.PutAttribute(flowFile, "generated.timestamp", time.Now().Format(time.RFC3339))
	session.PutAttribute(flowFile, "generated.size", fmt.Sprintf("%d", fileSize))
	session.PutAttribute(flowFile, "generator.type", "GenerateFlowFile")

	// Add custom text as attribute if provided
	if customText != "" {
		session.PutAttribute(flowFile, "custom.text", customText)
	}

	// Transfer to success relationship
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Generated FlowFile",
		"flowFileId", flowFile.ID,
		"size", fileSize,
		"filename", fmt.Sprintf("generated_%s.txt", flowFile.ID.String()[:8]))

	return nil
}

// Validate validates the processor configuration
func (p *GenerateFlowFileProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Additional validation
	if fileSizeStr, exists := config.Properties["File Size"]; exists {
		var fileSize int64
		if _, err := fmt.Sscanf(fileSizeStr, "%d", &fileSize); err != nil || fileSize <= 0 {
			results = append(results, types.ValidationResult{
				Property: "File Size",
				Valid:    false,
				Message:  "File Size must be a positive integer",
			})
		} else if fileSize > 1024*1024*100 { // 100MB limit
			results = append(results, types.ValidationResult{
				Property: "File Size",
				Valid:    false,
				Message:  "File Size cannot exceed 100MB",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *GenerateFlowFileProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed for this processor
}

// generateContent generates content based on configuration
func (p *GenerateFlowFileProcessor) generateContent(baseContent, customText string, unique bool, targetSize int64) string {
	if baseContent == "" {
		baseContent = "Hello, DataBridge!"
	}

	// Add custom text if provided
	if customText != "" {
		baseContent = fmt.Sprintf("%s - %s", baseContent, customText)
	}

	// Add unique identifier if requested
	if unique {
		baseContent = fmt.Sprintf("[%s] %s", uuid.New().String()[:8], baseContent)
	}

	// Add timestamp
	baseContent = fmt.Sprintf("[%s] %s", time.Now().Format("2006-01-02 15:04:05"), baseContent)

	// Repeat content to reach target size
	var result string
	for int64(len(result)) < targetSize {
		if int64(len(result)+len(baseContent)) <= targetSize {
			result += baseContent + "\n"
		} else {
			// Add partial content to reach exact size
			remaining := targetSize - int64(len(result))
			result += baseContent[:remaining]
			break
		}
	}

	return result
}