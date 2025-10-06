package security

import (
	"errors"
	"time"
)

var (
	ErrInvalidConfig = errors.New("invalid security configuration")
)

// SecurityConfig holds security settings
type SecurityConfig struct {
	Enabled               bool
	AuthProviders         []string
	JWTSecret             string
	JWTIssuer             string
	TokenDuration         time.Duration
	RefreshDuration       time.Duration
	RequireHTTPS          bool
	SessionTimeout        time.Duration
	MaxLoginAttempts      int
	LockoutDuration       time.Duration
	PasswordPolicy        PasswordPolicy
	EncryptionKey         string
	AuditEnabled          bool
	RateLimitEnabled      bool
	RateLimitRPS          float64
	RateLimitBurst        int
	APIKeysEnabled        bool
	APIKeyCleanupInterval time.Duration
}

// PasswordPolicy defines password rules
type PasswordPolicy struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireNumber  bool
	RequireSpecial bool
	MaxAge         time.Duration // Password expiration
	PreventReuse   int           // Number of previous passwords to prevent reuse
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Enabled:               false, // Disabled by default for easier development
		AuthProviders:         []string{"jwt", "apikey"},
		JWTSecret:             "", // Must be set by user
		JWTIssuer:             "databridge",
		TokenDuration:         24 * time.Hour,
		RefreshDuration:       7 * 24 * time.Hour,
		RequireHTTPS:          false, // Should be true in production
		SessionTimeout:        30 * time.Minute,
		MaxLoginAttempts:      5,
		LockoutDuration:       15 * time.Minute,
		PasswordPolicy:        DefaultPasswordPolicy(),
		EncryptionKey:         "", // Must be set by user
		AuditEnabled:          true,
		RateLimitEnabled:      true,
		RateLimitRPS:          10,
		RateLimitBurst:        20,
		APIKeysEnabled:        true,
		APIKeyCleanupInterval: 24 * time.Hour,
	}
}

// DefaultPasswordPolicy returns default password policy
func DefaultPasswordPolicy() PasswordPolicy {
	return PasswordPolicy{
		MinLength:      8,
		RequireUpper:   true,
		RequireLower:   true,
		RequireNumber:  true,
		RequireSpecial: false,
		MaxAge:         90 * 24 * time.Hour, // 90 days
		PreventReuse:   3,
	}
}

// Validate validates the security configuration
func (c *SecurityConfig) Validate() error {
	if c.Enabled {
		if c.JWTSecret == "" {
			return errors.New("JWT secret is required when security is enabled")
		}

		if len(c.JWTSecret) < 32 {
			return errors.New("JWT secret must be at least 32 characters")
		}

		if c.EncryptionKey == "" {
			return errors.New("encryption key is required when security is enabled")
		}

		if len(c.EncryptionKey) < 16 {
			return errors.New("encryption key must be at least 16 characters")
		}

		if c.TokenDuration <= 0 {
			return errors.New("token duration must be positive")
		}

		if c.RefreshDuration <= 0 {
			return errors.New("refresh duration must be positive")
		}

		if c.RefreshDuration < c.TokenDuration {
			return errors.New("refresh duration must be greater than token duration")
		}

		if c.MaxLoginAttempts <= 0 {
			return errors.New("max login attempts must be positive")
		}

		if c.LockoutDuration <= 0 {
			return errors.New("lockout duration must be positive")
		}
	}

	return c.PasswordPolicy.Validate()
}

// Validate validates the password policy
func (p *PasswordPolicy) Validate() error {
	if p.MinLength < 4 {
		return errors.New("minimum password length must be at least 4")
	}

	if p.MinLength > 128 {
		return errors.New("minimum password length cannot exceed 128")
	}

	return nil
}

// ValidatePassword validates a password against the policy
func (p *PasswordPolicy) ValidatePassword(password string) error {
	if len(password) < p.MinLength {
		return errors.New("password does not meet minimum length requirement")
	}

	if p.RequireUpper && !containsUpper(password) {
		return errors.New("password must contain at least one uppercase letter")
	}

	if p.RequireLower && !containsLower(password) {
		return errors.New("password must contain at least one lowercase letter")
	}

	if p.RequireNumber && !containsNumber(password) {
		return errors.New("password must contain at least one number")
	}

	if p.RequireSpecial && !containsSpecial(password) {
		return errors.New("password must contain at least one special character")
	}

	return nil
}

// Helper functions for password validation

func containsUpper(s string) bool {
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func containsLower(s string) bool {
	for _, c := range s {
		if c >= 'a' && c <= 'z' {
			return true
		}
	}
	return false
}

func containsNumber(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func containsSpecial(s string) bool {
	special := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	for _, c := range s {
		for _, sp := range special {
			if c == sp {
				return true
			}
		}
	}
	return false
}

// IsProviderEnabled checks if an authentication provider is enabled
func (c *SecurityConfig) IsProviderEnabled(provider string) bool {
	for _, p := range c.AuthProviders {
		if p == provider {
			return true
		}
	}
	return false
}
