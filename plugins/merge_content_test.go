package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Note: Mock implementations are already defined in generate_flowfile_test.go
// These additional helpers extend the mock session for merge testing

// mockProcessSessionWithQueue extends mockProcessSession for batch operations
type mockProcessSessionWithQueue struct {
	*mockProcessSession
	queue []*types.FlowFile
}

func newMockProcessSessionWithQueue() *mockProcessSessionWithQueue {
	return &mockProcessSessionWithQueue{
		mockProcessSession: newMockProcessSession(),
		queue:              make([]*types.FlowFile, 0),
	}
}

func (s *mockProcessSessionWithQueue) addToQueue(ff *types.FlowFile) {
	s.queue = append(s.queue, ff)
}

func (s *mockProcessSessionWithQueue) GetBatch(maxResults int) []*types.FlowFile {
	count := maxResults
	if count > len(s.queue) {
		count = len(s.queue)
	}
	if count == 0 {
		return nil
	}
	result := s.queue[:count]
	s.queue = s.queue[count:]
	return result
}

func (s *mockProcessSessionWithQueue) Clone(original *types.FlowFile) *types.FlowFile {
	flowFile := types.NewFlowFile()
	// Copy attributes
	for k, v := range original.Attributes {
		flowFile.UpdateAttribute(k, v)
	}
	s.createdFlowFiles = append(s.createdFlowFiles, flowFile)
	s.attributes[flowFile.ID] = make(map[string]string)
	for k, v := range original.Attributes {
		s.attributes[flowFile.ID][k] = v
	}
	return flowFile
}

func (s *mockProcessSessionWithQueue) getTransferred(relationship types.Relationship) []*types.FlowFile {
	var result []*types.FlowFile
	for _, ff := range s.createdFlowFiles {
		if rel, exists := s.transfers[ff.ID]; exists && rel.Name == relationship.Name {
			result = append(result, ff)
		}
	}
	return result
}

func (s *mockProcessSessionWithQueue) getRemovedFlowFiles() []uuid.UUID {
	return s.removals
}

func TestNewMergeContentProcessor(t *testing.T) {
	processor := NewMergeContentProcessor()

	if processor == nil {
		t.Fatal("NewMergeContentProcessor should return a processor")
	}

	info := processor.GetInfo()
	if info.Name != "MergeContent" {
		t.Errorf("Expected processor name MergeContent, got %s", info.Name)
	}

	if info.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", info.Version)
	}

	if len(info.Properties) != 11 {
		t.Errorf("Expected 11 properties, got %d", len(info.Properties))
	}

	if len(info.Relationships) != 3 {
		t.Errorf("Expected 3 relationships, got %d", len(info.Relationships))
	}

	// Check relationships
	expectedRels := map[string]bool{
		"merged":   false,
		"original": false,
		"failure":  false,
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

func TestMergeContentProcessorInitialize(t *testing.T) {
	processor := NewMergeContentProcessor()

	// Test successful initialization
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Minimum Number of Entries": "2",
			"Maximum Number of Entries": "10",
		},
	}

	err := processor.Initialize(ctx)
	if err != nil {
		t.Errorf("Initialize should succeed with valid properties: %v", err)
	}

	// Test with invalid minimum entries
	ctxInvalid := &mockProcessorContext{
		properties: map[string]string{
			"Minimum Number of Entries": "0",
		},
	}

	err = processor.Initialize(ctxInvalid)
	if err == nil {
		t.Error("Initialize should fail with zero minimum entries")
	}

	// Test with negative maximum entries
	ctxInvalid2 := &mockProcessorContext{
		properties: map[string]string{
			"Maximum Number of Entries": "-5",
		},
	}

	err = processor.Initialize(ctxInvalid2)
	if err == nil {
		t.Error("Initialize should fail with negative maximum entries")
	}
}

