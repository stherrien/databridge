package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// ValidationResult contains validation outcome
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// PluginValidator validates plugin safety
type PluginValidator struct {
	allowUnsigned     bool
	trustedPublishers []string
	checksum          bool
	logger            *logrus.Logger
}

// ValidatorConfig configures the validator
type ValidatorConfig struct {
	AllowUnsigned     bool
	TrustedPublishers []string
	RequireChecksum   bool
}

// NewPluginValidator creates a new plugin validator
func NewPluginValidator(config ValidatorConfig, logger *logrus.Logger) *PluginValidator {
	return &PluginValidator{
		allowUnsigned:     config.AllowUnsigned,
		trustedPublishers: config.TrustedPublishers,
		checksum:          config.RequireChecksum,
		logger:            logger,
	}
}

// Validate validates a plugin
func (v *PluginValidator) Validate(pluginPath string) ValidationResult {
	result := ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check if path exists
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Plugin path does not exist: %s", pluginPath))
		return result
	}

	// Validate manifest
	manifestPath := filepath.Join(pluginPath, "manifest.json")
	if err := v.validateManifestFile(manifestPath); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Manifest validation failed: %v", err))
		return result
	}

	// Read manifest
	manifest, err := v.readManifest(manifestPath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to read manifest: %v", err))
		return result
	}

	// Validate manifest schema
	if err := v.validateManifestSchema(manifest); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid manifest schema: %v", err))
		return result
	}

	// Validate checksum if required
	if v.checksum {
		if err := v.validateChecksum(pluginPath, manifest); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("Checksum validation failed: %v", err))
		}
	}

	// Check for signature (optional)
	if !v.allowUnsigned {
		if err := v.validateSignature(pluginPath); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Plugin is not signed: %v", err))
		}
	}

	// Validate publisher if configured
	if len(v.trustedPublishers) > 0 {
		if !v.isTrustedPublisher(manifest["author"].(string)) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Plugin publisher not in trusted list: %s", manifest["author"]))
		}
	}

	// Validate plugin structure
	if errs := v.validateStructure(pluginPath, manifest); len(errs) > 0 {
		result.Warnings = append(result.Warnings, errs...)
	}

	// Security scan hooks (placeholder for future integration)
	if warnings := v.performSecurityScan(pluginPath); len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}

	v.logger.WithFields(logrus.Fields{
		"pluginPath": pluginPath,
		"valid":      result.Valid,
		"errors":     len(result.Errors),
		"warnings":   len(result.Warnings),
	}).Debug("Plugin validation completed")

	return result
}

// ValidateManifest validates a manifest without full plugin validation
func (v *PluginValidator) ValidateManifest(manifest map[string]interface{}) error {
	return v.validateManifestSchema(manifest)
}

// validateManifestFile checks if manifest file exists and is readable
func (v *PluginValidator) validateManifestFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access manifest: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("manifest path is a directory")
	}

	if info.Size() == 0 {
		return fmt.Errorf("manifest file is empty")
	}

	if info.Size() > 1024*1024 { // 1MB limit
		return fmt.Errorf("manifest file too large (>1MB)")
	}

	return nil
}

// readManifest reads and parses the manifest
func (v *PluginValidator) readManifest(path string) (map[string]interface{}, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return manifest, nil
}

// validateManifestSchema validates the manifest schema
func (v *PluginValidator) validateManifestSchema(manifest map[string]interface{}) error {
	// Required fields
	requiredFields := []string{"id", "name", "version", "type"}
	for _, field := range requiredFields {
		if _, exists := manifest[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate ID format
	if id, ok := manifest["id"].(string); !ok || id == "" {
		return fmt.Errorf("invalid or empty id field")
	}

	// Validate name
	if name, ok := manifest["name"].(string); !ok || name == "" {
		return fmt.Errorf("invalid or empty name field")
	}

	// Validate version format (basic check)
	if version, ok := manifest["version"].(string); !ok || version == "" {
		return fmt.Errorf("invalid or empty version field")
	}

	// Validate type
	validTypes := map[string]bool{
		"processor":  true,
		"controller": true,
		"reporting":  true,
	}
	pluginType, ok := manifest["type"].(string)
	if !ok || !validTypes[pluginType] {
		return fmt.Errorf("invalid plugin type: %v", manifest["type"])
	}

	// Validate author if present
	if author, exists := manifest["author"]; exists {
		if _, ok := author.(string); !ok {
			return fmt.Errorf("author field must be a string")
		}
	}

	// Validate description if present
	if desc, exists := manifest["description"]; exists {
		if _, ok := desc.(string); !ok {
			return fmt.Errorf("description field must be a string")
		}
	}

	// Validate dependencies if present
	if deps, exists := manifest["dependencies"]; exists {
		depArray, ok := deps.([]interface{})
		if !ok {
			return fmt.Errorf("dependencies must be an array")
		}
		for i, dep := range depArray {
			if _, ok := dep.(string); !ok {
				return fmt.Errorf("dependency at index %d must be a string", i)
			}
		}
	}

	// Validate tags if present
	if tags, exists := manifest["tags"]; exists {
		tagArray, ok := tags.([]interface{})
		if !ok {
			return fmt.Errorf("tags must be an array")
		}
		for i, tag := range tagArray {
			if _, ok := tag.(string); !ok {
				return fmt.Errorf("tag at index %d must be a string", i)
			}
		}
	}

	return nil
}

// validateChecksum validates plugin checksums
func (v *PluginValidator) validateChecksum(pluginPath string, manifest map[string]interface{}) error {
	checksumFile := filepath.Join(pluginPath, "checksum.txt")

	// Check if checksum file exists
	if _, err := os.Stat(checksumFile); os.IsNotExist(err) {
		return fmt.Errorf("checksum file not found")
	}

	// Read expected checksums
	expectedChecksums, err := v.readChecksumFile(checksumFile)
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}

	// Get entry point if specified
	entryPoint, _ := manifest["entryPoint"].(string)
	if entryPoint == "" {
		return nil // No entry point to validate
	}

	// Calculate actual checksum
	entryPointPath := filepath.Join(pluginPath, entryPoint)
	actualChecksum, err := v.calculateChecksum(entryPointPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Compare checksums
	expectedChecksum, exists := expectedChecksums[entryPoint]
	if !exists {
		return fmt.Errorf("no checksum found for entry point: %s", entryPoint)
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch for %s (expected: %s, actual: %s)",
			entryPoint, expectedChecksum, actualChecksum)
	}

	return nil
}

// readChecksumFile reads checksums from a file
func (v *PluginValidator) readChecksumFile(path string) (map[string]string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	checksums := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		checksums[parts[1]] = parts[0]
	}

	return checksums, nil
}

