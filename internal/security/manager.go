package security

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	ErrSecurityNotEnabled = errors.New("security is not enabled")
	ErrProviderNotFound   = errors.New("authentication provider not found")
)

// AuthManager manages authentication and authorization
type AuthManager struct {
	config        SecurityConfig
	providers     map[AuthType]AuthProvider
	rbacManager   *RBACManager
	apiKeyManager *APIKeyManager
	jwtManager    *JWTManager
	encryptor     *Encryptor
	auditLogger   *AuditLogger
	userRepo      UserRepository
	loginAttempts map[string]*LoginAttempts
	mu            sync.RWMutex
	logger        *logrus.Logger
}

// LoginAttempts tracks failed login attempts
type LoginAttempts struct {
	Count       int
	LastAttempt time.Time
	LockedUntil *time.Time
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(
	config SecurityConfig,
	userRepo UserRepository,
	roleRepo RoleRepository,
	apiKeyRepo APIKeyRepository,
	auditRepo AuditRepository,
	logger *logrus.Logger,
) (*AuthManager, error) {
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid security config: %w", err)
	}

	// Create JWT manager
	jwtManager := NewJWTManager(
		config.JWTSecret,
		config.JWTIssuer,
		config.TokenDuration,
		config.RefreshDuration,
	)

	// Create encryptor
	encryptor, err := NewEncryptor(config.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	// Create RBAC manager
	rbacManager := NewRBACManager(roleRepo)

	// Create API key manager
	apiKeyManager := NewAPIKeyManager(apiKeyRepo)

	// Create audit logger
	auditLogger := NewAuditLogger(logger, auditRepo)

	// Create auth manager
	manager := &AuthManager{
		config:        config,
		providers:     make(map[AuthType]AuthProvider),
		rbacManager:   rbacManager,
		apiKeyManager: apiKeyManager,
		jwtManager:    jwtManager,
		encryptor:     encryptor,
		auditLogger:   auditLogger,
		userRepo:      userRepo,
		loginAttempts: make(map[string]*LoginAttempts),
		logger:        logger,
	}

	// Register providers
	if config.IsProviderEnabled("basic") {
		manager.RegisterProvider(NewBasicAuthProvider(userRepo))
	}

	if config.IsProviderEnabled("jwt") {
		manager.RegisterProvider(NewJWTAuthProvider(jwtManager, userRepo))
	}

	if config.IsProviderEnabled("apikey") && config.APIKeysEnabled {
		manager.RegisterProvider(NewAPIKeyAuthProvider(apiKeyManager, userRepo))
	}

	return manager, nil
}

// Initialize initializes the auth manager
func (m *AuthManager) Initialize(ctx context.Context) error {
	// Initialize RBAC with predefined roles
	if err := m.rbacManager.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize RBAC: %w", err)
	}

	// Start JWT cleanup routine
	if m.jwtManager != nil {
		m.jwtManager.StartCleanupRoutine(1 * time.Hour)
	}

	// Start API key cleanup routine
	if m.apiKeyManager != nil && m.config.APIKeysEnabled {
		m.apiKeyManager.StartCleanupRoutine(ctx, m.config.APIKeyCleanupInterval)
	}

	m.logger.Info("Authentication manager initialized")
	return nil
}

// RegisterProvider registers an authentication provider
func (m *AuthManager) RegisterProvider(provider AuthProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[provider.GetType()] = provider
}

// Authenticate authenticates a user with the given credentials
func (m *AuthManager) Authenticate(ctx context.Context, credentials Credentials) (*User, error) {
	if !m.config.Enabled {
		return nil, ErrSecurityNotEnabled
	}

	// Check if user is locked out
	if credentials.Username != "" && m.isLockedOut(credentials.Username) {
		m.auditLogger.LogLogin(ctx, nil, "", "", false)
		return nil, errors.New("account is locked due to too many failed attempts")
	}

	provider, ok := m.providers[credentials.Type]
	if !ok {
		return nil, ErrProviderNotFound
	}

	user, err := provider.Authenticate(ctx, credentials)
	if err != nil {
		// Track failed login attempts
		if credentials.Username != "" {
			m.recordFailedAttempt(credentials.Username)
		}
		m.auditLogger.LogLogin(ctx, nil, "", "", false)
		return nil, err
	}

	// Clear failed login attempts on success
	if credentials.Username != "" {
		m.clearFailedAttempts(credentials.Username)
	}

	m.auditLogger.LogLogin(ctx, user, "", "", true)
	return user, nil
}

