package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getGetFileInfo()
	plugin.RegisterBuiltInProcessor("GetFile", func() types.Processor {
		return NewGetFileProcessor()
	}, info)
}

func getGetFileInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"GetFile",
		"GetFile",
		"1.0.0",
		"DataBridge",
		"Reads files from a directory with configurable filtering and batch processing",
		[]string{"file", "source", "ingest"},
	)
}

// GetFileProcessor reads files from a directory
type GetFileProcessor struct {
	*types.BaseProcessor
}

// NewGetFileProcessor creates a new GetFile processor
func NewGetFileProcessor() *GetFileProcessor {
	info := types.ProcessorInfo{
		Name:        "GetFile",
		Description: "Reads files from a directory with configurable filtering and batch processing",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"file", "source", "ingest"},
		Properties: []types.PropertySpec{
			{
				Name:         "Input Directory",
				DisplayName:  "Input Directory",
				Description:  "Directory to scan for files",
				Required:     true,
				DefaultValue: "",
				Type:         "directory",
				Placeholder:  "/path/to/input/directory",
				HelpText:     "Select or enter the directory path to monitor for new files",
			},
			{
				Name:         "File Filter",
				DisplayName:  "File Filter",
				Description:  "Glob pattern for file matching",
				Required:     false,
				DefaultValue: "*",
				Type:         "string",
				Placeholder:  "*.txt or data-*.json",
				HelpText:     "Examples: *.txt (all text files), data-*.csv (CSV files starting with 'data-'), report_[0-9]*.pdf",
			},
			{
				Name:         "Keep Source File",
				DisplayName:  "Keep Source File",
				Description:  "Whether to keep or delete source file after ingestion",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
			},
			{
				Name:         "Recurse Subdirectories",
				DisplayName:  "Recurse Subdirectories",
				Description:  "Whether to scan subdirectories",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
			},
			{
				Name:         "Minimum File Age",
				DisplayName:  "Minimum File Age",
				Description:  "Minimum age before file is picked up (e.g., 10s, 1m, 1h)",
				Required:     false,
				DefaultValue: "0s",
				Type:         "string",
			},
			{
				Name:         "Maximum File Age",
				DisplayName:  "Maximum File Age",
				Description:  "Maximum age (0s = no limit, e.g., 10s, 1m, 1h)",
				Required:     false,
				DefaultValue: "0s",
				Type:         "string",
			},
			{
				Name:         "Batch Size",
				DisplayName:  "Batch Size",
				Description:  "Max files to process per execution",
				Required:     false,
				DefaultValue: "10",
				Pattern:      `^\d+$`,
				Type:         "number",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &GetFileProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *GetFileProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing GetFile processor")

	// Validate required properties
	if !ctx.HasProperty("Input Directory") {
		return fmt.Errorf("Input Directory property is required")
	}

	inputDir := ctx.GetPropertyValue("Input Directory")
	if inputDir == "" {
		return fmt.Errorf("Input Directory cannot be empty")
	}

	// Check if directory exists
	info, err := os.Stat(inputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("input directory does not exist: %s", inputDir)
		}
		return fmt.Errorf("cannot access input directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("input directory is not a directory: %s", inputDir)
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *GetFileProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get processor context to access properties
	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		return fmt.Errorf("processor context not available")
	}

	// Read configuration properties
	inputDir := processorCtx.GetPropertyValue("Input Directory")
	fileFilter := processorCtx.GetPropertyValue("File Filter")
	if fileFilter == "" {
		fileFilter = "*"
	}

	keepSource := processorCtx.GetPropertyValue("Keep Source File") == "true"
	recurse := processorCtx.GetPropertyValue("Recurse Subdirectories") == "true"
	minAgeStr := processorCtx.GetPropertyValue("Minimum File Age")
	maxAgeStr := processorCtx.GetPropertyValue("Maximum File Age")
	batchSizeStr := processorCtx.GetPropertyValue("Batch Size")

	// Parse batch size
	batchSize := 10
	if batchSizeStr != "" {
		if val, err := strconv.Atoi(batchSizeStr); err == nil && val > 0 {
			batchSize = val
		}
	}

	// Parse age durations
	minAge, _ := time.ParseDuration(minAgeStr)
	maxAge, _ := time.ParseDuration(maxAgeStr)

	// Find matching files
	files, err := p.findFiles(inputDir, fileFilter, recurse, minAge, maxAge, batchSize)
	if err != nil {
		logger.Error("Failed to find files", "error", err)
		return fmt.Errorf("failed to find files: %w", err)
	}

	if len(files) == 0 {
		logger.Debug("No files found matching criteria")
		return nil
	}

	logger.Info("Found files to process", "count", len(files))

	// Process each file
	for _, filePath := range files {
		if err := p.processFile(filePath, session, keepSource, logger); err != nil {
			logger.Error("Failed to process file", "file", filePath, "error", err)
			// Continue processing other files
		}
	}

	return nil
}

// findFiles finds files matching the criteria
func (p *GetFileProcessor) findFiles(inputDir, pattern string, recurse bool, minAge, maxAge time.Duration, batchSize int) ([]string, error) {
	var matchedFiles []string
	now := time.Now()

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}

		// Skip directories
		if info.IsDir() {
			if !recurse && path != inputDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Match pattern
		matched, err := filepath.Match(pattern, info.Name())
		if err != nil || !matched {
			return nil
		}

		// Check file age
		fileAge := now.Sub(info.ModTime())
		if minAge > 0 && fileAge < minAge {
			return nil // File too new
		}
		if maxAge > 0 && fileAge > maxAge {
			return nil // File too old
		}

		matchedFiles = append(matchedFiles, path)

		// Stop if we've reached batch size
		if len(matchedFiles) >= batchSize {
			return filepath.SkipAll
		}

		return nil
	}

	if err := filepath.Walk(inputDir, walkFunc); err != nil && err != filepath.SkipAll {
		return nil, err
	}

	return matchedFiles, nil
}

// processFile processes a single file
func (p *GetFileProcessor) processFile(filePath string, session types.ProcessSession, keepSource bool, logger types.Logger) error {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Failed to read file", "file", filePath, "error", err)
		// Create a failure FlowFile with error information
		flowFile := session.Create()
		session.PutAttribute(flowFile, "filename", filepath.Base(filePath))
		session.PutAttribute(flowFile, "path", filepath.Dir(filePath))
		session.PutAttribute(flowFile, "absolute.path", filePath)
		session.PutAttribute(flowFile, "error.message", err.Error())
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		logger.Error("Failed to stat file", "file", filePath, "error", err)
		return err
	}

	// Create FlowFile
	flowFile := session.Create()

	// Write content
	if err := session.Write(flowFile, content); err != nil {
		logger.Error("Failed to write content to FlowFile", "file", filePath, "error", err)
		session.Remove(flowFile)
		return err
	}

	// Set attributes
	session.PutAttribute(flowFile, "filename", fileInfo.Name())
	session.PutAttribute(flowFile, "path", filepath.Dir(filePath))
	session.PutAttribute(flowFile, "absolute.path", filePath)
	session.PutAttribute(flowFile, "file.size", fmt.Sprintf("%d", fileInfo.Size()))
	session.PutAttribute(flowFile, "file.lastModified", fileInfo.ModTime().Format(time.RFC3339))
	session.PutAttribute(flowFile, "file.permissions", fileInfo.Mode().String())

	// Transfer to success
	session.Transfer(flowFile, types.RelationshipSuccess)

	logger.Info("Successfully processed file",
		"file", filePath,
		"size", fileInfo.Size(),
		"flowFileId", flowFile.ID)

	// Delete source file if requested
	if !keepSource {
		if err := os.Remove(filePath); err != nil {
			logger.Warn("Failed to delete source file", "file", filePath, "error", err)
			// Don't fail the overall processing
		} else {
			logger.Debug("Deleted source file", "file", filePath)
		}
	}

	return nil
}

