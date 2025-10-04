package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// ArchivalPolicy defines archival rules for data lifecycle management
type ArchivalPolicy struct {
	// Age-based archival
	MaxAge time.Duration // Archive data older than this

	// Size-based archival
	MaxRepositorySize int64 // Archive when repository exceeds size

	// Count-based archival
	MaxFlowFiles int // Archive when FlowFile count exceeds

	// Content-based archival
	ArchivePatterns []string // Archive FlowFiles matching patterns

	// Destination
	ArchiveLocation string // Path to archive storage

	// Retention
	DeleteAfterArchive bool          // Delete from active storage after archive
	ArchiveRetention   time.Duration // How long to keep archived data
}

// ArchivalManager handles data lifecycle and archival operations
type ArchivalManager struct {
	policy         ArchivalPolicy
	flowFileRepo   FlowFileRepository
	contentRepo    ContentRepository
	provenanceRepo ProvenanceRepository
	backupManager  *RepositoryBackupManager
	archived       map[uuid.UUID]*ArchivalRecord
	mu             sync.RWMutex
}

// ArchivalRecord tracks archived FlowFiles
type ArchivalRecord struct {
	FlowFileID     uuid.UUID `json:"flowFileId"`
	ArchivedAt     time.Time `json:"archivedAt"`
	OriginalSize   int64     `json:"originalSize"`
	ArchivePath    string    `json:"archivePath"`
	ArchiveBackupID uuid.UUID `json:"archiveBackupId"`
	Reason         string    `json:"reason"` // "age", "size", "pattern", "manual"
}

// ArchivalStats provides archival metrics
type ArchivalStats struct {
	TotalArchived    int
	TotalSize        int64
	ArchivedByAge    int
	ArchivedBySize   int
	ArchivedByPattern int
	ArchivedManually int
	LastArchivalTime time.Time
}

// NewArchivalManager creates a new archival manager
func NewArchivalManager(
	policy ArchivalPolicy,
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
	backupManager *RepositoryBackupManager,
) *ArchivalManager {
	return &ArchivalManager{
		policy:         policy,
		flowFileRepo:   flowFileRepo,
		contentRepo:    contentRepo,
		provenanceRepo: provenanceRepo,
		backupManager:  backupManager,
		archived:       make(map[uuid.UUID]*ArchivalRecord),
	}
}

// SetPolicy updates the archival policy
func (m *ArchivalManager) SetPolicy(policy ArchivalPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policy = policy
}

// GetPolicy returns the current archival policy
func (m *ArchivalManager) GetPolicy() ArchivalPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policy
}

// EvaluateArchival checks if archival is needed based on policy
func (m *ArchivalManager) EvaluateArchival() (bool, string, error) {
	m.mu.RLock()
	policy := m.policy
	m.mu.RUnlock()

	// Check count-based archival
	if policy.MaxFlowFiles > 0 {
		count, err := m.flowFileRepo.Count()
		if err != nil {
			return false, "", err
		}
		if count > policy.MaxFlowFiles {
			return true, fmt.Sprintf("FlowFile count (%d) exceeds maximum (%d)", count, policy.MaxFlowFiles), nil
		}
	}

	// Check size-based archival
	if policy.MaxRepositorySize > 0 {
		size, err := m.getRepositorySize()
		if err != nil {
			return false, "", err
		}
		if size > policy.MaxRepositorySize {
			return true, fmt.Sprintf("Repository size (%d) exceeds maximum (%d)", size, policy.MaxRepositorySize), nil
		}
	}

	// Check age-based archival
	if policy.MaxAge > 0 {
		oldFlowFiles, err := m.findOldFlowFiles(policy.MaxAge)
		if err != nil {
			return false, "", err
		}
		if len(oldFlowFiles) > 0 {
			return true, fmt.Sprintf("Found %d FlowFiles older than %s", len(oldFlowFiles), policy.MaxAge), nil
		}
	}

	return false, "", nil
}

