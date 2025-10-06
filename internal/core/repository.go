package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/pkg/types"
)

// Repository interfaces define the contract for data persistence
type FlowFileRepository interface {
	Store(flowFile *types.FlowFile) error
	Get(id uuid.UUID) (*types.FlowFile, error)
	Delete(id uuid.UUID) error
	List(limit, offset int) ([]*types.FlowFile, error)
	Count() (int, error)
	UpdateAttributes(id uuid.UUID, attributes map[string]string) error
	Close() error
}

type ContentRepository interface {
	Store(content []byte) (*types.ContentClaim, error)
	Get(claim *types.ContentClaim) ([]byte, error)
	GetReader(claim *types.ContentClaim) (io.ReadCloser, error)
	GetWriter() (ContentWriter, error)
	Delete(claim *types.ContentClaim) error
	IncrementRef(claim *types.ContentClaim) error
	DecrementRef(claim *types.ContentClaim) error
	ListClaims() ([]*types.ContentClaim, error)
	Read(claim *types.ContentClaim) (io.ReadCloser, error)
	Write(claim *types.ContentClaim, data []byte) error
	Close() error
}

type ProvenanceRepository interface {
	Store(event *ProvenanceEvent) error
	Query(query ProvenanceQuery) ([]*ProvenanceEvent, error)
	GetLineage(flowFileId uuid.UUID) (*LineageGraph, error)
	GetEvents(offset, limit int) ([]*ProvenanceEvent, error)
	AddEvent(event *ProvenanceEvent) error
	Close() error
}

// ContentWriter provides streaming write capabilities
type ContentWriter interface {
	io.Writer
	Claim() *types.ContentClaim
	Close() error
}

// ProvenanceEvent represents a data lineage event
type ProvenanceEvent struct {
	ID            uuid.UUID           `json:"id"`
	EventType     string              `json:"eventType"`
	FlowFileID    uuid.UUID           `json:"flowFileId"`
	ProcessorID   uuid.UUID           `json:"processorId"`
	ProcessorName string              `json:"processorName"`
	EventTime     time.Time           `json:"eventTime"`
	Duration      time.Duration       `json:"duration"`
	Attributes    map[string]string   `json:"attributes"`
	ParentIDs     []uuid.UUID         `json:"parentIds,omitempty"`
	ChildIDs      []uuid.UUID         `json:"childIds,omitempty"`
	Details       string              `json:"details"`
	ContentClaim  *types.ContentClaim `json:"contentClaim,omitempty"`
}

// ProvenanceQuery defines search criteria for provenance events
type ProvenanceQuery struct {
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	FlowFileID  *uuid.UUID `json:"flowFileId,omitempty"`
	ProcessorID *uuid.UUID `json:"processorId,omitempty"`
	EventType   string     `json:"eventType,omitempty"`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
}

// LineageGraph represents data flow lineage
type LineageGraph struct {
	Nodes []LineageNode `json:"nodes"`
	Links []LineageLink `json:"links"`
}

type LineageNode struct {
	ID          uuid.UUID `json:"id"`
	FlowFileID  uuid.UUID `json:"flowFileId"`
	ProcessorID uuid.UUID `json:"processorId"`
	EventType   string    `json:"eventType"`
	Timestamp   time.Time `json:"timestamp"`
}

type LineageLink struct {
	Source uuid.UUID `json:"source"`
	Target uuid.UUID `json:"target"`
}

// BadgerFlowFileRepository implements FlowFileRepository using BadgerDB
type BadgerFlowFileRepository struct {
	db *badger.DB
	mu sync.RWMutex
}

// NewBadgerFlowFileRepository creates a new BadgerDB-based FlowFile repository
func NewBadgerFlowFileRepository(dbPath string) (*BadgerFlowFileRepository, error) {
	opts := badger.DefaultOptions(dbPath).
		WithLogger(nil).      // Disable badger logging
		WithSyncWrites(false) // Async writes for performance

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	return &BadgerFlowFileRepository{
		db: db,
	}, nil
}

// Store persists a FlowFile to the repository
func (r *BadgerFlowFileRepository) Store(flowFile *types.FlowFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := json.Marshal(flowFile)
	if err != nil {
		return fmt.Errorf("failed to marshal FlowFile: %w", err)
	}

	return r.db.Update(func(txn *badger.Txn) error {
		key := fmt.Sprintf("flowfile:%s", flowFile.ID.String())
		return txn.Set([]byte(key), data)
	})
}

// Get retrieves a FlowFile from the repository
func (r *BadgerFlowFileRepository) Get(id uuid.UUID) (*types.FlowFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var flowFile *types.FlowFile

	err := r.db.View(func(txn *badger.Txn) error {
		key := fmt.Sprintf("flowfile:%s", id.String())
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			flowFile = &types.FlowFile{}
			return json.Unmarshal(val, flowFile)
		})
	})

	if err == badger.ErrKeyNotFound {
		return nil, fmt.Errorf("FlowFile not found: %s", id.String())
	}

	return flowFile, err
}

