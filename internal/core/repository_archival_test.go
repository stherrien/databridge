package core

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewArchivalManager(t *testing.T) {
	policy := ArchivalPolicy{
		MaxAge:             24 * time.Hour,
		MaxRepositorySize:  1024 * 1024 * 1024,
		MaxFlowFiles:       10000,
		DeleteAfterArchive: true,
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	backupMgr := &RepositoryBackupManager{}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	assert.NotNil(t, manager)
	assert.Equal(t, policy.MaxAge, manager.policy.MaxAge)
	assert.NotNil(t, manager.archived)
}

func TestSetGetPolicy(t *testing.T) {
	manager := NewArchivalManager(
		ArchivalPolicy{},
		new(MockFlowFileRepo),
		new(MockContentRepo),
		new(MockProvenanceRepo),
		&RepositoryBackupManager{},
	)

	newPolicy := ArchivalPolicy{
		MaxAge:       48 * time.Hour,
		MaxFlowFiles: 5000,
	}

	manager.SetPolicy(newPolicy)
	retrievedPolicy := manager.GetPolicy()

	assert.Equal(t, newPolicy.MaxAge, retrievedPolicy.MaxAge)
	assert.Equal(t, newPolicy.MaxFlowFiles, retrievedPolicy.MaxFlowFiles)
}

func TestEvaluateArchival(t *testing.T) {
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	t.Run("Count-Based Archival Needed", func(t *testing.T) {
		policy := ArchivalPolicy{
			MaxFlowFiles: 100,
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, &RepositoryBackupManager{})

		ffRepo.On("Count").Return(150, nil).Once()

		needed, reason, err := manager.EvaluateArchival()
		assert.NoError(t, err)
		assert.True(t, needed)
		assert.Contains(t, reason, "FlowFile count")
	})

	t.Run("Size-Based Archival Needed", func(t *testing.T) {
		policy := ArchivalPolicy{
			MaxRepositorySize: 1000,
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, &RepositoryBackupManager{})

		claims := []*types.ContentClaim{
			{Length: 600},
			{Length: 500},
		}
		contentRepo.On("ListClaims").Return(claims, nil).Once()

		needed, reason, err := manager.EvaluateArchival()
		assert.NoError(t, err)
		assert.True(t, needed)
		assert.Contains(t, reason, "Repository size")
	})

	t.Run("Age-Based Archival Needed", func(t *testing.T) {
		policy := ArchivalPolicy{
			MaxAge: 24 * time.Hour,
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, &RepositoryBackupManager{})

		oldFF := types.NewFlowFile()
		oldFF.UpdatedAt = time.Now().Add(-48 * time.Hour)

		ffRepo.On("List", 0, 0).Return([]*types.FlowFile{oldFF}, nil).Once()

		needed, reason, err := manager.EvaluateArchival()
		assert.NoError(t, err)
		assert.True(t, needed)
		assert.Contains(t, reason, "FlowFiles older than")
	})

	t.Run("No Archival Needed", func(t *testing.T) {
		policy := ArchivalPolicy{
			MaxFlowFiles: 1000,
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, &RepositoryBackupManager{})

		ffRepo.On("Count").Return(500, nil).Once()

		needed, _, err := manager.EvaluateArchival()
		assert.NoError(t, err)
		assert.False(t, needed)
	})
}

func TestArchiveOldData(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	policy := ArchivalPolicy{
		MaxAge:             24 * time.Hour,
		DeleteAfterArchive: true,
	}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	// Create old FlowFiles
	oldFF1 := types.NewFlowFile()
	oldFF1.UpdatedAt = time.Now().Add(-48 * time.Hour)

	oldFF2 := types.NewFlowFile()
	oldFF2.UpdatedAt = time.Now().Add(-36 * time.Hour)

	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{oldFF1, oldFF2}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)
	ffRepo.On("Delete", mock.Anything).Return(nil)

	archived, err := manager.ArchiveOldData()
	assert.NoError(t, err)
	assert.Equal(t, 2, archived)

	// Verify archival records created
	records := manager.ListArchived()
	assert.Len(t, records, 2)
}

