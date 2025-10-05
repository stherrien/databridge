package core

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

// Mock repositories for testing
type MockFlowFileRepo struct {
	mock.Mock
}

func (m *MockFlowFileRepo) Store(ff *types.FlowFile) error {
	args := m.Called(ff)
	return args.Error(0)
}

func (m *MockFlowFileRepo) Get(id uuid.UUID) (*types.FlowFile, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.FlowFile), args.Error(1)
}

func (m *MockFlowFileRepo) Delete(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockFlowFileRepo) List(offset, limit int) ([]*types.FlowFile, error) {
	args := m.Called(offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.FlowFile), args.Error(1)
}

func (m *MockFlowFileRepo) Count() (int, error) {
	args := m.Called()
	return args.Int(0), args.Error(1)
}

func (m *MockFlowFileRepo) UpdateAttributes(id uuid.UUID, attributes map[string]string) error {
	args := m.Called(id, attributes)
	return args.Error(0)
}

func (m *MockFlowFileRepo) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockContentRepo struct {
	mock.Mock
}

func (m *MockContentRepo) Write(claim *types.ContentClaim, data []byte) error {
	args := m.Called(claim, data)
	return args.Error(0)
}

func (m *MockContentRepo) Read(claim *types.ContentClaim) (io.ReadCloser, error) {
	args := m.Called(claim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockContentRepo) Remove(claim *types.ContentClaim) error {
	args := m.Called(claim)
	return args.Error(0)
}

func (m *MockContentRepo) ListClaims() ([]*types.ContentClaim, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.ContentClaim), args.Error(1)
}

func (m *MockContentRepo) Store(content []byte) (*types.ContentClaim, error) {
	args := m.Called(content)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.ContentClaim), args.Error(1)
}

func (m *MockContentRepo) Get(claim *types.ContentClaim) ([]byte, error) {
	args := m.Called(claim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockContentRepo) GetReader(claim *types.ContentClaim) (io.ReadCloser, error) {
	args := m.Called(claim)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockContentRepo) GetWriter() (ContentWriter, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ContentWriter), args.Error(1)
}

func (m *MockContentRepo) Delete(claim *types.ContentClaim) error {
	args := m.Called(claim)
	return args.Error(0)
}

func (m *MockContentRepo) IncrementRef(claim *types.ContentClaim) error {
	args := m.Called(claim)
	return args.Error(0)
}

func (m *MockContentRepo) DecrementRef(claim *types.ContentClaim) error {
	args := m.Called(claim)
	return args.Error(0)
}

func (m *MockContentRepo) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockProvenanceRepo struct {
	mock.Mock
}

func (m *MockProvenanceRepo) AddEvent(event *ProvenanceEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *MockProvenanceRepo) GetEvent(id int64) (*ProvenanceEvent, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProvenanceEvent), args.Error(1)
}

func (m *MockProvenanceRepo) GetEvents(offset, limit int) ([]*ProvenanceEvent, error) {
	args := m.Called(offset, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ProvenanceEvent), args.Error(1)
}

func (m *MockProvenanceRepo) SearchEvents(query string, limit int) ([]*ProvenanceEvent, error) {
	args := m.Called(query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ProvenanceEvent), args.Error(1)
}

func (m *MockProvenanceRepo) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockProvenanceRepo) Store(event *ProvenanceEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *MockProvenanceRepo) Query(query ProvenanceQuery) ([]*ProvenanceEvent, error) {
	args := m.Called(query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ProvenanceEvent), args.Error(1)
}

func (m *MockProvenanceRepo) GetLineage(flowFileId uuid.UUID) (*LineageGraph, error) {
	args := m.Called(flowFileId)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*LineageGraph), args.Error(1)
}

func TestNewRepositoryBackupManager(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	assert.NotNil(t, manager)
	assert.Equal(t, tmpDir, manager.backupDir)
	assert.NotNil(t, manager.flowFileRepo)
	assert.NotNil(t, manager.contentRepo)
	assert.NotNil(t, manager.provenanceRepo)
}