// calculateChecksum calculates SHA256 checksum of a file
func (v *PluginValidator) calculateChecksum(path string) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// validateSignature validates plugin signature (placeholder)
func (v *PluginValidator) validateSignature(pluginPath string) error {
	signatureFile := filepath.Join(pluginPath, "signature.sig")

	// Check if signature file exists
	if _, err := os.Stat(signatureFile); os.IsNotExist(err) {
		return fmt.Errorf("signature file not found")
	}

	// Placeholder for actual signature verification
	// In a real implementation, this would:
	// 1. Read the signature file
	// 2. Extract the public key or use a trusted key
	// 3. Verify the signature against plugin files
	// 4. Check certificate validity and chain of trust

	v.logger.Debug("Signature validation not fully implemented")
	return nil
}

// isTrustedPublisher checks if publisher is trusted
func (v *PluginValidator) isTrustedPublisher(publisher string) bool {
	for _, trusted := range v.trustedPublishers {
		if strings.EqualFold(publisher, trusted) {
			return true
		}
	}
	return false
}

// validateStructure validates plugin directory structure
func (v *PluginValidator) validateStructure(pluginPath string, manifest map[string]interface{}) []string {
	var warnings []string

	// Check if entry point exists
	if entryPoint, ok := manifest["entryPoint"].(string); ok && entryPoint != "" {
		entryPointPath := filepath.Join(pluginPath, entryPoint)
		if _, err := os.Stat(entryPointPath); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("Entry point file not found: %s", entryPoint))
		}
	}

	// Check for README
	readmePath := filepath.Join(pluginPath, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		warnings = append(warnings, "No README.md found")
	}

	// Check for LICENSE
	licensePath := filepath.Join(pluginPath, "LICENSE")
	if _, err := os.Stat(licensePath); os.IsNotExist(err) {
		warnings = append(warnings, "No LICENSE file found")
	}

	return warnings
}

// performSecurityScan performs basic security checks
func (v *PluginValidator) performSecurityScan(pluginPath string) []string {
	var warnings []string

	// Placeholder for security scanning
	// In a real implementation, this could:
	// 1. Scan for known vulnerabilities
	// 2. Check for suspicious patterns
	// 3. Validate permissions
	// 4. Check for malicious code patterns
	// 5. Integrate with external security tools

	// Check file permissions
	entries, err := ioutil.ReadDir(pluginPath)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to scan directory: %v", err))
		return warnings
	}

	for _, entry := range entries {
		// Check for executable files outside of expected locations
		if entry.Mode()&0111 != 0 && !strings.HasSuffix(entry.Name(), ".so") {
			warnings = append(warnings, fmt.Sprintf("Unexpected executable file: %s", entry.Name()))
		}

		// Check for hidden files
		if strings.HasPrefix(entry.Name(), ".") && entry.Name() != "." && entry.Name() != ".." {
			warnings = append(warnings, fmt.Sprintf("Hidden file detected: %s", entry.Name()))
		}
	}

	return warnings
}

// ValidatePluginCompatibility checks if plugin is compatible with current system
func (v *PluginValidator) ValidatePluginCompatibility(manifest map[string]interface{}) error {
	// Check for minimum version requirements
	if minVersion, ok := manifest["minVersion"].(string); ok {
		// Placeholder for version comparison
		// In real implementation, compare with current DataBridge version
		_ = minVersion
	}

	// Check for required features
	if requiredFeatures, ok := manifest["requiredFeatures"].([]interface{}); ok {
		for _, feature := range requiredFeatures {
			if featureStr, ok := feature.(string); ok {
				// Placeholder for feature checking
				// In real implementation, verify that required features are available
				_ = featureStr
			}
		}
	}

	return nil
}

// GenerateChecksum generates a checksum file for a plugin
func (v *PluginValidator) GenerateChecksum(pluginPath string) error {
	checksumFile := filepath.Join(pluginPath, "checksum.txt")

	var checksums []string

	// Walk through plugin directory
	err := filepath.Walk(pluginPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and the checksum file itself
		if info.IsDir() || filepath.Base(path) == "checksum.txt" {
			return nil
		}

		// Calculate checksum
		checksum, err := v.calculateChecksum(path)
		if err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %w", path, err)
		}

		// Get relative path
		relPath, err := filepath.Rel(pluginPath, path)
		if err != nil {
			return err
		}

		checksums = append(checksums, fmt.Sprintf("%s  %s", checksum, relPath))
		return nil
	})

	if err != nil {
		return err
	}

	// Write checksums to file
	content := strings.Join(checksums, "\n") + "\n"
	return ioutil.WriteFile(checksumFile, []byte(content), 0644)
}
