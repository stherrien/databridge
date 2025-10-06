package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test constants
const (
	testFilename  = "test.txt"
	invalidStr    = "invalid"
	tmpDir        = "/tmp"
	validationVal = "value"
)

func TestNewFlowFileValidator(t *testing.T) {
	validator := NewFlowFileValidator()
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.schemas)
}

func TestValidateQuick(t *testing.T) {
	validator := NewFlowFileValidator()

	t.Run("Valid FlowFile", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Size = 100
		err := validator.ValidateQuick(ff)
		assert.NoError(t, err)
	})

	t.Run("Nil FlowFile", func(t *testing.T) {
		err := validator.ValidateQuick(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("Negative Size", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Size = -100
		err := validator.ValidateQuick(ff)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "negative")
	})

	t.Run("Invalid ContentClaim", func(t *testing.T) {
		ff := NewFlowFile()
		ff.ContentClaim = &ContentClaim{
			Length:   -1,
			Offset:   -1,
			RefCount: -1,
		}
		err := validator.ValidateQuick(ff)
		assert.Error(t, err)
	})
}

func TestSchemaValidation(t *testing.T) {
	validator := NewFlowFileValidator()

	// Create test schema
	schema := &ValidationSchema{
		Name:        "test",
		Description: "Test schema",
		Required:    []string{"filename", "path"},
		Constraints: map[string]ConstraintConfig{
			"filename": {
				Type:      "string",
				MinLength: 1,
				MaxLength: 255,
			},
			"size": {
				Type:     "number",
				MinValue: 0,
				MaxValue: 1000000,
			},
		},
	}

	validator.RegisterSchema(schema)

	t.Run("Valid FlowFile", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Attributes["filename"] = testFilename
		ff.Attributes["path"] = tmpDir + "/" + testFilename
		ff.Attributes["size"] = "1000"

		errors := validator.Validate(ff, "test")
		assert.Empty(t, errors)
	})

	t.Run("Missing Required Attribute", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Attributes["filename"] = testFilename
		// Missing "path"

		errors := validator.Validate(ff, "test")
		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0].Error(), "required attribute")
	})

	t.Run("String Length Constraint", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Attributes["filename"] = "" // Too short
		ff.Attributes["path"] = tmpDir

		errors := validator.Validate(ff, "test")
		assert.NotEmpty(t, errors)
	})

	t.Run("Number Range Constraint", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Attributes["filename"] = testFilename
		ff.Attributes["path"] = tmpDir
		ff.Attributes["size"] = "2000000" // Too large

		errors := validator.Validate(ff, "test")
		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0].Error(), "exceeds maximum")
	})

	t.Run("Invalid Number", func(t *testing.T) {
		ff := NewFlowFile()
		ff.Attributes["filename"] = testFilename
		ff.Attributes["path"] = tmpDir
		ff.Attributes["size"] = "not-a-number"

		errors := validator.Validate(ff, "test")
		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0].Error(), "not a valid number")
	})

	t.Run("Unknown Schema", func(t *testing.T) {
		ff := NewFlowFile()
		errors := validator.Validate(ff, "nonexistent")
		assert.NotEmpty(t, errors)
		assert.Contains(t, errors[0].Error(), "not found")
	})
}

