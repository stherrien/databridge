package security

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// UserRepository defines the interface for user storage
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*User, error)
}

// RoleRepository defines the interface for role storage
type RoleRepository interface {
	Create(ctx context.Context, role *Role) error
	GetByID(ctx context.Context, id string) (*Role, error)
	GetByName(ctx context.Context, name string) (*Role, error)
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Role, error)
	AssignToUser(ctx context.Context, userID, roleID string) error
	RemoveFromUser(ctx context.Context, userID, roleID string) error
	GetUserRoles(ctx context.Context, userID string) ([]*Role, error)
	GrantPermissionToUser(ctx context.Context, userID string, permission Permission) error
	RevokePermissionFromUser(ctx context.Context, userID string, permission Permission) error
}

// APIKeyRepository defines the interface for API key storage
type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *APIKey) error
	GetByID(ctx context.Context, id string) (*APIKey, error)
	GetByUserID(ctx context.Context, userID string) ([]*APIKey, error)
	Update(ctx context.Context, apiKey *APIKey) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]*APIKey, error)
}

// AuditRepository defines the interface for audit log storage
type AuditRepository interface {
	Create(ctx context.Context, event *AuditEvent) error
	GetByID(ctx context.Context, id string) (*AuditEvent, error)
	Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error)
	Delete(ctx context.Context, id string) error
}

// In-memory implementations for testing and simple deployments

// InMemoryUserRepository is an in-memory implementation of UserRepository
type InMemoryUserRepository struct {
	users      map[string]*User
	byEmail    map[string]string // email -> userID
	byUsername map[string]string // username -> userID
	mu         sync.RWMutex
}

// NewInMemoryUserRepository creates a new in-memory user repository
func NewInMemoryUserRepository() *InMemoryUserRepository {
	return &InMemoryUserRepository{
		users:      make(map[string]*User),
		byEmail:    make(map[string]string),
		byUsername: make(map[string]string),
	}
}

func (r *InMemoryUserRepository) Create(ctx context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.ID]; exists {
		return ErrUserExists
	}

	if _, exists := r.byUsername[user.Username]; exists {
		return ErrUserExists
	}

	if user.Email != "" {
		if _, exists := r.byEmail[user.Email]; exists {
			return ErrUserExists
		}
	}

	r.users[user.ID] = user
	r.byUsername[user.Username] = user.ID
	if user.Email != "" {
		r.byEmail[user.Email] = user.ID
	}

	return nil
}

func (r *InMemoryUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, exists := r.users[id]
	if !exists {
		return nil, ErrUserNotFound
	}

	return user, nil
}

func (r *InMemoryUserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID, exists := r.byUsername[username]
	if !exists {
		return nil, ErrUserNotFound
	}

	return r.users[userID], nil
}

func (r *InMemoryUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID, exists := r.byEmail[email]
	if !exists {
		return nil, ErrUserNotFound
	}

	return r.users[userID], nil
}

func (r *InMemoryUserRepository) Update(ctx context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.users[user.ID]; !exists {
		return ErrUserNotFound
	}

	user.UpdatedAt = time.Now()
	r.users[user.ID] = user
	r.byUsername[user.Username] = user.ID
	if user.Email != "" {
		r.byEmail[user.Email] = user.ID
	}

	return nil
}

func (r *InMemoryUserRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, exists := r.users[id]
	if !exists {
		return ErrUserNotFound
	}

	delete(r.users, id)
	delete(r.byUsername, user.Username)
	if user.Email != "" {
		delete(r.byEmail, user.Email)
	}

	return nil
}

func (r *InMemoryUserRepository) List(ctx context.Context) ([]*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]*User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, user)
	}

	return users, nil
}

// InMemoryRoleRepository is an in-memory implementation of RoleRepository
type InMemoryRoleRepository struct {
	roles     map[string]*Role
	byName    map[string]string       // roleName -> roleID
	userRoles map[string][]string     // userID -> roleIDs
	userPerms map[string][]Permission // userID -> permissions
	mu        sync.RWMutex
}

