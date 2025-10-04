package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ValidationRule represents a validation rule for FlowFile attributes or content
type ValidationRule struct {
	Name        string
	Description string
	Validate    func(*FlowFile) error
}

// ValidationSchema defines validation rules for FlowFiles
type ValidationSchema struct {
	Name        string
	Description string
	Rules       []ValidationRule
	Required    []string // Required attribute names
	Constraints map[string]ConstraintConfig
}

// ConstraintConfig defines constraints for specific attributes
type ConstraintConfig struct {
	Type         string   // "string", "number", "boolean", "regex"
	MinLength    int      // For strings
	MaxLength    int      // For strings
	MinValue     float64  // For numbers
	MaxValue     float64  // For numbers
	Pattern      string   // Regex pattern
	AllowedValues []string // Whitelist of values
}

// FlowFileValidator provides validation capabilities
type FlowFileValidator struct {
	schemas map[string]*ValidationSchema
}

// NewFlowFileValidator creates a new validator
func NewFlowFileValidator() *FlowFileValidator {
	return &FlowFileValidator{
		schemas: make(map[string]*ValidationSchema),
	}
}

// RegisterSchema registers a validation schema
func (v *FlowFileValidator) RegisterSchema(schema *ValidationSchema) {
	v.schemas[schema.Name] = schema
}

// Validate validates a FlowFile against a schema
func (v *FlowFileValidator) Validate(flowFile *FlowFile, schemaName string) []error {
	schema, exists := v.schemas[schemaName]
	if !exists {
		return []error{fmt.Errorf("schema '%s' not found", schemaName)}
	}

	var errors []error

	// Check required attributes
	for _, required := range schema.Required {
		if _, exists := flowFile.Attributes[required]; !exists {
			errors = append(errors, fmt.Errorf("required attribute '%s' is missing", required))
		}
	}

	// Apply constraints
	for attr, constraint := range schema.Constraints {
		value, exists := flowFile.Attributes[attr]
		if !exists {
			continue // Only validate if present
		}

		if err := v.validateConstraint(attr, value, constraint); err != nil {
			errors = append(errors, err)
		}
	}

	// Apply custom rules
	for _, rule := range schema.Rules {
		if err := rule.Validate(flowFile); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", rule.Name, err))
		}
	}

	return errors
}

// validateConstraint validates a value against a constraint
func (v *FlowFileValidator) validateConstraint(attr, value string, constraint ConstraintConfig) error {
	switch constraint.Type {
	case "string":
		if constraint.MinLength > 0 && len(value) < constraint.MinLength {
			return fmt.Errorf("attribute '%s' length %d is less than minimum %d", attr, len(value), constraint.MinLength)
		}
		if constraint.MaxLength > 0 && len(value) > constraint.MaxLength {
			return fmt.Errorf("attribute '%s' length %d exceeds maximum %d", attr, len(value), constraint.MaxLength)
		}

	case "number":
		num, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("attribute '%s' value '%s' is not a valid number", attr, value)
		}
		if constraint.MinValue != 0 && num < constraint.MinValue {
			return fmt.Errorf("attribute '%s' value %f is less than minimum %f", attr, num, constraint.MinValue)
		}
		if constraint.MaxValue != 0 && num > constraint.MaxValue {
			return fmt.Errorf("attribute '%s' value %f exceeds maximum %f", attr, num, constraint.MaxValue)
		}

	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("attribute '%s' value '%s' is not a valid boolean", attr, value)
		}

	case "regex":
		matched, err := regexp.MatchString(constraint.Pattern, value)
		if err != nil {
			return fmt.Errorf("invalid regex pattern for attribute '%s': %w", attr, err)
		}
		if !matched {
			return fmt.Errorf("attribute '%s' value '%s' does not match pattern '%s'", attr, value, constraint.Pattern)
		}
	}

	// Check allowed values whitelist
	if len(constraint.AllowedValues) > 0 {
		found := false
		for _, allowed := range constraint.AllowedValues {
			if value == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("attribute '%s' value '%s' is not in allowed values: %s",
				attr, value, strings.Join(constraint.AllowedValues, ", "))
		}
	}

	return nil
}

// ValidateQuick performs quick validation without a schema
func (v *FlowFileValidator) ValidateQuick(flowFile *FlowFile) error {
	// Basic sanity checks
	if flowFile == nil {
		return fmt.Errorf("flowFile is nil")
	}

	if flowFile.ID.String() == "00000000-0000-0000-0000-000000000000" {
		return fmt.Errorf("flowFile has invalid UUID")
	}

	if flowFile.Attributes == nil {
		return fmt.Errorf("flowFile attributes map is nil")
	}

	if flowFile.Size < 0 {
		return fmt.Errorf("flowFile size cannot be negative: %d", flowFile.Size)
	}

	if flowFile.ContentClaim != nil {
		if flowFile.ContentClaim.Length < 0 {
			return fmt.Errorf("content claim length cannot be negative: %d", flowFile.ContentClaim.Length)
		}
		if flowFile.ContentClaim.Offset < 0 {
			return fmt.Errorf("content claim offset cannot be negative: %d", flowFile.ContentClaim.Offset)
		}
		if flowFile.ContentClaim.RefCount < 0 {
			return fmt.Errorf("content claim refCount cannot be negative: %d", flowFile.ContentClaim.RefCount)
		}
	}

	return nil
}

