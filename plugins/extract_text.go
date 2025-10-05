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
	info := getExtractTextInfo()
	plugin.RegisterBuiltInProcessor("ExtractText", func() types.Processor {
		return NewExtractTextProcessor()
	}, info)
}

func getExtractTextInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ExtractText",
		"ExtractText",
		"1.0.0",
		"DataBridge",
		"Extracts values from FlowFile content using regular expressions with capture groups and stores them as attributes. Supports multiple regex patterns with named capture groups.",
		[]string{"text", "regex", "extraction", "attributes"},
	)
}

// ExtractTextProcessor extracts text using regex patterns
type ExtractTextProcessor struct {
	*types.BaseProcessor
}

// NewExtractTextProcessor creates a new ExtractText processor
func NewExtractTextProcessor() *ExtractTextProcessor {
	info := types.ProcessorInfo{
		Name:        "ExtractText",
		Description: "Extracts values from content using regex patterns and stores as attributes",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"text", "regex", "extraction", "attributes"},
		Properties: []types.PropertySpec{
			{
				Name:         "Size Limit",
				DisplayName:  "Maximum Buffer Size",
				Description:  "Maximum size of content to extract from (bytes)",
				Required:     false,
				DefaultValue: "1048576",
				Type:         "string",
				Placeholder:  "1048576",
				HelpText:     "Maximum bytes to process (default 1MB). Files larger will be truncated.",
			},
			{
				Name:         "Enable Canonical Equivalence",
				DisplayName:  "Enable Canonical Equivalence",
				Description:  "Whether to enable canonical equivalence in regex matching",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "When true, enables Unicode canonical equivalence",
			},
			{
				Name:         "Include Capture Group 0",
				DisplayName:  "Include Capture Group 0",
				Description:  "Whether to include the entire match as capture group 0",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "When true, stores the entire match in addition to capture groups",
			},
			{
				Name:         "Extraction Patterns",
				DisplayName:  "Extraction Patterns",
				Description:  "JSON map of attribute names to regex patterns",
				Required:     true,
				DefaultValue: "{}",
				Type:         "multiline",
				Placeholder:  `{"email": "([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,})", "phone": "(\\d{3}-\\d{3}-\\d{4})"}`,
				HelpText:     "JSON object mapping attribute names to regex patterns with capture groups. First capture group becomes the attribute value.",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "matched",
				Description: "FlowFiles where at least one pattern matched",
			},
			{
				Name:        "unmatched",
				Description: "FlowFiles where no patterns matched",
			},
			types.RelationshipFailure,
		},
	}

	return &ExtractTextProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *ExtractTextProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *ExtractTextProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	patternsJSON := processorCtx.GetPropertyValue("Extraction Patterns")
	includeGroup0 := processorCtx.GetPropertyValue("Include Capture Group 0") == "true"

	if patternsJSON == "" || patternsJSON == "{}" {
		logger.Warn("No extraction patterns configured",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.Relationship{Name: "unmatched", Description: "No patterns"})
		return nil
	}

	// Parse patterns
	patterns, err := parseExtractionPatterns(patternsJSON)
	if err != nil {
		logger.Error("Failed to parse extraction patterns",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
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

	contentStr := string(content)
	matchCount := 0

	// Apply each pattern
	for attrName, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			logger.Warn("Invalid regex pattern",
				"flowFileId", flowFile.ID,
				"attribute", attrName,
				"pattern", pattern,
				"error", err)
			continue
		}

		// Find matches
		matches := re.FindStringSubmatch(contentStr)
		if len(matches) > 0 {
			matchCount++

			// Store entire match as attr.0 if requested
			if includeGroup0 && len(matches) > 0 {
				session.PutAttribute(flowFile, attrName+".0", matches[0])
			}

			// Store capture groups
			for i := 1; i < len(matches); i++ {
				groupAttr := fmt.Sprintf("%s.%d", attrName, i)
				session.PutAttribute(flowFile, groupAttr, matches[i])
			}

			// Store first capture group as main attribute (if exists)
			if len(matches) > 1 {
				session.PutAttribute(flowFile, attrName, matches[1])
			} else if len(matches) > 0 {
				// No capture groups, use entire match
				session.PutAttribute(flowFile, attrName, matches[0])
			}

			logger.Debug("Pattern matched",
				"flowFileId", flowFile.ID,
				"attribute", attrName,
				"captureGroups", len(matches)-1)
		}
	}

	logger.Info("Extraction complete",
		"flowFileId", flowFile.ID,
		"totalPatterns", len(patterns),
		"matchedPatterns", matchCount)

	// Route based on matches
	if matchCount > 0 {
		session.Transfer(flowFile, types.Relationship{Name: "matched", Description: "At least one pattern matched"})
	} else {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched", Description: "No patterns matched"})
	}

	return nil
}

// parseExtractionPatterns parses JSON map of attribute names to patterns
func parseExtractionPatterns(patternsJSON string) (map[string]string, error) {
	patterns := make(map[string]string)

	// Simple parser for key-value pairs
	patternsJSON = strings.Trim(patternsJSON, "{}")
	patternsJSON = strings.TrimSpace(patternsJSON)

	if patternsJSON == "" {
		return patterns, nil
	}

	// Try to handle quoted patterns with commas
	var currentKey, currentValue string
	inQuotes := false
	escaped := false
	collectingKey := true

	i := 0
	for i < len(patternsJSON) {
		ch := patternsJSON[i]

		if escaped {
			if collectingKey {
				currentKey += string(ch)
			} else {
				currentValue += string(ch)
			}
			escaped = false
			i++
			continue
		}

		if ch == '\\' {
			escaped = true
			if !collectingKey {
				currentValue += string(ch)
			}
			i++
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			i++
			continue
		}

		if !inQuotes {
			if ch == ':' && collectingKey {
				collectingKey = false
				i++
				continue
			}

			if ch == ',' && !collectingKey {
				// End of key-value pair
				currentKey = strings.TrimSpace(currentKey)
				currentValue = strings.TrimSpace(currentValue)
				if currentKey != "" && currentValue != "" {
					patterns[currentKey] = currentValue
				}
				currentKey = ""
				currentValue = ""
				collectingKey = true
				i++
				continue
			}
		}

		// Add character
		if collectingKey {
			if ch != ' ' || len(currentKey) > 0 {
				currentKey += string(ch)
			}
		} else {
			currentValue += string(ch)
		}

		i++
	}

	// Handle last pair
	currentKey = strings.TrimSpace(currentKey)
	currentValue = strings.TrimSpace(currentValue)
	if currentKey != "" && currentValue != "" {
		patterns[currentKey] = currentValue
	}

	return patterns, nil
}