// Delete removes a FlowFile from the repository
func (r *BadgerFlowFileRepository) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.db.Update(func(txn *badger.Txn) error {
		key := fmt.Sprintf("flowfile:%s", id.String())
		return txn.Delete([]byte(key))
	})
}

// List retrieves FlowFiles with pagination
func (r *BadgerFlowFileRepository) List(limit, offset int) ([]*types.FlowFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var flowFiles []*types.FlowFile
	count := 0

	err := r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = limit
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("flowfile:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if count < offset {
				count++
				continue
			}
			if len(flowFiles) >= limit {
				break
			}

			item := it.Item()
			err := item.Value(func(val []byte) error {
				var flowFile types.FlowFile
				if err := json.Unmarshal(val, &flowFile); err != nil {
					return err
				}
				flowFiles = append(flowFiles, &flowFile)
				return nil
			})
			if err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return flowFiles, err
}

// Count returns the total number of FlowFiles in the repository
func (r *BadgerFlowFileRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	err := r.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Only need keys for counting
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("flowfile:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})

	return count, err
}

// UpdateAttributes updates FlowFile attributes
func (r *BadgerFlowFileRepository) UpdateAttributes(id uuid.UUID, attributes map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.db.Update(func(txn *badger.Txn) error {
		key := fmt.Sprintf("flowfile:%s", id.String())

		// Get existing FlowFile
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		var flowFile types.FlowFile
		err = item.Value(func(val []byte) error {
			return json.Unmarshal(val, &flowFile)
		})
		if err != nil {
			return err
		}

		// Update attributes
		for k, v := range attributes {
			flowFile.Attributes[k] = v
		}
		flowFile.UpdatedAt = time.Now()

		// Store updated FlowFile
		data, err := json.Marshal(&flowFile)
		if err != nil {
			return err
		}

		return txn.Set([]byte(key), data)
	})
}

// Close closes the repository
func (r *BadgerFlowFileRepository) Close() error {
	return r.db.Close()
}

// FileSystemContentRepository implements ContentRepository using filesystem storage
type FileSystemContentRepository struct {
	basePath string
	mu       sync.RWMutex
}

// NewFileSystemContentRepository creates a new filesystem-based content repository
func NewFileSystemContentRepository(basePath string) (*FileSystemContentRepository, error) {
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create content repository directory: %w", err)
	}

	return &FileSystemContentRepository{
		basePath: basePath,
	}, nil
}

// Store stores content and returns a content claim
func (r *FileSystemContentRepository) Store(content []byte) (*types.ContentClaim, error) {
	claim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "default",
		Section:   time.Now().Format("2006/01/02"),
		Offset:    0,
		Length:    int64(len(content)),
		RefCount:  1,
	}

	sectionPath := filepath.Join(r.basePath, claim.Container, claim.Section)
	if err := os.MkdirAll(sectionPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create section directory: %w", err)
	}

	filePath := filepath.Join(sectionPath, claim.ID.String())
	if err := os.WriteFile(filePath, content, 0600); err != nil {
		return nil, fmt.Errorf("failed to write content file: %w", err)
	}

	return claim, nil
}

// Get retrieves content by claim
func (r *FileSystemContentRepository) Get(claim *types.ContentClaim) ([]byte, error) {
	filePath := filepath.Join(r.basePath, claim.Container, claim.Section, claim.ID.String())

	// #nosec G304 - filePath is constructed from repository basePath and validated claim
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read content file: %w", err)
	}

	// Return the requested slice
	if claim.Offset > 0 || claim.Length < int64(len(content)) {
		start := claim.Offset
		end := start + claim.Length
		if start > int64(len(content)) {
			return nil, fmt.Errorf("offset exceeds content length")
		}
		if end > int64(len(content)) {
			end = int64(len(content))
		}
		content = content[start:end]
	}

	return content, nil
}

// GetReader returns a reader for the content claim
func (r *FileSystemContentRepository) GetReader(claim *types.ContentClaim) (io.ReadCloser, error) {
	filePath := filepath.Join(r.basePath, claim.Container, claim.Section, claim.ID.String())

	// #nosec G304 - filePath is constructed from repository basePath and validated claim
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open content file: %w", err)
	}

	if claim.Offset > 0 {
		if _, err := file.Seek(claim.Offset, io.SeekStart); err != nil {
			_ = file.Close() // Best effort cleanup on error
			return nil, fmt.Errorf("failed to seek to offset: %w", err)
		}
	}

	return file, nil
}