func TestArchiveByPattern(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	policy := ArchivalPolicy{
		ArchivePatterns:    []string{"archive-me.txt"},
		DeleteAfterArchive: false,
	}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	// Create matching FlowFile
	matchingFF := types.NewFlowFile()
	matchingFF.Attributes["filename"] = "archive-me.txt"

	nonMatchingFF := types.NewFlowFile()
	nonMatchingFF.Attributes["filename"] = "keep-me.txt"

	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{matchingFF, nonMatchingFF}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)

	archived, err := manager.ArchiveByPattern()
	assert.NoError(t, err)
	assert.Equal(t, 1, archived)

	// Verify only matching FlowFile was archived
	records := manager.ListArchived()
	assert.Len(t, records, 1)
	assert.Equal(t, matchingFF.ID, records[0].FlowFileID)
}

func TestArchiveFlowFile(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	t.Run("Archive with Deletion", func(t *testing.T) {
		policy := ArchivalPolicy{
			DeleteAfterArchive: true,
			ArchiveLocation:    "/archive",
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

		ff := types.NewFlowFile()
		ff.Size = 1024

		ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil).Once()
		contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil).Once()
		provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil).Once()
		ffRepo.On("Delete", ff.ID).Return(nil).Once()

		err := manager.ArchiveFlowFile(ff, "manual")
		assert.NoError(t, err)

		// Verify archival record
		record, err := manager.GetArchivedRecord(ff.ID)
		assert.NoError(t, err)
		assert.Equal(t, ff.ID, record.FlowFileID)
		assert.Equal(t, int64(1024), record.OriginalSize)
		assert.Equal(t, "manual", record.Reason)
		assert.Equal(t, "/archive", record.ArchivePath)

		ffRepo.AssertCalled(t, "Delete", ff.ID)
	})

	t.Run("Archive without Deletion", func(t *testing.T) {
		policy := ArchivalPolicy{
			DeleteAfterArchive: false,
		}

		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

		ff := types.NewFlowFile()

		ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil).Once()
		contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil).Once()
		provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil).Once()

		err := manager.ArchiveFlowFile(ff, "test")
		assert.NoError(t, err)

		// Verify Delete was not called
		ffRepo.AssertNotCalled(t, "Delete", ff.ID)
	})

	t.Run("Archive Already Archived FlowFile", func(t *testing.T) {
		policy := ArchivalPolicy{}
		manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

		ff := types.NewFlowFile()

		ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil).Once()
		contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil).Once()
		provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil).Once()

		// Archive first time
		err := manager.ArchiveFlowFile(ff, "test")
		assert.NoError(t, err)

		// Try to archive again
		err = manager.ArchiveFlowFile(ff, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already archived")
	})
}

func TestRestoreFromArchive(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	policy := ArchivalPolicy{
		DeleteAfterArchive: true,
	}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	// Archive a FlowFile first
	ff := types.NewFlowFile()
	ff.Attributes["test"] = "value"

	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)
	ffRepo.On("Delete", ff.ID).Return(nil)

	err := manager.ArchiveFlowFile(ff, "test")
	assert.NoError(t, err)

	// Setup restore mocks
	ffRepo.On("Store", mock.AnythingOfType("*types.FlowFile")).Return(nil)

	// Restore from archive
	err = manager.RestoreFromArchive(ff.ID)
	assert.NoError(t, err)

	// Verify FlowFile is no longer in archive
	_, err = manager.GetArchivedRecord(ff.ID)
	assert.Error(t, err)
}

func TestListArchived(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	manager := NewArchivalManager(ArchivalPolicy{}, ffRepo, contentRepo, provRepo, backupMgr)

	// Archive multiple FlowFiles
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)

	ff1 := types.NewFlowFile()
	ff2 := types.NewFlowFile()

	manager.ArchiveFlowFile(ff1, "age")
	manager.ArchiveFlowFile(ff2, "pattern")

	records := manager.ListArchived()
	assert.Len(t, records, 2)

	ids := []uuid.UUID{records[0].FlowFileID, records[1].FlowFileID}
	assert.Contains(t, ids, ff1.ID)
	assert.Contains(t, ids, ff2.ID)
}

