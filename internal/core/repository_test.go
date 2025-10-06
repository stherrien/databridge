package core

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

func TestBadgerFlowFileRepository(t *testing.T) {
	// Create temporary directory for test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_flowfile_db")

	// Create repository
	repo, err := NewBadgerFlowFileRepository(dbPath)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	// Test Store and Get
	flowFile := types.NewFlowFile()
	flowFile.Attributes["test-key"] = "test-value"
	flowFile.Size = 100

	err = repo.Store(flowFile)
	if err != nil {
		t.Errorf("Store failed: %v", err)
	}

	retrieved, err := repo.Get(flowFile.ID)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}

	if retrieved.ID != flowFile.ID {
		t.Error("Retrieved FlowFile should have same ID")
	}

	if retrieved.Attributes["test-key"] != "test-value" {
		t.Error("Retrieved FlowFile should have same attributes")
	}

	if retrieved.Size != flowFile.Size {
		t.Error("Retrieved FlowFile should have same size")
	}

	// Test Get non-existent FlowFile
	nonExistentID := uuid.New()
	_, err = repo.Get(nonExistentID)
	if err == nil {
		t.Error("Get should return error for non-existent FlowFile")
	}

	// Test Update (Store existing FlowFile)
	flowFile.Attributes["updated-key"] = "updated-value"
	flowFile.Size = 200

	err = repo.Store(flowFile)
	if err != nil {
		t.Errorf("Update failed: %v", err)
	}

	updated, err := repo.Get(flowFile.ID)
	if err != nil {
		t.Errorf("Get after update failed: %v", err)
	}

	if updated.Attributes["updated-key"] != "updated-value" {
		t.Error("Updated FlowFile should have new attributes")
	}

	if updated.Size != 200 {
		t.Error("Updated FlowFile should have new size")
	}

	// Test UpdateAttributes
	newAttributes := map[string]string{
		"attr1": "value1",
		"attr2": "value2",
	}

	err = repo.UpdateAttributes(flowFile.ID, newAttributes)
	if err != nil {
		t.Errorf("UpdateAttributes failed: %v", err)
	}

	updated, err = repo.Get(flowFile.ID)
	if err != nil {
		t.Errorf("Get after UpdateAttributes failed: %v", err)
	}

	for key, expectedValue := range newAttributes {
		if actualValue, exists := updated.Attributes[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected attribute %s=%s, got %s (exists: %v)",
				key, expectedValue, actualValue, exists)
		}
	}

	// Test List
	// Store a few more FlowFiles
	for i := 0; i < 5; i++ {
		testFlowFile := types.NewFlowFile()
		testFlowFile.Attributes["index"] = string(rune('0' + i))
		repo.Store(testFlowFile)
	}

	flowFiles, err := repo.List(3, 0)
	if err != nil {
		t.Errorf("List failed: %v", err)
	}

	if len(flowFiles) != 3 {
		t.Errorf("Expected 3 FlowFiles, got %d", len(flowFiles))
	}

	// Test List with offset
	flowFiles, err = repo.List(2, 2)
	if err != nil {
		t.Errorf("List with offset failed: %v", err)
	}

	if len(flowFiles) != 2 {
		t.Errorf("Expected 2 FlowFiles with offset, got %d", len(flowFiles))
	}

	// Test Delete
	err = repo.Delete(flowFile.ID)
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	_, err = repo.Get(flowFile.ID)
	if err == nil {
		t.Error("Get should fail for deleted FlowFile")
	}

	// Test Delete non-existent FlowFile (should not error)
	err = repo.Delete(uuid.New())
	if err != nil {
		t.Errorf("Delete of non-existent FlowFile should not error: %v", err)
	}
}

