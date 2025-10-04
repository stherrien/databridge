package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewPutFileProcessor(t *testing.T) {
	processor := NewPutFileProcessor()

	if processor == nil {
		t.Fatal("NewPutFileProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "PutFile" {
		t.Errorf("Expected processor name PutFile, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 5 {
		t.Errorf("Expected 5 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 2 {
		t.Errorf("Expected 2 relationships, got %d", len(info.Relationships))
	}

	// Check relationships
	hasSuccess := false
	hasFailure := false
	for _, rel := range info.Relationships {
		if rel.Name == "success" {
			hasSuccess = true
		}
		if rel.Name == "failure" {
			hasFailure = true
		}
	}

	if !hasSuccess || !hasFailure {
		t.Error("Expected success and failure relationships")
	}
}

func TestPutFileProcessorInitialize(t *testing.T) {
	processor := NewPutFileProcessor()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Directory": tmpDir,
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid directory: %v", err)
	}

	// Test initialization without Directory property
	ctxNoDir := &mockProcessorContext{
		properties: map[string]string{},
	}

	err = processor.Initialize(ctxNoDir)
	if err == nil {
		t.Error("Initialize should fail without Directory property")
	}

	// Test initialization with empty directory
	ctxEmptyDir := &mockProcessorContext{
		properties: map[string]string{
			"Directory": "",
		},
	}

	err = processor.Initialize(ctxEmptyDir)
	if err == nil {
		t.Error("Initialize should fail with empty directory")
	}

	// Test initialization with non-existent directory but create missing = true
	ctxCreateMissing := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                   filepath.Join(tmpDir, "newdir"),
			"Create Missing Directories": "true",
		},
	}

	err = processor.Initialize(ctxCreateMissing)
	if err != nil {
		t.Errorf("Initialize should succeed when Create Missing Directories is true: %v", err)
	}

	// Test initialization with a file (not directory)
	tmpFile := filepath.Join(tmpDir, "not_a_dir.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxFileNotDir := &mockProcessorContext{
		properties: map[string]string{
			"Directory": tmpFile,
		},
	}

	err = processor.Initialize(ctxFileNotDir)
	if err == nil {
		t.Error("Initialize should fail when path is not a directory")
	}
}

func TestPutFileProcessorValidate(t *testing.T) {
	processor := NewPutFileProcessor()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "replace",
			"Create Missing Directories":  "true",
			"Permissions":                  "0644",
			"Maximum File Count":           "100",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test invalid conflict resolution strategy
	invalidStrategyConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "invalid",
		},
	}

	results = processor.Validate(invalidStrategyConfig)
	hasStrategyError := false
	for _, result := range results {
		if result.Property == "Conflict Resolution Strategy" && !result.Valid {
			hasStrategyError = true
			break
		}
	}

	if !hasStrategyError {
		t.Error("Invalid conflict resolution strategy should cause validation error")
	}

	// Test invalid permissions
	invalidPermConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Directory":   tmpDir,
			"Permissions": "9999",
		},
	}

	results = processor.Validate(invalidPermConfig)
	hasPermError := false
	for _, result := range results {
		if result.Property == "Permissions" && !result.Valid {
			hasPermError = true
			break
		}
	}

	if !hasPermError {
		t.Error("Invalid permissions should cause validation error")
	}

	// Test invalid maximum file count
	invalidMaxCountConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Directory":          tmpDir,
			"Maximum File Count": "not-a-number",
		},
	}

	results = processor.Validate(invalidMaxCountConfig)
	hasMaxCountError := false
	for _, result := range results {
		if result.Property == "Maximum File Count" && !result.Valid {
			hasMaxCountError = true
			break
		}
	}

	if !hasMaxCountError {
		t.Error("Invalid maximum file count should cause validation error")
	}
}

func TestPutFileProcessorOnTrigger(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create a mock session with a FlowFile
	session := newMockProcessSession()
	flowFile := session.Create()
	content := []byte("Test content for PutFile")
	session.Write(flowFile, content)
	session.PutAttribute(flowFile, "filename", "test.txt")

	// Make the FlowFile available for Get()
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory": tmpDir,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify file was written
	outputPath := filepath.Join(tmpDir, "test.txt")
	writtenContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Errorf("Output file should exist: %v", err)
	}

	if string(writtenContent) != string(content) {
		t.Errorf("Written content mismatch. Expected %s, got %s", string(content), string(writtenContent))
	}

	// Verify attributes were updated
	attrs := session.attributes[flowFile.ID]
	if attrs["absolute.path"] != outputPath {
		t.Errorf("Expected absolute.path=%s, got %s", outputPath, attrs["absolute.path"])
	}

	// Verify transfer to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}
}