func TestCreateBackup(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Setup test data
	flowFiles := []*types.FlowFile{
		types.NewFlowFile(),
		types.NewFlowFile(),
	}
	flowFiles[0].Attributes["test"] = "value1"
	flowFiles[1].Attributes["test"] = "value2"

	claims := []*types.ContentClaim{
		{Container: "test", Section: "section1", Offset: 0, Length: 100},
	}

	events := []*ProvenanceEvent{
		{ID: uuid.New(), EventType: "CREATE"},
	}

	ffRepo.On("List", 0, 0).Return(flowFiles, nil)
	contentRepo.On("ListClaims").Return(claims, nil)
	contentRepo.On("Read", mock.Anything).Return(io.NopCloser(bytes.NewReader(make([]byte, 100))), nil)
	provRepo.On("GetEvents", 0, 0).Return(events, nil)

	t.Run("Full Backup Uncompressed", func(t *testing.T) {
		opts := BackupOptions{
			Type:     "full",
			Compress: false,
		}

		metadata, err := manager.CreateBackup(opts)
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, "full", metadata.Type)
		assert.Equal(t, 2, metadata.FlowFileCount)
		assert.False(t, metadata.Compressed)

		// Verify backup directory exists
		backupPath := filepath.Join(tmpDir, metadata.ID.String())
		_, err = os.Stat(backupPath)
		assert.NoError(t, err)

		// Verify metadata file exists
		metadataPath := filepath.Join(backupPath, "metadata.json")
		_, err = os.Stat(metadataPath)
		assert.NoError(t, err)
	})

	t.Run("Full Backup Compressed", func(t *testing.T) {
		opts := BackupOptions{
			Type:     "full",
			Compress: true,
		}

		metadata, err := manager.CreateBackup(opts)
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.True(t, metadata.Compressed)
	})

	t.Run("Incremental Backup", func(t *testing.T) {
		baseID := uuid.New()
		opts := BackupOptions{
			Type:       "incremental",
			Compress:   true,
			BaseBackup: &baseID,
		}

		metadata, err := manager.CreateBackup(opts)
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, "incremental", metadata.Type)
		assert.Equal(t, baseID, *metadata.BaseBackupID)
	})
}

func TestBackupRestore(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Create test FlowFiles
	ff1 := types.NewFlowFile()
	ff1.Attributes["key"] = "value1"
	ff1.Size = 100

	ff2 := types.NewFlowFile()
	ff2.Attributes["key"] = "value2"
	ff2.Size = 200

	flowFiles := []*types.FlowFile{ff1, ff2}

	claims := []*types.ContentClaim{}
	events := []*ProvenanceEvent{}

	// Setup mocks for backup
	ffRepo.On("List", 0, 0).Return(flowFiles, nil)
	contentRepo.On("ListClaims").Return(claims, nil)
	provRepo.On("GetEvents", 0, 0).Return(events, nil)

	// Create backup
	opts := BackupOptions{
		Type:     "full",
		Compress: true,
	}

	metadata, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	// Setup mocks for restore
	ffRepo.On("Store", mock.AnythingOfType("*types.FlowFile")).Return(nil)

	// Restore backup
	err = manager.RestoreBackup(metadata.ID)
	assert.NoError(t, err)

	// Verify restore was called
	ffRepo.AssertCalled(t, "Store", mock.AnythingOfType("*types.FlowFile"))
}

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Setup empty data for backups
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*ProvenanceEvent{}, nil)

	// Create multiple backups
	opts := BackupOptions{Type: "full", Compress: true}

	backup1, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	backup2, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	// List backups
	backups, err := manager.ListBackups()
	assert.NoError(t, err)
	assert.Len(t, backups, 2)

	// Verify backups are present
	ids := []uuid.UUID{backups[0].ID, backups[1].ID}
	assert.Contains(t, ids, backup1.ID)
	assert.Contains(t, ids, backup2.ID)
}

