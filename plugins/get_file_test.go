package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewGetFileProcessor(t *testing.T) {
	processor := NewGetFileProcessor()

	if processor == nil {
		t.Fatal("NewGetFileProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "GetFile" {
		t.Errorf("Expected processor name GetFile, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 7 {
		t.Errorf("Expected 7 properties, got %d", len(info.Properties))
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

func TestGetFileProcessorInitialize(t *testing.T) {
	processor := NewGetFileProcessor()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpDir,
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid directory: %v", err)
	}

	// Test initialization without Input Directory property
	ctxNoDir := &mockProcessorContext{
		properties: map[string]string{},
	}

	err = processor.Initialize(ctxNoDir)
	if err == nil {
		t.Error("Initialize should fail without Input Directory property")
	}

	// Test initialization with empty directory
	ctxEmptyDir := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": "",
		},
	}

	err = processor.Initialize(ctxEmptyDir)
	if err == nil {
		t.Error("Initialize should fail with empty directory")
	}

	// Test initialization with non-existent directory
	ctxInvalidDir := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": "/nonexistent/directory/path",
		},
	}

	err = processor.Initialize(ctxInvalidDir)
	if err == nil {
		t.Error("Initialize should fail with non-existent directory")
	}

	// Test initialization with a file (not directory)
	tmpFile := filepath.Join(tmpDir, "not_a_dir.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxFileNotDir := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpFile,
		},
	}

	err = processor.Initialize(ctxFileNotDir)
	if err == nil {
		t.Error("Initialize should fail when path is not a directory")
	}
}

func TestGetFileProcessorValidate(t *testing.T) {
	processor := NewGetFileProcessor()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Input Directory":        tmpDir,
			"File Filter":            "*.txt",
			"Keep Source File":       "true",
			"Recurse Subdirectories": "false",
			"Minimum File Age":       "10s",
			"Maximum File Age":       "1h",
			"Batch Size":             "5",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test invalid batch size
	invalidBatchConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Input Directory": tmpDir,
			"Batch Size":      "not-a-number",
		},
	}

	results = processor.Validate(invalidBatchConfig)
	hasBatchError := false
	for _, result := range results {
		if result.Property == "Batch Size" && !result.Valid {
			hasBatchError = true
			break
		}
	}

	if !hasBatchError {
		t.Error("Invalid batch size should cause validation error")
	}

	// Test invalid minimum age
	invalidMinAgeConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Input Directory":  tmpDir,
			"Minimum File Age": "invalid-duration",
		},
	}

	results = processor.Validate(invalidMinAgeConfig)
	hasAgeError := false
	for _, result := range results {
		if result.Property == "Minimum File Age" && !result.Valid {
			hasAgeError = true
			break
		}
	}

	if !hasAgeError {
		t.Error("Invalid minimum age should cause validation error")
	}

	// Test invalid maximum age
	invalidMaxAgeConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Input Directory":  tmpDir,
			"Maximum File Age": "invalid-duration",
		},
	}

	results = processor.Validate(invalidMaxAgeConfig)
	hasMaxAgeError := false
	for _, result := range results {
		if result.Property == "Maximum File Age" && !result.Valid {
			hasMaxAgeError = true
			break
		}
	}

	if !hasMaxAgeError {
		t.Error("Invalid maximum age should cause validation error")
	}

	// Test non-existent directory
	nonExistentConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Input Directory": "/nonexistent/path",
		},
	}

	results = processor.Validate(nonExistentConfig)
	hasDirError := false
	for _, result := range results {
		if result.Property == "Input Directory" && !result.Valid {
			hasDirError = true
			break
		}
	}

	if !hasDirError {
		t.Error("Non-existent directory should cause validation error")
	}
}