func TestMergeContentProcessorValidate(t *testing.T) {
	processor := NewMergeContentProcessor()

	// Test valid configuration
	validConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Merge Strategy":            "Bin-Packing",
			"Merge Format":              "Binary Concatenation",
			"Minimum Number of Entries": "2",
			"Maximum Number of Entries": "100",
			"Minimum Group Size":        "0",
			"Maximum Group Size":        "1048576",
			"Max Bin Age":               "30s",
		},
	}

	results := processor.Validate(validConfig)
	for _, result := range results {
		if !result.Valid {
			t.Errorf("Valid configuration should pass validation: %s - %s", result.Property, result.Message)
		}
	}

	// Test invalid minimum entries
	invalidConfig := types.ProcessorConfig{
		Properties: map[string]string{
			"Minimum Number of Entries": "0",
		},
	}

	results = processor.Validate(invalidConfig)
	hasError := false
	for _, result := range results {
		if !result.Valid && result.Property == "Minimum Number of Entries" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("Should have validation error for invalid minimum entries")
	}

	// Test invalid maximum entries (less than minimum)
	invalidConfig2 := types.ProcessorConfig{
		Properties: map[string]string{
			"Minimum Number of Entries": "10",
			"Maximum Number of Entries": "5",
		},
	}

	results = processor.Validate(invalidConfig2)
	hasError = false
	for _, result := range results {
		if !result.Valid && result.Property == "Maximum Number of Entries" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("Should have validation error when max < min")
	}

	// Test invalid duration format
	invalidConfig3 := types.ProcessorConfig{
		Properties: map[string]string{
			"Max Bin Age": "invalid-duration",
		},
	}

	results = processor.Validate(invalidConfig3)
	hasError = false
	for _, result := range results {
		if !result.Valid && result.Property == "Max Bin Age" {
			hasError = true
		}
	}
	if !hasError {
		t.Error("Should have validation error for invalid duration")
	}
}

func TestMergeContentBinPackingStrategy(t *testing.T) {
	processor := NewMergeContentProcessor()

	// Create test FlowFiles with different sizes
	flowFiles := []*types.FlowFile{
		{ID: uuid.New(), Size: 100},
		{ID: uuid.New(), Size: 200},
		{ID: uuid.New(), Size: 150},
		{ID: uuid.New(), Size: 300},
		{ID: uuid.New(), Size: 50},
	}

	// Test with max count constraint
	groups := processor.binPackingStrategy(flowFiles, 1, 3, 0, 0)
	if len(groups) != 2 {
		t.Errorf("Expected 2 groups with max count 3, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("Expected first group to have 3 items, got %d", len(groups[0]))
	}
	if len(groups[1]) != 2 {
		t.Errorf("Expected second group to have 2 items, got %d", len(groups[1]))
	}

	// Test with max size constraint
	groups = processor.binPackingStrategy(flowFiles, 1, 0, 0, 400)
	if len(groups) < 2 {
		t.Errorf("Expected at least 2 groups with size constraint 400, got %d", len(groups))
	}

	// Test with no constraints
	groups = processor.binPackingStrategy(flowFiles, 1, 0, 0, 0)
	if len(groups) != 1 {
		t.Errorf("Expected 1 group with no constraints, got %d", len(groups))
	}
	if len(groups[0]) != 5 {
		t.Errorf("Expected group to have 5 items, got %d", len(groups[0]))
	}
}

func TestMergeContentDefragmentStrategy(t *testing.T) {
	processor := NewMergeContentProcessor()

	// Create test FlowFiles with fragment attributes
	flowFiles := []*types.FlowFile{
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"fragment.identifier": "group1",
				"fragment.index":      "0",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"fragment.identifier": "group1",
				"fragment.index":      "2",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"fragment.identifier": "group1",
				"fragment.index":      "1",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"fragment.identifier": "group2",
				"fragment.index":      "0",
			},
		},
	}

	groups := processor.defragmentStrategy(flowFiles)

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(groups))
	}

	// Find the group with 3 items (group1)
	var group1 []*types.FlowFile
	for _, group := range groups {
		if len(group) == 3 {
			group1 = group
			break
		}
	}

	if group1 == nil {
		t.Fatal("Could not find group1 with 3 items")
	}

	// Verify fragments are sorted by index
	for i, ff := range group1 {
		index, _ := ff.GetAttribute("fragment.index")
		expectedIndex := string(rune('0' + i))
		if index != expectedIndex {
			t.Errorf("Expected fragment index %s at position %d, got %s", expectedIndex, i, index)
		}
	}
}

func TestMergeContentAttributeBasedStrategy(t *testing.T) {
	processor := NewMergeContentProcessor()

	// Create test FlowFiles with correlation attributes
	flowFiles := []*types.FlowFile{
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"merge.group": "A",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"merge.group": "B",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"merge.group": "A",
			},
		},
		{
			ID: uuid.New(),
			Attributes: map[string]string{
				"merge.group": "C",
			},
		},
	}

	groups := processor.attributeBasedStrategy(flowFiles, "merge.group")

	if len(groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}

	// Count FlowFiles by group
	groupCounts := make(map[int]int)
	for _, group := range groups {
		groupCounts[len(group)]++
	}

	// Should have one group with 2 items (A) and two groups with 1 item each (B, C)
	if groupCounts[2] != 1 {
		t.Errorf("Expected 1 group with 2 items, got %d", groupCounts[2])
	}
	if groupCounts[1] != 2 {
		t.Errorf("Expected 2 groups with 1 item, got %d", groupCounts[1])
	}
}

