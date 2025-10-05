package types

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewFlowFile(t *testing.T) {
	flowFile := NewFlowFile()

	// Test basic properties
	if flowFile.ID == (uuid.UUID{}) {
		t.Error("FlowFile should have a valid UUID")
	}

	if flowFile.Attributes == nil {
		t.Error("FlowFile should have initialized attributes map")
	}

	if flowFile.Size != 0 {
		t.Errorf("FlowFile size should be 0, got %d", flowFile.Size)
	}

	if flowFile.CreatedAt.IsZero() {
		t.Error("FlowFile should have CreatedAt timestamp set")
	}

	if flowFile.UpdatedAt.IsZero() {
		t.Error("FlowFile should have UpdatedAt timestamp set")
	}

	if flowFile.Lineage.LineageStartDate.IsZero() {
		t.Error("FlowFile should have LineageStartDate set")
	}
}

func TestFlowFileBuilder(t *testing.T) {
	// Test builder pattern
	attributes := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	contentClaim := &ContentClaim{
		ID:        uuid.New(),
		Container: "test",
		Section:   "section1",
		Offset:    0,
		Length:    100,
		RefCount:  1,
	}

	parent := NewFlowFile()
	parent.Attributes["parentKey"] = "parentValue"

	flowFile := NewFlowFileBuilder().
		WithAttributes(attributes).
		WithAttribute("key3", "value3").
		WithContentClaim(contentClaim).
		WithParent(parent).
		Build()

	// Test attributes
	if len(flowFile.Attributes) != 3 {
		t.Errorf("Expected 3 attributes, got %d", len(flowFile.Attributes))
	}

	for key, expectedValue := range attributes {
		if actualValue, exists := flowFile.Attributes[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected attribute %s=%s, got %s (exists: %v)", key, expectedValue, actualValue, exists)
		}
	}

	if flowFile.Attributes["key3"] != "value3" {
		t.Errorf("Expected key3=value3, got %s", flowFile.Attributes["key3"])
	}

	// Test content claim
	if flowFile.ContentClaim == nil {
		t.Error("FlowFile should have content claim")
	}

	if flowFile.ContentClaim.ID != contentClaim.ID {
		t.Error("FlowFile should have correct content claim ID")
	}

	if flowFile.Size != contentClaim.Length {
		t.Errorf("FlowFile size should be %d, got %d", contentClaim.Length, flowFile.Size)
	}

	// Test parent lineage
	if len(flowFile.Lineage.ParentUUIDs) != 1 {
		t.Errorf("Expected 1 parent UUID, got %d", len(flowFile.Lineage.ParentUUIDs))
	}

	if flowFile.Lineage.ParentUUIDs[0] != parent.ID {
		t.Error("FlowFile should have correct parent UUID")
	}

	if !flowFile.Lineage.LineageStartDate.Equal(parent.Lineage.LineageStartDate) {
		t.Error("FlowFile should inherit parent's lineage start date")
	}
}

func TestFlowFileClone(t *testing.T) {
	// Create original FlowFile
	original := NewFlowFile()
	original.Attributes["key1"] = "value1"
	original.Attributes["key2"] = "value2"
	original.ContentClaim = &ContentClaim{
		ID:        uuid.New(),
		Container: "test",
		Section:   "section1",
		Offset:    0,
		Length:    50,
		RefCount:  1,
	}
	original.Size = 50

	// Clone it
	clone := original.Clone()

	// Test that IDs are different
	if clone.ID == original.ID {
		t.Error("Clone should have different ID than original")
	}

	// Test that attributes are copied
	if len(clone.Attributes) != len(original.Attributes) {
		t.Errorf("Clone should have same number of attributes as original: %d vs %d",
			len(clone.Attributes), len(original.Attributes))
	}

	for key, value := range original.Attributes {
		if cloneValue, exists := clone.Attributes[key]; !exists || cloneValue != value {
			t.Errorf("Clone should have attribute %s=%s, got %s (exists: %v)",
				key, value, cloneValue, exists)
		}
	}

	// Test that modifying clone attributes doesn't affect original
	clone.Attributes["newKey"] = "newValue"
	if _, exists := original.Attributes["newKey"]; exists {
		t.Error("Modifying clone attributes should not affect original")
	}

	// Test content claim reference
	if clone.ContentClaim == nil {
		t.Error("Clone should have content claim")
	}

	if clone.ContentClaim.ID != original.ContentClaim.ID {
		t.Error("Clone should reference same content claim")
	}

	if clone.ContentClaim.RefCount != original.ContentClaim.RefCount+1 {
		t.Errorf("Clone content claim should have incremented ref count: %d vs %d",
			clone.ContentClaim.RefCount, original.ContentClaim.RefCount+1)
	}

	// Test lineage
	if len(clone.Lineage.ParentUUIDs) != 1 {
		t.Errorf("Clone should have 1 parent UUID, got %d", len(clone.Lineage.ParentUUIDs))
	}

	if clone.Lineage.ParentUUIDs[0] != original.ID {
		t.Error("Clone should have original as parent")
	}

	if !clone.Lineage.LineageStartDate.Equal(original.Lineage.LineageStartDate) {
		t.Error("Clone should inherit original's lineage start date")
	}
}

