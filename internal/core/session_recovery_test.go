package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewSessionRecoveryManager(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{
		CheckpointDir:      tmpDir,
		CheckpointInterval: 5 * time.Second,
		MaxCheckpointAge:   1 * time.Hour,
		AutoRecovery:       true,
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, err := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	assert.Equal(t, tmpDir, mgr.checkpointDir)
	assert.True(t, mgr.recoveryEnabled)

	// Verify checkpoint directory was created
	_, err = os.Stat(tmpDir)
	assert.NoError(t, err)
}

func TestCheckpointSession(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{
		CheckpointDir:      tmpDir,
		CheckpointInterval: 5 * time.Second,
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	session := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	// Create some FlowFiles in the session
	ff1 := session.Create()
	ff2 := session.Create()
	ff1.Attributes["test"] = "value1"
	ff2.Attributes["test"] = "value2"

	err := mgr.CheckpointSession(session)
	assert.NoError(t, err)

	// Verify checkpoint exists in memory
	assert.Contains(t, mgr.checkpoints, session.id)

	// Verify checkpoint file exists on disk
	checkpointPath := filepath.Join(tmpDir, session.id.String()+".json")
	_, err = os.Stat(checkpointPath)
	assert.NoError(t, err)

	// Verify checkpoint data
	checkpoint := mgr.checkpoints[session.id]
	assert.Equal(t, session.id, checkpoint.SessionID)
	assert.Len(t, checkpoint.Creations, 2)
	assert.False(t, checkpoint.Committed)
	assert.False(t, checkpoint.RolledBack)
}

func TestRecoverSession(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{
		CheckpointDir:      tmpDir,
		CheckpointInterval: 5 * time.Second,
	}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create and checkpoint a session
	originalSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	ff1 := originalSession.Create()
	ff2 := originalSession.Create()
	ff1.Attributes["test"] = "value1"
	ff2.Attributes["test"] = "value2"

	mgr.CheckpointSession(originalSession)

	// Recover the session
	recoveredSession, err := mgr.RecoverSession(originalSession.id, ctx, logger)
	assert.NoError(t, err)
	assert.NotNil(t, recoveredSession)

	// Verify recovered state
	assert.Equal(t, originalSession.id, recoveredSession.id)
	assert.Len(t, recoveredSession.creations, 2)
	assert.Equal(t, "value1", recoveredSession.creations[0].Attributes["test"])
	assert.Equal(t, "value2", recoveredSession.creations[1].Attributes["test"])
}

func TestRecoverNonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	nonExistentID := types.NewFlowFile().ID

	_, err := mgr.RecoverSession(nonExistentID, ctx, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no checkpoint found")
}

func TestRecoverCommittedSession(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	ffRepo.On("Store", mock.Anything).Return(nil)

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create, checkpoint, and commit a session
	session := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session.Create()
	mgr.CheckpointSession(session)

	// Mark as committed
	mgr.mu.Lock()
	mgr.checkpoints[session.id].Committed = true
	mgr.mu.Unlock()

	// Try to recover
	_, err := mgr.RecoverSession(session.id, ctx, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already finalized")
}

func TestRecoverAllSessions(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create multiple sessions
	session1 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session2 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session3 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	session1.Create()
	session2.Create()
	session3.Create()

	// Checkpoint all
	mgr.CheckpointSession(session1)
	mgr.CheckpointSession(session2)
	mgr.CheckpointSession(session3)

	// Mark one as committed (shouldn't be recovered)
	mgr.mu.Lock()
	mgr.checkpoints[session2.id].Committed = true
	mgr.mu.Unlock()

	// Recover all
	recovered, err := mgr.RecoverAllSessions(ctx, logger)
	assert.NoError(t, err)
	assert.Len(t, recovered, 2) // session2 should not be recovered

	// Verify recovered sessions
	recoveredIDs := []interface{}{recovered[0].id, recovered[1].id}
	assert.Contains(t, recoveredIDs, session1.id)
	assert.Contains(t, recoveredIDs, session3.id)
	assert.NotContains(t, recoveredIDs, session2.id)
}

func TestDeleteCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	session := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	// Checkpoint session
	mgr.CheckpointSession(session)

	checkpointPath := filepath.Join(tmpDir, session.id.String()+".json")
	_, err := os.Stat(checkpointPath)
	assert.NoError(t, err)

	// Delete checkpoint
	err = mgr.DeleteCheckpoint(session.id)
	assert.NoError(t, err)

	// Verify checkpoint is removed
	assert.NotContains(t, mgr.checkpoints, session.id)

	_, err = os.Stat(checkpointPath)
	assert.True(t, os.IsNotExist(err))
}

func TestListCheckpoints(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create and checkpoint multiple sessions
	session1 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session2 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session3 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	mgr.CheckpointSession(session1)
	mgr.CheckpointSession(session2)
	mgr.CheckpointSession(session3)

	checkpoints := mgr.ListCheckpoints()
	assert.Len(t, checkpoints, 3)
}

func TestPurgeOldCheckpoints(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create sessions
	oldSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	newSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	mgr.CheckpointSession(oldSession)
	mgr.CheckpointSession(newSession)

	// Mark old session as committed and backdated
	mgr.mu.Lock()
	mgr.checkpoints[oldSession.id].Committed = true
	mgr.checkpoints[oldSession.id].CheckpointTime = time.Now().Add(-2 * time.Hour)
	mgr.mu.Unlock()

	// Purge checkpoints older than 1 hour
	purged, err := mgr.PurgeOldCheckpoints(1 * time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, 1, purged)

	// Verify old checkpoint is removed
	assert.NotContains(t, mgr.checkpoints, oldSession.id)
	assert.Contains(t, mgr.checkpoints, newSession.id)
}

func TestGetRecoveryStats(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()

	// Create sessions with different states
	session1 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session2 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session3 := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	mgr.CheckpointSession(session1)
	mgr.CheckpointSession(session2)
	mgr.CheckpointSession(session3)

	// Mark sessions
	mgr.mu.Lock()
	mgr.checkpoints[session2.id].Committed = true
	mgr.checkpoints[session3.id].RolledBack = true
	mgr.mu.Unlock()

	stats := mgr.GetRecoveryStats()

	assert.Equal(t, 3, stats.TotalCheckpoints)
	assert.Equal(t, 1, stats.PendingSessions)
	assert.Equal(t, 1, stats.CommittedSessions)
	assert.Equal(t, 1, stats.RolledBackSessions)
	assert.True(t, stats.RecoveryEnabled)
}

func TestEnableDisableRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	mgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	assert.True(t, mgr.IsRecoveryEnabled())

	mgr.DisableRecovery()
	assert.False(t, mgr.IsRecoveryEnabled())

	mgr.EnableRecovery()
	assert.True(t, mgr.IsRecoveryEnabled())
}

func TestCheckpointIsolatedSession(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	recoveryMgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	// Create isolated session
	isolationMgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 10})
	tenant := &TenantContext{
		TenantID:  "tenant1",
		Namespace: "tenant1-ns",
	}
	isolationMgr.RegisterTenant(tenant)
	isolationMgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	ctx := context.Background()
	isolatedSession, _ := isolationMgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	// Checkpoint isolated session
	err := recoveryMgr.CheckpointIsolatedSession(isolatedSession)
	assert.NoError(t, err)

	// Verify tenant context was saved
	checkpoint := recoveryMgr.checkpoints[isolatedSession.id]
	assert.Equal(t, "tenant1", checkpoint.TenantID)
}