func TestMergeContentBinaryConcatenation(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create test FlowFiles with content
	ff1 := session.Create()
	session.Write(ff1, []byte("Hello"))

	ff2 := session.Create()
	session.Write(ff2, []byte("World"))

	ff3 := session.Create()
	session.Write(ff3, []byte("!"))

	group := []*types.FlowFile{ff1, ff2, ff3}

	result := processor.binaryConcatenation(session, group)

	expected := "HelloWorld!"
	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}
}

func TestMergeContentTextConcatenation(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create test FlowFiles with content
	ff1 := session.Create()
	session.Write(ff1, []byte("Line1"))

	ff2 := session.Create()
	session.Write(ff2, []byte("Line2"))

	ff3 := session.Create()
	session.Write(ff3, []byte("Line3"))

	group := []*types.FlowFile{ff1, ff2, ff3}

	// Test with newline delimiter
	result := processor.textConcatenation(session, group, "\n")
	expected := "Line1\nLine2\nLine3"
	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}

	// Test with tab delimiter
	result = processor.textConcatenation(session, group, "\t")
	expected = "Line1\tLine2\tLine3"
	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}

	// Test with custom delimiter
	result = processor.textConcatenation(session, group, " | ")
	expected = "Line1 | Line2 | Line3"
	if string(result) != expected {
		t.Errorf("Expected %s, got %s", expected, string(result))
	}
}

func TestMergeContentGetDelimiter(t *testing.T) {
	processor := NewMergeContentProcessor()

	tests := []struct {
		strategy   string
		customText string
		expected   string
	}{
		{"None", "", ""},
		{"Newline", "", "\n"},
		{"Tab", "", "\t"},
		{"Custom", "||", "||"},
		{"Unknown", "", "\n"}, // Default to newline
	}

	for _, test := range tests {
		result := processor.getDelimiter(test.strategy, test.customText)
		if result != test.expected {
			t.Errorf("For strategy %s, expected %q, got %q", test.strategy, test.expected, result)
		}
	}
}

func TestMergeContentOnTriggerBinPacking(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create test FlowFiles
	ff1 := session.Create()
	session.Write(ff1, []byte("Content1"))

	ff2 := session.Create()
	session.Write(ff2, []byte("Content2"))

	ff3 := session.Create()
	session.Write(ff3, []byte("Content3"))

	// Add to session queue
	session.addToQueue(ff1)
	session.addToQueue(ff2)
	session.addToQueue(ff3)

	// Create processor context
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy":            "Bin-Packing",
			"Merge Format":              "Binary Concatenation",
			"Minimum Number of Entries": "2",
			"Maximum Number of Entries": "10",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger failed: %v", err)
	}

	// Verify merged FlowFile was created
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) != 1 {
		t.Errorf("Expected 1 merged FlowFile, got %d", len(merged))
	}

	if len(merged) > 0 {
		content, _ := session.Read(merged[0])
		expected := "Content1Content2Content3"
		if string(content) != expected {
			t.Errorf("Expected merged content %s, got %s", expected, string(content))
		}

		// Check merge attributes
		if count, exists := merged[0].GetAttribute("merge.count"); !exists || count != "3" {
			t.Errorf("Expected merge.count=3, got %v", count)
		}
	}

	// Verify originals were removed
	removed := session.getRemovedFlowFiles()
	if len(removed) != 3 {
		t.Errorf("Expected 3 removed FlowFile IDs, got %d", len(removed))
	}
}

func TestMergeContentOnTriggerTextConcatenation(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create test FlowFiles
	ff1 := session.Create()
	session.Write(ff1, []byte("Line1"))

	ff2 := session.Create()
	session.Write(ff2, []byte("Line2"))

	// Add to session queue
	session.addToQueue(ff1)
	session.addToQueue(ff2)

	// Create processor context
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy":            "Bin-Packing",
			"Merge Format":              "Text Concatenation",
			"Delimiter Strategy":        "Newline",
			"Minimum Number of Entries": "1",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger failed: %v", err)
	}

	// Verify merged FlowFile content
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) > 0 {
		content, _ := session.Read(merged[0])
		expected := "Line1\nLine2"
		if string(content) != expected {
			t.Errorf("Expected merged content %s, got %s", expected, string(content))
		}
	}
}