func TestFileSystemContentRepository(t *testing.T) {
	// Create temporary directory for test content repository
	tempDir := t.TempDir()

	// Create repository
	repo, err := NewFileSystemContentRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to create content repository: %v", err)
	}
	defer repo.Close()

	// Test Store and Get
	testContent := []byte("This is test content for the repository")
	claim, err := repo.Store(testContent)
	if err != nil {
		t.Errorf("Store failed: %v", err)
	}

	if claim == nil {
		t.Error("Store should return a content claim")
	}

	if claim.Length != int64(len(testContent)) {
		t.Errorf("Content claim length should be %d, got %d", len(testContent), claim.Length)
	}

	if claim.RefCount != 1 {
		t.Errorf("Content claim ref count should be 1, got %d", claim.RefCount)
	}

	// Test Get
	retrievedContent, err := repo.Get(claim)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}

	if !bytes.Equal(retrievedContent, testContent) {
		t.Error("Retrieved content should match original content")
	}

	// Test GetReader
	reader, err := repo.GetReader(claim)
	if err != nil {
		t.Errorf("GetReader failed: %v", err)
	}
	defer reader.Close()

	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Errorf("Reading from reader failed: %v", err)
	}

	if !bytes.Equal(readContent, testContent) {
		t.Error("Content from reader should match original content")
	}

	// Test GetWriter
	writer, err := repo.GetWriter()
	if err != nil {
		t.Errorf("GetWriter failed: %v", err)
	}

	newContent := []byte("New content written through writer")
	_, err = writer.Write(newContent)
	if err != nil {
		t.Errorf("Writer.Write failed: %v", err)
	}

	newClaim := writer.Claim()
	err = writer.Close()
	if err != nil {
		t.Errorf("Writer.Close failed: %v", err)
	}

	if newClaim.Length != int64(len(newContent)) {
		t.Errorf("New claim length should be %d, got %d", len(newContent), newClaim.Length)
	}

	// Verify written content
	writtenContent, err := repo.Get(newClaim)
	if err != nil {
		t.Errorf("Get after write failed: %v", err)
	}

	if !bytes.Equal(writtenContent, newContent) {
		t.Error("Written content should match what was written")
	}

	// Test partial content retrieval with offset and length
	largeContent := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	largeClaim, err := repo.Store(largeContent)
	if err != nil {
		t.Errorf("Store large content failed: %v", err)
	}

	// Create partial claim
	partialClaim := &types.ContentClaim{
		ID:        largeClaim.ID,
		Container: largeClaim.Container,
		Section:   largeClaim.Section,
		Offset:    10,
		Length:    5,
		RefCount:  1,
	}

	partialContent, err := repo.Get(partialClaim)
	if err != nil {
		t.Errorf("Get partial content failed: %v", err)
	}

	expectedPartial := largeContent[10:15]
	if !bytes.Equal(partialContent, expectedPartial) {
		t.Errorf("Partial content should be %s, got %s", string(expectedPartial), string(partialContent))
	}

	// Test reference counting
	err = repo.IncrementRef(claim)
	if err != nil {
		t.Errorf("IncrementRef failed: %v", err)
	}

	if claim.RefCount != 2 {
		t.Errorf("Ref count should be 2 after increment, got %d", claim.RefCount)
	}

	err = repo.DecrementRef(claim)
	if err != nil {
		t.Errorf("DecrementRef failed: %v", err)
	}

	if claim.RefCount != 1 {
		t.Errorf("Ref count should be 1 after decrement, got %d", claim.RefCount)
	}

	// Test Delete
	err = repo.Delete(claim)
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// Verify file is deleted
	_, err = repo.Get(claim)
	if err == nil {
		t.Error("Get should fail for deleted content")
	}

	// Test Delete non-existent content (should not panic)
	nonExistentClaim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "test",
		Section:   "section",
		Offset:    0,
		Length:    100,
	}

	err = repo.Delete(nonExistentClaim)
	// Delete may or may not return an error for non-existent files, both are acceptable
}