// GetWriter returns a writer for new content
func (r *FileSystemContentRepository) GetWriter() (ContentWriter, error) {
	claim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "default",
		Section:   time.Now().Format("2006/01/02"),
		Offset:    0,
		RefCount:  1,
	}

	sectionPath := filepath.Join(r.basePath, claim.Container, claim.Section)
	if err := os.MkdirAll(sectionPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create section directory: %w", err)
	}

	filePath := filepath.Join(sectionPath, claim.ID.String())
	// #nosec G304 - filePath is constructed from repository basePath and generated UUID
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create content file: %w", err)
	}

	return &fileContentWriter{
		file:  file,
		claim: claim,
		repo:  r,
	}, nil
}

// Delete removes content by claim
func (r *FileSystemContentRepository) Delete(claim *types.ContentClaim) error {
	filePath := filepath.Join(r.basePath, claim.Container, claim.Section, claim.ID.String())
	return os.Remove(filePath)
}

// IncrementRef increments reference count (no-op for filesystem)
func (r *FileSystemContentRepository) IncrementRef(claim *types.ContentClaim) error {
	claim.RefCount++
	return nil
}

// DecrementRef decrements reference count (no-op for filesystem)
func (r *FileSystemContentRepository) DecrementRef(claim *types.ContentClaim) error {
	claim.RefCount--
	return nil
}

// ListClaims returns all content claims in the repository
func (r *FileSystemContentRepository) ListClaims() ([]*types.ContentClaim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var claims []*types.ContentClaim

	// Walk through the content directory structure
	err := filepath.Walk(r.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Parse the file path to extract claim information
		relPath, err := filepath.Rel(r.basePath, path)
		if err != nil {
			return err
		}

		// Split path into components: container/section/id
		parts := filepath.SplitList(relPath)
		if len(parts) == 0 {
			parts = []string{filepath.Dir(relPath), filepath.Base(relPath)}
		}

		// Extract container and section from path
		container := "default"
		section := ""
		filename := filepath.Base(path)

		dirPath := filepath.Dir(relPath)
		if dirPath != "." {
			pathParts := []string{}
			for dirPath != "." && dirPath != "" {
				base := filepath.Base(dirPath)
				pathParts = append([]string{base}, pathParts...)
				dirPath = filepath.Dir(dirPath)
			}
			if len(pathParts) > 0 {
				container = pathParts[0]
				if len(pathParts) > 1 {
					section = filepath.Join(pathParts[1:]...)
				}
			}
		}

		// Parse UUID from filename
		id, err := uuid.Parse(filename)
		if err != nil {
			// Skip files that aren't valid UUIDs
			return nil
		}

		claim := &types.ContentClaim{
			ID:        id,
			Container: container,
			Section:   section,
			Offset:    0,
			Length:    info.Size(),
			RefCount:  1,
		}

		claims = append(claims, claim)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list content claims: %w", err)
	}

	return claims, nil
}

// Read returns a reader for the content claim
func (r *FileSystemContentRepository) Read(claim *types.ContentClaim) (io.ReadCloser, error) {
	return r.GetReader(claim)
}

// Write writes data to a content claim
func (r *FileSystemContentRepository) Write(claim *types.ContentClaim, data []byte) error {
	sectionPath := filepath.Join(r.basePath, claim.Container, claim.Section)
	if err := os.MkdirAll(sectionPath, 0750); err != nil {
		return fmt.Errorf("failed to create section directory: %w", err)
	}

	filePath := filepath.Join(sectionPath, claim.ID.String())
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write content file: %w", err)
	}

	claim.Length = int64(len(data))
	return nil
}

// Close closes the repository
func (r *FileSystemContentRepository) Close() error {
	return nil
}

// fileContentWriter implements ContentWriter
type fileContentWriter struct {
	file   *os.File
	claim  *types.ContentClaim
	repo   *FileSystemContentRepository
	length int64
}

// Write writes data to the content file
func (w *fileContentWriter) Write(p []byte) (n int, err error) {
	n, err = w.file.Write(p)
	w.length += int64(n)
	return n, err
}

// Claim returns the content claim
func (w *fileContentWriter) Claim() *types.ContentClaim {
	w.claim.Length = w.length
	return w.claim
}

// Close closes the writer and updates the claim
func (w *fileContentWriter) Close() error {
	w.claim.Length = w.length
	return w.file.Close()
}

// InMemoryFlowFileRepository implements FlowFileRepository in memory (for testing)
type InMemoryFlowFileRepository struct {
	mu        sync.RWMutex
	flowFiles map[uuid.UUID]*types.FlowFile
}

// NewInMemoryFlowFileRepository creates a new in-memory FlowFile repository
func NewInMemoryFlowFileRepository() *InMemoryFlowFileRepository {
	return &InMemoryFlowFileRepository{
		flowFiles: make(map[uuid.UUID]*types.FlowFile),
	}
}

