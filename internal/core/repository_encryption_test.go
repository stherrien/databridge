package core

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewRepositoryEncryption(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "master-key-1",
	}

	encryption := NewRepositoryEncryption(config)

	assert.NotNil(t, encryption)
	assert.Equal(t, config.Enabled, encryption.config.Enabled)
	assert.Equal(t, config.Algorithm, encryption.config.Algorithm)
	assert.NotNil(t, encryption.keys)
}

func TestInitialize(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "master-key",
	}

	encryption := NewRepositoryEncryption(config)

	t.Run("Valid Master Key", func(t *testing.T) {
		masterKey := make([]byte, 32)
		for i := range masterKey {
			masterKey[i] = byte(i)
		}

		err := encryption.Initialize(masterKey)
		assert.NoError(t, err)

		// Verify key was stored
		key, err := encryption.GetActiveKey()
		assert.NoError(t, err)
		assert.Equal(t, "master-key", key.ID)
		assert.Equal(t, masterKey, key.Key)
		assert.True(t, key.Active)
	})

	t.Run("Invalid Key Size", func(t *testing.T) {
		invalidKey := make([]byte, 16) // Wrong size
		err := encryption.Initialize(invalidKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid master key size")
	})
}

func TestGenerateMasterKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:   true,
		Algorithm: "AES-256-GCM",
		KeySize:   32,
	}

	encryption := NewRepositoryEncryption(config)

	key1, err := encryption.GenerateMasterKey()
	assert.NoError(t, err)
	assert.Len(t, key1, 32)

	key2, err := encryption.GenerateMasterKey()
	assert.NoError(t, err)
	assert.Len(t, key2, 32)

	// Keys should be different (random)
	assert.NotEqual(t, key1, key2)
}

