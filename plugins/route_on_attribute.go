package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getRouteOnAttributeInfo()
	plugin.RegisterBuiltInProcessor("RouteOnAttribute", func() types.Processor {
		return NewRouteOnAttributeProcessor()
	}, info)
}

func getRouteOnAttributeInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"RouteOnAttribute",
		"RouteOnAttribute",
		"1.0.0",
		"DataBridge",
		"Routes FlowFiles to different relationships based on attribute values. Supports multiple routing rules with expression language and pattern matching.",
		[]string{"routing", "attributes", "conditional", "filter"},
	)
}

// RouteOnAttributeProcessor routes FlowFiles based on attribute conditions
type RouteOnAttributeProcessor struct {
	*types.BaseProcessor
}

// NewRouteOnAttributeProcessor creates a new RouteOnAttribute processor
func NewRouteOnAttributeProcessor() *RouteOnAttributeProcessor {
	info := types.ProcessorInfo{
		Name:        "RouteOnAttribute",
		Description: "Routes FlowFiles to different relationships based on attribute values",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"routing", "attributes", "conditional", "filter"},
		Properties: []types.PropertySpec{
			{
				Name:         "Routing Strategy",
				DisplayName:  "Routing Strategy",
				Description:  "How to route FlowFiles when multiple rules match",
				Required:     true,
				DefaultValue: "Route to Property name",
				AllowedValues: []string{
					"Route to Property name",
					"Route to 'matched' if all match",
					"Route to 'matched' if any matches",
				},
				Type:     "select",
				HelpText: "Route to Property name: Create relationship for each matching rule; All match: Route to 'matched' only if all rules match; Any matches: Route to 'matched' if any rule matches",
			},
			{
				Name:         "Routing Rules",
				DisplayName:  "Routing Rules",
				Description:  "JSON map of relationship names to attribute conditions",
				Required:     true,
				DefaultValue: "{}",
				Type:         "multiline",
				Placeholder:  `{"large": "${file.size} > 1000000", "text": "${mime.type} == 'text/plain'"}`,
				HelpText:     "JSON object mapping relationship names to conditional expressions. Supports: ==, !=, >, <, >=, <=, matches (regex), contains, startsWith, endsWith",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "unmatched",
				Description: "FlowFiles that don't match any routing rule",
			},
			{
				Name:        "matched",
				Description: "FlowFiles that match routing criteria (when using 'matched' strategy)",
			},
			types.RelationshipFailure,
		},
	}

	return &RouteOnAttributeProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *RouteOnAttributeProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *RouteOnAttributeProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	routingStrategy := processorCtx.GetPropertyValue("Routing Strategy")
	rulesJSON := processorCtx.GetPropertyValue("Routing Rules")

	if rulesJSON == "" || rulesJSON == "{}" {
		logger.Warn("No routing rules configured, routing to unmatched",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.Relationship{Name: "unmatched", Description: "No routing rules"})
		return nil
	}

	// Parse routing rules
	rules, err := parseRoutingRules(rulesJSON)
	if err != nil {
		logger.Error("Failed to parse routing rules",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Evaluate rules
	matchedRules := make([]string, 0)
	for ruleName, condition := range rules {
		matched, err := evaluateCondition(condition, flowFile.Attributes)
		if err != nil {
			logger.Warn("Failed to evaluate routing rule",
				"flowFileId", flowFile.ID,
				"rule", ruleName,
				"condition", condition,
				"error", err)
			continue
		}

		if matched {
			matchedRules = append(matchedRules, ruleName)
			logger.Debug("Routing rule matched",
				"flowFileId", flowFile.ID,
				"rule", ruleName,
				"condition", condition)
		}
	}

	// Route based on strategy
	routed := false

	switch routingStrategy {
	case "Route to Property name":
		// Route to each matched rule's relationship
		if len(matchedRules) > 0 {
			for _, ruleName := range matchedRules {
				session.Transfer(flowFile, types.Relationship{
					Name:        ruleName,
					Description: fmt.Sprintf("Matched rule: %s", ruleName),
				})
				routed = true
			}
		}

	case "Route to 'matched' if all match":
		// Route to 'matched' only if ALL rules matched
		if len(matchedRules) == len(rules) && len(rules) > 0 {
			session.Transfer(flowFile, types.Relationship{Name: "matched", Description: "All rules matched"})
			routed = true
		}

	case "Route to 'matched' if any matches":
		// Route to 'matched' if ANY rule matched
		if len(matchedRules) > 0 {
			session.Transfer(flowFile, types.Relationship{Name: "matched", Description: "At least one rule matched"})
			routed = true
		}
	}

	// Route to unmatched if no rules matched
	if !routed {
		logger.Info("No routing rules matched, routing to unmatched",
			"flowFileId", flowFile.ID,
			"totalRules", len(rules))
		session.Transfer(flowFile, types.Relationship{Name: "unmatched", Description: "No rules matched"})
	} else {
		logger.Info("FlowFile routed successfully",
			"flowFileId", flowFile.ID,
			"matchedRules", len(matchedRules),
			"totalRules", len(rules))
	}

	return nil
}

// parseRoutingRules parses JSON map of rule names to conditions
func parseRoutingRules(rulesJSON string) (map[string]string, error) {
	rules := make(map[string]string)

	// Try proper JSON parsing first
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err == nil {
		return rules, nil
	}

	// Fallback to simple parser
	rulesJSON = strings.Trim(rulesJSON, "{}")
	rulesJSON = strings.TrimSpace(rulesJSON)

	if rulesJSON == "" {
		return rules, nil
	}

	pairs := strings.Split(rulesJSON, ",")
	for _, pair := range pairs {
		colonIdx := strings.Index(pair, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(pair[:colonIdx])
		value := strings.TrimSpace(pair[colonIdx+1:])

		key = strings.Trim(key, "\"'")
		value = strings.Trim(value, "\"'")

		rules[key] = value
	}

	return rules, nil
}

// evaluateCondition evaluates a conditional expression
// Supports: ==, !=, >, <, >=, <=, matches, contains, startsWith, endsWith
func evaluateCondition(condition string, attributes map[string]string) (bool, error) {
	// Resolve ${attribute} expressions first
	resolved := resolveExpression(condition, attributes)

	// Check for operators
	operators := []string{">=", "<=", "==", "!=", ">", "<"}
	for _, op := range operators {
		if strings.Contains(resolved, op) {
			parts := strings.SplitN(resolved, op, 2)
			if len(parts) != 2 {
				return false, fmt.Errorf("invalid condition format: %s", condition)
			}

			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])

			return evaluateComparison(left, op, right)
		}
	}

	// Check for string functions
	if strings.Contains(resolved, " matches ") {
		parts := strings.SplitN(resolved, " matches ", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid matches condition: %s", condition)
		}
		pattern := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %s", pattern)
		}
		return re.MatchString(strings.TrimSpace(parts[0])), nil
	}

	if strings.Contains(resolved, " contains ") {
		parts := strings.SplitN(resolved, " contains ", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid contains condition: %s", condition)
		}
		left := strings.TrimSpace(parts[0])
		right := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		return strings.Contains(left, right), nil
	}

	if strings.Contains(resolved, " startsWith ") {
		parts := strings.SplitN(resolved, " startsWith ", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid startsWith condition: %s", condition)
		}
		left := strings.TrimSpace(parts[0])
		right := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		return strings.HasPrefix(left, right), nil
	}

	if strings.Contains(resolved, " endsWith ") {
		parts := strings.SplitN(resolved, " endsWith ", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid endsWith condition: %s", condition)
		}
		left := strings.TrimSpace(parts[0])
		right := strings.Trim(strings.TrimSpace(parts[1]), "'\"")
		return strings.HasSuffix(left, right), nil
	}

	// If no operator found, treat as boolean expression
	// Non-empty string is true
	return resolved != "" && resolved != "false" && resolved != "0", nil
}

// evaluateComparison evaluates a comparison expression
func evaluateComparison(left, op, right string) (bool, error) {
	// Remove quotes from string literals
	left = strings.Trim(left, "'\"")
	right = strings.Trim(right, "'\"")

	// Try numeric comparison first
	leftNum, leftErr := strconv.ParseFloat(left, 64)
	rightNum, rightErr := strconv.ParseFloat(right, 64)

	if leftErr == nil && rightErr == nil {
		// Numeric comparison
		switch op {
		case "==":
			return leftNum == rightNum, nil
		case "!=":
			return leftNum != rightNum, nil
		case ">":
			return leftNum > rightNum, nil
		case "<":
			return leftNum < rightNum, nil
		case ">=":
			return leftNum >= rightNum, nil
		case "<=":
			return leftNum <= rightNum, nil
		}
	}

	// String comparison
	switch op {
	case "==":
		return left == right, nil
	case "!=":
		return left != right, nil
	case ">":
		return left > right, nil
	case "<":
		return left < right, nil
	case ">=":
		return left >= right, nil
	case "<=":
		return left <= right, nil
	}

	return false, fmt.Errorf("unsupported operator: %s", op)
}

