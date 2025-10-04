package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptor(t *testing.T) {
	masterKey := "test-master-key-must-be-at-least-16-chars"

	t.Run("Create encryptor", func(t *testing.T) {
		encryptor, err := NewEncryptor(masterKey)
		require.NoError(t, err)
		assert.NotNil(t, encryptor)
	})

	t.Run("Invalid master key", func(t *testing.T) {
		encryptor, err := NewEncryptor("short")
		assert.Error(t, err)
		assert.Nil(t, encryptor)
	})

	t.Run("Encrypt and decrypt data", func(t *testing.T) {
		encryptor, err := NewEncryptor(masterKey)
		require.NoError(t, err)

		plaintext := []byte("sensitive data")
		ciphertext, err := encryptor.Encrypt(plaintext)
		require.NoError(t, err)
		assert.NotEqual(t, plaintext, ciphertext)

		decrypted, err := encryptor.Decrypt(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("Encrypt and decrypt string", func(t *testing.T) {
		encryptor, err := NewEncryptor(masterKey)
		require.NoError(t, err)

		plaintext := "my secret password"
		ciphertext, err := encryptor.EncryptString(plaintext)
		require.NoError(t, err)
		assert.NotEqual(t, plaintext, ciphertext)

		decrypted, err := encryptor.DecryptString(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("Decrypt invalid ciphertext", func(t *testing.T) {
		encryptor, err := NewEncryptor(masterKey)
		require.NoError(t, err)

		decrypted, err := encryptor.Decrypt([]byte("invalid"))
		assert.Error(t, err)
		assert.Nil(t, decrypted)
	})

	t.Run("Different keys produce different ciphertexts", func(t *testing.T) {
		encryptor1, _ := NewEncryptor("key1-must-be-at-least-16-chars")
		encryptor2, _ := NewEncryptor("key2-must-be-at-least-16-chars")

		plaintext := []byte("test data")
		ciphertext1, _ := encryptor1.Encrypt(plaintext)
		ciphertext2, _ := encryptor2.Encrypt(plaintext)

		// Different keys should produce different ciphertexts
		assert.NotEqual(t, ciphertext1, ciphertext2)

		// Decrypt with wrong key should fail
		_, err := encryptor1.Decrypt(ciphertext2)
		assert.Error(t, err)
	})
}

func TestPropertyEncryptor(t *testing.T) {
	masterKey := "test-master-key-must-be-at-least-16-chars"
	encryptor, err := NewEncryptor(masterKey)
	require.NoError(t, err)

	sensitiveFields := DefaultSensitiveFields()
	propEncryptor := NewPropertyEncryptor(encryptor, sensitiveFields)

	t.Run("Identify sensitive fields", func(t *testing.T) {
		assert.True(t, propEncryptor.IsSensitive("password"))
		assert.True(t, propEncryptor.IsSensitive("api_key"))
		assert.True(t, propEncryptor.IsSensitive("secret"))
		assert.True(t, propEncryptor.IsSensitive("token"))
		assert.False(t, propEncryptor.IsSensitive("username"))
		assert.False(t, propEncryptor.IsSensitive("email"))
	})

	t.Run("Encrypt properties", func(t *testing.T) {
		properties := map[string]string{
			"username": "testuser",
			"password": "secret123",
			"api_key":  "key123",
			"url":      "http://example.com",
		}

		encrypted, err := propEncryptor.EncryptProperties(properties)
		require.NoError(t, err)

		// Non-sensitive fields should not be encrypted
		assert.False(t, encrypted["username"].Encrypted)
		assert.Equal(t, "testuser", encrypted["username"].Value)

		assert.False(t, encrypted["url"].Encrypted)
		assert.Equal(t, "http://example.com", encrypted["url"].Value)

		// Sensitive fields should be encrypted
		assert.True(t, encrypted["password"].Encrypted)
		assert.NotEqual(t, "secret123", encrypted["password"].Value)

		assert.True(t, encrypted["api_key"].Encrypted)
		assert.NotEqual(t, "key123", encrypted["api_key"].Value)
	})

	t.Run("Decrypt properties", func(t *testing.T) {
		properties := map[string]string{
			"username": "testuser",
			"password": "secret123",
			"api_key":  "key123",
		}

		encrypted, err := propEncryptor.EncryptProperties(properties)
		require.NoError(t, err)

		decrypted, err := propEncryptor.DecryptProperties(encrypted)
		require.NoError(t, err)

		assert.Equal(t, properties["username"], decrypted["username"])
		assert.Equal(t, properties["password"], decrypted["password"])
		assert.Equal(t, properties["api_key"], decrypted["api_key"])
	})
}

func TestGenerateRandomKey(t *testing.T) {
	t.Run("Generate random key", func(t *testing.T) {
		key, err := GenerateRandomKey(32)
		require.NoError(t, err)
		assert.Len(t, key, 32)
	})

	t.Run("Generate random key string", func(t *testing.T) {
		keyStr, err := GenerateRandomKeyString(32)
		require.NoError(t, err)
		assert.NotEmpty(t, keyStr)
	})

	t.Run("Keys are unique", func(t *testing.T) {
		key1, _ := GenerateRandomKey(32)
		key2, _ := GenerateRandomKey(32)
		assert.NotEqual(t, key1, key2)
	})
}
