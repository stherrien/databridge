package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewSessionIsolationManager(t *testing.T) {
	quota := ResourceQuota{
		MaxSessions:    10,
		MaxFlowFiles:   1000,
		MaxContentSize: 1024 * 1024 * 1024,
	}

	mgr := NewSessionIsolationManager(quota)

	assert.NotNil(t, mgr)
	assert.Equal(t, quota, mgr.defaultQuota)
	assert.True(t, mgr.isolationEnabled)
	assert.NotNil(t, mgr.tenants)
}

func TestRegisterTenant(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	t.Run("Register New Tenant", func(t *testing.T) {
		tenant := &TenantContext{
			TenantID:  "tenant1",
			Name:      "Test Tenant",
			Namespace: "tenant1-ns",
			Metadata:  map[string]string{"env": "test"},
		}

		err := mgr.RegisterTenant(tenant)
		assert.NoError(t, err)
		assert.True(t, tenant.Enabled)
		assert.NotZero(t, tenant.CreatedAt)
	})

	t.Run("Register Duplicate Tenant", func(t *testing.T) {
		tenant := &TenantContext{
			TenantID: "tenant1",
			Name:     "Duplicate",
		}

		err := mgr.RegisterTenant(tenant)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestUnregisterTenant(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	tenant := &TenantContext{
		TenantID:  "tenant1",
		Name:      "Test Tenant",
		Namespace: "tenant1-ns",
	}

	mgr.RegisterTenant(tenant)

	t.Run("Unregister Existing Tenant", func(t *testing.T) {
		err := mgr.UnregisterTenant("tenant1")
		assert.NoError(t, err)

		_, err = mgr.GetTenant("tenant1")
		assert.Error(t, err)
	})

	t.Run("Unregister Non-existent Tenant", func(t *testing.T) {
		err := mgr.UnregisterTenant("nonexistent")
		assert.Error(t, err)
	})
}

func TestGetTenant(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	tenant := &TenantContext{
		TenantID:  "tenant1",
		Name:      "Test Tenant",
		Namespace: "tenant1-ns",
	}

	mgr.RegisterTenant(tenant)

	t.Run("Get Existing Tenant", func(t *testing.T) {
		retrieved, err := mgr.GetTenant("tenant1")
		assert.NoError(t, err)
		assert.Equal(t, "tenant1", retrieved.TenantID)
		assert.Equal(t, "Test Tenant", retrieved.Name)
	})

	t.Run("Get Non-existent Tenant", func(t *testing.T) {
		_, err := mgr.GetTenant("nonexistent")
		assert.Error(t, err)
	})
}

func TestListTenants(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	mgr.RegisterTenant(&TenantContext{TenantID: "tenant1", Name: "Tenant 1"})
	mgr.RegisterTenant(&TenantContext{TenantID: "tenant2", Name: "Tenant 2"})
	mgr.RegisterTenant(&TenantContext{TenantID: "tenant3", Name: "Tenant 3"})

	tenants := mgr.ListTenants()
	assert.Len(t, tenants, 3)

	ids := make([]string, len(tenants))
	for i, t := range tenants {
		ids[i] = t.TenantID
	}

	assert.Contains(t, ids, "tenant1")
	assert.Contains(t, ids, "tenant2")
	assert.Contains(t, ids, "tenant3")
}

func TestSetTenantRepositories(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	t.Run("Set Repositories for Existing Tenant", func(t *testing.T) {
		err := mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)
		assert.NoError(t, err)
	})

	t.Run("Set Repositories for Non-existent Tenant", func(t *testing.T) {
		err := mgr.SetTenantRepositories("nonexistent", ffRepo, contentRepo, provRepo)
		assert.Error(t, err)
	})
}

func TestCreateIsolatedSession(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{
		MaxSessions:  5,
		MaxFlowFiles: 100,
	})

	tenant := &TenantContext{
		TenantID:  "tenant1",
		Name:      "Test Tenant",
		Namespace: "tenant1-ns",
	}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	t.Run("Create Session for Valid Tenant", func(t *testing.T) {
		session, err := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, session)
		assert.Equal(t, "tenant1", session.GetTenantID())
		assert.Equal(t, "tenant1-ns", session.GetNamespace())
	})

	t.Run("Create Session for Non-existent Tenant", func(t *testing.T) {
		_, err := mgr.CreateIsolatedSession("nonexistent", logger, ctx, nil, nil)
		assert.Error(t, err)
	})

	t.Run("Create Session Exceeds Quota", func(t *testing.T) {
		// Create max sessions
		for i := 0; i < 5; i++ {
			mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
		}

		// Try to create one more
		_, err := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max sessions limit")
	})
}

func TestGetTenantSessions(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 10})

	tenant := &TenantContext{TenantID: "tenant1", Namespace: "ns1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	// Create multiple sessions
	mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
	mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
	mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	sessions, err := mgr.GetTenantSessions("tenant1")
	assert.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestGetTenantStats(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{
		MaxSessions:  10,
		MaxFlowFiles: 100,
	})

	tenant := &TenantContext{TenantID: "tenant1", Namespace: "ns1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	ffRepo.On("Count").Return(50, nil)

	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	// Create sessions
	mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
	mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	stats, err := mgr.GetTenantStats("tenant1")
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, "tenant1", stats.TenantID)
	assert.Equal(t, 2, stats.ActiveSessions)
	assert.Equal(t, 50, stats.FlowFileCount)
}

