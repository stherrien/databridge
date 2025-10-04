package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewSplitTextProcessor(t *testing.T) {
	processor := NewSplitTextProcessor()

	if processor == nil {
		t.Fatal("NewSplitTextProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "SplitText" {
		t.Errorf("Expected processor name SplitText, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 4 {
		t.Errorf("Expected 4 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 3 {
		t.Errorf("Expected 3 relationships, got %d", len(info.Relationships))
	}

	// Check relationships
	expectedRels := map[string]bool{
		"success": false,
		"splits":  false,
		"failure": false,
	}

	for _, rel := range info.Relationships {
		if _, exists := expectedRels[rel.Name]; exists {
			expectedRels[rel.Name] = true
		}
	}

	for name, found := range expectedRels {
		if !found {
			t.Errorf("Expected relationship %s not found", name)
		}
	}
}

func TestSplitTextProcessorInitialize(t *testing.T) {
	processor := NewSplitTextProcessor()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "5",
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid properties: %v", err)
	}

	// Test with invalid line split count
	ctxInvalid := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "0",
		},
	}

	err = processor.Initialize(ctxInvalid)
	if err == nil {
		t.Error("Initialize should fail with zero line split count")
	}
}

func TestSplitTextProcessorValidate(t *testing.T) {
	processor := NewSplitTextProcessor()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Line Split Count":         "10",
			"Remove Trailing Newlines": "true",
			"Header Line Count":        "2",
			"Maximum Fragment Size":    "1024",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test invalid line split count
	invalidCountConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Line Split Count": "not-a-number",
		},
	}

	results = processor.Validate(invalidCountConfig)
	hasCountError := false
	for _, result := range results {
		if result.Property == "Line Split Count" && !result.Valid {
			hasCountError = true
			break
		}
	}

	if !hasCountError {
		t.Error("Invalid line split count should cause validation error")
	}

	// Test negative header count
	negativeHeaderConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Header Line Count": "-1",
		},
	}

	results = processor.Validate(negativeHeaderConfig)
	hasHeaderError := false
	for _, result := range results {
		if result.Property == "Header Line Count" && !result.Valid {
			hasHeaderError = true
			break
		}
	}

	if !hasHeaderError {
		t.Error("Negative header count should cause validation error")
	}
}

func TestSplitTextProcessorSimpleSplit(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	session.Write(flowFile, []byte(content))
	session.PutAttribute(flowFile, "filename", "test.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "2",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Count split FlowFiles (original + splits)
	splitCount := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			splitCount++
		}
	}

	// With 5 lines and split count of 2, should create 3 splits
	expectedSplits := 3
	if splitCount != expectedSplits {
		t.Errorf("Expected %d split FlowFiles, got %d", expectedSplits, splitCount)
	}

	// Verify original transferred to success
	originalTransferred := false
	if rel, exists := session.transfers[flowFile.ID]; exists && rel.Name == "success" {
		originalTransferred = true
	}

	if !originalTransferred {
		t.Error("Original FlowFile should be transferred to success")
	}
}

func TestSplitTextProcessorWithHeaders(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Header1\nHeader2\nData1\nData2\nData3"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count":  "2",
			"Header Line Count": "2",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify split contents contain headers
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			content := session.writtenContent[ff.ID]
			contentStr := string(content)

			// Each split should contain headers
			if !strings.Contains(contentStr, "Header1") || !strings.Contains(contentStr, "Header2") {
				t.Errorf("Split should contain headers. Got: %s", contentStr)
			}
		}
	}
}

func TestSplitTextProcessorRemoveTrailingNewlines(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count":         "1",
			"Remove Trailing Newlines": "true",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Check that splits don't have trailing newlines
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			content := session.writtenContent[ff.ID]
			contentStr := string(content)

			if strings.HasSuffix(contentStr, "\n") {
				t.Errorf("Split should not have trailing newline when removal is enabled. Got: %q", contentStr)
			}
		}
	}
}

func TestSplitTextProcessorKeepTrailingNewlines(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count":         "1",
			"Remove Trailing Newlines": "false",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Check that splits have trailing newlines
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			content := session.writtenContent[ff.ID]
			contentStr := string(content)

			if !strings.HasSuffix(contentStr, "\n") {
				t.Errorf("Split should have trailing newline when removal is disabled. Got: %q", contentStr)
			}
		}
	}
}

func TestSplitTextProcessorFragmentAttributes(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2\nLine 3"
	session.Write(flowFile, []byte(content))
	session.PutAttribute(flowFile, "filename", "original.txt")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "1",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify fragment attributes
	fragmentIndex := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			attrs := session.attributes[ff.ID]

			// Check required attributes
			if attrs["fragment.index"] == "" {
				t.Error("Split should have fragment.index attribute")
			}

			if attrs["fragment.count"] != "3" {
				t.Errorf("Expected fragment.count=3, got %s", attrs["fragment.count"])
			}

			if attrs["fragment.identifier"] == "" {
				t.Error("Split should have fragment.identifier attribute")
			}

			// Check filename generation
			expectedFilename := "original_split_" + attrs["fragment.index"] + ".txt"
			if attrs["filename"] != expectedFilename {
				t.Errorf("Expected filename=%s, got %s", expectedFilename, attrs["filename"])
			}

			fragmentIndex++
		}
	}
}

