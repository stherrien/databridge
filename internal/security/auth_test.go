package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	password := "testPassword123!"

	hash, err := HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	// Verify password
	err = VerifyPassword(password, hash)
	assert.NoError(t, err)

	// Wrong password should fail
	err = VerifyPassword("wrongPassword", hash)
	assert.Error(t, err)
}

func TestBasicAuthProvider(t *testing.T) {
	ctx := context.Background()
	userRepo := NewInMemoryUserRepository()

	// Create test user
	passwordHash, err := HashPassword("testPass123")
	require.NoError(t, err)

	testUser := &User{
		ID:           "user1",
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: passwordHash,
		Enabled:      true,
	}
	err = userRepo.Create(ctx, testUser)
	require.NoError(t, err)

	provider := NewBasicAuthProvider(userRepo)

	t.Run("Successful authentication", func(t *testing.T) {
		creds := Credentials{
			Type:     AuthTypeBasic,
			Username: "testuser",
			Password: "testPass123",
		}

		user, authErr := provider.Authenticate(ctx, creds)
		require.NoError(t, authErr)
		assert.Equal(t, testUser.ID, user.ID)
		assert.Equal(t, testUser.Username, user.Username)
	})

	t.Run("Wrong password", func(t *testing.T) {
		creds := Credentials{
			Type:     AuthTypeBasic,
			Username: "testuser",
			Password: "wrongPassword",
		}

		user, authErr := provider.Authenticate(ctx, creds)
		assert.Error(t, authErr)
		assert.Nil(t, user)
		assert.Equal(t, ErrInvalidCredentials, authErr)
	})

	t.Run("Non-existent user", func(t *testing.T) {
		creds := Credentials{
			Type:     AuthTypeBasic,
			Username: "nonexistent",
			Password: "password",
		}

		user, authErr := provider.Authenticate(ctx, creds)
		assert.Error(t, authErr)
		assert.Nil(t, user)
		assert.Equal(t, ErrInvalidCredentials, authErr)
	})

	t.Run("Disabled user", func(t *testing.T) {
		disabledUser := &User{
			ID:           "user2",
			Username:     "disabled",
			Email:        "disabled@example.com",
			PasswordHash: passwordHash,
			Enabled:      false,
		}
		err = userRepo.Create(ctx, disabledUser)
		require.NoError(t, err)

		creds := Credentials{
			Type:     AuthTypeBasic,
			Username: "disabled",
			Password: "testPass123",
		}

		user, err := provider.Authenticate(ctx, creds)
		assert.Error(t, err)
		assert.Nil(t, user)
	})

	t.Run("Empty credentials", func(t *testing.T) {
		creds := Credentials{
			Type:     AuthTypeBasic,
			Username: "",
			Password: "",
		}

		user, err := provider.Authenticate(ctx, creds)
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestAPIKeyAuthProvider(t *testing.T) {
	ctx := context.Background()
	userRepo := NewInMemoryUserRepository()
	apiKeyRepo := NewInMemoryAPIKeyRepository()

	// Create test user
	testUser := &User{
		ID:       "user1",
		Username: "testuser",
		Enabled:  true,
	}
	err := userRepo.Create(ctx, testUser)
	require.NoError(t, err)

	apiKeyManager := NewAPIKeyManager(apiKeyRepo)
	provider := NewAPIKeyAuthProvider(apiKeyManager, userRepo)

	t.Run("Successful authentication", func(t *testing.T) {
		// Generate API key
		apiKey, rawKey, err := apiKeyManager.GenerateAPIKey(
			ctx,
			testUser.ID,
			"Test Key",
			"Test Description",
			[]string{"*"},
			nil,
		)
		require.NoError(t, err)
		assert.NotEmpty(t, rawKey)
		assert.NotEmpty(t, apiKey.ID)

		// Authenticate with key
		creds := Credentials{
			Type:   AuthTypeAPIKey,
			APIKey: rawKey,
		}

		user, err := provider.Authenticate(ctx, creds)
		require.NoError(t, err)
		assert.Equal(t, testUser.ID, user.ID)
	})

	t.Run("Invalid API key", func(t *testing.T) {
		creds := Credentials{
			Type:   AuthTypeAPIKey,
			APIKey: "invalid-key",
		}

		user, err := provider.Authenticate(ctx, creds)
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestSecureCompare(t *testing.T) {
	t.Run("Equal strings", func(t *testing.T) {
		result := SecureCompare("password123", "password123")
		assert.True(t, result)
	})

	t.Run("Different strings", func(t *testing.T) {
		result := SecureCompare("password123", "password456")
		assert.False(t, result)
	})

	t.Run("Empty strings", func(t *testing.T) {
		result := SecureCompare("", "")
		assert.True(t, result)
	})
}