func TestRecoverableSessionCommit(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	ffRepo.On("Store", mock.Anything).Return(nil)

	recoveryMgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	baseSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	// Wrap with recovery
	recoverableSession := recoveryMgr.WrapWithRecovery(baseSession)

	// Create FlowFile
	recoverableSession.Create()

	// Commit
	err := recoverableSession.Commit()
	assert.NoError(t, err)

	// Verify checkpoint was deleted
	assert.NotContains(t, recoveryMgr.checkpoints, baseSession.id)
}

func TestRecoverableSessionRollback(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	recoveryMgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	baseSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)

	// Wrap with recovery
	recoverableSession := recoveryMgr.WrapWithRecovery(baseSession)

	// Create FlowFile
	recoverableSession.Create()

	// Rollback
	recoverableSession.Rollback()
	// Rollback returns no error

	// Verify checkpoint was deleted
	assert.NotContains(t, recoveryMgr.checkpoints, baseSession.id)
}

func TestAutoCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	recoveryMgr, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	baseSession := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	recoverableSession := recoveryMgr.WrapWithRecovery(baseSession)

	// Start auto-checkpointing every 100ms
	recoverableSession.AutoCheckpoint(ctx, 100*time.Millisecond)

	// Wait for at least one checkpoint
	time.Sleep(150 * time.Millisecond)

	// Verify checkpoint exists
	assert.Contains(t, recoveryMgr.checkpoints, baseSession.id)

	// Wait for context to finish
	<-ctx.Done()
}

func TestLoadCheckpoints(t *testing.T) {
	tmpDir := t.TempDir()

	config := RecoveryConfig{CheckpointDir: tmpDir}

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	logger := &types.MockLogger{}

	// Create first manager and checkpoint a session
	mgr1, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	ctx := context.Background()
	session := NewProcessSession(ffRepo, contentRepo, provRepo, logger, ctx, nil, nil, nil)
	session.Create()
	mgr1.CheckpointSession(session)

	// Create second manager - should load existing checkpoints
	mgr2, _ := NewSessionRecoveryManager(config, ffRepo, contentRepo, provRepo, logger)

	// Verify checkpoint was loaded
	assert.Contains(t, mgr2.checkpoints, session.id)
}