// NewInMemoryRoleRepository creates a new in-memory role repository
func NewInMemoryRoleRepository() *InMemoryRoleRepository {
	return &InMemoryRoleRepository{
		roles:     make(map[string]*Role),
		byName:    make(map[string]string),
		userRoles: make(map[string][]string),
		userPerms: make(map[string][]Permission),
	}
}

func (r *InMemoryRoleRepository) Create(ctx context.Context, role *Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.roles[role.ID]; exists {
		return ErrRoleExists
	}

	if _, exists := r.byName[role.Name]; exists {
		return ErrRoleExists
	}

	r.roles[role.ID] = role
	r.byName[role.Name] = role.ID

	return nil
}

func (r *InMemoryRoleRepository) GetByID(ctx context.Context, id string) (*Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, exists := r.roles[id]
	if !exists {
		return nil, ErrRoleNotFound
	}

	return role, nil
}

func (r *InMemoryRoleRepository) GetByName(ctx context.Context, name string) (*Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roleID, exists := r.byName[name]
	if !exists {
		return nil, ErrRoleNotFound
	}

	return r.roles[roleID], nil
}

func (r *InMemoryRoleRepository) Update(ctx context.Context, role *Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.roles[role.ID]; !exists {
		return ErrRoleNotFound
	}

	r.roles[role.ID] = role
	r.byName[role.Name] = role.ID

	return nil
}

func (r *InMemoryRoleRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	role, exists := r.roles[id]
	if !exists {
		return ErrRoleNotFound
	}

	delete(r.roles, id)
	delete(r.byName, role.Name)

	// Remove role from all users
	for userID, roleIDs := range r.userRoles {
		newRoles := make([]string, 0)
		for _, rid := range roleIDs {
			if rid != id {
				newRoles = append(newRoles, rid)
			}
		}
		r.userRoles[userID] = newRoles
	}

	return nil
}

func (r *InMemoryRoleRepository) List(ctx context.Context) ([]*Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]*Role, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, role)
	}

	return roles, nil
}

func (r *InMemoryRoleRepository) AssignToUser(ctx context.Context, userID, roleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.roles[roleID]; !exists {
		return ErrRoleNotFound
	}

	roles, ok := r.userRoles[userID]
	if !ok {
		r.userRoles[userID] = []string{roleID}
		return nil
	}

	// Check if already assigned
	for _, rid := range roles {
		if rid == roleID {
			return nil
		}
	}

	r.userRoles[userID] = append(roles, roleID)
	return nil
}

func (r *InMemoryRoleRepository) RemoveFromUser(ctx context.Context, userID, roleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	roles, ok := r.userRoles[userID]
	if !ok {
		return nil
	}

	newRoles := make([]string, 0)
	for _, rid := range roles {
		if rid != roleID {
			newRoles = append(newRoles, rid)
		}
	}

	r.userRoles[userID] = newRoles
	return nil
}

func (r *InMemoryRoleRepository) GetUserRoles(ctx context.Context, userID string) ([]*Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roleIDs, ok := r.userRoles[userID]
	if !ok {
		return []*Role{}, nil
	}

	roles := make([]*Role, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if role, exists := r.roles[roleID]; exists {
			roles = append(roles, role)
		}
	}

	return roles, nil
}

func (r *InMemoryRoleRepository) GrantPermissionToUser(ctx context.Context, userID string, permission Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	perms, ok := r.userPerms[userID]
	if !ok {
		r.userPerms[userID] = []Permission{permission}
		return nil
	}

	// Check if already granted
	for _, p := range perms {
		if p.Resource == permission.Resource && p.Action == permission.Action && p.Scope == permission.Scope {
			return nil
		}
	}

	r.userPerms[userID] = append(perms, permission)
	return nil
}

func (r *InMemoryRoleRepository) RevokePermissionFromUser(ctx context.Context, userID string, permission Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	perms, ok := r.userPerms[userID]
	if !ok {
		return nil
	}

	newPerms := make([]Permission, 0)
	for _, p := range perms {
		if !(p.Resource == permission.Resource && p.Action == permission.Action && p.Scope == permission.Scope) {
			newPerms = append(newPerms, p)
		}
	}

	r.userPerms[userID] = newPerms
	return nil
}