func TestFlowFileAttributeOperations(t *testing.T) {
	flowFile := NewFlowFile()
	originalUpdateTime := flowFile.UpdatedAt

	// Test UpdateAttribute
	time.Sleep(1 * time.Millisecond) // Ensure time difference
	flowFile.UpdateAttribute("testKey", "testValue")

	if flowFile.Attributes["testKey"] != "testValue" {
		t.Errorf("Expected testKey=testValue, got %s", flowFile.Attributes["testKey"])
	}

	if !flowFile.UpdatedAt.After(originalUpdateTime) {
		t.Error("UpdatedAt should be updated when attribute is modified")
	}

	// Test GetAttribute
	value, exists := flowFile.GetAttribute("testKey")
	if !exists {
		t.Error("GetAttribute should return true for existing attribute")
	}

	if value != "testValue" {
		t.Errorf("GetAttribute should return testValue, got %s", value)
	}

	// Test GetAttribute for non-existent key
	_, exists = flowFile.GetAttribute("nonExistentKey")
	if exists {
		t.Error("GetAttribute should return false for non-existent attribute")
	}

	// Test RemoveAttribute
	originalUpdateTime = flowFile.UpdatedAt
	time.Sleep(1 * time.Millisecond)
	flowFile.RemoveAttribute("testKey")

	if _, exists := flowFile.Attributes["testKey"]; exists {
		t.Error("Attribute should be removed after RemoveAttribute")
	}

	if !flowFile.UpdatedAt.After(originalUpdateTime) {
		t.Error("UpdatedAt should be updated when attribute is removed")
	}

	// Test removing non-existent attribute
	flowFile.RemoveAttribute("nonExistentKey") // Should not panic
}

func TestContentClaim(t *testing.T) {
	claim := &ContentClaim{
		ID:        uuid.New(),
		Container: "testContainer",
		Section:   "testSection",
		Offset:    100,
		Length:    200,
		RefCount:  1,
	}

	// Test basic properties
	if claim.ID == (uuid.UUID{}) {
		t.Error("ContentClaim should have valid UUID")
	}

	if claim.Container != "testContainer" {
		t.Errorf("Expected container=testContainer, got %s", claim.Container)
	}

	if claim.Section != "testSection" {
		t.Errorf("Expected section=testSection, got %s", claim.Section)
	}

	if claim.Offset != 100 {
		t.Errorf("Expected offset=100, got %d", claim.Offset)
	}

	if claim.Length != 200 {
		t.Errorf("Expected length=200, got %d", claim.Length)
	}

	if claim.RefCount != 1 {
		t.Errorf("Expected refCount=1, got %d", claim.RefCount)
	}
}

func TestLineageInfo(t *testing.T) {
	now := time.Now()
	parentUUID := uuid.New()

	lineage := LineageInfo{
		LineageStartDate: now,
		ParentUUIDs:      []uuid.UUID{parentUUID},
		SourceQueue:      "testQueue",
	}

	if !lineage.LineageStartDate.Equal(now) {
		t.Error("LineageStartDate should match set value")
	}

	if len(lineage.ParentUUIDs) != 1 {
		t.Errorf("Expected 1 parent UUID, got %d", len(lineage.ParentUUIDs))
	}

	if lineage.ParentUUIDs[0] != parentUUID {
		t.Error("Parent UUID should match set value")
	}

	if lineage.SourceQueue != "testQueue" {
		t.Errorf("Expected sourceQueue=testQueue, got %s", lineage.SourceQueue)
	}
}

// Benchmark tests
func BenchmarkNewFlowFile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewFlowFile()
	}
}

func BenchmarkFlowFileBuilder(b *testing.B) {
	attributes := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	for i := 0; i < b.N; i++ {
		NewFlowFileBuilder().
			WithAttributes(attributes).
			WithAttribute("key4", "value4").
			Build()
	}
}

func BenchmarkFlowFileClone(b *testing.B) {
	flowFile := NewFlowFile()
	flowFile.Attributes["key1"] = "value1"
	flowFile.Attributes["key2"] = "value2"
	flowFile.ContentClaim = &ContentClaim{
		ID:        uuid.New(),
		Container: "test",
		Section:   "section1",
		Offset:    0,
		Length:    100,
		RefCount:  1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flowFile.Clone()
	}
}

func BenchmarkAttributeOperations(b *testing.B) {
	flowFile := NewFlowFile()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i%10))
		value := "value" + string(rune(i%10))
		flowFile.UpdateAttribute(key, value)
		_, _ = flowFile.GetAttribute(key)
	}
}
