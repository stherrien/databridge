package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// SessionCheckpoint represents a saved session state
type SessionCheckpoint struct {
	SessionID      uuid.UUID                        `json:"sessionId"`
	TenantID       string                           `json:"tenantId,omitempty"`
	ProcessorID    uuid.UUID                        `json:"processorId"`
	CheckpointTime time.Time                        `json:"checkpointTime"`
	FlowFiles      map[uuid.UUID]*types.FlowFile    `json:"flowFiles"`
	Transfers      map[uuid.UUID]types.Relationship `json:"transfers"`
	Removals       []uuid.UUID                      `json:"removals"`
	Creations      []*types.FlowFile                `json:"creations"`
	Modifications  map[uuid.UUID]*types.FlowFile    `json:"modifications"`
	Committed      bool                             `json:"committed"`
	RolledBack     bool                             `json:"rolledBack"`
}

// SessionRecoveryManager handles crash recovery and session restoration
type SessionRecoveryManager struct {
	mu                 sync.RWMutex
	checkpointDir      string
	checkpoints        map[uuid.UUID]*SessionCheckpoint
	flowFileRepo       FlowFileRepository
	contentRepo        ContentRepository
	provenanceRepo     ProvenanceRepository
	logger             types.Logger
	recoveryEnabled    bool
	checkpointInterval time.Duration
}

// RecoveryConfig configures session recovery behavior
type RecoveryConfig struct {
	CheckpointDir      string
	CheckpointInterval time.Duration
	MaxCheckpointAge   time.Duration
	AutoRecovery       bool
}