func TestSplitTextProcessorMaximumFragmentSize(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	// Create content that will exceed max size
	content := "This is a long line that will be truncated\nAnother line"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count":      "1",
			"Maximum Fragment Size": "20",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify splits don't exceed max size
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			content := session.writtenContent[ff.ID]

			if len(content) > 20 {
				t.Errorf("Split size %d exceeds maximum of 20", len(content))
			}
		}
	}
}

func TestSplitTextProcessorEmptyContent(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	session.Write(flowFile, []byte(""))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "1",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// No splits should be created for empty content
	splitCount := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			splitCount++
		}
	}

	if splitCount != 0 {
		t.Errorf("Expected 0 splits for empty content, got %d", splitCount)
	}

	// Original should still be transferred to success
	if rel, exists := session.transfers[flowFile.ID]; !exists || rel.Name != "success" {
		t.Error("Original FlowFile should be transferred to success")
	}
}

func TestSplitTextProcessorSingleLine(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Single line"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "1",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should create 1 split
	splitCount := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			splitCount++
		}
	}

	if splitCount != 1 {
		t.Errorf("Expected 1 split for single line, got %d", splitCount)
	}
}

func TestSplitTextProcessorDifferentLineEndings(t *testing.T) {
	processor := NewSplitTextProcessor()

	testCases := []struct {
		name    string
		content string
	}{
		{"Unix (LF)", "Line 1\nLine 2\nLine 3"},
		{"Windows (CRLF)", "Line 1\r\nLine 2\r\nLine 3"},
		{"Mac (CR)", "Line 1\rLine 2\rLine 3"},
		{"Mixed", "Line 1\nLine 2\r\nLine 3\r"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			session := newMockProcessSession()
			flowFile := session.Create()

			session.Write(flowFile, []byte(tc.content))
			session.createdFlowFiles = []*types.FlowFile{flowFile}

			processorCtx := &mockProcessorContext{
				properties: map[string]string{
					"Line Split Count": "1",
				},
			}

			ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

			err := processor.OnTrigger(ctx, session)
			if err != nil {
				t.Errorf("OnTrigger should not return error: %v", err)
			}

			// Should create 3 splits regardless of line ending type
			splitCount := 0
			for _, ff := range session.createdFlowFiles {
				if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
					splitCount++
				}
			}

			if splitCount != 3 {
				t.Errorf("Expected 3 splits for different line endings, got %d", splitCount)
			}
		})
	}
}

func TestSplitTextProcessorLargeSplitCount(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2\nLine 3"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "100", // Larger than content
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should create 1 split (all lines in one)
	splitCount := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			splitCount++

			// Verify all lines are in this split
			content := string(session.writtenContent[ff.ID])
			if !strings.Contains(content, "Line 1") || !strings.Contains(content, "Line 2") || !strings.Contains(content, "Line 3") {
				t.Error("Single split should contain all lines")
			}
		}
	}

	if splitCount != 1 {
		t.Errorf("Expected 1 split when split count exceeds line count, got %d", splitCount)
	}
}

func TestSplitTextProcessorNoFlowFile(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "1",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error when no FlowFile: %v", err)
	}
}

func TestSplitTextProcessorOnTriggerWithoutContext(t *testing.T) {
	processor := NewSplitTextProcessor()
	session := newMockProcessSession()
	ctx := context.Background() // No processor context

	err := processor.OnTrigger(ctx, session)
	if err == nil {
		t.Error("OnTrigger should return error when processor context is not available")
	}
}

func TestSplitTextProcessorOnStopped(t *testing.T) {
	processor := NewSplitTextProcessor()
	ctx := context.Background()

	// OnStopped should not panic
	processor.OnStopped(ctx)
}

func TestSplitTextProcessorHeaderCountExceedsLines(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2"
	session.Write(flowFile, []byte(content))
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count":  "1",
			"Header Line Count": "10", // More than total lines
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Should handle gracefully - when header count exceeds total lines,
	// processor treats all lines as data (no headers)
	splitCount := 0
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			splitCount++
		}
	}

	// With 2 lines and split count of 1, should create 2 splits
	if splitCount != 2 {
		t.Errorf("Expected 2 splits when header count exceeds total lines, got %d", splitCount)
	}
}

func TestSplitTextProcessorFilenameWithoutExtension(t *testing.T) {
	processor := NewSplitTextProcessor()

	session := newMockProcessSession()
	flowFile := session.Create()

	content := "Line 1\nLine 2"
	session.Write(flowFile, []byte(content))
	session.PutAttribute(flowFile, "filename", "testfile")
	session.createdFlowFiles = []*types.FlowFile{flowFile}

	processorCtx := &mockProcessorContext{
		properties: map[string]string{
			"Line Split Count": "1",
		},
	}

	ctx := context.WithValue(context.Background(), "processorContext", processorCtx)

	err := processor.OnTrigger(ctx, session)
	if err != nil {
		t.Errorf("OnTrigger should not return error: %v", err)
	}

	// Verify filename generation works without extension
	for _, ff := range session.createdFlowFiles {
		if rel, exists := session.transfers[ff.ID]; exists && rel.Name == "splits" {
			attrs := session.attributes[ff.ID]
			filename := attrs["filename"]

			if !strings.HasPrefix(filename, "testfile_split_") {
				t.Errorf("Expected filename to start with 'testfile_split_', got %s", filename)
			}
		}
	}
}
