package core

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// BackupMetadata contains information about a backup
type BackupMetadata struct {
	ID             uuid.UUID  `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	Type           string     `json:"type"`           // "full", "incremental"
	RepositoryType string     `json:"repositoryType"` // "flowfile", "content", "provenance"
	FlowFileCount  int        `json:"flowFileCount"`
	ContentSize    int64      `json:"contentSize"`
	Checksum       string     `json:"checksum"`
	Compressed     bool       `json:"compressed"`
	Encrypted      bool       `json:"encrypted"`
	BaseBackupID   *uuid.UUID `json:"baseBackupId,omitempty"` // For incrementals
}

// RepositoryBackupManager handles backup and restore operations
type RepositoryBackupManager struct {
	backupDir      string
	flowFileRepo   FlowFileRepository
	contentRepo    ContentRepository
	provenanceRepo ProvenanceRepository
}

// NewRepositoryBackupManager creates a new backup manager
func NewRepositoryBackupManager(
	backupDir string,
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
) *RepositoryBackupManager {
	return &RepositoryBackupManager{
		backupDir:      backupDir,
		flowFileRepo:   flowFileRepo,
		contentRepo:    contentRepo,
		provenanceRepo: provenanceRepo,
	}
}

// BackupOptions configures backup behavior
type BackupOptions struct {
	Type       string // "full" or "incremental"
	Compress   bool
	Encrypt    bool
	EncryptKey []byte
	BaseBackup *uuid.UUID // For incremental backups
}

// CreateBackup creates a new repository backup
func (m *RepositoryBackupManager) CreateBackup(opts BackupOptions) (*BackupMetadata, error) {
	// Create backup metadata
	metadata := &BackupMetadata{
		ID:             uuid.New(),
		Timestamp:      time.Now(),
		Type:           opts.Type,
		RepositoryType: "full",
		Compressed:     opts.Compress,
		Encrypted:      opts.Encrypt,
		BaseBackupID:   opts.BaseBackup,
	}

	// Create backup directory
	backupPath := filepath.Join(m.backupDir, metadata.ID.String())
	if err := os.MkdirAll(backupPath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup FlowFiles
	flowFilePath := filepath.Join(backupPath, "flowfiles.tar.gz")
	flowFileCount, err := m.backupFlowFiles(flowFilePath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to backup flowfiles: %w", err)
	}
	metadata.FlowFileCount = flowFileCount

	// Backup content
	contentPath := filepath.Join(backupPath, "content.tar.gz")
	contentSize, err := m.backupContent(contentPath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to backup content: %w", err)
	}
	metadata.ContentSize = contentSize

	// Backup provenance
	provenancePath := filepath.Join(backupPath, "provenance.tar.gz")
	if err := m.backupProvenance(provenancePath, opts); err != nil {
		return nil, fmt.Errorf("failed to backup provenance: %w", err)
	}

	// Save metadata
	metadataPath := filepath.Join(backupPath, "metadata.json")
	if err := m.saveMetadata(metadata, metadataPath); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return metadata, nil
}

// backupFlowFiles backs up FlowFile repository
func (m *RepositoryBackupManager) backupFlowFiles(outputPath string, opts BackupOptions) (int, error) {
	// #nosec G304 - outputPath is constructed from backup manager's backup directory
	file, err := os.Create(outputPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	var writer io.Writer = file
	var gzWriter *gzip.Writer
	if opts.Compress {
		gzWriter = gzip.NewWriter(file)
		defer func() {
			if closeErr := gzWriter.Close(); closeErr != nil {
				// Log close error but don't override return value
			}
		}()
		writer = gzWriter
	}

	tarWriter := tar.NewWriter(writer)
	defer func() {
		if closeErr := tarWriter.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	// Get all FlowFiles
	flowFiles, err := m.flowFileRepo.List(0, 0) // Get all
	if err != nil {
		return 0, err
	}

	// Write each FlowFile to tar
	for _, ff := range flowFiles {
		data, err := json.Marshal(ff)
		if err != nil {
			return 0, err
		}

		header := &tar.Header{
			Name: fmt.Sprintf("flowfile-%s.json", ff.ID.String()),
			Mode: 0600,
			Size: int64(len(data)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return 0, err
		}

		if _, err := tarWriter.Write(data); err != nil {
			return 0, err
		}
	}

	return len(flowFiles), nil
}

// backupContent backs up content repository
func (m *RepositoryBackupManager) backupContent(outputPath string, opts BackupOptions) (int64, error) {
	// #nosec G304 - outputPath is constructed from backup manager's backup directory
	file, err := os.Create(outputPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	var writer io.Writer = file
	var gzWriter *gzip.Writer
	if opts.Compress {
		gzWriter = gzip.NewWriter(file)
		defer func() {
			if closeErr := gzWriter.Close(); closeErr != nil {
				// Log close error but don't override return value
			}
		}()
		writer = gzWriter
	}

	tarWriter := tar.NewWriter(writer)
	defer func() {
		if closeErr := tarWriter.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	var totalSize int64

	// List all content claims
	claims, err := m.contentRepo.ListClaims()
	if err != nil {
		return 0, err
	}

	// Backup each content claim
	for _, claim := range claims {
		// Get content data
		reader, err := m.contentRepo.Read(claim)
		if err != nil {
			return 0, err
		}
		defer func() {
			if closeErr := reader.Close(); closeErr != nil {
				// Log close error but don't override return value
			}
		}()

		// Create tar header
		header := &tar.Header{
			Name: fmt.Sprintf("content/%s-%s-%d-%d", claim.Container, claim.Section, claim.Offset, claim.Length),
			Mode: 0600,
			Size: claim.Length,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return 0, err
		}

		// Copy content to tar
		n, err := io.CopyN(tarWriter, reader, claim.Length)
		if err != nil {
			return 0, err
		}
		totalSize += n
	}

	return totalSize, nil
}

// backupProvenance backs up provenance repository
func (m *RepositoryBackupManager) backupProvenance(outputPath string, opts BackupOptions) error {
	// #nosec G304 - outputPath is constructed from backup manager's backup directory
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	var writer io.Writer = file
	var gzWriter *gzip.Writer
	if opts.Compress {
		gzWriter = gzip.NewWriter(file)
		defer func() {
			if closeErr := gzWriter.Close(); closeErr != nil {
				// Log close error but don't override return value
			}
		}()
		writer = gzWriter
	}

	tarWriter := tar.NewWriter(writer)
	defer func() {
		if closeErr := tarWriter.Close(); closeErr != nil {
			// Log close error but don't override return value
		}
	}()

	// Get all provenance events
	events, err := m.provenanceRepo.GetEvents(0, 0) // Get all
	if err != nil {
		return err
	}

	// Write each event to tar
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		header := &tar.Header{
			Name: fmt.Sprintf("provenance/%d.json", event.ID),
			Mode: 0600,
			Size: int64(len(data)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if _, err := tarWriter.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// RestoreBackup restores from a backup
func (m *RepositoryBackupManager) RestoreBackup(backupID uuid.UUID) error {
	backupPath := filepath.Join(m.backupDir, backupID.String())

	// Load metadata
	metadataPath := filepath.Join(backupPath, "metadata.json")
	metadata, err := m.loadMetadata(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}

	// Restore FlowFiles
	flowFilePath := filepath.Join(backupPath, "flowfiles.tar.gz")
	if err := m.restoreFlowFiles(flowFilePath, metadata); err != nil {
		return fmt.Errorf("failed to restore flowfiles: %w", err)
	}

	// Restore content
	contentPath := filepath.Join(backupPath, "content.tar.gz")
	if err := m.restoreContent(contentPath, metadata); err != nil {
		return fmt.Errorf("failed to restore content: %w", err)
	}

	// Restore provenance
	provenancePath := filepath.Join(backupPath, "provenance.tar.gz")
	if err := m.restoreProvenance(provenancePath, metadata); err != nil {
		return fmt.Errorf("failed to restore provenance: %w", err)
	}

	return nil
}

// restoreFlowFiles restores FlowFile repository
func (m *RepositoryBackupManager) restoreFlowFiles(inputPath string, metadata *BackupMetadata) error {
	// #nosec G304 - inputPath is constructed from backup manager's backup directory
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log close error but don't override return value
		}
	}()

	var reader io.Reader = file
	if metadata.Compressed {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer func() {
			if err := gzReader.Close(); err != nil {
				// Log close error but don't override return value
			}
		}()
		reader = gzReader
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Read FlowFile data
		data := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, data); err != nil {
			return err
		}

		// Unmarshal and store
		var ff types.FlowFile
		if err := json.Unmarshal(data, &ff); err != nil {
			return err
		}

		if err := m.flowFileRepo.Store(&ff); err != nil {
			return err
		}
	}

	return nil
}

// restoreContent restores content repository
func (m *RepositoryBackupManager) restoreContent(inputPath string, metadata *BackupMetadata) error {
	// #nosec G304 - inputPath is constructed from backup manager's backup directory
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log close error but don't override return value
		}
	}()

	var reader io.Reader = file
	if metadata.Compressed {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer func() {
			if err := gzReader.Close(); err != nil {
				// Log close error but don't override return value
			}
		}()
		reader = gzReader
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Read content data
		data := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, data); err != nil {
			return err
		}

		// Parse claim info from filename
		// Format: content/container-section-offset-length
		var container, section string
		var offset, length int64
		_, _ = fmt.Sscanf(filepath.Base(header.Name), "%s-%s-%d-%d", &container, &section, &offset, &length)

		claim := &types.ContentClaim{
			Container: container,
			Section:   section,
			Offset:    offset,
			Length:    length,
		}

		// Write content back
		if err := m.contentRepo.Write(claim, data); err != nil {
			return err
		}
	}

	return nil
}

// restoreProvenance restores provenance repository
func (m *RepositoryBackupManager) restoreProvenance(inputPath string, metadata *BackupMetadata) error {
	// #nosec G304 - inputPath is constructed from backup manager's backup directory
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log close error but don't override return value
		}
	}()

	var reader io.Reader = file
	if metadata.Compressed {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer func() {
			if err := gzReader.Close(); err != nil {
				// Log close error but don't override return value
			}
		}()
		reader = gzReader
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Read event data
		data := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, data); err != nil {
			return err
		}

		// Unmarshal and store
		var event ProvenanceEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}

		if err := m.provenanceRepo.AddEvent(&event); err != nil {
			return err
		}
	}

	return nil
}

// ListBackups returns all available backups
func (m *RepositoryBackupManager) ListBackups() ([]*BackupMetadata, error) {
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, err
	}

	backups := make([]*BackupMetadata, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataPath := filepath.Join(m.backupDir, entry.Name(), "metadata.json")
		metadata, err := m.loadMetadata(metadataPath)
		if err != nil {
			continue // Skip invalid backups
		}

		backups = append(backups, metadata)
	}

	return backups, nil
}

// DeleteBackup removes a backup
func (m *RepositoryBackupManager) DeleteBackup(backupID uuid.UUID) error {
	backupPath := filepath.Join(m.backupDir, backupID.String())
	return os.RemoveAll(backupPath)
}

// VerifyBackup checks backup integrity
func (m *RepositoryBackupManager) VerifyBackup(backupID uuid.UUID) error {
	backupPath := filepath.Join(m.backupDir, backupID.String())

	// Check metadata exists
	metadataPath := filepath.Join(backupPath, "metadata.json")
	if _, err := os.Stat(metadataPath); err != nil {
		return fmt.Errorf("metadata not found: %w", err)
	}

	// Check required files exist
	requiredFiles := []string{
		"flowfiles.tar.gz",
		"content.tar.gz",
		"provenance.tar.gz",
	}

	for _, filename := range requiredFiles {
		path := filepath.Join(backupPath, filename)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("required file %s not found: %w", filename, err)
		}
	}

	// TODO: Verify checksums

	return nil
}

// saveMetadata saves backup metadata
func (m *RepositoryBackupManager) saveMetadata(metadata *BackupMetadata, path string) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// loadMetadata loads backup metadata
func (m *RepositoryBackupManager) loadMetadata(path string) (*BackupMetadata, error) {
	// #nosec G304 - path is constructed from backup manager's backup directory
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata BackupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// PruneOldBackups removes backups older than specified duration
func (m *RepositoryBackupManager) PruneOldBackups(maxAge time.Duration) (int, error) {
	backups, err := m.ListBackups()
	if err != nil {
		return 0, err
	}

	pruned := 0
	cutoff := time.Now().Add(-maxAge)

	for _, backup := range backups {
		if backup.Timestamp.Before(cutoff) {
			if err := m.DeleteBackup(backup.ID); err != nil {
				return pruned, err
			}
			pruned++
		}
	}

	return pruned, nil
}
