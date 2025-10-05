package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getPutFileInfo()
	plugin.RegisterBuiltInProcessor("PutFile", func() types.Processor {
		return NewPutFileProcessor()
	}, info)
}

func getPutFileInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"PutFile",
		"PutFile",
		"1.0.0",
		"DataBridge",
		"Writes FlowFiles to the file system with configurable conflict resolution",
		[]string{"file", "output", "write"},
	)
}

// PutFileProcessor writes FlowFiles to the file system
type PutFileProcessor struct {
	*types.BaseProcessor
}

// ConflictResolutionStrategy defines how to handle file conflicts
type ConflictResolutionStrategy string

const (
	ConflictFail    ConflictResolutionStrategy = "fail"
	ConflictReplace ConflictResolutionStrategy = "replace"
	ConflictIgnore  ConflictResolutionStrategy = "ignore"
	ConflictRename  ConflictResolutionStrategy = "rename"
)

// NewPutFileProcessor creates a new PutFile processor
func NewPutFileProcessor() *PutFileProcessor {
	info := types.ProcessorInfo{
		Name:        "PutFile",
		Description: "Writes FlowFiles to the file system with configurable conflict resolution",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"file", "output", "write"},
		Properties: []types.PropertySpec{
			{
				Name:         "Directory",
				DisplayName:  "Directory",
				Description:  "Output directory where files will be written",
				Required:     true,
				DefaultValue: "",
				Type:         "directory",
				Placeholder:  "/path/to/output/directory",
				HelpText:     "Select or enter the directory where files will be saved",
			},
			{
				Name:         "Conflict Resolution Strategy",
				DisplayName:  "Conflict Resolution Strategy",
				Description:  "How to handle existing files",
				Required:     false,
				DefaultValue: "fail",
				AllowedValues: []string{"fail", "replace", "ignore", "rename"},
				Type:         "select",
				HelpText:     "fail: error if file exists; replace: overwrite; ignore: skip; rename: add suffix",
			},
			{
				Name:         "Create Missing Directories",
				DisplayName:  "Create Missing Directories",
				Description:  "Whether to create missing directories",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
			},
			{
				Name:         "Permissions",
				DisplayName:  "Permissions",
				Description:  "File permissions in octal format",
				Required:     false,
				DefaultValue: "0644",
				Pattern:      `^0[0-7]{3}$`,
				Type:         "permission",
				Placeholder:  "0644",
				HelpText:     "Common: 0644 (rw-r--r--), 0755 (rwxr-xr-x), 0600 (rw-------)",
			},
			{
				Name:         "Maximum File Count",
				DisplayName:  "Maximum File Count",
				Description:  "Maximum number of files in directory (-1 = unlimited)",
				Required:     false,
				DefaultValue: "-1",
				Pattern:      `^-?\d+$`,
				Type:         "number",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &PutFileProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *PutFileProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing PutFile processor")

	// Validate required properties
	if !ctx.HasProperty("Directory") {
		return fmt.Errorf("Directory property is required")
	}

	directory := ctx.GetPropertyValue("Directory")
	if directory == "" {
		return fmt.Errorf("Directory cannot be empty")
	}

	// Check if directory exists or if we should create it
	createMissing := ctx.GetPropertyValue("Create Missing Directories") != "false"

	info, err := os.Stat(directory)
	if err != nil {
		if os.IsNotExist(err) && createMissing {
			// Will create on first write
			logger.Info("Directory will be created on first write", "directory", directory)
			return nil
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", directory)
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *PutFileProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	directory := processorCtx.GetPropertyValue("Directory")
	strategyStr := processorCtx.GetPropertyValue("Conflict Resolution Strategy")
	if strategyStr == "" {
		strategyStr = "fail"
	}
	strategy := ConflictResolutionStrategy(strategyStr)

	createMissing := processorCtx.GetPropertyValue("Create Missing Directories") != "false"
	permissionsStr := processorCtx.GetPropertyValue("Permissions")
	if permissionsStr == "" {
		permissionsStr = "0644"
	}
	maxFileCountStr := processorCtx.GetPropertyValue("Maximum File Count")

	// Parse permissions
	permissions, err := strconv.ParseUint(permissionsStr, 8, 32)
	if err != nil {
		logger.Error("Invalid permissions format", "permissions", permissionsStr, "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Parse max file count
	maxFileCount := -1
	if maxFileCountStr != "" {
		if val, err := strconv.Atoi(maxFileCountStr); err == nil {
			maxFileCount = val
		}
	}

	// Create directory if needed
	if createMissing {
		if err := os.MkdirAll(directory, 0755); err != nil {
			logger.Error("Failed to create directory", "directory", directory, "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return nil
		}
	}

	// Check max file count
	if maxFileCount > 0 {
		count, err := p.countFilesInDirectory(directory)
		if err != nil {
			logger.Error("Failed to count files in directory", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return nil
		}

		if count >= maxFileCount {
			logger.Error("Maximum file count reached", "count", count, "max", maxFileCount)
			session.Transfer(flowFile, types.RelationshipFailure)
			return nil
		}
	}

	// Get filename from attribute or generate one
	filename, hasFilename := flowFile.GetAttribute("filename")
	if !hasFilename || filename == "" {
		filename = fmt.Sprintf("%s.dat", flowFile.ID.String())
	}

	// Construct full path
	fullPath := filepath.Join(directory, filename)

	// Handle conflicts
	finalPath, err := p.handleConflict(fullPath, strategy, logger)
	if err != nil {
		logger.Error("Failed to handle file conflict", "path", fullPath, "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	if finalPath == "" {
		// Conflict resolution was "ignore"
		logger.Info("Ignoring existing file", "path", fullPath)
		session.Transfer(flowFile, types.RelationshipSuccess)
		return nil
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content", "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Write file
	if err := os.WriteFile(finalPath, content, os.FileMode(permissions)); err != nil {
		logger.Error("Failed to write file", "path", finalPath, "error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Update attributes
	session.PutAttribute(flowFile, "absolute.path", finalPath)
	session.PutAttribute(flowFile, "filename", filepath.Base(finalPath))
	session.PutAttribute(flowFile, "path", filepath.Dir(finalPath))

	// Transfer to success
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Successfully wrote file",
		"path", finalPath,
		"size", len(content),
		"flowFileId", flowFile.ID)

	return nil
}

// handleConflict handles file conflicts based on strategy
func (p *PutFileProcessor) handleConflict(path string, strategy ConflictResolutionStrategy, logger types.Logger) (string, error) {
	// Check if file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// No conflict
		return path, nil
	}
	if err != nil {
		return "", err
	}

	// File exists, apply strategy
	switch strategy {
	case ConflictFail:
		return "", fmt.Errorf("file already exists: %s", path)

	case ConflictReplace:
		logger.Debug("Replacing existing file", "path", path)
		return path, nil

	case ConflictIgnore:
		logger.Debug("Ignoring existing file", "path", path)
		return "", nil

	case ConflictRename:
		// Generate unique filename
		dir := filepath.Dir(path)
		ext := filepath.Ext(path)
		base := filepath.Base(path)
		nameWithoutExt := base[:len(base)-len(ext)]

		for i := 1; i < 1000; i++ {
			newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", nameWithoutExt, i, ext))
			if _, err := os.Stat(newPath); os.IsNotExist(err) {
				logger.Debug("Renaming to avoid conflict", "original", path, "new", newPath)
				return newPath, nil
			}
		}
		return "", fmt.Errorf("could not find unique filename after 1000 attempts")

	default:
		return "", fmt.Errorf("unknown conflict resolution strategy: %s", strategy)
	}
}

// countFilesInDirectory counts files in a directory
func (p *PutFileProcessor) countFilesInDirectory(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}

	return count, nil
}

// Validate validates the processor configuration
func (p *PutFileProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate Directory (check parent exists if create missing is false)
	if directory, exists := config.Properties["Directory"]; exists && directory != "" {
		createMissing := config.Properties["Create Missing Directories"] != "false"

		info, err := os.Stat(directory)
		if err != nil {
			if os.IsNotExist(err) {
				if !createMissing {
					results = append(results, types.ValidationResult{
						Property: "Directory",
						Valid:    false,
						Message:  "Directory does not exist and Create Missing Directories is false",
					})
				}
				// If createMissing is true, we should check if parent exists
				parent := filepath.Dir(directory)
				if parentInfo, err := os.Stat(parent); err != nil {
					results = append(results, types.ValidationResult{
						Property: "Directory",
						Valid:    false,
						Message:  fmt.Sprintf("Parent directory does not exist: %s", parent),
					})
				} else if !parentInfo.IsDir() {
					results = append(results, types.ValidationResult{
						Property: "Directory",
						Valid:    false,
						Message:  "Parent path is not a directory",
					})
				}
			} else {
				results = append(results, types.ValidationResult{
					Property: "Directory",
					Valid:    false,
					Message:  fmt.Sprintf("Cannot access directory: %v", err),
				})
			}
		} else if !info.IsDir() {
			results = append(results, types.ValidationResult{
				Property: "Directory",
				Valid:    false,
				Message:  "Path is not a directory",
			})
		}
	}

	// Validate Conflict Resolution Strategy
	if strategy, exists := config.Properties["Conflict Resolution Strategy"]; exists {
		validStrategies := []string{"fail", "replace", "ignore", "rename"}
		valid := false
		for _, vs := range validStrategies {
			if strategy == vs {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "Conflict Resolution Strategy",
				Valid:    false,
				Message:  "Invalid strategy. Must be: fail, replace, ignore, or rename",
			})
		}
	}

	// Validate Permissions
	if permStr, exists := config.Properties["Permissions"]; exists && permStr != "" {
		if _, err := strconv.ParseUint(permStr, 8, 32); err != nil {
			results = append(results, types.ValidationResult{
				Property: "Permissions",
				Valid:    false,
				Message:  "Invalid octal permissions format (use: 0644, 0755, etc.)",
			})
		}
	}

	// Validate Maximum File Count
	if maxCountStr, exists := config.Properties["Maximum File Count"]; exists && maxCountStr != "" {
		if _, err := strconv.Atoi(maxCountStr); err != nil {
			results = append(results, types.ValidationResult{
				Property: "Maximum File Count",
				Valid:    false,
				Message:  "Maximum File Count must be an integer",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *PutFileProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed for this processor
}