func TestGetFileProcessorOnTrigger(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create test files
	testFiles := []struct {
		name    string
		content string
	}{
		{"test1.txt", "Content 1"},
		{"test2.txt", "Content 2"},
		{"test3.log", "Log content"},
	}

	for _, tf := range testFiles {
		path := filepath.Join(tmpDir, tf.name)
		if err := os.WriteFile(path, []byte(tf.content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Test processing with default filter (all files)
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpDir,
			"File Filter":     "*",
			"Batch Size":      "10",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify FlowFiles were created
	if len(session.createdFlowFiles) != 3 {
		t.Errorf("Expected 3 FlowFiles created, got %d", len(session.createdFlowFiles))
	}

	// Verify content and attributes
	for _, ff := range session.createdFlowFiles {
		content, exists := session.writtenContent[ff.ID]
		if !exists {
			t.Error("Content should be written to FlowFile")
			continue
		}

		attrs := session.attributes[ff.ID]
		if attrs == nil {
			t.Error("FlowFile should have attributes")
			continue
		}

		// Check required attributes
		requiredAttrs := []string{"filename", "path", "absolute.path", "file.size", "file.lastModified"}
		for _, attr := range requiredAttrs {
			if _, exists := attrs[attr]; !exists {
				t.Errorf("FlowFile should have attribute %s", attr)
			}
		}

		// Verify transfer to success
		if rel, exists := session.transfers[ff.ID]; !exists {
			t.Error("FlowFile should be transferred")
		} else if rel.Name != "success" {
			t.Error("FlowFile should be transferred to success relationship")
		}

		// Verify content matches one of the test files
		contentStr := string(content)
		validContent := false
		for _, tf := range testFiles {
			if contentStr == tf.content {
				validContent = true
				break
			}
		}
		if !validContent {
			t.Errorf("Unexpected content: %s", contentStr)
		}
	}
}

func TestGetFileProcessorWithFileFilter(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create test files with different extensions
	files := map[string]string{
		"test1.txt": "Text 1",
		"test2.txt": "Text 2",
		"test.log":  "Log",
		"test.json": "{}",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Test with *.txt filter
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpDir,
			"File Filter":     "*.txt",
			"Batch Size":      "10",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should only find .txt files
	if len(session.createdFlowFiles) != 2 {
		t.Errorf("Expected 2 .txt FlowFiles, got %d", len(session.createdFlowFiles))
	}

	// Verify all are .txt files
	for _, ff := range session.createdFlowFiles {
		attrs := session.attributes[ff.ID]
		filename := attrs["filename"]
		matched, err := filepath.Match("*.txt", filename)
		if err != nil || !matched {
			t.Errorf("Expected .txt file, got %s", filename)
		}
	}
}

func TestGetFileProcessorWithBatchSize(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create 5 test files
	for i := 1; i <= 5; i++ {
		path := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".txt")
		if err := os.WriteFile(path, []byte("Content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Test with batch size of 3
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpDir,
			"Batch Size":      "3",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should only process 3 files due to batch size
	if len(session.createdFlowFiles) != 3 {
		t.Errorf("Expected 3 FlowFiles (batch size), got %d", len(session.createdFlowFiles))
	}
}

func TestGetFileProcessorKeepSourceFile(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Test content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test with Keep Source File = true
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory":  tmpDir,
			"Keep Source File": "true",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify file still exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Source file should still exist when Keep Source File is true")
	}

	// Test with Keep Source File = false (delete)
	testFile2 := filepath.Join(tmpDir, "test2.txt")
	if err := os.WriteFile(testFile2, []byte("Test content 2"), 0644); err != nil {
		t.Fatal(err)
	}

	session2 := newMockProcessSession()
	processorCtx2 := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory":  tmpDir,
			"Keep Source File": "false",
			"File Filter":      "test2.txt",
		},
	}

	ctx2 := context.WithValue(context.Background(), "processorContext", processorCtx2)

	err = processor.OnTrigger(ctx2, session2)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(testFile2); !os.IsNotExist(err) {
		t.Error("Source file should be deleted when Keep Source File is false")
	}
}

func TestGetFileProcessorRecurseSubdirectories(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create subdirectory structure
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files in root and subdirectory
	rootFile := filepath.Join(tmpDir, "root.txt")
	subFile := filepath.Join(subDir, "sub.txt")

	if err := os.WriteFile(rootFile, []byte("Root content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subFile, []byte("Sub content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test without recursion
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory":        tmpDir,
			"Recurse Subdirectories": "false",
			"Keep Source File":       "true", // Keep files for second test
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should only find root file
	if len(session.createdFlowFiles) != 1 {
		t.Errorf("Expected 1 FlowFile (no recursion), got %d", len(session.createdFlowFiles))
	}

	// Test with recursion
	session2 := newMockProcessSession()
	processorCtx2 := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory":        tmpDir,
			"Recurse Subdirectories": "true",
			"Keep Source File":       "true", // Keep files for this test
		},
	}

	ctx2 := context.WithValue(context.Background(), "processorContext", processorCtx2)

	err = processor.OnTrigger(ctx2, session2)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should find both files
	if len(session2.createdFlowFiles) != 2 {
		t.Errorf("Expected 2 FlowFiles (with recursion), got %d", len(session2.createdFlowFiles))
	}
}

func TestGetFileProcessorMinimumFileAge(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Create an old file
	oldFile := filepath.Join(tmpDir, "old.txt")
	if err := os.WriteFile(oldFile, []byte("Old content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Modify the file's timestamp to be 2 seconds old
	oldTime := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	// Create a new file
	newFile := filepath.Join(tmpDir, "new.txt")
	if err := os.WriteFile(newFile, []byte("New content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test with minimum age of 1 second (should only get old file)
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory":  tmpDir,
			"Minimum File Age": "1s",
			"Keep Source File": "true",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should only find the old file
	if len(session.createdFlowFiles) != 1 {
		t.Errorf("Expected 1 FlowFile (old file only), got %d", len(session.createdFlowFiles))
	}

	if len(session.createdFlowFiles) > 0 {
		attrs := session.attributes[session.createdFlowFiles[0].ID]
		if attrs["filename"] != "old.txt" {
			t.Errorf("Expected old.txt, got %s", attrs["filename"])
		}
	}
}

func TestGetFileProcessorNoFiles(t *testing.T) {
	processor := NewGetFileProcessor()
	tmpDir := t.TempDir()

	// Empty directory
	session := newMockProcessSession()
	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Input Directory": tmpDir,
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// No FlowFiles should be created
	if len(session.createdFlowFiles) != 0 {
		t.Errorf("Expected 0 FlowFiles, got %d", len(session.createdFlowFiles))
	}
}

func TestGetFileProcessorOnTriggerWithoutContext(t *testing.T) {
	processor := NewGetFileProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestGetFileProcessorOnStopped(t *testing.T) {
	processor := NewGetFileProcessor()
	ctx := context.Background()

	// OnStopped should not panic
	processor.OnStopped(ctx)
}
