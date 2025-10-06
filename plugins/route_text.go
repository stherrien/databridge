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
	info := getRouteTextInfo()
	plugin.RegisterBuiltInProcessor("RouteText", func() types.Processor {
		return NewRouteTextProcessor()
	}, info)
}

func getRouteTextInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"RouteText",
		"RouteText",
		"1.0.0",
		"DataBridge",
		"Routes FlowFiles based on text content patterns using regex matching. FlowFiles are routed to dynamically created relationships based on matching rules.",
		[]string{"routing", "text", "regex", "pattern", "match"},
	)
}

// RouteTextProcessor routes FlowFiles based on text content patterns
type RouteTextProcessor struct {
	*types.BaseProcessor
	compiledPatterns map[string]*regexp.Regexp
}

// NewRouteTextProcessor creates a new RouteText processor
func NewRouteTextProcessor() *RouteTextProcessor {
	info := types.ProcessorInfo{
		Name:        "RouteText",
		Description: "Routes FlowFiles based on text content patterns",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"routing", "text", "regex", "pattern", "match"},
		Properties: []types.PropertySpec{
			{
				Name:          "Matching Strategy",
				DisplayName:   "Matching Strategy",
				Description:   "How to match the content against patterns",
				Required:      true,
				DefaultValue:  "Contains",
				AllowedValues: []string{"Contains", "Starts With", "Ends With", "Equals", "Regex"},
				Type:          "select",
				HelpText:      "Contains: Pattern appears anywhere; Starts With: Begins with pattern; Ends With: Ends with pattern; Equals: Exact match; Regex: Regular expression",
			},
			{
				Name:        "Route Pattern 1",
				DisplayName: "Route Pattern 1",
				Description: "Pattern to match for route 1",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'ERROR' or '^ERROR:.*'",
				HelpText:    "FlowFiles matching this pattern will be routed to 'match_1'",
			},
			{
				Name:         "Route Name 1",
				DisplayName:  "Route Name 1",
				Description:  "Name for route 1 relationship",
				Required:     false,
				DefaultValue: "match_1",
				Type:         "string",
				Placeholder:  "match_1",
				HelpText:     "Relationship name for FlowFiles matching pattern 1",
			},
			{
				Name:        "Route Pattern 2",
				DisplayName: "Route Pattern 2",
				Description: "Pattern to match for route 2",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'WARNING'",
				HelpText:    "FlowFiles matching this pattern will be routed to 'match_2'",
			},
			{
				Name:         "Route Name 2",
				DisplayName:  "Route Name 2",
				Description:  "Name for route 2 relationship",
				Required:     false,
				DefaultValue: "match_2",
				Type:         "string",
				Placeholder:  "match_2",
				HelpText:     "Relationship name for FlowFiles matching pattern 2",
			},
			{
				Name:        "Route Pattern 3",
				DisplayName: "Route Pattern 3",
				Description: "Pattern to match for route 3",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'INFO'",
				HelpText:    "FlowFiles matching this pattern will be routed to 'match_3'",
			},
			{
				Name:         "Route Name 3",
				DisplayName:  "Route Name 3",
				Description:  "Name for route 3 relationship",
				Required:     false,
				DefaultValue: "match_3",
				Type:         "string",
				Placeholder:  "match_3",
				HelpText:     "Relationship name for FlowFiles matching pattern 3",
			},
			{
				Name:          "Case Sensitive",
				DisplayName:   "Case Sensitive",
				Description:   "Whether pattern matching is case sensitive",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, matching is case sensitive. When false, case is ignored.",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "unmatched",
				Description: "FlowFiles that don't match any pattern",
			},
			types.RelationshipFailure,
		},
	}

	return &RouteTextProcessor{
		BaseProcessor:    types.NewBaseProcessor(info),
		compiledPatterns: make(map[string]*regexp.Regexp),
	}
}

// Initialize initializes the processor
func (p *RouteTextProcessor) Initialize(ctx types.ProcessorContext) error {
	// Pre-compile regex patterns if strategy is Regex
	strategy := ctx.GetPropertyValue("Matching Strategy")
	if strategy == "Regex" {
		for i := 1; i <= 3; i++ {
			patternKey := fmt.Sprintf("Route Pattern %d", i)
			pattern := ctx.GetPropertyValue(patternKey)
			if pattern != "" {
				re, err := regexp.Compile(pattern)
				if err != nil {
					return fmt.Errorf("invalid regex pattern %d: %w", i, err)
				}
				p.compiledPatterns[patternKey] = re
			}
		}
	}
	return nil
}

// OnTrigger processes FlowFiles
func (p *RouteTextProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	strategy := processorCtx.GetPropertyValue("Matching Strategy")
	caseSensitive := processorCtx.GetPropertyValue("Case Sensitive") == "true"

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
	if !caseSensitive {
		contentStr = strings.ToLower(contentStr)
	}

	// Check each route pattern
	matched := false
	for i := 1; i <= 3; i++ {
		patternKey := fmt.Sprintf("Route Pattern %d", i)
		nameKey := fmt.Sprintf("Route Name %d", i)

		pattern := processorCtx.GetPropertyValue(patternKey)
		if pattern == "" {
			continue
		}

		routeName := processorCtx.GetPropertyValue(nameKey)
		if routeName == "" {
			routeName = fmt.Sprintf("match_%d", i)
		}

		if !caseSensitive && strategy != "Regex" {
			pattern = strings.ToLower(pattern)
		}

		isMatch := false
		switch strategy {
		case "Contains":
			isMatch = strings.Contains(contentStr, pattern)
		case "Starts With":
			isMatch = strings.HasPrefix(contentStr, pattern)
		case "Ends With":
			isMatch = strings.HasSuffix(contentStr, pattern)
		case "Equals":
			isMatch = contentStr == pattern
		case "Regex":
			if re, exists := p.compiledPatterns[patternKey]; exists {
				isMatch = re.MatchString(string(content)) // Use original content for regex
			}
		}

		if isMatch {
			session.PutAttribute(flowFile, "route.matched", routeName)
			session.PutAttribute(flowFile, "route.pattern", pattern)
			session.Transfer(flowFile, types.Relationship{Name: routeName})
			matched = true
			logger.Info("FlowFile matched pattern",
				"flowFileId", flowFile.ID,
				"route", routeName,
				"pattern", pattern)
			break
		}
	}

	if !matched {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched"})
		logger.Debug("FlowFile did not match any pattern",
			"flowFileId", flowFile.ID)
	}

	return nil
}