// ArchiveOldData archives data based on age policy
func (m *ArchivalManager) ArchiveOldData() (int, error) {
	m.mu.RLock()
	maxAge := m.policy.MaxAge
	m.mu.RUnlock()

	if maxAge == 0 {
		return 0, nil
	}

	oldFlowFiles, err := m.findOldFlowFiles(maxAge)
	if err != nil {
		return 0, err
	}

	archived := 0
	for _, ff := range oldFlowFiles {
		if err := m.ArchiveFlowFile(ff, "age"); err != nil {
			return archived, err
		}
		archived++
	}

	return archived, nil
}

// ArchiveByPattern archives FlowFiles matching patterns
func (m *ArchivalManager) ArchiveByPattern() (int, error) {
	m.mu.RLock()
	patterns := m.policy.ArchivePatterns
	m.mu.RUnlock()

	if len(patterns) == 0 {
		return 0, nil
	}

	matchingFlowFiles, err := m.findMatchingFlowFiles(patterns)
	if err != nil {
		return 0, err
	}

	archived := 0
	for _, ff := range matchingFlowFiles {
		if err := m.ArchiveFlowFile(ff, "pattern"); err != nil {
			return archived, err
		}
		archived++
	}

	return archived, nil
}

// ArchiveFlowFile archives a single FlowFile
func (m *ArchivalManager) ArchiveFlowFile(ff *types.FlowFile, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already archived
	if _, exists := m.archived[ff.ID]; exists {
		return fmt.Errorf("FlowFile %s is already archived", ff.ID)
	}

	// Create backup for archival
	opts := BackupOptions{
		Type:     "full",
		Compress: true,
	}

	metadata, err := m.backupManager.CreateBackup(opts)
	if err != nil {
		return fmt.Errorf("failed to create archive backup: %w", err)
	}

	// Create archival record
	record := &ArchivalRecord{
		FlowFileID:      ff.ID,
		ArchivedAt:      time.Now(),
		OriginalSize:    ff.Size,
		ArchivePath:     m.policy.ArchiveLocation,
		ArchiveBackupID: metadata.ID,
		Reason:          reason,
	}

	m.archived[ff.ID] = record

	// Delete from active storage if policy allows
	if m.policy.DeleteAfterArchive {
		if err := m.flowFileRepo.Delete(ff.ID); err != nil {
			return fmt.Errorf("failed to delete FlowFile after archive: %w", err)
		}
	}

	return nil
}

// RestoreFromArchive restores an archived FlowFile
func (m *ArchivalManager) RestoreFromArchive(flowFileID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, exists := m.archived[flowFileID]
	if !exists {
		return fmt.Errorf("FlowFile %s not found in archive", flowFileID)
	}

	// Restore from backup
	if err := m.backupManager.RestoreBackup(record.ArchiveBackupID); err != nil {
		return fmt.Errorf("failed to restore from archive: %w", err)
	}

	// Remove from archived records
	delete(m.archived, flowFileID)

	return nil
}

// ListArchived returns all archived FlowFiles
func (m *ArchivalManager) ListArchived() []*ArchivalRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	records := make([]*ArchivalRecord, 0, len(m.archived))
	for _, record := range m.archived {
		records = append(records, record)
	}

	return records
}

// GetArchivedRecord retrieves archival record for a FlowFile
func (m *ArchivalManager) GetArchivedRecord(flowFileID uuid.UUID) (*ArchivalRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.archived[flowFileID]
	if !exists {
		return nil, fmt.Errorf("FlowFile %s not found in archive", flowFileID)
	}

	return record, nil
}