// InMemoryAPIKeyRepository is an in-memory implementation of APIKeyRepository
type InMemoryAPIKeyRepository struct {
	keys map[string]*APIKey
	mu   sync.RWMutex
}

// NewInMemoryAPIKeyRepository creates a new in-memory API key repository
func NewInMemoryAPIKeyRepository() *InMemoryAPIKeyRepository {
	return &InMemoryAPIKeyRepository{
		keys: make(map[string]*APIKey),
	}
}

func (r *InMemoryAPIKeyRepository) Create(ctx context.Context, apiKey *APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.keys[apiKey.ID]; exists {
		return errors.New("API key already exists")
	}

	r.keys[apiKey.ID] = apiKey
	return nil
}

func (r *InMemoryAPIKeyRepository) GetByID(ctx context.Context, id string) (*APIKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key, exists := r.keys[id]
	if !exists {
		return nil, ErrAPIKeyNotFound
	}

	return key, nil
}

func (r *InMemoryAPIKeyRepository) GetByUserID(ctx context.Context, userID string) ([]*APIKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]*APIKey, 0)
	for _, key := range r.keys {
		if key.UserID == userID {
			keys = append(keys, key)
		}
	}

	return keys, nil
}

func (r *InMemoryAPIKeyRepository) Update(ctx context.Context, apiKey *APIKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.keys[apiKey.ID]; !exists {
		return ErrAPIKeyNotFound
	}

	r.keys[apiKey.ID] = apiKey
	return nil
}

func (r *InMemoryAPIKeyRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.keys[id]; !exists {
		return ErrAPIKeyNotFound
	}

	delete(r.keys, id)
	return nil
}

func (r *InMemoryAPIKeyRepository) List(ctx context.Context) ([]*APIKey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]*APIKey, 0, len(r.keys))
	for _, key := range r.keys {
		keys = append(keys, key)
	}

	return keys, nil
}

// InMemoryAuditRepository is an in-memory implementation of AuditRepository
type InMemoryAuditRepository struct {
	events map[string]*AuditEvent
	mu     sync.RWMutex
}

// NewInMemoryAuditRepository creates a new in-memory audit repository
func NewInMemoryAuditRepository() *InMemoryAuditRepository {
	return &InMemoryAuditRepository{
		events: make(map[string]*AuditEvent),
	}
}

func (r *InMemoryAuditRepository) Create(ctx context.Context, event *AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if event.ID == "" {
		event.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}

	r.events[event.ID] = event
	return nil
}

func (r *InMemoryAuditRepository) GetByID(ctx context.Context, id string) (*AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	event, exists := r.events[id]
	if !exists {
		return nil, errors.New("audit event not found")
	}

	return event, nil
}

func (r *InMemoryAuditRepository) Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	events := make([]*AuditEvent, 0)
	for _, event := range r.events {
		if r.matchesFilter(event, filter) {
			events = append(events, event)
		}
	}

	// Apply limit and offset
	if filter.Offset > 0 && filter.Offset < len(events) {
		events = events[filter.Offset:]
	}

	if filter.Limit > 0 && filter.Limit < len(events) {
		events = events[:filter.Limit]
	}

	return events, nil
}

func (r *InMemoryAuditRepository) matchesFilter(event *AuditEvent, filter AuditFilter) bool {
	if filter.UserID != "" && event.UserID != filter.UserID {
		return false
	}

	if filter.Action != "" && event.Action != filter.Action {
		return false
	}

	if filter.Resource != "" && event.Resource != filter.Resource {
		return false
	}

	if filter.Result != "" && event.Result != filter.Result {
		return false
	}

	if filter.StartTime != nil && event.Timestamp.Before(*filter.StartTime) {
		return false
	}

	if filter.EndTime != nil && event.Timestamp.After(*filter.EndTime) {
		return false
	}

	return true
}

func (r *InMemoryAuditRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.events, id)
	return nil
}
