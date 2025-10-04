package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSecurityConfig(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		config := DefaultSecurityConfig()
		assert.False(t, config.Enabled) // Disabled by default
		assert.NotEmpty(t, config.AuthProviders)
		assert.True(t, config.AuditEnabled)
		assert.True(t, config.RateLimitEnabled)
	})

	t.Run("Valid config", func(t *testing.T) {
		config := SecurityConfig{
			Enabled:          true,
			JWTSecret:        "this-is-a-very-long-secret-key-for-testing",
			EncryptionKey:    "encryption-key-16",
			TokenDuration:    1 * time.Hour,
			RefreshDuration:  24 * time.Hour,
			MaxLoginAttempts: 5,
			LockoutDuration:  15 * time.Minute,
			PasswordPolicy:   DefaultPasswordPolicy(),
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("Invalid - JWT secret too short", func(t *testing.T) {
		config := DefaultSecurityConfig()
		config.Enabled = true
		config.JWTSecret = "short"
		config.EncryptionKey = "encryption-key-16"

		err := config.Validate()
		assert.Error(t, err)
	})

	t.Run("Invalid - encryption key too short", func(t *testing.T) {
		config := DefaultSecurityConfig()
		config.Enabled = true
		config.JWTSecret = "this-is-a-very-long-secret-key-for-testing"
		config.EncryptionKey = "short"

		err := config.Validate()
		assert.Error(t, err)
	})

	t.Run("Invalid - refresh duration less than token duration", func(t *testing.T) {
		config := SecurityConfig{
			Enabled:          true,
			JWTSecret:        "this-is-a-very-long-secret-key-for-testing",
			EncryptionKey:    "encryption-key-16",
			TokenDuration:    24 * time.Hour,
			RefreshDuration:  1 * time.Hour, // Less than token duration
			MaxLoginAttempts: 5,
			LockoutDuration:  15 * time.Minute,
			PasswordPolicy:   DefaultPasswordPolicy(),
		}

		err := config.Validate()
		assert.Error(t, err)
	})

	t.Run("IsProviderEnabled", func(t *testing.T) {
		config := SecurityConfig{
			AuthProviders: []string{"jwt", "apikey"},
		}

		assert.True(t, config.IsProviderEnabled("jwt"))
		assert.True(t, config.IsProviderEnabled("apikey"))
		assert.False(t, config.IsProviderEnabled("basic"))
	})
}

func TestPasswordPolicy(t *testing.T) {
	t.Run("Default policy", func(t *testing.T) {
		policy := DefaultPasswordPolicy()
		assert.Equal(t, 8, policy.MinLength)
		assert.True(t, policy.RequireUpper)
		assert.True(t, policy.RequireLower)
		assert.True(t, policy.RequireNumber)
	})

	t.Run("Validate policy", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength: 8,
		}
		err := policy.Validate()
		assert.NoError(t, err)
	})

	t.Run("Invalid policy - min length too small", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength: 2,
		}
		err := policy.Validate()
		assert.Error(t, err)
	})

	t.Run("Valid password", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength:      8,
			RequireUpper:   true,
			RequireLower:   true,
			RequireNumber:  true,
			RequireSpecial: true,
		}

		err := policy.ValidatePassword("Passw0rd!")
		assert.NoError(t, err)
	})

	t.Run("Invalid - too short", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength: 8,
		}

		err := policy.ValidatePassword("short")
		assert.Error(t, err)
	})

	t.Run("Invalid - no uppercase", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength:    8,
			RequireUpper: true,
		}

		err := policy.ValidatePassword("password123")
		assert.Error(t, err)
	})

	t.Run("Invalid - no lowercase", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength:    8,
			RequireLower: true,
		}

		err := policy.ValidatePassword("PASSWORD123")
		assert.Error(t, err)
	})

	t.Run("Invalid - no number", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength:     8,
			RequireNumber: true,
		}

		err := policy.ValidatePassword("Password")
		assert.Error(t, err)
	})

	t.Run("Invalid - no special character", func(t *testing.T) {
		policy := PasswordPolicy{
			MinLength:      8,
			RequireSpecial: true,
		}

		err := policy.ValidatePassword("Password123")
		assert.Error(t, err)
	})
}
