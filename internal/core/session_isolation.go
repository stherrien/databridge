package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shawntherrien/databridge/pkg/types"
)

// TenantContext represents a tenant's isolation context
type TenantContext struct {
	TenantID    string
	Name        string
	Namespace   string
	Metadata    map[string]string
	CreatedAt   time.Time
	ResourceQuota ResourceQuota
	Enabled     bool
}

// ResourceQuota defines resource limits for a tenant
type ResourceQuota struct {
	MaxSessions      int
	MaxFlowFiles     int
	MaxContentSize   int64
	MaxProcessors    int
	MaxConnections   int
	RateLimitPerSec  int
}

// IsolatedSession wraps ProcessSession with tenant isolation
type IsolatedSession struct {
	*ProcessSessionImpl
	tenantID    string
	namespace   string
	isolationMgr *SessionIsolationManager
}

// SessionIsolationManager manages multi-tenant session isolation
type SessionIsolationManager struct {
	mu                sync.RWMutex
	tenants           map[string]*TenantContext
	tenantSessions    map[string]map[uuid.UUID]*IsolatedSession
	tenantRepos       map[string]*TenantRepositories
	defaultQuota      ResourceQuota
	isolationEnabled  bool
}

// TenantRepositories holds isolated repositories for a tenant
type TenantRepositories struct {
	flowFileRepo   FlowFileRepository
	contentRepo    ContentRepository
	provenanceRepo ProvenanceRepository
}

// NewSessionIsolationManager creates a new session isolation manager
func NewSessionIsolationManager(defaultQuota ResourceQuota) *SessionIsolationManager {
	return &SessionIsolationManager{
		tenants:          make(map[string]*TenantContext),
		tenantSessions:   make(map[string]map[uuid.UUID]*IsolatedSession),
		tenantRepos:      make(map[string]*TenantRepositories),
		defaultQuota:     defaultQuota,
		isolationEnabled: true,
	}
}

// RegisterTenant registers a new tenant
func (m *SessionIsolationManager) RegisterTenant(tenant *TenantContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[tenant.TenantID]; exists {
		return fmt.Errorf("tenant %s already registered", tenant.TenantID)
	}

	// Apply default quota if not specified
	if tenant.ResourceQuota.MaxSessions == 0 {
		tenant.ResourceQuota = m.defaultQuota
	}

	tenant.CreatedAt = time.Now()
	tenant.Enabled = true

	m.tenants[tenant.TenantID] = tenant
	m.tenantSessions[tenant.TenantID] = make(map[uuid.UUID]*IsolatedSession)

	return nil
}

// UnregisterTenant removes a tenant
func (m *SessionIsolationManager) UnregisterTenant(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[tenantID]; !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	// Close all tenant sessions
	sessions := m.tenantSessions[tenantID]
	for _, session := range sessions {
		session.Rollback()
	}

	delete(m.tenants, tenantID)
	delete(m.tenantSessions, tenantID)
	delete(m.tenantRepos, tenantID)

	return nil
}

// GetTenant retrieves a tenant context
func (m *SessionIsolationManager) GetTenant(tenantID string) (*TenantContext, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	return tenant, nil
}

// ListTenants returns all registered tenants
func (m *SessionIsolationManager) ListTenants() []*TenantContext {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := make([]*TenantContext, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants
}

// SetTenantRepositories sets isolated repositories for a tenant
func (m *SessionIsolationManager) SetTenantRepositories(
	tenantID string,
	flowFileRepo FlowFileRepository,
	contentRepo ContentRepository,
	provenanceRepo ProvenanceRepository,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[tenantID]; !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	m.tenantRepos[tenantID] = &TenantRepositories{
		flowFileRepo:   flowFileRepo,
		contentRepo:    contentRepo,
		provenanceRepo: provenanceRepo,
	}

	return nil
}

// CreateIsolatedSession creates a new isolated session for a tenant
func (m *SessionIsolationManager) CreateIsolatedSession(
	tenantID string,
	logger types.Logger,
	ctx context.Context,
	inputQueues []*FlowFileQueue,
	outputConnections []*Connection,
) (*IsolatedSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify tenant exists
	tenant, exists := m.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	if !tenant.Enabled {
		return nil, fmt.Errorf("tenant %s is disabled", tenantID)
	}

	// Check session quota
	currentSessions := len(m.tenantSessions[tenantID])
	if currentSessions >= tenant.ResourceQuota.MaxSessions {
		return nil, fmt.Errorf("tenant %s has reached max sessions limit (%d)",
			tenantID, tenant.ResourceQuota.MaxSessions)
	}

	// Get tenant repositories
	repos, exists := m.tenantRepos[tenantID]
	if !exists {
		return nil, fmt.Errorf("no repositories configured for tenant %s", tenantID)
	}

	// Create base session with tenant's isolated repositories
	baseSession := NewProcessSession(
		repos.flowFileRepo,
		repos.contentRepo,
		repos.provenanceRepo,
		logger,
		ctx,
		inputQueues,
		outputConnections,
	)

	// Wrap with isolation
	isolatedSession := &IsolatedSession{
		ProcessSessionImpl: baseSession,
		tenantID:          tenantID,
		namespace:         tenant.Namespace,
		isolationMgr:      m,
	}

	// Track session
	m.tenantSessions[tenantID][baseSession.id] = isolatedSession

	return isolatedSession, nil
}

// CloseSession closes an isolated session
func (m *SessionIsolationManager) CloseSession(tenantID string, sessionID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessions, exists := m.tenantSessions[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	delete(sessions, sessionID)
	return nil
}

// GetTenantSessions returns all active sessions for a tenant
func (m *SessionIsolationManager) GetTenantSessions(tenantID string) ([]*IsolatedSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions, exists := m.tenantSessions[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	result := make([]*IsolatedSession, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, session)
	}

	return result, nil
}

// GetTenantStats returns statistics for a tenant
func (m *SessionIsolationManager) GetTenantStats(tenantID string) (*TenantStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}

	sessions := m.tenantSessions[tenantID]

	stats := &TenantStats{
		TenantID:       tenantID,
		ActiveSessions: len(sessions),
		Quota:          tenant.ResourceQuota,
	}

	// Calculate resource usage
	repos, exists := m.tenantRepos[tenantID]
	if exists && repos.flowFileRepo != nil {
		if count, err := repos.flowFileRepo.Count(); err == nil {
			stats.FlowFileCount = count
		}
	}

	return stats, nil
}

// TenantStats provides tenant resource usage statistics
type TenantStats struct {
	TenantID       string
	ActiveSessions int
	FlowFileCount  int
	ContentSize    int64
	Quota          ResourceQuota
}

// EnforceTenantQuota checks if operation is allowed under tenant quota
func (m *SessionIsolationManager) EnforceTenantQuota(tenantID string, operation string, count int) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	stats, err := m.GetTenantStats(tenantID)
	if err != nil {
		return err
	}

	switch operation {
	case "create_flowfile":
		if stats.FlowFileCount+count > tenant.ResourceQuota.MaxFlowFiles {
			return fmt.Errorf("tenant %s would exceed FlowFile quota (%d/%d)",
				tenantID, stats.FlowFileCount+count, tenant.ResourceQuota.MaxFlowFiles)
		}
	case "create_session":
		if stats.ActiveSessions+count > tenant.ResourceQuota.MaxSessions {
			return fmt.Errorf("tenant %s would exceed session quota (%d/%d)",
				tenantID, stats.ActiveSessions+count, tenant.ResourceQuota.MaxSessions)
		}
	}

	return nil
}

