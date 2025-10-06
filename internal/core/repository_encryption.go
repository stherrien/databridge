package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/pbkdf2"

	"github.com/shawntherrien/databridge/pkg/types"
)

const (
	trueStr = "true"
)

// RepositoryEncryptionConfig defines encryption settings for repository
type RepositoryEncryptionConfig struct {
	Enabled        bool
	Algorithm      string // "AES-256-GCM"
	KeySize        int    // 32 for AES-256
	MasterKeyID    string
	RotationPolicy KeyRotationPolicy
}

// KeyRotationPolicy defines key rotation rules
type KeyRotationPolicy struct {
	Enabled          bool
	RotationInterval int // Days between rotations
	MaxKeyAge        int // Maximum days a key can be used
	KeepOldKeys      int // Number of old keys to retain for decryption
}

// EncryptionKey represents a repository encryption key
type EncryptionKey struct {
	ID        string
	Key       []byte
	CreatedAt int64
	ExpiresAt int64
	Active    bool
	Version   int
}

// RepositoryEncryption handles data-at-rest encryption
type RepositoryEncryption struct {
	config   RepositoryEncryptionConfig
	keys     map[string]*EncryptionKey
	activeID string
	mu       sync.RWMutex
}

// NewRepositoryEncryption creates a new repository encryption manager
func NewRepositoryEncryption(config RepositoryEncryptionConfig) *RepositoryEncryption {
	return &RepositoryEncryption{
		config: config,
		keys:   make(map[string]*EncryptionKey),
	}
}

// Initialize sets up encryption with master key
func (e *RepositoryEncryption) Initialize(masterKey []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(masterKey) != e.config.KeySize {
		return fmt.Errorf("invalid master key size: expected %d, got %d", e.config.KeySize, len(masterKey))
	}

	// Create master encryption key
	keyID := e.config.MasterKeyID
	if keyID == "" {
		keyID = generateKeyID()
	}

	key := &EncryptionKey{
		ID:     keyID,
		Key:    masterKey,
		Active: true,
	}

	e.keys[keyID] = key
	e.activeID = keyID

	return nil
}

// GenerateMasterKey creates a new random master key
func (e *RepositoryEncryption) GenerateMasterKey() ([]byte, error) {
	key := make([]byte, e.config.KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}
	return key, nil
}

// DeriveKey derives an encryption key from password
func (e *RepositoryEncryption) DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, 100000, e.config.KeySize, sha256.New)
}

// EncryptData encrypts data using active encryption key
func (e *RepositoryEncryption) EncryptData(plaintext []byte) (*types.EncryptedData, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.config.Enabled {
		return nil, fmt.Errorf("repository encryption is not enabled")
	}

	key, exists := e.keys[e.activeID]
	if !exists {
		return nil, fmt.Errorf("active encryption key not found")
	}

	// Create cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &types.EncryptedData{
		Algorithm:  types.EncryptionAlgorithm(e.config.Algorithm),
		Ciphertext: ciphertext,
		Nonce:      nonce,
		KeyVersion: e.activeID,
	}, nil
}

// DecryptData decrypts data using specified key version
func (e *RepositoryEncryption) DecryptData(encrypted *types.EncryptedData) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.config.Enabled {
		return nil, fmt.Errorf("repository encryption is not enabled")
	}

	key, exists := e.keys[encrypted.KeyVersion]
	if !exists {
		return nil, fmt.Errorf("encryption key %s not found", encrypted.KeyVersion)
	}

	// Create cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Validate nonce size
	if len(encrypted.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: expected %d, got %d", gcm.NonceSize(), len(encrypted.Nonce))
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, encrypted.Nonce, encrypted.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// RotateKey creates a new encryption key and re-encrypts data
func (e *RepositoryEncryption) RotateKey() (*EncryptionKey, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Generate new key
	newKey := make([]byte, e.config.KeySize)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return nil, fmt.Errorf("failed to generate new key: %w", err)
	}

	// Get next version
	version := len(e.keys) + 1
	keyID := fmt.Sprintf("key-v%d", version)

	// Create new encryption key
	key := &EncryptionKey{
		ID:      keyID,
		Key:     newKey,
		Active:  true,
		Version: version,
	}

	// Deactivate old key
	if oldKey, exists := e.keys[e.activeID]; exists {
		oldKey.Active = false
	}

	// Store new key
	e.keys[keyID] = key
	e.activeID = keyID

	return key, nil
}

// GetActiveKey returns the current active encryption key
func (e *RepositoryEncryption) GetActiveKey() (*EncryptionKey, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key, exists := e.keys[e.activeID]
	if !exists {
		return nil, fmt.Errorf("active key not found")
	}

	return key, nil
}

// ListKeys returns all encryption keys
func (e *RepositoryEncryption) ListKeys() []*EncryptionKey {
	e.mu.RLock()
	defer e.mu.RUnlock()

	keys := make([]*EncryptionKey, 0, len(e.keys))
	for _, key := range e.keys {
		keys = append(keys, key)
	}

	return keys
}