// NewSessionRecoveryManager creates a new session recovery manager
func NewSessionRecoveryManager(
	config RecoveryConfig,
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
	logger types.Logger,
) (*SessionRecoveryManager, error) {
	// Create checkpoint directory
	if err := os.MkdirAll(config.CheckpointDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	mgr := &SessionRecoveryManager{
		checkpointDir:      config.CheckpointDir,
		checkpoints:        make(map[uuid.UUID]*SessionCheckpoint),
		flowFileRepo:       flowFileRepo,
		contentRepo:        contentRepo,
		provenanceRepo:     provenanceRepo,
		logger:             logger,
		recoveryEnabled:    true,
		checkpointInterval: config.CheckpointInterval,
	}

	// Load existing checkpoints
	if err := mgr.loadCheckpoints(); err != nil {
		logger.Warn("Failed to load checkpoints", "error", err)
	}

	return mgr, nil
}

// CheckpointSession saves session state for recovery
func (m *SessionRecoveryManager) CheckpointSession(session *ProcessSessionImpl) error {
	if !m.recoveryEnabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session.mu.RLock()
	checkpoint := &SessionCheckpoint{
		SessionID:      session.id,
		ProcessorID:    uuid.New(), // Would come from processor context
		CheckpointTime: time.Now(),
		FlowFiles:      make(map[uuid.UUID]*types.FlowFile),
		Transfers:      make(map[uuid.UUID]types.Relationship),
		Removals:       make([]uuid.UUID, len(session.removals)),
		Creations:      make([]*types.FlowFile, len(session.creations)),
		Modifications:  make(map[uuid.UUID]*types.FlowFile),
		Committed:      session.committed,
		RolledBack:     session.rolledBack,
	}

	// Deep copy session state
	for id, ff := range session.flowFiles {
		checkpoint.FlowFiles[id] = ff.Clone()
	}

	for id, rel := range session.transfers {
		checkpoint.Transfers[id] = rel
	}

	copy(checkpoint.Removals, session.removals)
	copy(checkpoint.Creations, session.creations)

	for id, ff := range session.modifications {
		checkpoint.Modifications[id] = ff.Clone()
	}

	session.mu.RUnlock()

	// Save checkpoint to disk
	if err := m.saveCheckpoint(checkpoint); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	m.checkpoints[checkpoint.SessionID] = checkpoint
	m.logger.Debug("Session checkpointed", "sessionId", checkpoint.SessionID)

	return nil
}

// CheckpointIsolatedSession checkpoints an isolated session with tenant context
func (m *SessionRecoveryManager) CheckpointIsolatedSession(session *IsolatedSession) error {
	if !m.recoveryEnabled {
		return nil
	}

	// Checkpoint base session
	err := m.CheckpointSession(session.ProcessSessionImpl)
	if err != nil {
		return err
	}

	// Add tenant context
	m.mu.Lock()
	checkpoint := m.checkpoints[session.id]
	if checkpoint != nil {
		checkpoint.TenantID = session.tenantID
	}
	m.mu.Unlock()

	return m.saveCheckpoint(checkpoint)
}

// RecoverSession recovers a session from checkpoint
func (m *SessionRecoveryManager) RecoverSession(
	sessionID uuid.UUID,
	ctx context.Context,
	logger types.Logger,
) (*ProcessSessionImpl, error) {
	m.mu.RLock()
	checkpoint, exists := m.checkpoints[sessionID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no checkpoint found for session %s", sessionID)
	}

	// Don't recover already committed/rolled back sessions
	if checkpoint.Committed || checkpoint.RolledBack {
		return nil, fmt.Errorf("session %s already finalized", sessionID)
	}

	// Create new session with recovered state
	session := &ProcessSessionImpl{
		id:             checkpoint.SessionID,
		flowFileRepo:   m.flowFileRepo,
		contentRepo:    m.contentRepo,
		provenanceRepo: m.provenanceRepo,
		logger:         logger,
		ctx:            ctx,
		flowFiles:      make(map[uuid.UUID]*types.FlowFile),
		transfers:      make(map[uuid.UUID]types.Relationship),
		removals:       make([]uuid.UUID, len(checkpoint.Removals)),
		creations:      make([]*types.FlowFile, len(checkpoint.Creations)),
		modifications:  make(map[uuid.UUID]*types.FlowFile),
		committed:      false,
		rolledBack:     false,
	}

	// Restore session state
	for id, ff := range checkpoint.FlowFiles {
		session.flowFiles[id] = ff.Clone()
	}

	for id, rel := range checkpoint.Transfers {
		session.transfers[id] = rel
	}

	copy(session.removals, checkpoint.Removals)
	copy(session.creations, checkpoint.Creations)

	for id, ff := range checkpoint.Modifications {
		session.modifications[id] = ff.Clone()
	}

	m.logger.Info("Session recovered", "sessionId", sessionID,
		"flowFiles", len(session.flowFiles),
		"creations", len(session.creations))

	return session, nil
}

// RecoverAllSessions recovers all incomplete sessions
func (m *SessionRecoveryManager) RecoverAllSessions(
	ctx context.Context,
	logger types.Logger,
) ([]*ProcessSessionImpl, error) {
	m.mu.RLock()
	checkpoints := make([]*SessionCheckpoint, 0, len(m.checkpoints))
	for _, cp := range m.checkpoints {
		if !cp.Committed && !cp.RolledBack {
			checkpoints = append(checkpoints, cp)
		}
	}
	m.mu.RUnlock()

	recovered := make([]*ProcessSessionImpl, 0, len(checkpoints))
	for _, cp := range checkpoints {
		session, err := m.RecoverSession(cp.SessionID, ctx, logger)
		if err != nil {
			logger.Warn("Failed to recover session",
				"sessionId", cp.SessionID,
				"error", err)
			continue
		}
		recovered = append(recovered, session)
	}

	m.logger.Info("Sessions recovered", "count", len(recovered))
	return recovered, nil
}

// DeleteCheckpoint removes a checkpoint after session completion
func (m *SessionRecoveryManager) DeleteCheckpoint(sessionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.checkpoints, sessionID)

	// Delete checkpoint file
	checkpointPath := filepath.Join(m.checkpointDir, fmt.Sprintf("%s.json", sessionID))
	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}

	return nil
}

// ListCheckpoints returns all active checkpoints
func (m *SessionRecoveryManager) ListCheckpoints() []*SessionCheckpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoints := make([]*SessionCheckpoint, 0, len(m.checkpoints))
	for _, cp := range m.checkpoints {
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints
}

// PurgeOldCheckpoints removes checkpoints older than specified duration
func (m *SessionRecoveryManager) PurgeOldCheckpoints(maxAge time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	purged := 0

	for id, cp := range m.checkpoints {
		if cp.CheckpointTime.Before(cutoff) && (cp.Committed || cp.RolledBack) {
			delete(m.checkpoints, id)

			checkpointPath := filepath.Join(m.checkpointDir, fmt.Sprintf("%s.json", id))
			if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
				m.logger.Warn("Failed to delete old checkpoint",
					"sessionId", id,
					"error", err)
			}

			purged++
		}
	}

	m.logger.Info("Purged old checkpoints", "count", purged)
	return purged, nil
}

