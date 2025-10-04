package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrInvalidKey        = errors.New("invalid encryption key")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrEncryptionFailed  = errors.New("encryption failed")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

const (
	// KeySize is the size of the AES key in bytes (256 bits)
	KeySize = 32
	// SaltSize is the size of the salt for key derivation
	SaltSize = 16
	// NonceSize is the size of the nonce for GCM
	NonceSize = 12
	// Iterations for PBKDF2
	PBKDF2Iterations = 100000
)

// Encryptor handles data encryption and decryption
type Encryptor struct {
	masterKey []byte
}

// NewEncryptor creates a new encryptor with a master key
func NewEncryptor(masterKey string) (*Encryptor, error) {
	if len(masterKey) < 16 {
		return nil, ErrInvalidKey
	}

	// Derive a key from the master key using SHA256
	hash := sha256.Sum256([]byte(masterKey))

	return &Encryptor{
		masterKey: hash[:],
	}, nil
}

// Encrypt encrypts data using AES-256-GCM
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncryptionFailed, err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM
func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.masterKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}

// EncryptString encrypts a string and returns base64-encoded ciphertext
func (e *Encryptor) EncryptString(plaintext string) (string, error) {
	ciphertext, err := e.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts base64-encoded ciphertext to a string
func (e *Encryptor) DecryptString(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: invalid base64", ErrInvalidCiphertext)
	}

	plaintext, err := e.Decrypt(data)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// EncryptedProperty stores encrypted processor properties
type EncryptedProperty struct {
	Value     string `json:"value"`
	Encrypted bool   `json:"encrypted"`
}

// NewEncryptedProperty creates a new encrypted property
func NewEncryptedProperty(value string, encrypted bool) EncryptedProperty {
	return EncryptedProperty{
		Value:     value,
		Encrypted: encrypted,
	}
}

// Decrypt decrypts the property value if encrypted
func (p *EncryptedProperty) Decrypt(encryptor *Encryptor) (string, error) {
	if !p.Encrypted {
		return p.Value, nil
	}

	return encryptor.DecryptString(p.Value)
}

// PropertyEncryptor handles encryption of processor properties
type PropertyEncryptor struct {
	encryptor       *Encryptor
	sensitiveFields []string
}

// NewPropertyEncryptor creates a new property encryptor
func NewPropertyEncryptor(encryptor *Encryptor, sensitiveFields []string) *PropertyEncryptor {
	return &PropertyEncryptor{
		encryptor:       encryptor,
		sensitiveFields: sensitiveFields,
	}
}

// DefaultSensitiveFields returns common sensitive field names
func DefaultSensitiveFields() []string {
	return []string{
		"password",
		"secret",
		"token",
		"api_key",
		"apikey",
		"private_key",
		"privatekey",
		"access_key",
		"accesskey",
		"client_secret",
		"clientsecret",
	}
}

// IsSensitive checks if a field name is sensitive
func (pe *PropertyEncryptor) IsSensitive(fieldName string) bool {
	lowerName := toLower(fieldName)
	for _, sensitive := range pe.sensitiveFields {
		if contains(lowerName, toLower(sensitive)) {
			return true
		}
	}
	return false
}

// EncryptProperties encrypts sensitive properties in a map
func (pe *PropertyEncryptor) EncryptProperties(properties map[string]string) (map[string]EncryptedProperty, error) {
	encrypted := make(map[string]EncryptedProperty)

	for key, value := range properties {
		if pe.IsSensitive(key) {
			encryptedValue, err := pe.encryptor.EncryptString(value)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt property %s: %w", key, err)
			}
			encrypted[key] = NewEncryptedProperty(encryptedValue, true)
		} else {
			encrypted[key] = NewEncryptedProperty(value, false)
		}
	}

	return encrypted, nil
}

// DecryptProperties decrypts encrypted properties back to a map
func (pe *PropertyEncryptor) DecryptProperties(properties map[string]EncryptedProperty) (map[string]string, error) {
	decrypted := make(map[string]string)

	for key, prop := range properties {
		value, err := prop.Decrypt(pe.encryptor)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt property %s: %w", key, err)
		}
		decrypted[key] = value
	}

	return decrypted, nil
}

// DeriveKey derives a key from a password using PBKDF2
func DeriveKey(password string, salt []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			panic(err)
		}
	}

	return pbkdf2.Key([]byte(password), salt, PBKDF2Iterations, KeySize, sha256.New)
}

// GenerateRandomKey generates a random encryption key
func GenerateRandomKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateRandomKeyString generates a random encryption key as base64 string
func GenerateRandomKeyString(size int) (string, error) {
	key, err := GenerateRandomKey(size)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Helper functions

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
