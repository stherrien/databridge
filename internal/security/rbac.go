package security

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrRoleNotFound       = errors.New("role not found")
	ErrRoleExists         = errors.New("role already exists")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrInvalidPermission  = errors.New("invalid permission")
)

// Action defines permission actions
type Action string

const (
	ActionRead    Action = "read"
	ActionWrite   Action = "write"
	ActionExecute Action = "execute"
	ActionDelete  Action = "delete"
	ActionAdmin   Action = "admin"
)

// Permission represents a specific access right
type Permission struct {
	Resource string // e.g., "processor", "flow", "connection", "user", "role"
	Action   Action // read, write, execute, delete, admin
	Scope    string // optional: specific resource ID or "*" for all
}

// String returns a string representation of the permission
func (p Permission) String() string {
	if p.Scope != "" && p.Scope != "*" {
		return fmt.Sprintf("%s:%s:%s", p.Resource, p.Action, p.Scope)
	}
	return fmt.Sprintf("%s:%s", p.Resource, p.Action)
}

// Matches checks if this permission matches the requested permission
func (p Permission) Matches(requested Permission) bool {
	// Check resource
	if p.Resource != "*" && p.Resource != requested.Resource {
		return false
	}

	// Check action (admin grants all actions)
	if p.Action != ActionAdmin && p.Action != requested.Action {
		return false
	}

	// Check scope
	if p.Scope != "" && p.Scope != "*" && p.Scope != requested.Scope {
		return false
	}

	return true
}

// Role groups permissions
type Role struct {
	ID          string
	Name        string
	Description string
	Permissions []Permission
	CreatedAt   string
	UpdatedAt   string
}

// HasPermission checks if the role has a specific permission
func (r *Role) HasPermission(permission Permission) bool {
	for _, p := range r.Permissions {
		if p.Matches(permission) {
			return true
		}
	}
	return false
}

// Predefined role names
const (
	RoleAdmin     = "admin"
	RoleOperator  = "operator"
	RoleMonitor   = "monitor"
	RoleDeveloper = "developer"
)

// GetPredefinedRoles returns the predefined roles
func GetPredefinedRoles() []*Role {
	return []*Role{
		{
			ID:          "role-admin",
			Name:        RoleAdmin,
			Description: "Full access to everything",
			Permissions: []Permission{
				{Resource: "*", Action: ActionAdmin, Scope: "*"},
			},
		},
		{
			ID:          "role-operator",
			Name:        RoleOperator,
			Description: "Can manage flows and processors",
			Permissions: []Permission{
				{Resource: "flow", Action: ActionRead, Scope: "*"},
				{Resource: "flow", Action: ActionWrite, Scope: "*"},
				{Resource: "flow", Action: ActionExecute, Scope: "*"},
				{Resource: "flow", Action: ActionDelete, Scope: "*"},
				{Resource: "processor", Action: ActionRead, Scope: "*"},
				{Resource: "processor", Action: ActionWrite, Scope: "*"},
				{Resource: "processor", Action: ActionExecute, Scope: "*"},
				{Resource: "processor", Action: ActionDelete, Scope: "*"},
				{Resource: "connection", Action: ActionRead, Scope: "*"},
				{Resource: "connection", Action: ActionWrite, Scope: "*"},
				{Resource: "connection", Action: ActionDelete, Scope: "*"},
				{Resource: "monitoring", Action: ActionRead, Scope: "*"},
			},
		},
		{
			ID:          "role-monitor",
			Name:        RoleMonitor,
			Description: "Read-only access to monitoring",
			Permissions: []Permission{
				{Resource: "flow", Action: ActionRead, Scope: "*"},
				{Resource: "processor", Action: ActionRead, Scope: "*"},
				{Resource: "connection", Action: ActionRead, Scope: "*"},
				{Resource: "monitoring", Action: ActionRead, Scope: "*"},
				{Resource: "system", Action: ActionRead, Scope: "*"},
			},
		},
		{
			ID:          "role-developer",
			Name:        RoleDeveloper,
			Description: "Can create/test flows but not deploy to production",
			Permissions: []Permission{
				{Resource: "flow", Action: ActionRead, Scope: "*"},
				{Resource: "flow", Action: ActionWrite, Scope: "*"},
				{Resource: "processor", Action: ActionRead, Scope: "*"},
				{Resource: "processor", Action: ActionWrite, Scope: "*"},
				{Resource: "processor", Action: ActionExecute, Scope: "*"},
				{Resource: "connection", Action: ActionRead, Scope: "*"},
				{Resource: "connection", Action: ActionWrite, Scope: "*"},
				{Resource: "monitoring", Action: ActionRead, Scope: "*"},
			},
		},
	}
}