// RemoveKey removes an old encryption key
func (e *RepositoryEncryption) RemoveKey(keyID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if keyID == e.activeID {
		return fmt.Errorf("cannot remove active key")
	}

	key, exists := e.keys[keyID]
	if !exists {
		return fmt.Errorf("key %s not found", keyID)
	}

	if key.Active {
		return fmt.Errorf("cannot remove active key")
	}

	delete(e.keys, keyID)
	return nil
}

// ReEncryptWithNewKey re-encrypts data with a different key
func (e *RepositoryEncryption) ReEncryptWithNewKey(encrypted *types.EncryptedData, newKeyID string) (*types.EncryptedData, error) {
	// Decrypt with old key
	plaintext, err := e.DecryptData(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt with old key: %w", err)
	}

	// Temporarily change active key
	e.mu.Lock()
	oldActiveID := e.activeID
	e.activeID = newKeyID
	e.mu.Unlock()

	// Encrypt with new key
	newEncrypted, err := e.EncryptData(plaintext)

	// Restore old active key
	e.mu.Lock()
	e.activeID = oldActiveID
	e.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to encrypt with new key: %w", err)
	}

	// Securely delete plaintext
	types.SecureDelete(plaintext)

	return newEncrypted, nil
}

// EncryptFlowFile encrypts a FlowFile's content
func (e *RepositoryEncryption) EncryptFlowFile(ff *types.FlowFile, content []byte) (*types.EncryptedData, error) {
	encrypted, err := e.EncryptData(content)
	if err != nil {
		return nil, err
	}

	// Add encryption metadata to FlowFile attributes
	ff.Attributes["encryption.enabled"] = trueStr
	ff.Attributes["encryption.algorithm"] = string(encrypted.Algorithm)
	ff.Attributes["encryption.keyVersion"] = encrypted.KeyVersion
	ff.Attributes["encryption.nonce"] = hex.EncodeToString(encrypted.Nonce)

	return encrypted, nil
}

// DecryptFlowFile decrypts a FlowFile's content
func (e *RepositoryEncryption) DecryptFlowFile(ff *types.FlowFile, encrypted *types.EncryptedData) ([]byte, error) {
	// Verify FlowFile is encrypted
	if ff.Attributes["encryption.enabled"] != trueStr {
		return nil, fmt.Errorf("FlowFile is not encrypted")
	}

	return e.DecryptData(encrypted)
}

// IsFlowFileEncrypted checks if a FlowFile has encrypted content
func (e *RepositoryEncryption) IsFlowFileEncrypted(ff *types.FlowFile) bool {
	return ff.Attributes["encryption.enabled"] == trueStr
}

// ExportKey exports an encryption key (encrypted)
func (e *RepositoryEncryption) ExportKey(keyID string, password string) ([]byte, error) {
	e.mu.RLock()
	key, exists := e.keys[keyID]
	e.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}

	// Generate salt
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from password
	derivedKey := e.DeriveKey(password, salt)

	// Create cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt key
	ciphertext := gcm.Seal(nil, nonce, key.Key, nil)

	// Combine salt + nonce + ciphertext
	exported := append(salt, nonce...)
	exported = append(exported, ciphertext...)

	return exported, nil
}

// ImportKey imports an encryption key (decrypts)
func (e *RepositoryEncryption) ImportKey(keyID string, encrypted []byte, password string) error {
	if len(encrypted) < 32+12 { // salt + nonce minimum
		return fmt.Errorf("invalid encrypted key data")
	}

	// Extract salt, nonce, and ciphertext
	salt := encrypted[:32]
	nonce := encrypted[32 : 32+12]
	ciphertext := encrypted[32+12:]

	// Derive decryption key from password
	derivedKey := e.DeriveKey(password, salt)

	// Create cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt key
	keyData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt key: %w", err)
	}

	// Store key
	e.mu.Lock()
	defer e.mu.Unlock()

	key := &EncryptionKey{
		ID:     keyID,
		Key:    keyData,
		Active: false,
	}

	e.keys[keyID] = key

	return nil
}

// GetConfig returns the current encryption configuration
func (e *RepositoryEncryption) GetConfig() RepositoryEncryptionConfig {
	return e.config
}

// SetConfig updates the encryption configuration
func (e *RepositoryEncryption) SetConfig(config RepositoryEncryptionConfig) {
	e.config = config
}

// Helper functions

// generateKeyID generates a unique key ID
func generateKeyID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// EncryptionMetrics provides encryption statistics
type EncryptionMetrics struct {
	TotalKeys         int
	ActiveKeyID       string
	EncryptionEnabled bool
	Algorithm         string
	KeySize           int
}

// GetMetrics returns encryption metrics
func (e *RepositoryEncryption) GetMetrics() EncryptionMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return EncryptionMetrics{
		TotalKeys:         len(e.keys),
		ActiveKeyID:       e.activeID,
		EncryptionEnabled: e.config.Enabled,
		Algorithm:         e.config.Algorithm,
		KeySize:           e.config.KeySize,
	}
}