// UpdateTenantQuota updates a tenant's resource quota
func (m *SessionIsolationManager) UpdateTenantQuota(tenantID string, quota ResourceQuota) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	tenant.ResourceQuota = quota
	return nil
}

// EnableTenant enables a tenant
func (m *SessionIsolationManager) EnableTenant(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	tenant.Enabled = true
	return nil
}

// DisableTenant disables a tenant
func (m *SessionIsolationManager) DisableTenant(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, exists := m.tenants[tenantID]
	if !exists {
		return fmt.Errorf("tenant %s not found", tenantID)
	}

	tenant.Enabled = false

	// Close all active sessions
	sessions := m.tenantSessions[tenantID]
	for _, session := range sessions {
		session.Rollback()
	}

	return nil
}

// IsolatedSession methods

// GetTenantID returns the tenant ID for this session
func (s *IsolatedSession) GetTenantID() string {
	return s.tenantID
}

// GetNamespace returns the namespace for this session
func (s *IsolatedSession) GetNamespace() string {
	return s.namespace
}

// Commit commits the session with tenant context
func (s *IsolatedSession) Commit() error {
	// Check tenant quota before commit
	err := s.isolationMgr.EnforceTenantQuota(s.tenantID, "create_flowfile", len(s.creations))
	if err != nil {
		return fmt.Errorf("quota enforcement failed: %w", err)
	}

	// Call base commit
	err = s.ProcessSessionImpl.Commit()
	if err != nil {
		return err
	}

	// Close session from isolation manager
	s.isolationMgr.CloseSession(s.tenantID, s.id)

	return nil
}

// Rollback rolls back the session
func (s *IsolatedSession) Rollback() {
	s.ProcessSessionImpl.Rollback()

	// Close session from isolation manager
	s.isolationMgr.CloseSession(s.tenantID, s.id)
}

// ValidateTenantAccess validates that a FlowFile belongs to this tenant
func (s *IsolatedSession) ValidateTenantAccess(flowFile *types.FlowFile) error {
	// Check if FlowFile has tenant metadata
	if tenantID, exists := flowFile.Attributes["tenant.id"]; exists {
		if tenantID != s.tenantID {
			return fmt.Errorf("access denied: FlowFile belongs to tenant %s", tenantID)
		}
	} else {
		// Tag FlowFile with tenant ID
		flowFile.Attributes["tenant.id"] = s.tenantID
		flowFile.Attributes["tenant.namespace"] = s.namespace
	}

	return nil
}

// Create creates a new FlowFile with tenant tagging
func (s *IsolatedSession) Create() *types.FlowFile {
	flowFile := s.ProcessSessionImpl.Create()

	// Tag with tenant information
	flowFile.Attributes["tenant.id"] = s.tenantID
	flowFile.Attributes["tenant.namespace"] = s.namespace
	flowFile.Attributes["tenant.created_at"] = time.Now().Format(time.RFC3339)

	return flowFile
}

// Get retrieves a FlowFile and validates tenant access
func (s *IsolatedSession) Get() *types.FlowFile {
	flowFile := s.ProcessSessionImpl.Get()
	if flowFile == nil {
		return nil
	}

	// Validate tenant access
	if err := s.ValidateTenantAccess(flowFile); err != nil {
		// Return to queue if access denied
		s.logger.Warn("Tenant access denied", "flowFileId", flowFile.ID, "error", err)
		return nil
	}

	return flowFile
}
