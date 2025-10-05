package plugins

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getFilterAttributeInfo()
	plugin.RegisterBuiltInProcessor("FilterAttribute", func() types.Processor {
		return NewFilterAttributeProcessor()
	}, info)
}

func getFilterAttributeInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"FilterAttribute",
		"FilterAttribute",
		"1.0.0",
		"DataBridge",
		"Filters FlowFiles based on attribute values using various comparison operators. FlowFiles that match the criteria are routed to 'matched', others to 'unmatched'.",
		[]string{"filter", "attribute", "conditional", "comparison"},
	)
}

// FilterAttributeProcessor filters FlowFiles based on attribute values
type FilterAttributeProcessor struct {
	*types.BaseProcessor
}

// NewFilterAttributeProcessor creates a new FilterAttribute processor
func NewFilterAttributeProcessor() *FilterAttributeProcessor {
	info := types.ProcessorInfo{
		Name:        "FilterAttribute",
		Description: "Filters FlowFiles based on attribute values",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"filter", "attribute", "conditional", "comparison"},
		Properties: []types.PropertySpec{
			{
				Name:          "Filter Mode",
				DisplayName:   "Filter Mode",
				Description:   "How to combine multiple filter rules",
				Required:      true,
				DefaultValue:  "AND",
				AllowedValues: []string{"AND", "OR"},
				Type:          "select",
				HelpText:      "AND: All rules must match; OR: Any rule must match",
			},
			{
				Name:        "Rule 1 Attribute",
				DisplayName: "Rule 1 Attribute",
				Description: "Attribute name for rule 1",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'filename'",
				HelpText:    "Name of the attribute to evaluate",
			},
			{
				Name:          "Rule 1 Operator",
				DisplayName:   "Rule 1 Operator",
				Description:   "Comparison operator for rule 1",
				Required:      false,
				DefaultValue:  "Equals",
				AllowedValues: []string{"Equals", "Not Equals", "Contains", "Starts With", "Ends With", "Regex", "Greater Than", "Less Than", "Exists", "Not Exists"},
				Type:          "select",
				HelpText:      "Comparison operation to perform",
			},
			{
				Name:        "Rule 1 Value",
				DisplayName: "Rule 1 Value",
				Description: "Value to compare against for rule 1",
				Required:    false,
				Type:        "string",
				Placeholder: "comparison value",
				HelpText:    "Value to compare the attribute against (not used for Exists/Not Exists)",
			},
			{
				Name:        "Rule 2 Attribute",
				DisplayName: "Rule 2 Attribute",
				Description: "Attribute name for rule 2",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'mime.type'",
				HelpText:    "Name of the attribute to evaluate",
			},
			{
				Name:          "Rule 2 Operator",
				DisplayName:   "Rule 2 Operator",
				Description:   "Comparison operator for rule 2",
				Required:      false,
				DefaultValue:  "Equals",
				AllowedValues: []string{"Equals", "Not Equals", "Contains", "Starts With", "Ends With", "Regex", "Greater Than", "Less Than", "Exists", "Not Exists"},
				Type:          "select",
				HelpText:      "Comparison operation to perform",
			},
			{
				Name:        "Rule 2 Value",
				DisplayName: "Rule 2 Value",
				Description: "Value to compare against for rule 2",
				Required:    false,
				Type:        "string",
				Placeholder: "comparison value",
				HelpText:    "Value to compare the attribute against (not used for Exists/Not Exists)",
			},
			{
				Name:        "Rule 3 Attribute",
				DisplayName: "Rule 3 Attribute",
				Description: "Attribute name for rule 3",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'file.size'",
				HelpText:    "Name of the attribute to evaluate",
			},
			{
				Name:          "Rule 3 Operator",
				DisplayName:   "Rule 3 Operator",
				Description:   "Comparison operator for rule 3",
				Required:      false,
				DefaultValue:  "Equals",
				AllowedValues: []string{"Equals", "Not Equals", "Contains", "Starts With", "Ends With", "Regex", "Greater Than", "Less Than", "Exists", "Not Exists"},
				Type:          "select",
				HelpText:      "Comparison operation to perform",
			},
			{
				Name:        "Rule 3 Value",
				DisplayName: "Rule 3 Value",
				Description: "Value to compare against for rule 3",
				Required:    false,
				Type:        "string",
				Placeholder: "comparison value",
				HelpText:    "Value to compare the attribute against (not used for Exists/Not Exists)",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "matched",
				Description: "FlowFiles that match the filter criteria",
			},
			{
				Name:        "unmatched",
				Description: "FlowFiles that don't match the filter criteria",
			},
			types.RelationshipFailure,
		},
	}

	return &FilterAttributeProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *FilterAttributeProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *FilterAttributeProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	filterMode := processorCtx.GetPropertyValue("Filter Mode")

	// Evaluate each rule
	var ruleResults []bool
	for i := 1; i <= 3; i++ {
		attrKey := fmt.Sprintf("Rule %d Attribute", i)
		opKey := fmt.Sprintf("Rule %d Operator", i)
		valueKey := fmt.Sprintf("Rule %d Value", i)

		attrName := processorCtx.GetPropertyValue(attrKey)
		if attrName == "" {
			continue
		}

		operator := processorCtx.GetPropertyValue(opKey)
		expectedValue := processorCtx.GetPropertyValue(valueKey)

		result, err := p.evaluateRule(flowFile, attrName, operator, expectedValue)
		if err != nil {
			logger.Error("Failed to evaluate rule",
				"flowFileId", flowFile.ID,
				"rule", i,
				"attribute", attrName,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		ruleResults = append(ruleResults, result)
	}

	// If no rules were configured, route to unmatched
	if len(ruleResults) == 0 {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched"})
		return nil
	}

	// Apply filter mode
	matched := false
	if filterMode == "AND" {
		matched = true
		for _, result := range ruleResults {
			if !result {
				matched = false
				break
			}
		}
	} else { // OR
		for _, result := range ruleResults {
			if result {
				matched = true
				break
			}
		}
	}

	if matched {
		session.Transfer(flowFile, types.Relationship{Name: "matched"})
		logger.Debug("FlowFile matched filter",
			"flowFileId", flowFile.ID,
			"filterMode", filterMode)
	} else {
		session.Transfer(flowFile, types.Relationship{Name: "unmatched"})
		logger.Debug("FlowFile did not match filter",
			"flowFileId", flowFile.ID,
			"filterMode", filterMode)
	}

	return nil
}

// evaluateRule evaluates a single filter rule
func (p *FilterAttributeProcessor) evaluateRule(flowFile *types.FlowFile, attrName, operator, expectedValue string) (bool, error) {
	actualValue, exists := flowFile.GetAttribute(attrName)

	switch operator {
	case "Exists":
		return exists, nil
	case "Not Exists":
		return !exists, nil
	case "Equals":
		return exists && actualValue == expectedValue, nil
	case "Not Equals":
		return !exists || actualValue != expectedValue, nil
	case "Contains":
		return exists && strings.Contains(actualValue, expectedValue), nil
	case "Starts With":
		return exists && strings.HasPrefix(actualValue, expectedValue), nil
	case "Ends With":
		return exists && strings.HasSuffix(actualValue, expectedValue), nil
	case "Regex":
		if !exists {
			return false, nil
		}
		matched, err := regexp.MatchString(expectedValue, actualValue)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %w", err)
		}
		return matched, nil
	case "Greater Than":
		if !exists {
			return false, nil
		}
		actualNum, err1 := strconv.ParseFloat(actualValue, 64)
		expectedNum, err2 := strconv.ParseFloat(expectedValue, 64)
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("cannot compare non-numeric values")
		}
		return actualNum > expectedNum, nil
	case "Less Than":
		if !exists {
			return false, nil
		}
		actualNum, err1 := strconv.ParseFloat(actualValue, 64)
		expectedNum, err2 := strconv.ParseFloat(expectedValue, 64)
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("cannot compare non-numeric values")
		}
		return actualNum < expectedNum, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}