// PurgeExpiredArchives removes archived data past retention period
func (m *ArchivalManager) PurgeExpiredArchives() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.policy.ArchiveRetention == 0 {
		return 0, nil
	}

	cutoff := time.Now().Add(-m.policy.ArchiveRetention)
	purged := 0

	for id, record := range m.archived {
		if record.ArchivedAt.Before(cutoff) {
			// Delete backup
			if err := m.backupManager.DeleteBackup(record.ArchiveBackupID); err != nil {
				return purged, err
			}

			delete(m.archived, id)
			purged++
		}
	}

	return purged, nil
}

// GetStats returns archival statistics
func (m *ArchivalManager) GetStats() ArchivalStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := ArchivalStats{}

	var lastArchival time.Time
	for _, record := range m.archived {
		stats.TotalArchived++
		stats.TotalSize += record.OriginalSize

		switch record.Reason {
		case "age":
			stats.ArchivedByAge++
		case "size":
			stats.ArchivedBySize++
		case "pattern":
			stats.ArchivedByPattern++
		case "manual":
			stats.ArchivedManually++
		}

		if record.ArchivedAt.After(lastArchival) {
			lastArchival = record.ArchivedAt
		}
	}

	stats.LastArchivalTime = lastArchival
	return stats
}

// RunArchivalCycle performs a complete archival cycle
func (m *ArchivalManager) RunArchivalCycle() (*ArchivalStats, error) {
	// Check if archival is needed
	needed, reason, err := m.EvaluateArchival()
	if err != nil {
		return nil, err
	}

	if !needed {
		return &ArchivalStats{}, nil
	}

	// Archive by age
	archivedByAge, err := m.ArchiveOldData()
	if err != nil {
		return nil, fmt.Errorf("age-based archival failed: %w (reason: %s)", err, reason)
	}

	// Archive by pattern
	archivedByPattern, err := m.ArchiveByPattern()
	if err != nil {
		return nil, fmt.Errorf("pattern-based archival failed: %w", err)
	}

	// Purge expired archives
	purged, err := m.PurgeExpiredArchives()
	if err != nil {
		return nil, fmt.Errorf("purge expired archives failed: %w", err)
	}

	stats := m.GetStats()

	// Log archival cycle results
	fmt.Printf("Archival cycle completed: %d by age, %d by pattern, %d purged\n",
		archivedByAge, archivedByPattern, purged)

	return &stats, nil
}

// Helper functions

// findOldFlowFiles finds FlowFiles older than maxAge
func (m *ArchivalManager) findOldFlowFiles(maxAge time.Duration) ([]*types.FlowFile, error) {
	allFlowFiles, err := m.flowFileRepo.List(0, 0) // Get all
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-maxAge)
	oldFlowFiles := make([]*types.FlowFile, 0)

	for _, ff := range allFlowFiles {
		// Use UpdatedAt as proxy for entry date
		if ff.UpdatedAt.Before(cutoff) {
			oldFlowFiles = append(oldFlowFiles, ff)
		}
	}

	return oldFlowFiles, nil
}

// findMatchingFlowFiles finds FlowFiles matching archive patterns
func (m *ArchivalManager) findMatchingFlowFiles(patterns []string) ([]*types.FlowFile, error) {
	allFlowFiles, err := m.flowFileRepo.List(0, 0) // Get all
	if err != nil {
		return nil, err
	}

	matchingFlowFiles := make([]*types.FlowFile, 0)

	for _, ff := range allFlowFiles {
		for _, pattern := range patterns {
			// Check if FlowFile attributes match pattern
			// Simple implementation - could use regex or glob patterns
			if filename, ok := ff.Attributes["filename"]; ok {
				if filename == pattern {
					matchingFlowFiles = append(matchingFlowFiles, ff)
					break
				}
			}
		}
	}

	return matchingFlowFiles, nil
}

// getRepositorySize calculates total repository size
func (m *ArchivalManager) getRepositorySize() (int64, error) {
	claims, err := m.contentRepo.ListClaims()
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for _, claim := range claims {
		totalSize += claim.Length
	}

	return totalSize, nil
}
