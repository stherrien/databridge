package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAPIKeyNotFound = errors.New("API key not found")
	ErrAPIKeyExpired  = errors.New("API key expired")
	ErrAPIKeyInvalid  = errors.New("API key invalid")
)

// APIKey represents a service API key
type APIKey struct {
	ID          string
	Key         string // Hashed for security
	Name        string
	Description string
	UserID      string
	Scopes      []string
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastUsedAt  *time.Time
	Enabled     bool
}

// IsExpired checks if the API key is expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// APIKeyManager manages API keys
type APIKeyManager struct {
	apiKeyRepo APIKeyRepository
	mu         sync.RWMutex
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager(apiKeyRepo APIKeyRepository) *APIKeyManager {
	return &APIKeyManager{
		apiKeyRepo: apiKeyRepo,
	}
}

// GenerateAPIKey generates a new API key for a user
func (m *APIKeyManager) GenerateAPIKey(ctx context.Context, userID, name, description string, scopes []string, expiresIn *time.Duration) (*APIKey, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate a random API key
	rawKey, err := generateRandomKey(32)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the key for storage
	hashedKey, err := hashAPIKey(rawKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash API key: %w", err)
	}

	now := time.Now()
	var expiresAt *time.Time
	if expiresIn != nil {
		expiry := now.Add(*expiresIn)
		expiresAt = &expiry
	}

	apiKey := &APIKey{
		ID:          fmt.Sprintf("key_%d", now.UnixNano()),
		Key:         hashedKey,
		Name:        name,
		Description: description,
		UserID:      userID,
		Scopes:      scopes,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		Enabled:     true,
	}

	if err := m.apiKeyRepo.Create(ctx, apiKey); err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	// Return the API key with the raw key (only time it's visible)
	return apiKey, rawKey, nil
}

// ValidateKey validates an API key and returns the associated key
func (m *APIKeyManager) ValidateKey(ctx context.Context, rawKey string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get all API keys and check each one
	// In a production system, you might want to optimize this with an index
	keys, err := m.apiKeyRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	for _, key := range keys {
		if !key.Enabled {
			continue
		}

		if key.IsExpired() {
			continue
		}

		// Verify the key
		if err := bcrypt.CompareHashAndPassword([]byte(key.Key), []byte(rawKey)); err == nil {
			return key, nil
		}
	}

	return nil, ErrAPIKeyInvalid
}

// GetAPIKey retrieves an API key by ID
func (m *APIKeyManager) GetAPIKey(ctx context.Context, keyID string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.apiKeyRepo.GetByID(ctx, keyID)
}

// GetUserAPIKeys retrieves all API keys for a user
func (m *APIKeyManager) GetUserAPIKeys(ctx context.Context, userID string) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.apiKeyRepo.GetByUserID(ctx, userID)
}

// ListAPIKeys lists all API keys
func (m *APIKeyManager) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.apiKeyRepo.List(ctx)
}

// UpdateAPIKey updates an API key
func (m *APIKeyManager) UpdateAPIKey(ctx context.Context, apiKey *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	apiKey.UpdatedAt = time.Now()
	return m.apiKeyRepo.Update(ctx, apiKey)
}

// RevokeAPIKey revokes an API key
func (m *APIKeyManager) RevokeAPIKey(ctx context.Context, keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, err := m.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return err
	}

	key.Enabled = false
	key.UpdatedAt = time.Now()

	return m.apiKeyRepo.Update(ctx, key)
}

// DeleteAPIKey deletes an API key
func (m *APIKeyManager) DeleteAPIKey(ctx context.Context, keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.apiKeyRepo.Delete(ctx, keyID)
}

// UpdateLastUsed updates the last used timestamp for an API key
func (m *APIKeyManager) UpdateLastUsed(ctx context.Context, keyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, err := m.apiKeyRepo.GetByID(ctx, keyID)
	if err != nil {
		return
	}

	now := time.Now()
	key.LastUsedAt = &now
	key.UpdatedAt = now

	// Fire and forget - don't block on this
	go func() {
		_ = m.apiKeyRepo.Update(ctx, key) // Best effort update
	}()
}

// HasScope checks if an API key has a specific scope
func (m *APIKeyManager) HasScope(apiKey *APIKey, scope string) bool {
	for _, s := range apiKey.Scopes {
		if s == "*" || s == scope {
			return true
		}
	}
	return false
}

// CleanupExpiredKeys removes expired API keys
func (m *APIKeyManager) CleanupExpiredKeys(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keys, err := m.apiKeyRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	now := time.Now()
	for _, key := range keys {
		if key.ExpiresAt != nil && now.After(*key.ExpiresAt) {
			// Delete expired keys
			if err := m.apiKeyRepo.Delete(ctx, key.ID); err != nil {
				// Log error but continue
				continue
			}
		}
	}

	return nil
}

// StartCleanupRoutine starts a goroutine that periodically cleans up expired keys
func (m *APIKeyManager) StartCleanupRoutine(ctx context.Context, interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = m.CleanupExpiredKeys(ctx) // Best effort cleanup
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return stop
}

// generateRandomKey generates a random API key
func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// hashAPIKey hashes an API key using bcrypt
func hashAPIKey(key string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