// Validate validates the processor configuration
func (p *GetFileProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate Input Directory
	if inputDir, exists := config.Properties["Input Directory"]; exists && inputDir != "" {
		info, err := os.Stat(inputDir)
		if err != nil {
			results = append(results, types.ValidationResult{
				Property: "Input Directory",
				Valid:    false,
				Message:  fmt.Sprintf("Cannot access directory: %v", err),
			})
		} else if !info.IsDir() {
			results = append(results, types.ValidationResult{
				Property: "Input Directory",
				Valid:    false,
				Message:  "Path is not a directory",
			})
		}
	}

	// Validate Batch Size
	if batchSizeStr, exists := config.Properties["Batch Size"]; exists && batchSizeStr != "" {
		batchSize, err := strconv.Atoi(batchSizeStr)
		if err != nil || batchSize <= 0 {
			results = append(results, types.ValidationResult{
				Property: "Batch Size",
				Valid:    false,
				Message:  "Batch Size must be a positive integer",
			})
		}
	}

	// Validate age durations
	if minAgeStr, exists := config.Properties["Minimum File Age"]; exists && minAgeStr != "" {
		if _, err := time.ParseDuration(minAgeStr); err != nil {
			results = append(results, types.ValidationResult{
				Property: "Minimum File Age",
				Valid:    false,
				Message:  "Invalid duration format (use: 10s, 1m, 1h, etc.)",
			})
		}
	}

	if maxAgeStr, exists := config.Properties["Maximum File Age"]; exists && maxAgeStr != "" {
		if _, err := time.ParseDuration(maxAgeStr); err != nil {
			results = append(results, types.ValidationResult{
				Property: "Maximum File Age",
				Valid:    false,
				Message:  "Invalid duration format (use: 10s, 1m, 1h, etc.)",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *GetFileProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed for this processor
}