func TestEncryptDecryptData(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "test-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	plaintext := []byte("This is sensitive repository data that needs encryption")

	t.Run("Successful Encryption and Decryption", func(t *testing.T) {
		// Encrypt
		encrypted, err := encryption.EncryptData(plaintext)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)
		assert.NotEqual(t, plaintext, encrypted.Ciphertext)
		assert.NotEmpty(t, encrypted.Nonce)
		assert.Equal(t, "AES-256-GCM", encrypted.Algorithm)
		assert.Equal(t, "test-key", encrypted.KeyVersion)

		// Decrypt
		decrypted, err := encryption.DecryptData(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("Encrypt When Disabled", func(t *testing.T) {
		disabledEncryption := NewRepositoryEncryption(RepositoryEncryptionConfig{
			Enabled: false,
		})

		_, err := disabledEncryption.EncryptData(plaintext)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not enabled")
	})

	t.Run("Decrypt with Wrong Key", func(t *testing.T) {
		encrypted, _ := encryption.EncryptData(plaintext)

		// Change the key version to non-existent
		encrypted.KeyVersion = "nonexistent-key"

		_, err := encryption.DecryptData(encrypted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRotateKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "key-v1",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	plaintext := []byte("Data to re-encrypt")

	// Encrypt with original key
	encrypted1, err := encryption.EncryptData(plaintext)
	assert.NoError(t, err)
	assert.Equal(t, "key-v1", encrypted1.KeyVersion)

	// Rotate key
	newKey, err := encryption.RotateKey()
	assert.NoError(t, err)
	assert.NotNil(t, newKey)
	assert.True(t, newKey.Active)
	assert.Equal(t, 2, newKey.Version)

	// Verify old key is inactive
	oldKey := encryption.keys["key-v1"]
	assert.False(t, oldKey.Active)

	// Encrypt with new key
	encrypted2, err := encryption.EncryptData(plaintext)
	assert.NoError(t, err)
	assert.Equal(t, "key-v2", encrypted2.KeyVersion)

	// Both should decrypt correctly
	decrypted1, err := encryption.DecryptData(encrypted1)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted1)

	decrypted2, err := encryption.DecryptData(encrypted2)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted2)
}

func TestReEncryptWithNewKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "key-v1",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	plaintext := []byte("Data to re-encrypt")

	// Encrypt with first key
	encrypted, err := encryption.EncryptData(plaintext)
	assert.NoError(t, err)
	assert.Equal(t, "key-v1", encrypted.KeyVersion)

	// Create second key
	newKey, err := encryption.RotateKey()
	assert.NoError(t, err)

	// Re-encrypt with new key
	reencrypted, err := encryption.ReEncryptWithNewKey(encrypted, newKey.ID)
	assert.NoError(t, err)
	assert.Equal(t, newKey.ID, reencrypted.KeyVersion)

	// Decrypt with new key
	decrypted, err := encryption.DecryptData(reencrypted)
	assert.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptDecryptFlowFile(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "test-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	ff := types.NewFlowFile()
	content := []byte("FlowFile content to encrypt")

	t.Run("Encrypt FlowFile Content", func(t *testing.T) {
		encrypted, err := encryption.EncryptFlowFile(ff, content)
		assert.NoError(t, err)
		assert.NotNil(t, encrypted)

		// Verify FlowFile attributes
		assert.Equal(t, "true", ff.Attributes["encryption.enabled"])
		assert.Equal(t, "AES-256-GCM", ff.Attributes["encryption.algorithm"])
		assert.Equal(t, "test-key", ff.Attributes["encryption.keyVersion"])
		assert.NotEmpty(t, ff.Attributes["encryption.nonce"])
	})

	t.Run("Check if Encrypted", func(t *testing.T) {
		assert.True(t, encryption.IsFlowFileEncrypted(ff))

		unencryptedFF := types.NewFlowFile()
		assert.False(t, encryption.IsFlowFileEncrypted(unencryptedFF))
	})

	t.Run("Decrypt FlowFile Content", func(t *testing.T) {
		encrypted, _ := encryption.EncryptFlowFile(ff, content)

		decrypted, err := encryption.DecryptFlowFile(ff, encrypted)
		assert.NoError(t, err)
		assert.Equal(t, content, decrypted)
	})

	t.Run("Decrypt Non-Encrypted FlowFile", func(t *testing.T) {
		nonEncryptedFF := types.NewFlowFile()
		encrypted := &types.EncryptedData{
			Ciphertext: []byte("data"),
		}

		_, err := encryption.DecryptFlowFile(nonEncryptedFF, encrypted)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not encrypted")
	})
}

func TestListKeys(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "key-v1",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	// Initially one key
	keys := encryption.ListKeys()
	assert.Len(t, keys, 1)

	// Rotate twice
	encryption.RotateKey()
	encryption.RotateKey()

	keys = encryption.ListKeys()
	assert.Len(t, keys, 3)
}

func TestRemoveKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "key-v1",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	// Rotate to create a second key
	newKey, _ := encryption.RotateKey()

	t.Run("Remove Inactive Key", func(t *testing.T) {
		err := encryption.RemoveKey("key-v1")
		assert.NoError(t, err)

		keys := encryption.ListKeys()
		assert.Len(t, keys, 1)
	})

	t.Run("Cannot Remove Active Key", func(t *testing.T) {
		err := encryption.RemoveKey(newKey.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "active key")
	})

	t.Run("Remove Non-existent Key", func(t *testing.T) {
		err := encryption.RemoveKey("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestExportImportKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "export-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	encryption.Initialize(masterKey)

	password := "strong-password-123"

	t.Run("Export and Import Key", func(t *testing.T) {
		// Export key
		exported, err := encryption.ExportKey("export-key", password)
		assert.NoError(t, err)
		assert.NotEmpty(t, exported)

		// Create new encryption instance
		newEncryption := NewRepositoryEncryption(config)

		// Import key
		err = newEncryption.ImportKey("imported-key", exported, password)
		assert.NoError(t, err)

		// Verify imported key
		importedKey := newEncryption.keys["imported-key"]
		assert.NotNil(t, importedKey)
		assert.Equal(t, masterKey, importedKey.Key)
	})

	t.Run("Import with Wrong Password", func(t *testing.T) {
		exported, _ := encryption.ExportKey("export-key", password)

		newEncryption := NewRepositoryEncryption(config)
		err := newEncryption.ImportKey("imported-key", exported, "wrong-password")
		assert.Error(t, err)
	})

	t.Run("Export Non-existent Key", func(t *testing.T) {
		_, err := encryption.ExportKey("nonexistent", password)
		assert.Error(t, err)
	})
}

func TestDeriveKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:   true,
		Algorithm: "AES-256-GCM",
		KeySize:   32,
	}

	encryption := NewRepositoryEncryption(config)

	password := "test-password"
	salt := []byte("test-salt-123456")

	key1 := encryption.DeriveKey(password, salt)
	key2 := encryption.DeriveKey(password, salt)

	// Same password and salt should produce same key
	assert.Equal(t, key1, key2)
	assert.Len(t, key1, 32)

	// Different salt should produce different key
	differentSalt := []byte("different-salt-123")
	key3 := encryption.DeriveKey(password, differentSalt)
	assert.NotEqual(t, key1, key3)
}

func TestGetActiveKey(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "active-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	t.Run("Get Active Key", func(t *testing.T) {
		key, err := encryption.GetActiveKey()
		assert.NoError(t, err)
		assert.NotNil(t, key)
		assert.Equal(t, "active-key", key.ID)
		assert.True(t, key.Active)
	})

	t.Run("No Active Key", func(t *testing.T) {
		emptyEncryption := NewRepositoryEncryption(config)
		_, err := emptyEncryption.GetActiveKey()
		assert.Error(t, err)
	})
}

func TestGetSetConfig(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "test",
	}

	encryption := NewRepositoryEncryption(config)

	retrievedConfig := encryption.GetConfig()
	assert.Equal(t, config.Enabled, retrievedConfig.Enabled)
	assert.Equal(t, config.Algorithm, retrievedConfig.Algorithm)

	newConfig := RepositoryEncryptionConfig{
		Enabled:   false,
		Algorithm: "AES-128-GCM",
		KeySize:   16,
	}

	encryption.SetConfig(newConfig)
	retrievedConfig = encryption.GetConfig()
	assert.Equal(t, newConfig.Enabled, retrievedConfig.Enabled)
	assert.Equal(t, newConfig.Algorithm, retrievedConfig.Algorithm)
}