func (r *InMemoryFlowFileRepository) Store(flowFile *types.FlowFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flowFiles[flowFile.ID] = flowFile
	return nil
}

func (r *InMemoryFlowFileRepository) Get(id uuid.UUID) (*types.FlowFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	flowFile, exists := r.flowFiles[id]
	if !exists {
		return nil, fmt.Errorf("FlowFile not found: %s", id.String())
	}
	return flowFile, nil
}

func (r *InMemoryFlowFileRepository) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.flowFiles, id)
	return nil
}

func (r *InMemoryFlowFileRepository) List(limit, offset int) ([]*types.FlowFile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var flowFiles []*types.FlowFile
	count := 0
	for _, ff := range r.flowFiles {
		if count < offset {
			count++
			continue
		}
		if len(flowFiles) >= limit {
			break
		}
		flowFiles = append(flowFiles, ff)
		count++
	}
	return flowFiles, nil
}

func (r *InMemoryFlowFileRepository) UpdateAttributes(id uuid.UUID, attributes map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	flowFile, exists := r.flowFiles[id]
	if !exists {
		return fmt.Errorf("FlowFile not found: %s", id.String())
	}
	for k, v := range attributes {
		flowFile.Attributes[k] = v
	}
	flowFile.UpdatedAt = time.Now()
	return nil
}

func (r *InMemoryFlowFileRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.flowFiles), nil
}

func (r *InMemoryFlowFileRepository) Close() error {
	return nil
}

// InMemoryContentRepository implements ContentRepository in memory (for testing)
type InMemoryContentRepository struct {
	mu      sync.RWMutex
	content map[uuid.UUID][]byte
}

// NewInMemoryContentRepository creates a new in-memory content repository
func NewInMemoryContentRepository() *InMemoryContentRepository {
	return &InMemoryContentRepository{
		content: make(map[uuid.UUID][]byte),
	}
}

func (r *InMemoryContentRepository) Store(content []byte) (*types.ContentClaim, error) {
	claim := &types.ContentClaim{
		ID:        uuid.New(),
		Container: "memory",
		Section:   "default",
		Offset:    0,
		Length:    int64(len(content)),
		RefCount:  1,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.content[claim.ID] = content
	return claim, nil
}

func (r *InMemoryContentRepository) Get(claim *types.ContentClaim) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	content, exists := r.content[claim.ID]
	if !exists {
		return nil, fmt.Errorf("content not found")
	}
	return content, nil
}

func (r *InMemoryContentRepository) GetReader(claim *types.ContentClaim) (io.ReadCloser, error) {
	content, err := r.Get(claim)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(io.NewSectionReader(&bytesReader{content}, 0, int64(len(content)))), nil
}

func (r *InMemoryContentRepository) GetWriter() (ContentWriter, error) {
	return &memoryContentWriter{
		repo: r,
		claim: &types.ContentClaim{
			ID:        uuid.New(),
			Container: "memory",
			Section:   "default",
			Offset:    0,
			RefCount:  1,
		},
	}, nil
}

func (r *InMemoryContentRepository) Delete(claim *types.ContentClaim) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.content, claim.ID)
	return nil
}

func (r *InMemoryContentRepository) IncrementRef(claim *types.ContentClaim) error {
	claim.RefCount++
	return nil
}

func (r *InMemoryContentRepository) DecrementRef(claim *types.ContentClaim) error {
	claim.RefCount--
	return nil
}

func (r *InMemoryContentRepository) ListClaims() ([]*types.ContentClaim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var claims []*types.ContentClaim
	for id, content := range r.content {
		claim := &types.ContentClaim{
			ID:        id,
			Container: "memory",
			Section:   "default",
			Offset:    0,
			Length:    int64(len(content)),
			RefCount:  1,
		}
		claims = append(claims, claim)
	}
	return claims, nil
}

func (r *InMemoryContentRepository) Read(claim *types.ContentClaim) (io.ReadCloser, error) {
	return r.GetReader(claim)
}

func (r *InMemoryContentRepository) Write(claim *types.ContentClaim, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.content[claim.ID] = data
	claim.Length = int64(len(data))
	return nil
}

func (r *InMemoryContentRepository) Close() error {
	return nil
}

type memoryContentWriter struct {
	repo   *InMemoryContentRepository
	claim  *types.ContentClaim
	buffer []byte
}

func (w *memoryContentWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *memoryContentWriter) Claim() *types.ContentClaim {
	w.claim.Length = int64(len(w.buffer))
	return w.claim
}

func (w *memoryContentWriter) Close() error {
	w.repo.mu.Lock()
	defer w.repo.mu.Unlock()
	w.repo.content[w.claim.ID] = w.buffer
	w.claim.Length = int64(len(w.buffer))
	return nil
}

type bytesReader struct {
	data []byte
}

func (r *bytesReader) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}