// Login authenticates a user and returns tokens
func (m *AuthManager) Login(ctx context.Context, username, password string) (accessToken, refreshToken string, user *User, err error) {
	if !m.config.Enabled {
		return "", "", nil, ErrSecurityNotEnabled
	}

	// Check if user is locked out
	if m.isLockedOut(username) {
		m.auditLogger.LogLogin(ctx, nil, "", "", false)
		return "", "", nil, errors.New("account is locked due to too many failed attempts")
	}

	// Authenticate user
	credentials := Credentials{
		Type:     AuthTypeBasic,
		Username: username,
		Password: password,
	}

	basicProvider, ok := m.providers[AuthTypeBasic]
	if !ok {
		return "", "", nil, errors.New("basic authentication not available")
	}

	user, err = basicProvider.Authenticate(ctx, credentials)
	if err != nil {
		m.recordFailedAttempt(username)
		m.auditLogger.LogLogin(ctx, nil, "", "", false)
		return "", "", nil, err
	}

	// Clear failed attempts
	m.clearFailedAttempts(username)

	// Generate tokens
	accessToken, err = m.jwtManager.GenerateToken(user)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err = m.jwtManager.GenerateRefreshToken(user)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	m.auditLogger.LogLogin(ctx, user, "", "", true)
	return accessToken, refreshToken, user, nil
}

// RefreshToken refreshes an access token using a refresh token
func (m *AuthManager) RefreshToken(ctx context.Context, refreshToken string) (string, error) {
	if !m.config.Enabled {
		return "", ErrSecurityNotEnabled
	}

	jwtProvider, ok := m.providers[AuthTypeBearer]
	if !ok {
		return "", errors.New("JWT authentication not available")
	}

	return jwtProvider.Refresh(ctx, refreshToken)
}

// Logout logs out a user
func (m *AuthManager) Logout(ctx context.Context, token string) error {
	if !m.config.Enabled {
		return ErrSecurityNotEnabled
	}

	// Validate token and get claims
	claims, err := m.jwtManager.ValidateToken(token)
	if err != nil {
		return err
	}

	// Revoke the token
	m.jwtManager.RevokeToken(claims.ID)

	// Log audit event
	user, _ := m.userRepo.GetByID(ctx, claims.UserID)
	if user != nil {
		m.auditLogger.LogLogout(ctx, user, "", "")
	}

	return nil
}

// GetRBACManager returns the RBAC manager
func (m *AuthManager) GetRBACManager() *RBACManager {
	return m.rbacManager
}

// GetAPIKeyManager returns the API key manager
func (m *AuthManager) GetAPIKeyManager() *APIKeyManager {
	return m.apiKeyManager
}

// GetJWTManager returns the JWT manager
func (m *AuthManager) GetJWTManager() *JWTManager {
	return m.jwtManager
}

// GetEncryptor returns the encryptor
func (m *AuthManager) GetEncryptor() *Encryptor {
	return m.encryptor
}

// GetAuditLogger returns the audit logger
func (m *AuthManager) GetAuditLogger() *AuditLogger {
	return m.auditLogger
}

// GetConfig returns the security configuration
func (m *AuthManager) GetConfig() SecurityConfig {
	return m.config
}

// IsEnabled returns whether security is enabled
func (m *AuthManager) IsEnabled() bool {
	return m.config.Enabled
}

// recordFailedAttempt records a failed login attempt
func (m *AuthManager) recordFailedAttempt(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	attempts, ok := m.loginAttempts[username]
	if !ok {
		attempts = &LoginAttempts{
			Count:       0,
			LastAttempt: time.Now(),
		}
		m.loginAttempts[username] = attempts
	}

	attempts.Count++
	attempts.LastAttempt = time.Now()

	// Lock account if max attempts exceeded
	if attempts.Count >= m.config.MaxLoginAttempts {
		lockUntil := time.Now().Add(m.config.LockoutDuration)
		attempts.LockedUntil = &lockUntil
		m.logger.WithField("username", username).Warn("Account locked due to too many failed login attempts")
	}
}

// clearFailedAttempts clears failed login attempts for a user
func (m *AuthManager) clearFailedAttempts(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.loginAttempts, username)
}

// isLockedOut checks if a user is locked out
func (m *AuthManager) isLockedOut(username string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	attempts, ok := m.loginAttempts[username]
	if !ok {
		return false
	}

	if attempts.LockedUntil == nil {
		return false
	}

	// Check if lockout has expired
	if time.Now().After(*attempts.LockedUntil) {
		// Lockout expired, clear it
		go func() {
			m.clearFailedAttempts(username)
		}()
		return false
	}

	return true
}

// GetUserRepository returns the user repository
func (m *AuthManager) GetUserRepository() UserRepository {
	return m.userRepo
}