func TestMergeContentOnTriggerDefragment(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create fragmented FlowFiles
	ff1 := session.Create()
	session.Write(ff1, []byte("Part1"))
	session.PutAttribute(ff1, "fragment.identifier", "doc1")
	session.PutAttribute(ff1, "fragment.index", "0")

	ff2 := session.Create()
	session.Write(ff2, []byte("Part2"))
	session.PutAttribute(ff2, "fragment.identifier", "doc1")
	session.PutAttribute(ff2, "fragment.index", "1")

	ff3 := session.Create()
	session.Write(ff3, []byte("Part3"))
	session.PutAttribute(ff3, "fragment.identifier", "doc1")
	session.PutAttribute(ff3, "fragment.index", "2")

	// Add to session queue
	session.addToQueue(ff1)
	session.addToQueue(ff2)
	session.addToQueue(ff3)

	// Create processor context
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy":            "Defragment",
			"Merge Format":              "Binary Concatenation",
			"Minimum Number of Entries": "1",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger failed: %v", err)
	}

	// Verify merged FlowFile
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) != 1 {
		t.Errorf("Expected 1 merged FlowFile, got %d", len(merged))
	}

	if len(merged) > 0 {
		content, _ := session.Read(merged[0])
		expected := "Part1Part2Part3"
		if string(content) != expected {
			t.Errorf("Expected merged content %s, got %s", expected, string(content))
		}
	}
}

func TestMergeContentOnTriggerMinEntries(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create only one FlowFile
	ff1 := session.Create()
	session.Write(ff1, []byte("Content1"))
	session.addToQueue(ff1)

	// Create processor context with minimum 2 entries
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy":            "Bin-Packing",
			"Merge Format":              "Binary Concatenation",
			"Minimum Number of Entries": "2",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger failed: %v", err)
	}

	// Verify no merge happened (routed to original)
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) != 0 {
		t.Errorf("Expected 0 merged FlowFiles, got %d", len(merged))
	}

	original := session.getTransferred(types.RelationshipOriginal)
	if len(original) != 1 {
		t.Errorf("Expected 1 original FlowFile, got %d", len(original))
	}
}

func TestMergeContentOnTriggerAttributeBased(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create FlowFiles with different correlation attributes
	ff1 := session.Create()
	session.Write(ff1, []byte("GroupA-1"))
	session.PutAttribute(ff1, "batch.id", "A")

	ff2 := session.Create()
	session.Write(ff2, []byte("GroupA-2"))
	session.PutAttribute(ff2, "batch.id", "A")

	ff3 := session.Create()
	session.Write(ff3, []byte("GroupB-1"))
	session.PutAttribute(ff3, "batch.id", "B")

	// Add to session queue
	session.addToQueue(ff1)
	session.addToQueue(ff2)
	session.addToQueue(ff3)

	// Create processor context
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy":            "Attribute-Based",
			"Merge Format":              "Binary Concatenation",
			"Correlation Attribute Name": "batch.id",
			"Minimum Number of Entries": "1",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger failed: %v", err)
	}

	// Verify two merged FlowFiles were created (one per group)
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) != 2 {
		t.Errorf("Expected 2 merged FlowFiles, got %d", len(merged))
	}

	// Check that one merged FlowFile contains both GroupA contents
	foundGroupA := false
	for _, mf := range merged {
		content, _ := session.Read(mf)
		if strings.Contains(string(content), "GroupA-1") && strings.Contains(string(content), "GroupA-2") {
			foundGroupA = true
		}
	}

	if !foundGroupA {
		t.Error("Expected to find merged FlowFile containing both GroupA contents")
	}
}

func TestMergeContentOnTriggerNoFlowFiles(t *testing.T) {
	processor := NewMergeContentProcessor()
	session := newMockProcessSessionWithQueue()

	// Create processor context
	ctx := &mockProcessorContext{
		properties: map[string]string{
			"Merge Strategy": "Bin-Packing",
			"Merge Format":   "Binary Concatenation",
		},
	}

	// Create context with processor context
	ctxWithProcessor := context.WithValue(context.Background(), "processorContext", ctx)

	// Execute processor with empty queue
	err := processor.OnTrigger(ctxWithProcessor, session)
	if err != nil {
		t.Fatalf("OnTrigger should not fail with empty queue: %v", err)
	}

	// Verify no FlowFiles were transferred
	merged := session.getTransferred(RelationshipMerged)
	if len(merged) != 0 {
		t.Errorf("Expected 0 merged FlowFiles, got %d", len(merged))
	}
}
