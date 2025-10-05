package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getSplitTextInfo()
	plugin.RegisterBuiltInProcessor("SplitText", func() types.Processor {
		return NewSplitTextProcessor()
	}, info)
}

func getSplitTextInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"SplitText",
		"SplitText",
		"1.0.0",
		"DataBridge",
		"Splits text content into multiple FlowFiles based on line count or size",
		[]string{"text", "split", "transform"},
	)
}

// SplitTextProcessor splits text content into multiple FlowFiles
type SplitTextProcessor struct {
	*types.BaseProcessor
}

// Define splits relationship
var (
	RelationshipSplits = types.Relationship{
		Name:        "splits",
		Description: "Individual split FlowFiles",
	}
)

// NewSplitTextProcessor creates a new SplitText processor
func NewSplitTextProcessor() *SplitTextProcessor {
	info := types.ProcessorInfo{
		Name:        "SplitText",
		Description: "Splits text content into multiple FlowFiles based on line count or size",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"text", "split", "transform"},
		Properties: []types.PropertySpec{
			{
				Name:         "Line Split Count",
				Description:  "Number of lines per split",
				Required:     false,
				DefaultValue: "1",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Remove Trailing Newlines",
				Description:  "Whether to remove trailing newlines from splits",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:         "Header Line Count",
				Description:  "Number of header lines to prepend to each split",
				Required:     false,
				DefaultValue: "0",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Maximum Fragment Size",
				Description:  "Maximum size in bytes per fragment (0 = unlimited)",
				Required:     false,
				DefaultValue: "0",
				Pattern:      `^\d+$`,
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,  // Original FlowFile
			RelationshipSplits,         // Split FlowFiles
			types.RelationshipFailure,
		},
	}

	return &SplitTextProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *SplitTextProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing SplitText processor")

	// Validate line split count
	lineSplitStr := ctx.GetPropertyValue("Line Split Count")
	if lineSplitStr != "" {
		lineCount, err := strconv.Atoi(lineSplitStr)
		if err != nil || lineCount < 1 {
			return fmt.Errorf("Line Split Count must be a positive integer")
		}
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *SplitTextProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	lineSplitStr := processorCtx.GetPropertyValue("Line Split Count")
	lineSplitCount := 1
	if lineSplitStr != "" {
		if val, err := strconv.Atoi(lineSplitStr); err == nil && val > 0 {
			lineSplitCount = val
		}
	}

	removeTrailing := processorCtx.GetPropertyValue("Remove Trailing Newlines") != "false"

	headerCountStr := processorCtx.GetPropertyValue("Header Line Count")
	headerCount := 0
	if headerCountStr != "" {
		if val, err := strconv.Atoi(headerCountStr); err == nil && val >= 0 {
			headerCount = val
		}
	}

	maxFragmentSizeStr := processorCtx.GetPropertyValue("Maximum Fragment Size")
	maxFragmentSize := 0
	if maxFragmentSizeStr != "" {
		if val, err := strconv.Atoi(maxFragmentSizeStr); err == nil && val >= 0 {
			maxFragmentSize = val
		}
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Split content into lines
	lines := p.splitLines(string(content))

	if len(lines) == 0 {
		logger.Debug("No lines to split")
		session.Transfer(flowFile, types.RelationshipSuccess)
		return nil
	}

	// Extract header lines
	var headerLines []string
	var dataLines []string

	if headerCount > 0 && len(lines) > headerCount {
		headerLines = lines[:headerCount]
		dataLines = lines[headerCount:]
	} else {
		dataLines = lines
	}

	// Split into fragments
	fragments := p.createFragments(dataLines, headerLines, lineSplitCount, maxFragmentSize, removeTrailing)

	if len(fragments) == 0 {
		logger.Debug("No fragments created")
		session.Transfer(flowFile, types.RelationshipSuccess)
		return nil
	}

	// Generate fragment identifier (parent UUID)
	fragmentIdentifier := flowFile.ID.String()

	// Create split FlowFiles
	for i, fragment := range fragments {
		splitFlowFile := session.CreateChild(flowFile)

		// Write fragment content
		if err := session.Write(splitFlowFile, []byte(fragment)); err != nil {
			logger.Error("Failed to write fragment", "index", i, "error", err)
			session.Remove(splitFlowFile)
			continue
		}

		// Set fragment attributes
		session.PutAttribute(splitFlowFile, "fragment.index", strconv.Itoa(i))
		session.PutAttribute(splitFlowFile, "fragment.count", strconv.Itoa(len(fragments)))
		session.PutAttribute(splitFlowFile, "fragment.identifier", fragmentIdentifier)
		session.PutAttribute(splitFlowFile, "segment.original.filename", flowFile.Attributes["filename"])

		// Copy relevant attributes from original
		if filename, exists := flowFile.GetAttribute("filename"); exists {
			// Generate fragment filename
			ext := ""
			base := filename
			if dotIdx := strings.LastIndex(filename, "."); dotIdx != -1 {
				ext = filename[dotIdx:]
				base = filename[:dotIdx]
			}
			fragmentFilename := fmt.Sprintf("%s_split_%d%s", base, i, ext)
			session.PutAttribute(splitFlowFile, "filename", fragmentFilename)
		}

		// Transfer to splits relationship
		session.Transfer(splitFlowFile, RelationshipSplits)

		logger.Debug("Created split FlowFile",
			"index", i,
			"size", len(fragment),
			"splitFlowFileId", splitFlowFile.ID)
	}

	// Transfer original to success
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Successfully split text",
		"originalFlowFileId", flowFile.ID,
		"fragmentCount", len(fragments),
		"lineSplitCount", lineSplitCount)

	return nil
}

// splitLines splits content into lines, handling different line endings
func (p *SplitTextProcessor) splitLines(content string) []string {
	// Replace CRLF with LF first
	content = strings.ReplaceAll(content, "\r\n", "\n")
	// Replace remaining CR with LF
	content = strings.ReplaceAll(content, "\r", "\n")

	// Split by LF
	lines := strings.Split(content, "\n")

	// Remove empty last line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// createFragments creates text fragments based on configuration
func (p *SplitTextProcessor) createFragments(dataLines, headerLines []string, linesPerSplit, maxSize int, removeTrailing bool) []string {
	var fragments []string

	// Calculate header size if present
	headerText := ""
	if len(headerLines) > 0 {
		headerText = strings.Join(headerLines, "\n") + "\n"
	}

	for i := 0; i < len(dataLines); i += linesPerSplit {
		end := i + linesPerSplit
		if end > len(dataLines) {
			end = len(dataLines)
		}

		// Get lines for this fragment
		fragmentLines := dataLines[i:end]

		// Build fragment content
		var fragmentContent strings.Builder

		// Add header if configured
		if headerText != "" {
			fragmentContent.WriteString(headerText)
		}

		// Add data lines
		fragmentContent.WriteString(strings.Join(fragmentLines, "\n"))

		// Add trailing newline unless removal is requested
		if !removeTrailing {
			fragmentContent.WriteString("\n")
		}

		content := fragmentContent.String()

		// Check max fragment size
		if maxSize > 0 && len(content) > maxSize {
			// Fragment too large, split it further
			truncated := content[:maxSize]
			fragments = append(fragments, truncated)
		} else {
			fragments = append(fragments, content)
		}
	}

	return fragments
}

// Validate validates the processor configuration
func (p *SplitTextProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate Line Split Count
	if countStr, exists := config.Properties["Line Split Count"]; exists && countStr != "" {
		count, err := strconv.Atoi(countStr)
		if err != nil || count < 1 {
			results = append(results, types.ValidationResult{
				Property: "Line Split Count",
				Valid:    false,
				Message:  "Line Split Count must be a positive integer",
			})
		}
	}

	// Validate Header Line Count
	if countStr, exists := config.Properties["Header Line Count"]; exists && countStr != "" {
		count, err := strconv.Atoi(countStr)
		if err != nil || count < 0 {
			results = append(results, types.ValidationResult{
				Property: "Header Line Count",
				Valid:    false,
				Message:  "Header Line Count must be a non-negative integer",
			})
		}
	}

	// Validate Maximum Fragment Size
	if sizeStr, exists := config.Properties["Maximum Fragment Size"]; exists && sizeStr != "" {
		size, err := strconv.Atoi(sizeStr)
		if err != nil || size < 0 {
			results = append(results, types.ValidationResult{
				Property: "Maximum Fragment Size",
				Valid:    false,
				Message:  "Maximum Fragment Size must be a non-negative integer",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *SplitTextProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed for this processor
}