func TestConstraintTypes(t *testing.T) {
	validator := NewFlowFileValidator()

	t.Run("Boolean Constraint", func(t *testing.T) {
		schema := &ValidationSchema{
			Name: "bool-test",
			Constraints: map[string]ConstraintConfig{
				"active": {
					Type: "boolean",
				},
			},
		}
		validator.RegisterSchema(schema)

		ff := NewFlowFile()
		ff.Attributes["active"] = "true"
		errors := validator.Validate(ff, "bool-test")
		assert.Empty(t, errors)

		ff.Attributes["active"] = invalidStr
		errors = validator.Validate(ff, "bool-test")
		assert.NotEmpty(t, errors)
	})

	t.Run("Regex Constraint", func(t *testing.T) {
		schema := &ValidationSchema{
			Name: "regex-test",
			Constraints: map[string]ConstraintConfig{
				"email": {
					Type:    "regex",
					Pattern: `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
				},
			},
		}
		validator.RegisterSchema(schema)

		ff := NewFlowFile()
		ff.Attributes["email"] = "test@example.com"
		errors := validator.Validate(ff, "regex-test")
		assert.Empty(t, errors)

		ff.Attributes["email"] = "invalid-email"
		errors = validator.Validate(ff, "regex-test")
		assert.NotEmpty(t, errors)
	})

	t.Run("Allowed Values Constraint", func(t *testing.T) {
		schema := &ValidationSchema{
			Name: "enum-test",
			Constraints: map[string]ConstraintConfig{
				"status": {
					Type:          "string",
					AllowedValues: []string{"pending", "processing", "complete"},
				},
			},
		}
		validator.RegisterSchema(schema)

		ff := NewFlowFile()
		ff.Attributes["status"] = "pending"
		errors := validator.Validate(ff, "enum-test")
		assert.Empty(t, errors)

		ff.Attributes["status"] = invalidStr
		errors = validator.Validate(ff, "enum-test")
		assert.NotEmpty(t, errors)
	})
}

func TestValidationRules(t *testing.T) {
	_ = NewFlowFileValidator()

	t.Run("RequireAttributeRule", func(t *testing.T) {
		rule := RequireAttributeRule("required-attr")
		ff := NewFlowFile()

		err := rule.Validate(ff)
		assert.Error(t, err)

		ff.Attributes["required-attr"] = validationVal
		err = rule.Validate(ff)
		assert.NoError(t, err)
	})

	t.Run("AttributePatternRule", func(t *testing.T) {
		rule := AttributePatternRule("code", `^[A-Z]{3}$`)
		ff := NewFlowFile()
		ff.Attributes["code"] = "ABC"

		err := rule.Validate(ff)
		assert.NoError(t, err)

		ff.Attributes["code"] = "123"
		err = rule.Validate(ff)
		assert.Error(t, err)
	})

	t.Run("MaxSizeRule", func(t *testing.T) {
		rule := MaxSizeRule(1000)
		ff := NewFlowFile()
		ff.Size = 500

		err := rule.Validate(ff)
		assert.NoError(t, err)

		ff.Size = 2000
		err = rule.Validate(ff)
		assert.Error(t, err)
	})

	t.Run("AttributeRangeRule", func(t *testing.T) {
		rule := AttributeRangeRule("count", 0, 100)
		ff := NewFlowFile()
		ff.Attributes["count"] = "50"

		err := rule.Validate(ff)
		assert.NoError(t, err)

		ff.Attributes["count"] = "150"
		err = rule.Validate(ff)
		assert.Error(t, err)

		ff.Attributes["count"] = invalidStr
		err = rule.Validate(ff)
		assert.Error(t, err)
	})

	t.Run("ContentTypeRule", func(t *testing.T) {
		rule := ContentTypeRule([]string{"application/json", "text/plain"})
		ff := NewFlowFile()
		ff.Attributes["mime.type"] = "application/json"

		err := rule.Validate(ff)
		assert.NoError(t, err)

		ff.Attributes["mime.type"] = "application/xml"
		err = rule.Validate(ff)
		assert.Error(t, err)
	})
}

func TestDefaultSchemas(t *testing.T) {
	t.Run("Default Schema", func(t *testing.T) {
		schema := GetDefaultSchema()
		assert.NotNil(t, schema)
		assert.Equal(t, "default", schema.Name)
		assert.NotEmpty(t, schema.Rules)
	})

	t.Run("Strict Schema", func(t *testing.T) {
		schema := GetStrictSchema()
		assert.NotNil(t, schema)
		assert.Equal(t, "strict", schema.Name)
		assert.NotEmpty(t, schema.Required)
		assert.NotEmpty(t, schema.Constraints)
		assert.NotEmpty(t, schema.Rules)
	})
}

func TestCustomRules(t *testing.T) {
	validator := NewFlowFileValidator()

	customRule := ValidationRule{
		Name:        "CustomRule",
		Description: "Custom validation rule",
		Validate: func(ff *FlowFile) error {
			if ff.Size > 0 && len(ff.Attributes) == 0 {
				return fmt.Errorf("flowFile with content must have attributes")
			}
			return nil
		},
	}

	schema := &ValidationSchema{
		Name:  "custom",
		Rules: []ValidationRule{customRule},
	}
	validator.RegisterSchema(schema)

	ff := NewFlowFile()
	ff.Size = 100
	errors := validator.Validate(ff, "custom")
	assert.NotEmpty(t, errors)

	ff.Attributes["key"] = validationVal
	errors = validator.Validate(ff, "custom")
	assert.Empty(t, errors)
}
