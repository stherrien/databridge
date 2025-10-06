package security

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserExists          = errors.New("user already exists")
	ErrTokenExpired        = errors.New("token expired")
	ErrTokenInvalid        = errors.New("token invalid")
	ErrUnsupportedAuthType = errors.New("unsupported authentication type")
)

// AuthType defines authentication methods
type AuthType string

const (
	AuthTypeBasic       AuthType = "basic"
	AuthTypeBearer      AuthType = "bearer"
	AuthTypeAPIKey      AuthType = "apikey"
	AuthTypeCertificate AuthType = "certificate"
)

// User represents an authenticated user
type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string // bcrypt hash
	Roles        []string
	Permissions  []Permission
	Attributes   map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Enabled      bool
}

// Credentials holds authentication data
type Credentials struct {
	Type        AuthType
	Username    string
	Password    string
	Token       string
	APIKey      string
	Certificate []byte
}

// AuthProvider defines authentication interface
type AuthProvider interface {
	Authenticate(ctx context.Context, credentials Credentials) (*User, error)
	Validate(ctx context.Context, token string) (*User, error)
	Refresh(ctx context.Context, token string) (string, error)
	GetType() AuthType
}

// BasicAuthProvider implements username/password authentication
type BasicAuthProvider struct {
	userRepo UserRepository
}

// NewBasicAuthProvider creates a new basic authentication provider
func NewBasicAuthProvider(userRepo UserRepository) *BasicAuthProvider {
	return &BasicAuthProvider{
		userRepo: userRepo,
	}
}

// GetType returns the authentication type
func (p *BasicAuthProvider) GetType() AuthType {
	return AuthTypeBasic
}

// Authenticate validates username and password
func (p *BasicAuthProvider) Authenticate(ctx context.Context, credentials Credentials) (*User, error) {
	if credentials.Type != AuthTypeBasic {
		return nil, ErrUnsupportedAuthType
	}

	if credentials.Username == "" || credentials.Password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := p.userRepo.GetByUsername(ctx, credentials.Username)
	if err != nil {
		if err == ErrUserNotFound {
			// Use constant time comparison to prevent timing attacks
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy.hash.to.prevent.timing"), []byte(credentials.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Enabled {
		return nil, ErrInvalidCredentials
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// Validate is not applicable for basic auth
func (p *BasicAuthProvider) Validate(ctx context.Context, token string) (*User, error) {
	return nil, errors.New("validate not supported for basic auth")
}

// Refresh is not applicable for basic auth
func (p *BasicAuthProvider) Refresh(ctx context.Context, token string) (string, error) {
	return "", errors.New("refresh not supported for basic auth")
}

// JWTAuthProvider implements JWT token-based authentication
type JWTAuthProvider struct {
	jwtManager *JWTManager
	userRepo   UserRepository
}

// NewJWTAuthProvider creates a new JWT authentication provider
func NewJWTAuthProvider(jwtManager *JWTManager, userRepo UserRepository) *JWTAuthProvider {
	return &JWTAuthProvider{
		jwtManager: jwtManager,
		userRepo:   userRepo,
	}
}

// GetType returns the authentication type
func (p *JWTAuthProvider) GetType() AuthType {
	return AuthTypeBearer
}

// Authenticate generates a JWT token for the user
func (p *JWTAuthProvider) Authenticate(ctx context.Context, credentials Credentials) (*User, error) {
	if credentials.Type != AuthTypeBearer && credentials.Type != AuthTypeBasic {
		return nil, ErrUnsupportedAuthType
	}

	// If basic credentials provided, validate them first
	if credentials.Username != "" && credentials.Password != "" {
		user, err := p.userRepo.GetByUsername(ctx, credentials.Username)
		if err != nil {
			if err == ErrUserNotFound {
				// Use constant time comparison to prevent timing attacks
				_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy.hash.to.prevent.timing"), []byte(credentials.Password))
				return nil, ErrInvalidCredentials
			}
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		if !user.Enabled {
			return nil, ErrInvalidCredentials
		}

		// Verify password
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password)); err != nil {
			return nil, ErrInvalidCredentials
		}

		return user, nil
	}

	// If token provided, validate it
	if credentials.Token != "" {
		return p.Validate(ctx, credentials.Token)
	}

	return nil, ErrInvalidCredentials
}

// Validate validates a JWT token and returns the user
func (p *JWTAuthProvider) Validate(ctx context.Context, token string) (*User, error) {
	claims, err := p.jwtManager.ValidateToken(token)
	if err != nil {
		return nil, err
	}

	user, err := p.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Enabled {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// Refresh generates a new token from a refresh token
func (p *JWTAuthProvider) Refresh(ctx context.Context, refreshToken string) (string, error) {
	claims, err := p.jwtManager.ValidateRefreshToken(refreshToken)
	if err != nil {
		return "", err
	}

	user, err := p.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Enabled {
		return "", ErrInvalidCredentials
	}

	// Generate new access token
	token, err := p.jwtManager.GenerateToken(user)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// APIKeyAuthProvider implements API key authentication
type APIKeyAuthProvider struct {
	apiKeyManager *APIKeyManager
	userRepo      UserRepository
}

// NewAPIKeyAuthProvider creates a new API key authentication provider
func NewAPIKeyAuthProvider(apiKeyManager *APIKeyManager, userRepo UserRepository) *APIKeyAuthProvider {
	return &APIKeyAuthProvider{
		apiKeyManager: apiKeyManager,
		userRepo:      userRepo,
	}
}

// GetType returns the authentication type
func (p *APIKeyAuthProvider) GetType() AuthType {
	return AuthTypeAPIKey
}

// Authenticate validates an API key
func (p *APIKeyAuthProvider) Authenticate(ctx context.Context, credentials Credentials) (*User, error) {
	if credentials.Type != AuthTypeAPIKey {
		return nil, ErrUnsupportedAuthType
	}

	if credentials.APIKey == "" {
		return nil, ErrInvalidCredentials
	}

	apiKey, err := p.apiKeyManager.ValidateKey(ctx, credentials.APIKey)
	if err != nil {
		return nil, err
	}

	user, err := p.userRepo.GetByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.Enabled {
		return nil, ErrInvalidCredentials
	}

	// Update last used timestamp
	p.apiKeyManager.UpdateLastUsed(ctx, apiKey.ID)

	return user, nil
}

// Validate validates an API key
func (p *APIKeyAuthProvider) Validate(ctx context.Context, apiKey string) (*User, error) {
	return p.Authenticate(ctx, Credentials{
		Type:   AuthTypeAPIKey,
		APIKey: apiKey,
	})
}

// Refresh is not applicable for API keys
func (p *APIKeyAuthProvider) Refresh(ctx context.Context, token string) (string, error) {
	return "", errors.New("refresh not supported for API key auth")
}

// HashPassword generates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword verifies a password against its hash
func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// SecureCompare performs a constant-time comparison of two strings
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