func TestPurgeExpiredArchives(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	policy := ArchivalPolicy{
		ArchiveRetention: 24 * time.Hour,
	}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	// Archive FlowFiles
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)

	ff1 := types.NewFlowFile()
	ff2 := types.NewFlowFile()

	manager.ArchiveFlowFile(ff1, "test")
	manager.ArchiveFlowFile(ff2, "test")

	// Manually set one to be expired
	manager.mu.Lock()
	manager.archived[ff1.ID].ArchivedAt = time.Now().Add(-48 * time.Hour)
	manager.mu.Unlock()

	// Purge expired archives
	purged, err := manager.PurgeExpiredArchives()
	assert.NoError(t, err)
	assert.Equal(t, 1, purged)

	// Verify only one archive remains
	records := manager.ListArchived()
	assert.Len(t, records, 1)
	assert.Equal(t, ff2.ID, records[0].FlowFileID)
}

func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	manager := NewArchivalManager(ArchivalPolicy{}, ffRepo, contentRepo, provRepo, backupMgr)

	// Archive FlowFiles with different reasons
	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)

	ff1 := types.NewFlowFile()
	ff1.Size = 100
	ff2 := types.NewFlowFile()
	ff2.Size = 200
	ff3 := types.NewFlowFile()
	ff3.Size = 300

	manager.ArchiveFlowFile(ff1, "age")
	manager.ArchiveFlowFile(ff2, "pattern")
	manager.ArchiveFlowFile(ff3, "manual")

	stats := manager.GetStats()

	assert.Equal(t, 3, stats.TotalArchived)
	assert.Equal(t, int64(600), stats.TotalSize)
	assert.Equal(t, 1, stats.ArchivedByAge)
	assert.Equal(t, 1, stats.ArchivedByPattern)
	assert.Equal(t, 1, stats.ArchivedManually)
	assert.False(t, stats.LastArchivalTime.IsZero())
}

func TestRunArchivalCycle(t *testing.T) {
	tmpDir := t.TempDir()
	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	backupMgr := NewRepositoryBackupManager(tmpDir, ffRepo, contentRepo, provRepo)

	policy := ArchivalPolicy{
		MaxAge:             24 * time.Hour,
		ArchivePatterns:    []string{"archive.txt"},
		ArchiveRetention:   48 * time.Hour,
		DeleteAfterArchive: false,
	}

	manager := NewArchivalManager(policy, ffRepo, contentRepo, provRepo, backupMgr)

	// Create old FlowFile
	oldFF := types.NewFlowFile()
	oldFF.UpdatedAt = time.Now().Add(-48 * time.Hour)

	// Create pattern matching FlowFile
	patternFF := types.NewFlowFile()
	patternFF.Attributes["filename"] = "archive.txt"

	ffRepo.On("List", 0, 0).Return([]*types.FlowFile{oldFF, patternFF}, nil)
	contentRepo.On("ListClaims").Return([]*types.ContentClaim{}, nil)
	provRepo.On("GetEvents", 0, 0).Return([]*types.ProvenanceEvent{}, nil)

	stats, err := manager.RunArchivalCycle()
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Verify archival occurred
	records := manager.ListArchived()
	assert.Len(t, records, 2)
}

func TestGetRepositorySize(t *testing.T) {
	contentRepo := new(MockContentRepo)

	manager := &ArchivalManager{
		contentRepo: contentRepo,
	}

	claims := []*types.ContentClaim{
		{Length: 1024},
		{Length: 2048},
		{Length: 512},
	}

	contentRepo.On("ListClaims").Return(claims, nil)

	size, err := manager.getRepositorySize()
	assert.NoError(t, err)
	assert.Equal(t, int64(3584), size)
}
