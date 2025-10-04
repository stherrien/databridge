package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionMatches(t *testing.T) {
	tests := []struct {
		name       string
		permission Permission
		requested  Permission
		matches    bool
	}{
		{
			name:       "Exact match",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "*"},
			requested:  Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			matches:    true,
		},
		{
			name:       "Wildcard resource",
			permission: Permission{Resource: "*", Action: ActionRead, Scope: "*"},
			requested:  Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			matches:    true,
		},
		{
			name:       "Admin action grants all",
			permission: Permission{Resource: "flow", Action: ActionAdmin, Scope: "*"},
			requested:  Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			matches:    true,
		},
		{
			name:       "Different resource",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "*"},
			requested:  Permission{Resource: "processor", Action: ActionRead, Scope: "*"},
			matches:    false,
		},
		{
			name:       "Different action",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "*"},
			requested:  Permission{Resource: "flow", Action: ActionWrite, Scope: "*"},
			matches:    false,
		},
		{
			name:       "Specific scope match",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			requested:  Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			matches:    true,
		},
		{
			name:       "Specific scope mismatch",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			requested:  Permission{Resource: "flow", Action: ActionRead, Scope: "flow2"},
			matches:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.permission.Matches(tt.requested)
			assert.Equal(t, tt.matches, result)
		})
	}
}

func TestRoleHasPermission(t *testing.T) {
	role := &Role{
		ID:   "role1",
		Name: "operator",
		Permissions: []Permission{
			{Resource: "flow", Action: ActionRead, Scope: "*"},
			{Resource: "flow", Action: ActionWrite, Scope: "*"},
			{Resource: "processor", Action: ActionRead, Scope: "*"},
		},
	}

	tests := []struct {
		name       string
		permission Permission
		expected   bool
	}{
		{
			name:       "Has permission",
			permission: Permission{Resource: "flow", Action: ActionRead, Scope: "flow1"},
			expected:   true,
		},
		{
			name:       "Does not have permission",
			permission: Permission{Resource: "flow", Action: ActionDelete, Scope: "flow1"},
			expected:   false,
		},
		{
			name:       "Has permission on different resource",
			permission: Permission{Resource: "processor", Action: ActionRead, Scope: "proc1"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := role.HasPermission(tt.permission)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRBACManager(t *testing.T) {
	ctx := context.Background()
	roleRepo := NewInMemoryRoleRepository()
	rbacManager := NewRBACManager(roleRepo)

	// Initialize with predefined roles
	err := rbacManager.Initialize(ctx)
	require.NoError(t, err)

	t.Run("List predefined roles", func(t *testing.T) {
		roles, err := rbacManager.ListRoles(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(roles), 4) // admin, operator, monitor, developer
	})

	t.Run("Get role by name", func(t *testing.T) {
		role, err := rbacManager.GetRoleByName(ctx, RoleAdmin)
		require.NoError(t, err)
		assert.Equal(t, RoleAdmin, role.Name)
		assert.NotEmpty(t, role.Permissions)
	})

	t.Run("Create custom role", func(t *testing.T) {
		customRole := &Role{
			ID:          "role_custom",
			Name:        "custom",
			Description: "Custom role",
			Permissions: []Permission{
				{Resource: "flow", Action: ActionRead, Scope: "*"},
			},
		}

		err := rbacManager.CreateRole(ctx, customRole)
		require.NoError(t, err)

		// Retrieve the role
		retrieved, err := rbacManager.GetRole(ctx, customRole.ID)
		require.NoError(t, err)
		assert.Equal(t, customRole.Name, retrieved.Name)
	})

	t.Run("Assign role to user", func(t *testing.T) {
		userID := "user1"
		adminRole, err := rbacManager.GetRoleByName(ctx, RoleAdmin)
		require.NoError(t, err)

		err = rbacManager.AssignRole(ctx, userID, adminRole.ID)
		require.NoError(t, err)

		// Get user roles
		roles, err := rbacManager.GetUserRoles(ctx, userID)
		require.NoError(t, err)
		assert.Len(t, roles, 1)
		assert.Equal(t, adminRole.ID, roles[0].ID)
	})

	t.Run("Check user permissions", func(t *testing.T) {
		userID := "user2"
		operatorRole, err := rbacManager.GetRoleByName(ctx, RoleOperator)
		require.NoError(t, err)

		err = rbacManager.AssignRole(ctx, userID, operatorRole.ID)
		require.NoError(t, err)

		user := &User{
			ID:       userID,
			Username: "operator",
			Roles:    []string{RoleOperator},
		}

		// Should have flow read permission
		hasPermission := rbacManager.HasPermission(ctx, user, Permission{
			Resource: "flow",
			Action:   ActionRead,
			Scope:    "*",
		})
		assert.True(t, hasPermission)

		// Should not have user admin permission
		hasPermission = rbacManager.HasPermission(ctx, user, Permission{
			Resource: "user",
			Action:   ActionAdmin,
			Scope:    "*",
		})
		assert.False(t, hasPermission)
	})

	t.Run("Admin has all permissions", func(t *testing.T) {
		userID := "user3"
		adminRole, err := rbacManager.GetRoleByName(ctx, RoleAdmin)
		require.NoError(t, err)

		err = rbacManager.AssignRole(ctx, userID, adminRole.ID)
		require.NoError(t, err)

		user := &User{
			ID:       userID,
			Username: "admin",
			Roles:    []string{RoleAdmin},
		}

		// Admin should have any permission
		hasPermission := rbacManager.HasPermission(ctx, user, Permission{
			Resource: "anything",
			Action:   ActionDelete,
			Scope:    "any",
		})
		assert.True(t, hasPermission)

		// IsAdmin check
		isAdmin := rbacManager.IsAdmin(ctx, user)
		assert.True(t, isAdmin)
	})

	t.Run("Cannot delete predefined role", func(t *testing.T) {
		adminRole, err := rbacManager.GetRoleByName(ctx, RoleAdmin)
		require.NoError(t, err)

		err = rbacManager.DeleteRole(ctx, adminRole.ID)
		assert.Error(t, err)
	})
}

func TestGetPredefinedRoles(t *testing.T) {
	roles := GetPredefinedRoles()

	assert.GreaterOrEqual(t, len(roles), 4)

	roleNames := make(map[string]bool)
	for _, role := range roles {
		roleNames[role.Name] = true
		assert.NotEmpty(t, role.ID)
		assert.NotEmpty(t, role.Description)
	}

	// Check all predefined roles exist
	assert.True(t, roleNames[RoleAdmin])
	assert.True(t, roleNames[RoleOperator])
	assert.True(t, roleNames[RoleMonitor])
	assert.True(t, roleNames[RoleDeveloper])
}