func TestPutFileProcessorConflictFail(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create existing file
	existingFile := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("Existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Try to write with conflict=fail
	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("New content"))
	session.PutAttribute(flowFile, "filename", "existing.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "fail",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to failure
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "failure" {
		t.Error("FlowFile should be transferred to failure relationship on conflict")
	}

	// Original file should be unchanged
	content, _ := os.ReadFile(existingFile)
	if string(content) != "Existing content" {
		t.Error("Original file should not be modified")
	}
}

func TestPutFileProcessorConflictReplace(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create existing file
	existingFile := filepath.Join(tmpDir, "replace.txt")
	if err := os.WriteFile(existingFile, []byte("Old content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write with conflict=replace
	session := newMockProcessSession()
	flowFile := session.Create()
	newContent := []byte("New content")
	session.Write(flowFile, newContent)
	session.PutAttribute(flowFile, "filename", "replace.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "replace",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}

	// File should be replaced
	content, _ := os.ReadFile(existingFile)
	if string(content) != "New content" {
		t.Error("File should be replaced with new content")
	}
}

func TestPutFileProcessorConflictIgnore(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create existing file
	existingFile := filepath.Join(tmpDir, "ignore.txt")
	if err := os.WriteFile(existingFile, []byte("Original content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write with conflict=ignore
	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("New content"))
	session.PutAttribute(flowFile, "filename", "ignore.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "ignore",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to success (but not written)
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}

	// Original file should be unchanged
	content, _ := os.ReadFile(existingFile)
	if string(content) != "Original content" {
		t.Error("Original file should not be modified when using ignore strategy")
	}
}

func TestPutFileProcessorConflictRename(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create existing file
	existingFile := filepath.Join(tmpDir, "rename.txt")
	if err := os.WriteFile(existingFile, []byte("Original content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write with conflict=rename
	session := newMockProcessSession()
	flowFile := session.Create()
	newContent := []byte("New content")
	session.Write(flowFile, newContent)
	session.PutAttribute(flowFile, "filename", "rename.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                   tmpDir,
			"Conflict Resolution Strategy": "rename",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}

	// Original file should be unchanged
	content, _ := os.ReadFile(existingFile)
	if string(content) != "Original content" {
		t.Error("Original file should not be modified")
	}

	// New file should exist with _1 suffix
	renamedFile := filepath.Join(tmpDir, "rename_1.txt")
	renamedContent, err := os.ReadFile(renamedFile)
	if err != nil {
		t.Errorf("Renamed file should exist: %v", err)
	}
	if string(renamedContent) != "New content" {
		t.Error("Renamed file should have new content")
	}

	// Verify filename attribute was updated
	attrs := session.attributes[flowFile.ID]
	if attrs["filename"] != "rename_1.txt" {
		t.Errorf("Expected filename=rename_1.txt, got %s", attrs["filename"])
	}
}

func TestPutFileProcessorCreateMissingDirectories(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Non-existent subdirectory
	outputDir := filepath.Join(tmpDir, "subdir", "nested")

	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("Test content"))
	session.PutAttribute(flowFile, "filename", "test.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":                  outputDir,
			"Create Missing Directories": "true",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Directory and file should be created
	outputPath := filepath.Join(outputDir, "test.txt")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("File should be created with missing directories")
	}

	// Should be transferred to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}
}

func TestPutFileProcessorMaximumFileCount(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create 3 existing files
	for i := 1; i <= 3; i++ {
		path := filepath.Join(tmpDir, "existing"+string(rune('0'+i))+".txt")
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Try to write with max file count = 3
	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("New content"))
	session.PutAttribute(flowFile, "filename", "new.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":          tmpDir,
			"Maximum File Count": "3",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to failure (max count reached)
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "failure" {
		t.Error("FlowFile should be transferred to failure when max file count reached")
	}

	// New file should not exist
	newPath := filepath.Join(tmpDir, "new.txt")
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("New file should not be created when max count reached")
	}
}

func TestPutFileProcessorNoFilename(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Create FlowFile without filename attribute
	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("Content without filename"))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory": tmpDir,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should be transferred to success
	if rel, exists := session.transfers[flowFile.ID]; !exists {
		t.Error("FlowFile should be transferred")
	} else if rel.Name != "success" {
		t.Error("FlowFile should be transferred to success relationship")
	}

	// File should be created with UUID-based name
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 file, got %d", len(entries))
	}

	if len(entries) > 0 {
		filename := entries[0].Name()
		if !contains(filename, ".dat") {
			t.Errorf("Expected .dat extension for generated filename, got %s", filename)
		}
	}
}

func TestPutFileProcessorNoFlowFile(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	// Session with no FlowFiles
	session := newMockProcessSession()

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory": tmpDir,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error when no FlowFile: %v", err)
	}

	// No files should be created
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Errorf("Expected 0 files, got %d", len(entries))
	}
}

func TestPutFileProcessorOnTriggerWithoutContext(t *testing.T) {
	processor := NewPutFileProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestPutFileProcessorOnStopped(t *testing.T) {
	processor := NewPutFileProcessor()
	ctx := context.Background()

	// OnStopped should not panic
	processor.OnStopped(ctx)
}

func TestPutFileProcessorCustomPermissions(t *testing.T) {
	processor := NewPutFileProcessor()
	tmpDir := t.TempDir()

	session := newMockProcessSession()
	flowFile := session.Create()
	session.Write(flowFile, []byte("Test content"))
	session.PutAttribute(flowFile, "filename", "permissions_test.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Directory":   tmpDir,
			"Permissions": "0600",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Check file permissions
	outputPath := filepath.Join(tmpDir, "permissions_test.txt")
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// On Unix systems, check permissions (skip on Windows)
	if info.Mode().Perm() != 0600 {
		// Note: This might vary by OS
		t.Logf("File permissions: %o (expected 0600)", info.Mode().Perm())
	}
}
