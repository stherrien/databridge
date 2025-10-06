package plugins

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getReplaceTextInfo()
	plugin.RegisterBuiltInProcessor("ReplaceText", func() types.Processor {
		return NewReplaceTextProcessor()
	}, info)
}

func getReplaceTextInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ReplaceText",
		"ReplaceText",
		"1.0.0",
		"DataBridge",
		"Performs regex-based text replacement and transformation on FlowFile content. Supports multiple replacement strategies including literal, regex, and line-by-line replacement.",
		[]string{"text", "transformation", "regex", "replacement"},
	)
}

// ReplaceTextProcessor performs text replacement operations
type ReplaceTextProcessor struct {
	*types.BaseProcessor
}

// NewReplaceTextProcessor creates a new ReplaceText processor
func NewReplaceTextProcessor() *ReplaceTextProcessor {
	info := types.ProcessorInfo{
		Name:        "ReplaceText",
		Description: "Performs regex-based text replacement and transformation on FlowFile content",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"text", "transformation", "regex", "replacement"},
		Properties: []types.PropertySpec{
			{
				Name:         "Replacement Strategy",
				DisplayName:  "Replacement Strategy",
				Description:  "Strategy for applying replacements",
				Required:     true,
				DefaultValue: "Regex Replace",
				AllowedValues: []string{
					"Regex Replace",
					"Literal Replace",
					"Always Replace",
					"Append",
					"Prepend",
				},
				Type:     "select",
				HelpText: "Regex Replace: Use regex pattern; Literal Replace: Exact string match; Always Replace: Replace entire content; Append/Prepend: Add text",
			},
			{
				Name:         "Search Value",
				DisplayName:  "Search Value",
				Description:  "The value to search for (regex pattern or literal string)",
				Required:     false,
				DefaultValue: "",
				Type:         "string",
				Placeholder:  "\\d{3}-\\d{3}-\\d{4}",
				HelpText:     "Regex pattern (Regex Replace) or literal string (Literal Replace) to search for",
			},
			{
				Name:         "Replacement Value",
				DisplayName:  "Replacement Value",
				Description:  "The value to replace with",
				Required:     true,
				DefaultValue: "",
				Type:         "multiline",
				Placeholder:  "replacement text or ${attribute}",
				HelpText:     "Text to replace with. Supports ${attribute} expressions and regex capture groups ($1, $2, etc.)",
			},
			{
				Name:          "Character Set",
				DisplayName:   "Character Set",
				Description:   "Character encoding for reading and writing content",
				Required:      false,
				DefaultValue:  "UTF-8",
				AllowedValues: []string{"UTF-8", "ASCII", "ISO-8859-1"},
				Type:          "select",
				HelpText:      "Character encoding to use when processing text",
			},
			{
				Name:         "Maximum Buffer Size",
				DisplayName:  "Maximum Buffer Size",
				Description:  "Maximum size of content to buffer in memory",
				Required:     false,
				DefaultValue: "1048576",
				Type:         "string",
				Placeholder:  "1048576",
				HelpText:     "Maximum bytes to buffer (default 1MB). Files larger will be streamed.",
			},
			{
				Name:          "Evaluation Mode",
				DisplayName:   "Evaluation Mode",
				Description:   "How to evaluate the replacement value",
				Required:      false,
				DefaultValue:  "Entire text",
				AllowedValues: []string{"Entire text", "Line-by-Line"},
				Type:          "select",
				HelpText:      "Entire text: Process whole content; Line-by-Line: Process each line separately",
			},
			{
				Name:          "Line-by-Line Evaluation Mode",
				DisplayName:   "Line-by-Line Evaluation Mode",
				Description:   "Whether to match all occurrences or only first in each line",
				Required:      false,
				DefaultValue:  "All",
				AllowedValues: []string{"All", "First-Line", "Last-Line", "Except-First-Line", "Except-Last-Line"},
				Type:          "select",
				HelpText:      "Controls which lines are processed in Line-by-Line mode",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &ReplaceTextProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *ReplaceTextProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *ReplaceTextProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	strategy := processorCtx.GetPropertyValue("Replacement Strategy")
	searchValue := processorCtx.GetPropertyValue("Search Value")
	replacementValue := processorCtx.GetPropertyValue("Replacement Value")
	evaluationMode := processorCtx.GetPropertyValue("Evaluation Mode")
	lineMode := processorCtx.GetPropertyValue("Line-by-Line Evaluation Mode")

	// Resolve expressions in replacement value
	replacementValue = resolveExpression(replacementValue, flowFile.Attributes)

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Apply replacement based on strategy
	var result string
	switch strategy {
	case "Regex Replace":
		result, err = p.regexReplace(string(content), searchValue, replacementValue, evaluationMode, lineMode)
	case "Literal Replace":
		result, err = p.literalReplace(string(content), searchValue, replacementValue, evaluationMode, lineMode)
	case "Always Replace":
		result = replacementValue
	case "Append":
		result = string(content) + replacementValue
	case "Prepend":
		result = replacementValue + string(content)
	default:
		err = fmt.Errorf("unsupported replacement strategy: %s", strategy)
	}

	if err != nil {
		logger.Error("Failed to apply replacement",
			"flowFileId", flowFile.ID,
			"strategy", strategy,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Write modified content
	if err := session.Write(flowFile, []byte(result)); err != nil {
		logger.Error("Failed to write modified content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	logger.Info("Successfully applied text replacement",
		"flowFileId", flowFile.ID,
		"strategy", strategy,
		"originalSize", len(content),
		"newSize", len(result))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}

// regexReplace performs regex-based replacement
func (p *ReplaceTextProcessor) regexReplace(content, pattern, replacement, evalMode, lineMode string) (string, error) {
	if pattern == "" {
		return content, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	if evalMode == "Line-by-Line" {
		return p.replaceLineByLine(content, func(line string) string {
			return re.ReplaceAllString(line, replacement)
		}, lineMode), nil
	}

	return re.ReplaceAllString(content, replacement), nil
}

// literalReplace performs literal string replacement
func (p *ReplaceTextProcessor) literalReplace(content, search, replacement, evalMode, lineMode string) (string, error) {
	if search == "" {
		return content, nil
	}

	if evalMode == "Line-by-Line" {
		return p.replaceLineByLine(content, func(line string) string {
			return strings.ReplaceAll(line, search, replacement)
		}, lineMode), nil
	}

	return strings.ReplaceAll(content, search, replacement), nil
}

// replaceLineByLine applies a replacement function line by line
func (p *ReplaceTextProcessor) replaceLineByLine(content string, replaceFn func(string) string, mode string) string {
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		shouldProcess := false

		switch mode {
		case "All":
			shouldProcess = true
		case "First-Line":
			shouldProcess = (i == 0)
		case "Last-Line":
			shouldProcess = (i == len(lines)-1)
		case "Except-First-Line":
			shouldProcess = (i != 0)
		case "Except-Last-Line":
			shouldProcess = (i != len(lines)-1)
		}

		if shouldProcess {
			lines[i] = replaceFn(line)
		}
	}

	return strings.Join(lines, "\n")
}