// Common validation rules

// RequireAttributeRule creates a rule that requires an attribute
func RequireAttributeRule(attrName string) ValidationRule {
	return ValidationRule{
		Name:        fmt.Sprintf("RequireAttribute(%s)", attrName),
		Description: fmt.Sprintf("Requires attribute '%s' to be present", attrName),
		Validate: func(ff *FlowFile) error {
			if _, exists := ff.Attributes[attrName]; !exists {
				return fmt.Errorf("required attribute '%s' is missing", attrName)
			}
			return nil
		},
	}
}

// AttributePatternRule creates a rule that validates an attribute against a regex
func AttributePatternRule(attrName, pattern string) ValidationRule {
	return ValidationRule{
		Name:        fmt.Sprintf("AttributePattern(%s)", attrName),
		Description: fmt.Sprintf("Validates attribute '%s' matches pattern '%s'", attrName, pattern),
		Validate: func(ff *FlowFile) error {
			value, exists := ff.Attributes[attrName]
			if !exists {
				return nil // Don't validate if not present
			}
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				return fmt.Errorf("invalid pattern: %w", err)
			}
			if !matched {
				return fmt.Errorf("attribute '%s' value '%s' does not match pattern", attrName, value)
			}
			return nil
		},
	}
}

// MaxSizeRule creates a rule that limits FlowFile size
func MaxSizeRule(maxBytes int64) ValidationRule {
	return ValidationRule{
		Name:        "MaxSize",
		Description: fmt.Sprintf("Limits FlowFile size to %d bytes", maxBytes),
		Validate: func(ff *FlowFile) error {
			if ff.Size > maxBytes {
				return fmt.Errorf("flowFile size %d exceeds maximum %d bytes", ff.Size, maxBytes)
			}
			return nil
		},
	}
}

// AttributeRangeRule creates a rule that validates numeric attribute range
func AttributeRangeRule(attrName string, min, max float64) ValidationRule {
	return ValidationRule{
		Name:        fmt.Sprintf("AttributeRange(%s)", attrName),
		Description: fmt.Sprintf("Validates attribute '%s' is between %f and %f", attrName, min, max),
		Validate: func(ff *FlowFile) error {
			value, exists := ff.Attributes[attrName]
			if !exists {
				return nil
			}
			num, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("attribute '%s' is not a number", attrName)
			}
			if num < min || num > max {
				return fmt.Errorf("attribute '%s' value %f is outside range [%f, %f]", attrName, num, min, max)
			}
			return nil
		},
	}
}

// ContentTypeRule creates a rule that validates MIME type
func ContentTypeRule(allowedTypes []string) ValidationRule {
	return ValidationRule{
		Name:        "ContentType",
		Description: "Validates FlowFile MIME type",
		Validate: func(ff *FlowFile) error {
			mimeType, exists := ff.Attributes["mime.type"]
			if !exists {
				return fmt.Errorf("mime.type attribute is required")
			}
			for _, allowed := range allowedTypes {
				if mimeType == allowed {
					return nil
				}
			}
			return fmt.Errorf("mime.type '%s' is not allowed. Allowed types: %s",
				mimeType, strings.Join(allowedTypes, ", "))
		},
	}
}

// Example validation schemas

// GetDefaultSchema returns a default validation schema
func GetDefaultSchema() *ValidationSchema {
	return &ValidationSchema{
		Name:        "default",
		Description: "Default validation schema for FlowFiles",
		Required:    []string{},
		Constraints: map[string]ConstraintConfig{
			"filename": {
				Type:      "string",
				MinLength: 1,
				MaxLength: 255,
			},
		},
		Rules: []ValidationRule{
			MaxSizeRule(100 * 1024 * 1024), // 100MB max
		},
	}
}

// GetStrictSchema returns a strict validation schema
func GetStrictSchema() *ValidationSchema {
	return &ValidationSchema{
		Name:        "strict",
		Description: "Strict validation schema with comprehensive checks",
		Required:    []string{"filename", "mime.type"},
		Constraints: map[string]ConstraintConfig{
			"filename": {
				Type:      "string",
				MinLength: 1,
				MaxLength: 255,
				Pattern:   `^[a-zA-Z0-9._-]+$`,
			},
			"mime.type": {
				Type: "string",
				AllowedValues: []string{
					"application/json",
					"application/xml",
					"text/plain",
					"text/csv",
					"application/octet-stream",
				},
			},
		},
		Rules: []ValidationRule{
			MaxSizeRule(50 * 1024 * 1024), // 50MB max
			RequireAttributeRule("path"),
		},
	}
}