func TestDeleteBackup(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Setup empty data
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*ProvenanceEvent{}, nil)

	// Create backup
	opts := BackupOptions{Type: "full", Compress: true}
	metadata, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	// Verify backup exists
	backupPath := filepath.Join(tmpDir, metadata.ID.String())
	_, err = os.Stat(backupPath)
	assert.NoError(t, err)

	// Delete backup
	err = manager.DeleteBackup(metadata.ID)
	assert.NoError(t, err)

	// Verify backup is deleted
	_, err = os.Stat(backupPath)
	assert.True(t, os.IsNotExist(err))
}

func TestVerifyBackup(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Setup empty data
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*ProvenanceEvent{}, nil)

	t.Run("Valid Backup", func(t *testing.T) {
		opts := BackupOptions{Type: "full", Compress: true}
		metadata, err := manager.CreateBackup(opts)
		assert.NoError(t, err)

		err = manager.VerifyBackup(metadata.ID)
		assert.NoError(t, err)
	})

	t.Run("Missing Backup", func(t *testing.T) {
		nonExistentID := uuid.New()
		err := manager.VerifyBackup(nonExistentID)
		assert.Error(t, err)
	})

	t.Run("Corrupted Backup", func(t *testing.T) {
		opts := BackupOptions{Type: "full", Compress: true}
		metadata, err := manager.CreateBackup(opts)
		assert.NoError(t, err)

		// Delete one of the required files
		backupPath := filepath.Join(tmpDir, metadata.ID.String())
		os.Remove(filepath.Join(backupPath, "flowfiles.tar.gz"))

		err = manager.VerifyBackup(metadata.ID)
		assert.Error(t, err)
	})
}

func TestPruneOldBackups(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	manager := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	// Setup empty data
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*ProvenanceEvent{}, nil)

	// Create multiple backups
	opts := BackupOptions{Type: "full", Compress: true}

	oldBackup, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	// Manually modify timestamp to make it old
	oldBackup.Timestamp = time.Now().Add(-48 * time.Hour)
	metadataPath := filepath.Join(tmpDir, oldBackup.ID.String(), "metadata.json")
	manager.saveMetadata(oldBackup, metadataPath)

	newBackup, err := manager.CreateBackup(opts)
	assert.NoError(t, err)

	// Prune backups older than 24 hours
	pruned, err := manager.PruneOldBackups(24 * time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, 1, pruned)

	// Verify old backup is deleted
	backups, err := manager.ListBackups()
	assert.NoError(t, err)
	assert.Len(t, backups, 1)
	assert.Equal(t, newBackup.ID, backups[0].ID)
}

func TestBackupMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	metadata := &BackupMetadata{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Type:          "full",
		FlowFileCount: 100,
		ContentSize:   1024 * 1024,
		Compressed:    true,
		Encrypted:     false,
	}

	manager := &RepositoryBackupManager{backupDir: tmpDir}

	t.Run("Save and Load Metadata", func(t *testing.T) {
		metadataPath := filepath.Join(tmpDir, "test-metadata.json")

		err := manager.saveMetadata(metadata, metadataPath)
		assert.NoError(t, err)

		loaded, err := manager.loadMetadata(metadataPath)
		assert.NoError(t, err)
		assert.Equal(t, metadata.ID, loaded.ID)
		assert.Equal(t, metadata.Type, loaded.Type)
		assert.Equal(t, metadata.FlowFileCount, loaded.FlowFileCount)
		assert.Equal(t, metadata.Compressed, loaded.Compressed)
	})

	t.Run("Load Non-existent Metadata", func(t *testing.T) {
		_, err := manager.loadMetadata(filepath.Join(tmpDir, "nonexistent.json"))
		assert.Error(t, err)
	})
}