// GetRecoveryStats returns recovery statistics
func (m *SessionRecoveryManager) GetRecoveryStats() RecoveryStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := RecoveryStats{
		TotalCheckpoints: len(m.checkpoints),
		RecoveryEnabled:  m.recoveryEnabled,
	}

	for _, cp := range m.checkpoints {
		if !cp.Committed && !cp.RolledBack {
			stats.PendingSessions++
		}
		if cp.Committed {
			stats.CommittedSessions++
		}
		if cp.RolledBack {
			stats.RolledBackSessions++
		}
	}

	return stats
}

// RecoveryStats provides recovery statistics
type RecoveryStats struct {
	TotalCheckpoints   int
	PendingSessions    int
	CommittedSessions  int
	RolledBackSessions int
	RecoveryEnabled    bool
}

// EnableRecovery enables session recovery
func (m *SessionRecoveryManager) EnableRecovery() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveryEnabled = true
}

// DisableRecovery disables session recovery
func (m *SessionRecoveryManager) DisableRecovery() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveryEnabled = false
}

// IsRecoveryEnabled returns whether recovery is enabled
func (m *SessionRecoveryManager) IsRecoveryEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recoveryEnabled
}

// Helper functions

// saveCheckpoint saves checkpoint to disk
func (m *SessionRecoveryManager) saveCheckpoint(checkpoint *SessionCheckpoint) error {
	checkpointPath := filepath.Join(m.checkpointDir, fmt.Sprintf("%s.json", checkpoint.SessionID))

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Write atomically
	tmpPath := checkpointPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	if err := os.Rename(tmpPath, checkpointPath); err != nil {
		_ = os.Remove(tmpPath) // Best effort cleanup
		return fmt.Errorf("failed to rename checkpoint: %w", err)
	}

	return nil
}

// loadCheckpoints loads all checkpoints from disk
func (m *SessionRecoveryManager) loadCheckpoints() error {
	entries, err := os.ReadDir(m.checkpointDir)
	if err != nil {
		return fmt.Errorf("failed to read checkpoint directory: %w", err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		checkpointPath := filepath.Join(m.checkpointDir, entry.Name())
		// #nosec G304 - checkpointPath is constructed from checkpoint directory and directory entry
		data, err := os.ReadFile(checkpointPath)
		if err != nil {
			m.logger.Warn("Failed to read checkpoint file",
				"file", entry.Name(),
				"error", err)
			continue
		}

		var checkpoint SessionCheckpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			m.logger.Warn("Failed to unmarshal checkpoint",
				"file", entry.Name(),
				"error", err)
			continue
		}

		m.checkpoints[checkpoint.SessionID] = &checkpoint
		loaded++
	}

	m.logger.Info("Loaded checkpoints", "count", loaded)
	return nil
}

// RecoverableSession wraps a session with automatic checkpointing
type RecoverableSession struct {
	*ProcessSessionImpl
	recoveryMgr *SessionRecoveryManager
}

// WrapWithRecovery wraps a session with recovery capabilities
func (m *SessionRecoveryManager) WrapWithRecovery(session *ProcessSessionImpl) *RecoverableSession {
	return &RecoverableSession{
		ProcessSessionImpl: session,
		recoveryMgr:        m,
	}
}

// Commit commits and removes checkpoint
func (s *RecoverableSession) Commit() error {
	// Checkpoint before commit
	if err := s.recoveryMgr.CheckpointSession(s.ProcessSessionImpl); err != nil {
		s.logger.Warn("Failed to checkpoint before commit", "error", err)
	}

	// Perform commit
	err := s.ProcessSessionImpl.Commit()
	if err != nil {
		return err
	}

	// Delete checkpoint on successful commit
	_ = s.recoveryMgr.DeleteCheckpoint(s.id) // Best effort deletion

	return nil
}

// Rollback rolls back and removes checkpoint
func (s *RecoverableSession) Rollback() {
	// Perform rollback
	s.ProcessSessionImpl.Rollback()

	// Delete checkpoint on successful rollback
	_ = s.recoveryMgr.DeleteCheckpoint(s.id) // Best effort deletion
}

// AutoCheckpoint creates a goroutine that periodically checkpoints the session
func (s *RecoverableSession) AutoCheckpoint(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if s.committed || s.rolledBack {
					return
				}
				if err := s.recoveryMgr.CheckpointSession(s.ProcessSessionImpl); err != nil {
					s.logger.Warn("Auto-checkpoint failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
