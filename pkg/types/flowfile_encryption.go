package types

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	trueStr = "true"
)

// EncryptionAlgorithm represents supported encryption algorithms
type EncryptionAlgorithm string

const (
	AlgorithmAES256GCM EncryptionAlgorithm = "AES-256-GCM"
	AlgorithmAES128GCM EncryptionAlgorithm = "AES-128-GCM"
)

// EncryptionConfig holds encryption configuration
type EncryptionConfig struct {
	Algorithm  EncryptionAlgorithm
	KeySize    int // Key size in bytes (16 for AES-128, 32 for AES-256)
	Iterations int // PBKDF2 iterations
	SaltSize   int // Salt size in bytes
	NonceSize  int // Nonce size in bytes
}

// EncryptedData represents encrypted content with metadata
type EncryptedData struct {
	Algorithm  EncryptionAlgorithm `json:"algorithm"`
	Ciphertext []byte              `json:"ciphertext"`
	Nonce      []byte              `json:"nonce"`
	Salt       []byte              `json:"salt"`
	KeyVersion string              `json:"keyVersion,omitempty"`
}

// FlowFileEncryption provides encryption/decryption for FlowFile content
type FlowFileEncryption struct {
	config EncryptionConfig
	keys   map[string][]byte // Key ID -> Key material
}

// NewFlowFileEncryption creates a new encryption handler
func NewFlowFileEncryption() *FlowFileEncryption {
	return &FlowFileEncryption{
		config: EncryptionConfig{
			Algorithm:  AlgorithmAES256GCM,
			KeySize:    32, // 256 bits
			Iterations: 100000,
			SaltSize:   16,
			NonceSize:  12,
		},
		keys: make(map[string][]byte),
	}
}

// SetConfig sets the encryption configuration
func (e *FlowFileEncryption) SetConfig(config EncryptionConfig) {
	e.config = config
}

// AddKey registers an encryption key
func (e *FlowFileEncryption) AddKey(keyID string, key []byte) error {
	if len(key) != e.config.KeySize {
		return fmt.Errorf("key size must be %d bytes, got %d", e.config.KeySize, len(key))
	}
	e.keys[keyID] = key
	return nil
}

// DeriveKey derives an encryption key from a password
func (e *FlowFileEncryption) DeriveKey(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, e.config.Iterations, e.config.KeySize, sha256.New)
}

// GenerateSalt generates a random salt
func (e *FlowFileEncryption) GenerateSalt() ([]byte, error) {
	salt := make([]byte, e.config.SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// Encrypt encrypts data using the specified key
func (e *FlowFileEncryption) Encrypt(plaintext []byte, keyID string) (*EncryptedData, error) {
	// Get key
	key, exists := e.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", keyID)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
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

	return &EncryptedData{
		Algorithm:  e.config.Algorithm,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		KeyVersion: keyID,
	}, nil
}

// EncryptWithPassword encrypts data using a password
func (e *FlowFileEncryption) EncryptWithPassword(plaintext []byte, password string) (*EncryptedData, error) {
	// Generate salt
	salt, err := e.GenerateSalt()
	if err != nil {
		return nil, err
	}

	// Derive key from password
	key := e.DeriveKey(password, salt)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
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

	return &EncryptedData{
		Algorithm:  e.config.Algorithm,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Salt:       salt,
	}, nil
}

// Decrypt decrypts data using the specified key
func (e *FlowFileEncryption) Decrypt(encrypted *EncryptedData, keyID string) ([]byte, error) {
	// Get key
	key, exists := e.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", keyID)
	}

	return e.decryptWithKey(encrypted, key)
}

// DecryptWithPassword decrypts data using a password
func (e *FlowFileEncryption) DecryptWithPassword(encrypted *EncryptedData, password string) ([]byte, error) {
	if encrypted.Salt == nil {
		return nil, fmt.Errorf("salt is required for password-based decryption")
	}

	// Derive key from password
	key := e.DeriveKey(password, encrypted.Salt)

	return e.decryptWithKey(encrypted, key)
}

// decryptWithKey performs the actual decryption
func (e *FlowFileEncryption) decryptWithKey(encrypted *EncryptedData, key []byte) ([]byte, error) {
	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Verify nonce size
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

// EncryptFlowFileContent encrypts the content of a FlowFile
func (e *FlowFileEncryption) EncryptFlowFileContent(ff *FlowFile, content []byte, keyID string) error {
	// Encrypt content
	encrypted, err := e.Encrypt(content, keyID)
	if err != nil {
		return err
	}

	// Add encryption metadata to attributes
	ff.Attributes["encryption.enabled"] = trueStr
	ff.Attributes["encryption.algorithm"] = string(encrypted.Algorithm)
	ff.Attributes["encryption.keyVersion"] = encrypted.KeyVersion
	ff.Attributes["encryption.nonce"] = base64.StdEncoding.EncodeToString(encrypted.Nonce)

	return nil
}

// IsEncrypted checks if a FlowFile has encrypted content
func (e *FlowFileEncryption) IsEncrypted(ff *FlowFile) bool {
	return ff.Attributes["encryption.enabled"] == trueStr
}

// GetEncryptionMetadata extracts encryption metadata from FlowFile attributes
func (e *FlowFileEncryption) GetEncryptionMetadata(ff *FlowFile) (*EncryptedData, error) {
	if !e.IsEncrypted(ff) {
		return nil, fmt.Errorf("flowFile is not encrypted")
	}

	algorithm := ff.Attributes["encryption.algorithm"]
	keyVersion := ff.Attributes["encryption.keyVersion"]
	nonceB64 := ff.Attributes["encryption.nonce"]

	if algorithm == "" || nonceB64 == "" {
		return nil, fmt.Errorf("incomplete encryption metadata")
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce encoding: %w", err)
	}

	return &EncryptedData{
		Algorithm:  EncryptionAlgorithm(algorithm),
		Nonce:      nonce,
		KeyVersion: keyVersion,
	}, nil
}

// RotateKey re-encrypts data with a new key
func (e *FlowFileEncryption) RotateKey(encrypted *EncryptedData, oldKeyID, newKeyID string) (*EncryptedData, error) {
	// Decrypt with old key
	plaintext, err := e.Decrypt(encrypted, oldKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt with old key: %w", err)
	}

	// Encrypt with new key
	newEncrypted, err := e.Encrypt(plaintext, newKeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt with new key: %w", err)
	}

	return newEncrypted, nil
}

// GenerateKey generates a random encryption key
func GenerateEncryptionKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	return key, nil
}

// EncodeKey encodes a key to base64 for storage
func EncodeKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

// DecodeKey decodes a base64-encoded key
func DecodeKey(encoded string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	return key, nil
}

// SecureDelete overwrites sensitive data before deletion
func SecureDelete(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
