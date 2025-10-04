package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManager(t *testing.T) {
	secret := "test-secret-key-must-be-32-chars-or-more"
	issuer := "test-issuer"
	tokenDuration := 1 * time.Hour
	refreshDuration := 24 * time.Hour

	manager := NewJWTManager(secret, issuer, tokenDuration, refreshDuration)

	testUser := &User{
		ID:       "user123",
		Username: "testuser",
		Email:    "test@example.com",
		Roles:    []string{"admin", "operator"},
	}

	t.Run("Generate access token", func(t *testing.T) {
		token, err := manager.GenerateToken(testUser)
		require.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("Generate refresh token", func(t *testing.T) {
		token, err := manager.GenerateRefreshToken(testUser)
		require.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("Validate valid token", func(t *testing.T) {
		token, err := manager.GenerateToken(testUser)
		require.NoError(t, err)

		claims, err := manager.ValidateToken(token)
		require.NoError(t, err)
		assert.Equal(t, testUser.ID, claims.UserID)
		assert.Equal(t, testUser.Username, claims.Username)
		assert.ElementsMatch(t, testUser.Roles, claims.Roles)
		assert.Equal(t, issuer, claims.Issuer)
	})

	t.Run("Validate invalid token", func(t *testing.T) {
		claims, err := manager.ValidateToken("invalid-token")
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("Validate expired token", func(t *testing.T) {
		// Create manager with very short duration
		shortManager := NewJWTManager(secret, issuer, 1*time.Millisecond, 1*time.Second)
		token, err := shortManager.GenerateToken(testUser)
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(10 * time.Millisecond)

		claims, err := shortManager.ValidateToken(token)
		assert.Error(t, err)
		assert.Nil(t, claims)
	})

	t.Run("Revoke token", func(t *testing.T) {
		token, err := manager.GenerateToken(testUser)
		require.NoError(t, err)

		// Validate before revocation
		claims, err := manager.ValidateToken(token)
		require.NoError(t, err)
		assert.NotNil(t, claims)

		// Revoke token
		manager.RevokeToken(claims.ID)

		// Validate after revocation - should fail
		claims, err = manager.ValidateToken(token)
		assert.Error(t, err)
	})

	t.Run("Cleanup revoked tokens", func(t *testing.T) {
		// This is mostly for coverage
		manager.CleanupRevokedTokens()
	})
}

func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		expectError bool
		expectToken string
	}{
		{
			name:        "Valid bearer token",
			header:      "Bearer abc123",
			expectError: false,
			expectToken: "abc123",
		},
		{
			name:        "Empty header",
			header:      "",
			expectError: true,
		},
		{
			name:        "No Bearer prefix",
			header:      "abc123",
			expectError: true,
		},
		{
			name:        "Wrong prefix",
			header:      "Basic abc123",
			expectError: true,
		},
		{
			name:        "Just Bearer",
			header:      "Bearer",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractTokenFromHeader(tt.header)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectToken, token)
			}
		})
	}
}