func TestGetMetrics(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "key-v1",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	// Rotate to create more keys
	encryption.RotateKey()
	encryption.RotateKey()

	metrics := encryption.GetMetrics()

	assert.Equal(t, 3, metrics.TotalKeys)
	assert.Equal(t, "key-v3", metrics.ActiveKeyID)
	assert.True(t, metrics.EncryptionEnabled)
	assert.Equal(t, "AES-256-GCM", metrics.Algorithm)
	assert.Equal(t, 32, metrics.KeySize)
}

func TestLargeDataEncryption(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "test-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	// Create large data (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Encrypt
	encrypted, err := encryption.EncryptData(largeData)
	assert.NoError(t, err)

	// Decrypt
	decrypted, err := encryption.DecryptData(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, largeData, decrypted)
}

func TestInvalidNonceSize(t *testing.T) {
	config := RepositoryEncryptionConfig{
		Enabled:     true,
		Algorithm:   "AES-256-GCM",
		KeySize:     32,
		MasterKeyID: "test-key",
	}

	encryption := NewRepositoryEncryption(config)
	masterKey := make([]byte, 32)
	encryption.Initialize(masterKey)

	encrypted := &types.EncryptedData{
		Algorithm:  "AES-256-GCM",
		Ciphertext: []byte("data"),
		Nonce:      []byte("invalid"), // Wrong size
		KeyVersion: "test-key",
	}

	_, err := encryption.DecryptData(encrypted)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid nonce size")
}
