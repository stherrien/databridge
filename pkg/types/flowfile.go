package types

import (
	"time"

	"github.com/google/uuid"
)

// FlowFile represents a data unit in the flow with metadata and content reference
type FlowFile struct {
	ID           uuid.UUID         `json:"id"`
	ContentClaim *ContentClaim     `json:"contentClaim,omitempty"`
	Attributes   map[string]string `json:"attributes"`
	Size         int64             `json:"size"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	Lineage      LineageInfo       `json:"lineage"`
}

// ContentClaim represents a reference to stored content
type ContentClaim struct {
	ID        uuid.UUID `json:"id"`
	Container string    `json:"container"`
	Section   string    `json:"section"`
	Offset    int64     `json:"offset"`
	Length    int64     `json:"length"`
	RefCount  int32     `json:"refCount"`
}

// LineageInfo tracks data lineage for provenance
type LineageInfo struct {
	LineageStartDate time.Time   `json:"lineageStartDate"`
	ParentUUIDs      []uuid.UUID `json:"parentUuids,omitempty"`
	SourceQueue      string      `json:"sourceQueue,omitempty"`
}

// FlowFileBuilder provides a builder pattern for creating FlowFiles
type FlowFileBuilder struct {
	flowFile *FlowFile
}

// NewFlowFile creates a new FlowFile with default values
func NewFlowFile() *FlowFile {
	now := time.Now()
	return &FlowFile{
		ID:         uuid.New(),
		Attributes: make(map[string]string),
		CreatedAt:  now,
		UpdatedAt:  now,
		Lineage: LineageInfo{
			LineageStartDate: now,
		},
	}
}

// NewFlowFileBuilder creates a new FlowFileBuilder
func NewFlowFileBuilder() *FlowFileBuilder {
	return &FlowFileBuilder{
		flowFile: NewFlowFile(),
	}
}

// WithAttribute adds an attribute to the FlowFile
func (b *FlowFileBuilder) WithAttribute(key, value string) *FlowFileBuilder {
	b.flowFile.Attributes[key] = value
	return b
}

// WithAttributes sets multiple attributes
func (b *FlowFileBuilder) WithAttributes(attrs map[string]string) *FlowFileBuilder {
	for k, v := range attrs {
		b.flowFile.Attributes[k] = v
	}
	return b
}

// WithContentClaim sets the content claim
func (b *FlowFileBuilder) WithContentClaim(claim *ContentClaim) *FlowFileBuilder {
	b.flowFile.ContentClaim = claim
	b.flowFile.Size = claim.Length
	return b
}

// WithParent sets parent lineage information
func (b *FlowFileBuilder) WithParent(parent *FlowFile) *FlowFileBuilder {
	b.flowFile.Lineage.ParentUUIDs = []uuid.UUID{parent.ID}
	b.flowFile.Lineage.LineageStartDate = parent.Lineage.LineageStartDate
	return b
}

// Build creates the final FlowFile
func (b *FlowFileBuilder) Build() *FlowFile {
	return b.flowFile
}

// Clone creates a deep copy of the FlowFile
func (f *FlowFile) Clone() *FlowFile {
	clone := &FlowFile{
		ID:        uuid.New(), // New ID for clone
		Size:      f.Size,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Lineage: LineageInfo{
			LineageStartDate: f.Lineage.LineageStartDate,
			ParentUUIDs:      []uuid.UUID{f.ID}, // Original becomes parent
			SourceQueue:      f.Lineage.SourceQueue,
		},
	}

	// Deep copy attributes
	clone.Attributes = make(map[string]string)
	for k, v := range f.Attributes {
		clone.Attributes[k] = v
	}

	// Copy content claim reference (increases ref count)
	if f.ContentClaim != nil {
		clone.ContentClaim = &ContentClaim{
			ID:        f.ContentClaim.ID,
			Container: f.ContentClaim.Container,
			Section:   f.ContentClaim.Section,
			Offset:    f.ContentClaim.Offset,
			Length:    f.ContentClaim.Length,
			RefCount:  f.ContentClaim.RefCount + 1, // Increment reference count
		}
	}

	return clone
}

// UpdateAttribute updates or adds an attribute
func (f *FlowFile) UpdateAttribute(key, value string) {
	f.Attributes[key] = value
	f.UpdatedAt = time.Now()
}

// RemoveAttribute removes an attribute
func (f *FlowFile) RemoveAttribute(key string) {
	delete(f.Attributes, key)
	f.UpdatedAt = time.Now()
}

// GetAttribute safely gets an attribute value
func (f *FlowFile) GetAttribute(key string) (string, bool) {
	value, exists := f.Attributes[key]
	return value, exists
}