func TestInMemoryProvenanceRepository(t *testing.T) {
	repo := &InMemoryProvenanceRepository{
		events: make(map[uuid.UUID]*ProvenanceEvent),
	}
	defer repo.Close()

	// Create test events
	event1 := &ProvenanceEvent{
		ID:            uuid.New(),
		EventType:     "CREATE",
		FlowFileID:    uuid.New(),
		ProcessorID:   uuid.New(),
		ProcessorName: "TestProcessor1",
		EventTime:     time.Now(),
		Details:       "Created FlowFile",
	}

	event2 := &ProvenanceEvent{
		ID:            uuid.New(),
		EventType:     "ROUTE",
		FlowFileID:    event1.FlowFileID, // Same FlowFile
		ProcessorID:   uuid.New(),
		ProcessorName: "TestProcessor2",
		EventTime:     time.Now().Add(time.Second),
		Details:       "Routed FlowFile",
	}

	event3 := &ProvenanceEvent{
		ID:            uuid.New(),
		EventType:     "DROP",
		FlowFileID:    uuid.New(),         // Different FlowFile
		ProcessorID:   event2.ProcessorID, // Same processor as event2
		ProcessorName: "TestProcessor2",
		EventTime:     time.Now().Add(2 * time.Second),
		Details:       "Dropped FlowFile",
	}

	// Test Store
	err := repo.Store(event1)
	if err != nil {
		t.Errorf("Store event1 failed: %v", err)
	}

	err = repo.Store(event2)
	if err != nil {
		t.Errorf("Store event2 failed: %v", err)
	}

	err = repo.Store(event3)
	if err != nil {
		t.Errorf("Store event3 failed: %v", err)
	}

	// Test Query - all events
	allQuery := ProvenanceQuery{
		Limit: 10,
	}

	results, err := repo.Query(allQuery)
	if err != nil {
		t.Errorf("Query all failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 events, got %d", len(results))
	}

	// Test Query - by FlowFile ID
	flowFileQuery := ProvenanceQuery{
		FlowFileID: &event1.FlowFileID,
		Limit:      10,
	}

	results, err = repo.Query(flowFileQuery)
	if err != nil {
		t.Errorf("Query by FlowFile ID failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 events for FlowFile, got %d", len(results))
	}

	// Verify both events are for the same FlowFile
	for _, event := range results {
		if event.FlowFileID != event1.FlowFileID {
			t.Error("Query by FlowFile ID should return events for specified FlowFile only")
		}
	}

	// Test Query - by Event Type
	eventTypeQuery := ProvenanceQuery{
		EventType: "ROUTE",
		Limit:     10,
	}

	results, err = repo.Query(eventTypeQuery)
	if err != nil {
		t.Errorf("Query by event type failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 ROUTE event, got %d", len(results))
	}

	if results[0].EventType != "ROUTE" {
		t.Error("Query by event type should return correct event type")
	}

	// Test Query - by Processor ID
	processorQuery := ProvenanceQuery{
		ProcessorID: &event2.ProcessorID,
		Limit:       10,
	}

	results, err = repo.Query(processorQuery)
	if err != nil {
		t.Errorf("Query by processor ID failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 events for processor, got %d", len(results))
	}

	// Test Query - with time range
	startTime := event1.EventTime.Add(-time.Second)
	endTime := event2.EventTime.Add(time.Second)

	timeRangeQuery := ProvenanceQuery{
		StartTime: &startTime,
		EndTime:   &endTime,
		Limit:     10,
	}

	results, err = repo.Query(timeRangeQuery)
	if err != nil {
		t.Errorf("Query by time range failed: %v", err)
	}

	// Should include event1 and event2, but not event3
	if len(results) != 2 {
		t.Errorf("Expected 2 events in time range, got %d", len(results))
	}

	// Test GetLineage
	lineage, err := repo.GetLineage(event1.FlowFileID)
	if err != nil {
		t.Errorf("GetLineage failed: %v", err)
	}

	if lineage == nil {
		t.Error("GetLineage should return a lineage graph")
	}

	// Implementation should return non-nil slices even if empty
	if lineage.Nodes == nil {
		t.Error("Lineage graph should have initialized nodes slice")
	}
	if lineage.Links == nil {
		t.Error("Lineage graph should have initialized links slice")
	}
}

func TestContentRepositoryMultipleWrites(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := NewFileSystemContentRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to create content repository: %v", err)
	}
	defer repo.Close()

	// Test multiple writes to same writer
	writer, err := repo.GetWriter()
	if err != nil {
		t.Errorf("GetWriter failed: %v", err)
	}

	// Write in chunks
	chunks := [][]byte{
		[]byte("First chunk "),
		[]byte("Second chunk "),
		[]byte("Third chunk"),
	}

	totalLength := 0
	for _, chunk := range chunks {
		n, writeErr := writer.Write(chunk)
		if writeErr != nil {
			t.Errorf("Write chunk failed: %v", writeErr)
		}
		if n != len(chunk) {
			t.Errorf("Write should return chunk length %d, got %d", len(chunk), n)
		}
		totalLength += n
	}

	claim := writer.Claim()
	if claim.Length != int64(totalLength) {
		t.Errorf("Claim length should be %d, got %d", totalLength, claim.Length)
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("Writer close failed: %v", err)
	}

	// Verify combined content
	content, err := repo.Get(claim)
	if err != nil {
		t.Errorf("Get combined content failed: %v", err)
	}

	expectedContent := "First chunk Second chunk Third chunk"
	if string(content) != expectedContent {
		t.Errorf("Combined content should be '%s', got '%s'", expectedContent, string(content))
	}
}

