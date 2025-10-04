package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFlowFileEncryption(t *testing.T) {
	encryption := NewFlowFileEncryption()
	assert.NotNil(t, encryption)
	assert.Equal(t, AlgorithmAES256GCM, encryption.config.Algorithm)
	assert.Equal(t, 32, encryption.config.KeySize)
	assert.NotNil(t, encryption.keys)
}

func TestGenerateAndEncodeKey(t *testing.T) {
	key, err := GenerateEncryptionKey(32)
	assert.NoError(t, err)
	assert.Len(t, key, 32)

	encoded := EncodeKey(key)
	assert.NotEmpty(t, encoded)

	decoded, err := DecodeKey(encoded)
	assert.NoError(t, err)
	assert.Equal(t, key, decoded)
}

func TestAddKey(t *testing.T) {
	encryption := NewFlowFileEncryption()

	t.Run("Valid Key", func(t *testing.T) {
		key, _ := GenerateEncryptionKey(32)
		err := encryption.AddKey("test-key", key)
		assert.NoError(t, err)
	})

	t.Run("Invalid Key Size", func(t *testing.T) {
		key, _ := GenerateEncryptionKey(16) // Wrong size
		err := encryption.AddKey("test-key", key)
		assert.Error(t, err)
	})
}

func TestEncryptDecrypt(t *testing.T) {
	encryption := NewFlowFileEncryption()
	key, _ := GenerateEncryptionKey(32)
	encryption.AddKey("test-key", key)

	plaintext := []byte("This is sensitive data that needs encryption")

	t.Run("Successful Encryption and Decryption", func(t *testing.T) {
		// Encrypt
		encrypted, err := encryption.Encrypt(plaintext, "test-key")
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.NotEqual(t, plaintext, encrypted.Ciphertext)
		assert.NotEmpty(t, encrypted.Nonce)
		assert.Equal(t, AlgorithmAES256GCM, encrypted.Algorithm)

		// Decrypt
		decrypted, err := encryption.Decrypt(encrypted, "test-key")
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("Encrypt with Non-existent Key", func(t *testing.T) {
		_, err := encryption.Encrypt(plaintext, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Decrypt with Wrong Key", func(t *testing.T) {
		encrypted, _ := encryption.Encrypt(plaintext, "test-key")

		wrongKey, _ := GenerateEncryptionKey(32)
		encryption.AddKey("wrong-key", wrongKey)

		_, err := encryption.Decrypt(encrypted, "wrong-key")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decryption failed")
	})
}

func TestPasswordBasedEncryption(t *testing.T) {
	encryption := NewFlowFileEncryption()
	password := "my-secret-password"
	plaintext := []byte("Sensitive data")

	t.Run("Encrypt and Decrypt with Password", func(t *testing.T) {
		// Encrypt
		encrypted, err := encryption.EncryptWithPassword(plaintext, password)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.NotEmpty(t, encrypted.Salt)
		assert.NotEmpty(t, encrypted.Nonce)

		// Decrypt
		decrypted, err := encryption.DecryptWithPassword(encrypted, password)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("Decrypt with Wrong Password", func(t *testing.T) {
		encrypted, _ := encryption.EncryptWithPassword(plaintext, password)

		_, err := encryption.DecryptWithPassword(encrypted, "wrong-password")
		assert.Error(t, err)
	})

	t.Run("Decrypt without Salt", func(t *testing.T) {
		encrypted := &EncryptedData{
			Ciphertext: []byte("data"),
			Nonce:      []byte("nonce"),
		}

		_, err := encryption.DecryptWithPassword(encrypted, password)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "salt is required")
	})
}

func TestDeriveKey(t *testing.T) {
	encryption := NewFlowFileEncryption()
	password := "test-password"
	salt, _ := encryption.GenerateSalt()

	key1 := encryption.DeriveKey(password, salt)
	key2 := encryption.DeriveKey(password, salt)

	// Same password and salt should produce same key
	assert.Equal(t, key1, key2)
	assert.Len(t, key1, encryption.config.KeySize)

	// Different salt should produce different key
	differentSalt, _ := encryption.GenerateSalt()
	key3 := encryption.DeriveKey(password, differentSalt)
	assert.NotEqual(t, key1, key3)
}

func TestGenerateSalt(t *testing.T) {
	encryption := NewFlowFileEncryption()

	salt1, err := encryption.GenerateSalt()
	assert.NoError(t, err)
	assert.Len(t, salt1, encryption.config.SaltSize)

	salt2, err := encryption.GenerateSalt()
	assert.NoError(t, err)
	assert.NotEqual(t, salt1, salt2) // Should be random
}

func TestFlowFileEncryption(t *testing.T) {
	encryption := NewFlowFileEncryption()
	key, _ := GenerateEncryptionKey(32)
	encryption.AddKey("test-key", key)

	ff := NewFlowFile()
	content := []byte("FlowFile content to encrypt")

	t.Run("Encrypt FlowFile Content", func(t *testing.T) {
		err := encryption.EncryptFlowFileContent(ff, content, "test-key")
		assert.NoError(t, err)

		assert.Equal(t, "true", ff.Attributes["encryption.enabled"])
		assert.Equal(t, string(AlgorithmAES256GCM), ff.Attributes["encryption.algorithm"])
		assert.Equal(t, "test-key", ff.Attributes["encryption.keyVersion"])
		assert.NotEmpty(t, ff.Attributes["encryption.nonce"])
	})

	t.Run("Check if Encrypted", func(t *testing.T) {
		assert.True(t, encryption.IsEncrypted(ff))

		unencryptedFF := NewFlowFile()
		assert.False(t, encryption.IsEncrypted(unencryptedFF))
	})

	t.Run("Get Encryption Metadata", func(t *testing.T) {
		metadata, err := encryption.GetEncryptionMetadata(ff)
		assert.NoError(t, err)
		assert.NotNil(t, metadata)
		assert.Equal(t, AlgorithmAES256GCM, metadata.Algorithm)
		assert.Equal(t, "test-key", metadata.KeyVersion)
		assert.NotEmpty(t, metadata.Nonce)
	})
}

func TestRotateKey(t *testing.T) {
	encryption := NewFlowFileEncryption()

	oldKey, _ := GenerateEncryptionKey(32)
	newKey, _ := GenerateEncryptionKey(32)

	encryption.AddKey("old-key", oldKey)
	encryption.AddKey("new-key", newKey)

	plaintext := []byte("Data to re-encrypt")

	// Encrypt with old key
	encrypted, err := encryption.Encrypt(plaintext, "old-key")
	assert.NoError(t, err)

	// Rotate to new key
	reencrypted, err := encryption.RotateKey(encrypted, "old-key", "new-key")
	assert.NoError(t, err)
	assert.NotNil(t, reencrypted)
	assert.Equal(t, "new-key", reencrypted.KeyVersion)

	// Decrypt with new key
	decrypted, err := encryption.Decrypt(reencrypted, "new-key")
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Should not decrypt with old key
	_, err = encryption.Decrypt(reencrypted, "old-key")
	assert.Error(t, err)
}

func TestSetConfig(t *testing.T) {
	encryption := NewFlowFileEncryption()

	customConfig := EncryptionConfig{
		Algorithm:  AlgorithmAES128GCM,
		KeySize:    16,
		Iterations: 50000,
		SaltSize:   32,
		NonceSize:  12,
	}

	encryption.SetConfig(customConfig)
	assert.Equal(t, AlgorithmAES128GCM, encryption.config.Algorithm)
	assert.Equal(t, 16, encryption.config.KeySize)
	assert.Equal(t, 50000, encryption.config.Iterations)
}

func TestSecureDelete(t *testing.T) {
	data := []byte("sensitive data to delete")
	original := string(data)

	SecureDelete(data)

	// All bytes should be zero
	for _, b := range data {
		assert.Equal(t, byte(0), b)
	}

	// Should not equal original
	assert.NotEqual(t, original, string(data))
}

func TestLargeDataEncryption(t *testing.T) {
	encryption := NewFlowFileEncryption()
	key, _ := GenerateEncryptionKey(32)
	encryption.AddKey("test-key", key)

	// Create large data (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Encrypt
	encrypted, err := encryption.Encrypt(largeData, "test-key")
	assert.NoError(t, err)

	// Decrypt
	decrypted, err := encryption.Decrypt(encrypted, "test-key")
	assert.NoError(t, err)
	assert.Equal(t, largeData, decrypted)
}

func TestInvalidNonceSize(t *testing.T) {
	encryption := NewFlowFileEncryption()
	key, _ := GenerateEncryptionKey(32)
	encryption.AddKey("test-key", key)

	encrypted := &EncryptedData{
		Algorithm:  AlgorithmAES256GCM,
		Ciphertext: []byte("data"),
		Nonce:      []byte("invalid"), // Wrong size
	}

	_, err := encryption.Decrypt(encrypted, "test-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nonce size")
}