func TestEnforceTenantQuota(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{
		MaxSessions:  5,
		MaxFlowFiles: 100,
	})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	ffRepo.On("Count").Return(90, nil)

	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	t.Run("Within Quota", func(t *testing.T) {
		err := mgr.EnforceTenantQuota("tenant1", "create_flowfile", 5)
		assert.NoError(t, err)
	})

	t.Run("Exceeds Quota", func(t *testing.T) {
		err := mgr.EnforceTenantQuota("tenant1", "create_flowfile", 15)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceed FlowFile quota")
	})
}

func TestUpdateTenantQuota(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 5})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	newQuota := ResourceQuota{
		MaxSessions:  20,
		MaxFlowFiles: 500,
	}

	err := mgr.UpdateTenantQuota("tenant1", newQuota)
	assert.NoError(t, err)

	retrieved, _ := mgr.GetTenant("tenant1")
	assert.Equal(t, 20, retrieved.ResourceQuota.MaxSessions)
	assert.Equal(t, 500, retrieved.ResourceQuota.MaxFlowFiles)
}

func TestEnableDisableTenant(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	t.Run("Disable Tenant", func(t *testing.T) {
		err := mgr.DisableTenant("tenant1")
		assert.NoError(t, err)

		retrieved, _ := mgr.GetTenant("tenant1")
		assert.False(t, retrieved.Enabled)
	})

	t.Run("Enable Tenant", func(t *testing.T) {
		err := mgr.EnableTenant("tenant1")
		assert.NoError(t, err)

		retrieved, _ := mgr.GetTenant("tenant1")
		assert.True(t, retrieved.Enabled)
	})

	t.Run("Create Session for Disabled Tenant", func(t *testing.T) {
		mgr.DisableTenant("tenant1")

		ffRepo := new(MockFlowFileRepo)
		contentRepo := new(MockContentRepo)
		provRepo := new(MockProvenanceRepo)
		mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

		logger := &types.MockLogger{}
		ctx := context.Background()

		_, err := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disabled")
	})
}

func TestIsolatedSessionCreate(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 10})

	tenant := &TenantContext{
		TenantID:  "tenant1",
		Namespace: "tenant1-ns",
	}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	session, _ := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	flowFile := session.Create()

	assert.NotNil(t, flowFile)
	assert.Equal(t, "tenant1", flowFile.Attributes["tenant.id"])
	assert.Equal(t, "tenant1-ns", flowFile.Attributes["tenant.namespace"])
	assert.NotEmpty(t, flowFile.Attributes["tenant.created_at"])
}

func TestValidateTenantAccess(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 10})

	tenant := &TenantContext{
		TenantID:  "tenant1",
		Namespace: "tenant1-ns",
	}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	session, _ := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	t.Run("Access Own FlowFile", func(t *testing.T) {
		ff := types.NewFlowFile()
		ff.Attributes["tenant.id"] = "tenant1"

		err := session.ValidateTenantAccess(ff)
		assert.NoError(t, err)
	})

	t.Run("Access Other Tenant FlowFile", func(t *testing.T) {
		ff := types.NewFlowFile()
		ff.Attributes["tenant.id"] = "tenant2"

		err := session.ValidateTenantAccess(ff)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
	})

	t.Run("Auto-Tag Untagged FlowFile", func(t *testing.T) {
		ff := types.NewFlowFile()

		err := session.ValidateTenantAccess(ff)
		assert.NoError(t, err)
		assert.Equal(t, "tenant1", ff.Attributes["tenant.id"])
		assert.Equal(t, "tenant1-ns", ff.Attributes["tenant.namespace"])
	})
}

func TestIsolatedSessionCommit(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{
		MaxSessions:  10,
		MaxFlowFiles: 100,
	})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)

	ffRepo.On("Count").Return(50, nil)
	ffRepo.On("Store", mock.Anything).Return(nil)

	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	session, _ := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	// Create a FlowFile
	session.Create()

	// Commit should enforce quota
	err := session.Commit()
	assert.NoError(t, err)

	// Session should be removed from tenant sessions
	sessions, _ := mgr.GetTenantSessions("tenant1")
	assert.Len(t, sessions, 0)
}

func TestCloseSession(t *testing.T) {
	mgr := NewSessionIsolationManager(ResourceQuota{MaxSessions: 10})

	tenant := &TenantContext{TenantID: "tenant1"}
	mgr.RegisterTenant(tenant)

	ffRepo := new(MockFlowFileRepo)
	contentRepo := new(MockContentRepo)
	provRepo := new(MockProvenanceRepo)
	mgr.SetTenantRepositories("tenant1", ffRepo, contentRepo, provRepo)

	logger := &types.MockLogger{}
	ctx := context.Background()

	session, _ := mgr.CreateIsolatedSession("tenant1", logger, ctx, nil, nil)

	err := mgr.CloseSession("tenant1", session.id)
	assert.NoError(t, err)

	sessions, _ := mgr.GetTenantSessions("tenant1")
	assert.Len(t, sessions, 0)
}