// Benchmark tests
func BenchmarkBadgerFlowFileStore(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "benchmark_db")

	repo, err := NewBadgerFlowFileRepository(dbPath)
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	flowFiles := make([]*types.FlowFile, b.N)
	for i := 0; i < b.N; i++ {
		flowFiles[i] = types.NewFlowFile()
		flowFiles[i].Attributes["benchmark"] = "true"
		flowFiles[i].Size = int64(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.Store(flowFiles[i])
	}
}

func BenchmarkBadgerFlowFileGet(b *testing.B) {
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "benchmark_db")

	repo, err := NewBadgerFlowFileRepository(dbPath)
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	// Pre-populate with FlowFiles
	flowFiles := make([]*types.FlowFile, b.N)
	for i := 0; i < b.N; i++ {
		flowFiles[i] = types.NewFlowFile()
		repo.Store(flowFiles[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.Get(flowFiles[i%b.N].ID)
	}
}

func BenchmarkFileSystemContentStore(b *testing.B) {
	tempDir := b.TempDir()
	repo, err := NewFileSystemContentRepository(tempDir)
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	content := bytes.Repeat([]byte("benchmark content "), 100) // ~1.8KB content

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.Store(content)
	}
}

func BenchmarkFileSystemContentGet(b *testing.B) {
	tempDir := b.TempDir()
	repo, err := NewFileSystemContentRepository(tempDir)
	if err != nil {
		b.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	content := bytes.Repeat([]byte("benchmark content "), 100) // ~1.8KB content

	// Pre-populate with content
	claims := make([]*types.ContentClaim, b.N)
	for i := 0; i < b.N; i++ {
		claims[i], _ = repo.Store(content)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.Get(claims[i%b.N])
	}
}

// Test helper functions
func TestRepositoryDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()

	// Test BadgerDB repository directory creation
	dbPath := filepath.Join(tempDir, "nested", "path", "badger_db")
	repo, err := NewBadgerFlowFileRepository(dbPath)
	if err != nil {
		t.Errorf("Repository should create nested directories: %v", err)
	} else {
		repo.Close()
	}

	// Verify directory was created
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		t.Error("BadgerDB should create directory structure")
	}

	// Test FileSystem repository directory creation
	contentPath := filepath.Join(tempDir, "nested", "content", "repo")
	contentRepo, err := NewFileSystemContentRepository(contentPath)
	if err != nil {
		t.Errorf("Content repository should create nested directories: %v", err)
	} else {
		contentRepo.Close()
	}

	// Verify directory was created
	if _, err := os.Stat(contentPath); os.IsNotExist(err) {
		t.Error("Content repository should create directory structure")
	}
}

func TestContentRepositoryErrorConditions(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := NewFileSystemContentRepository(tempDir)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	defer repo.Close()

	// Test Get with invalid claim
	invalidClaim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "nonexistent",
		Section:   "nonexistent",
		Offset:    0,
		Length:    100,
	}

	_, err = repo.Get(invalidClaim)
	if err == nil {
		t.Error("Get should return error for invalid content claim")
	}

	// Test Get with out-of-bounds offset
	content := []byte("short content")
	validClaim, err := repo.Store(content)
	if err != nil {
		t.Errorf("Store failed: %v", err)
	}

	outOfBoundsClaim := &types.ContentClaim{
		ID:        validClaim.ID,
		Container: validClaim.Container,
		Section:   validClaim.Section,
		Offset:    1000, // Way beyond content length
		Length:    100,
	}

	_, err = repo.Get(outOfBoundsClaim)
	if err == nil {
		t.Error("Get should return error for out-of-bounds offset")
	}

	// Test GetReader with invalid claim
	_, err = repo.GetReader(invalidClaim)
	if err == nil {
		t.Error("GetReader should return error for invalid content claim")
	}
}
