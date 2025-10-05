package plugins

import (
	"regexp"
)

// resolveExpression resolves ${attribute.name} expressions
// This is a shared utility function used by multiple processors
func resolveExpression(expr string, attributes map[string]string) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	result := re.ReplaceAllStringFunc(expr, func(match string) string {
		attrName := match[2 : len(match)-1]
		if value, exists := attributes[attrName]; exists {
			return value
		}
		return match
	})

	return result
}