// RBACManager manages roles and permissions
type RBACManager struct {
	roleRepo  RoleRepository
	userRoles map[string][]string // userID -> roleIDs (in-memory cache)
	mu        sync.RWMutex
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(roleRepo RoleRepository) *RBACManager {
	return &RBACManager{
		roleRepo:  roleRepo,
		userRoles: make(map[string][]string),
	}
}

// Initialize initializes the RBAC manager with predefined roles
func (m *RBACManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create predefined roles if they don't exist
	for _, role := range GetPredefinedRoles() {
		existing, err := m.roleRepo.GetByName(ctx, role.Name)
		if err == nil && existing != nil {
			// Role already exists, skip
			continue
		}

		if err := m.roleRepo.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create predefined role %s: %w", role.Name, err)
		}
	}

	return nil
}

// CreateRole creates a new role
func (m *RBACManager) CreateRole(ctx context.Context, role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.roleRepo.Create(ctx, role); err != nil {
		return err
	}

	return nil
}

// GetRole retrieves a role by ID
func (m *RBACManager) GetRole(ctx context.Context, roleID string) (*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.roleRepo.GetByID(ctx, roleID)
}

// GetRoleByName retrieves a role by name
func (m *RBACManager) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.roleRepo.GetByName(ctx, name)
}

// UpdateRole updates an existing role
func (m *RBACManager) UpdateRole(ctx context.Context, role *Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.roleRepo.Update(ctx, role)
}

// DeleteRole deletes a role
func (m *RBACManager) DeleteRole(ctx context.Context, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if it's a predefined role
	role, err := m.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return err
	}

	if role.Name == RoleAdmin || role.Name == RoleOperator ||
	   role.Name == RoleMonitor || role.Name == RoleDeveloper {
		return errors.New("cannot delete predefined role")
	}

	return m.roleRepo.Delete(ctx, roleID)
}

// ListRoles lists all roles
func (m *RBACManager) ListRoles(ctx context.Context) ([]*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.roleRepo.List(ctx)
}

// AssignRole assigns a role to a user
func (m *RBACManager) AssignRole(ctx context.Context, userID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify role exists
	_, err := m.roleRepo.GetByID(ctx, roleID)
	if err != nil {
		return err
	}

	if err := m.roleRepo.AssignToUser(ctx, userID, roleID); err != nil {
		return err
	}

	// Update cache
	if roles, ok := m.userRoles[userID]; ok {
		// Check if role already assigned
		for _, rid := range roles {
			if rid == roleID {
				return nil
			}
		}
		m.userRoles[userID] = append(roles, roleID)
	} else {
		m.userRoles[userID] = []string{roleID}
	}

	return nil
}

// RemoveRole removes a role from a user
func (m *RBACManager) RemoveRole(ctx context.Context, userID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.roleRepo.RemoveFromUser(ctx, userID, roleID); err != nil {
		return err
	}

	// Update cache
	if roles, ok := m.userRoles[userID]; ok {
		newRoles := make([]string, 0, len(roles))
		for _, rid := range roles {
			if rid != roleID {
				newRoles = append(newRoles, rid)
			}
		}
		m.userRoles[userID] = newRoles
	}

	return nil
}

// GetUserRoles gets all roles for a user
func (m *RBACManager) GetUserRoles(ctx context.Context, userID string) ([]*Role, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.roleRepo.GetUserRoles(ctx, userID)
}

// HasPermission checks if a user has a specific permission
func (m *RBACManager) HasPermission(ctx context.Context, user *User, permission Permission) bool {
	if user == nil {
		return false
	}

	// Get user roles
	roles, err := m.GetUserRoles(ctx, user.ID)
	if err != nil {
		return false
	}

	// Check each role
	for _, role := range roles {
		if role.HasPermission(permission) {
			return true
		}
	}

	// Check user-specific permissions
	for _, p := range user.Permissions {
		if p.Matches(permission) {
			return true
		}
	}

	return false
}

// CheckPermission checks permission and returns error if denied
func (m *RBACManager) CheckPermission(ctx context.Context, user *User, permission Permission) error {
	if !m.HasPermission(ctx, user, permission) {
		return ErrPermissionDenied
	}
	return nil
}

// GrantPermission grants a permission to a user
func (m *RBACManager) GrantPermission(ctx context.Context, userID string, permission Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.roleRepo.GrantPermissionToUser(ctx, userID, permission)
}

// RevokePermission revokes a permission from a user
func (m *RBACManager) RevokePermission(ctx context.Context, userID string, permission Permission) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.roleRepo.RevokePermissionFromUser(ctx, userID, permission)
}

// IsAdmin checks if a user has admin role
func (m *RBACManager) IsAdmin(ctx context.Context, user *User) bool {
	return m.HasPermission(ctx, user, Permission{
		Resource: "*",
		Action:   ActionAdmin,
		Scope:    "*",
	})
}
