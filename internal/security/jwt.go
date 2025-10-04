package security

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

// TokenClaims extends standard JWT claims
type TokenClaims struct {
	UserID   string   `json:"user_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

// JWTManager handles JWT operations
type JWTManager struct {
	secretKey        []byte
	issuer           string
	tokenDuration    time.Duration
	refreshDuration  time.Duration
	revokedTokens    map[string]time.Time // token ID -> revocation time
	mu               sync.RWMutex
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secretKey string, issuer string, tokenDuration, refreshDuration time.Duration) *JWTManager {
	return &JWTManager{
		secretKey:        []byte(secretKey),
		issuer:           issuer,
		tokenDuration:    tokenDuration,
		refreshDuration:  refreshDuration,
		revokedTokens:    make(map[string]time.Time),
	}
}

// GenerateToken generates an access token for a user
func (m *JWTManager) GenerateToken(user *User) (string, error) {
	now := time.Now()

	claims := TokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Roles:    user.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.tokenDuration)),
			NotBefore: jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// GenerateRefreshToken generates a refresh token for a user
func (m *JWTManager) GenerateRefreshToken(user *User) (string, error) {
	now := time.Now()

	claims := TokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Roles:    user.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshDuration)),
			NotBefore: jwt.NewNumericDate(now),
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return tokenString, nil
}

// ValidateToken validates and parses an access token
func (m *JWTManager) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	// Check if token is revoked
	m.mu.RLock()
	if revocationTime, revoked := m.revokedTokens[claims.ID]; revoked {
		m.mu.RUnlock()
		if claims.IssuedAt.Time.Before(revocationTime) {
			return nil, errors.New("token has been revoked")
		}
	} else {
		m.mu.RUnlock()
	}

	return claims, nil
}

// ValidateRefreshToken validates and parses a refresh token
func (m *JWTManager) ValidateRefreshToken(tokenString string) (*TokenClaims, error) {
	return m.ValidateToken(tokenString)
}

// RevokeToken revokes a token by its ID
func (m *JWTManager) RevokeToken(tokenID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokedTokens[tokenID] = time.Now()
}

// RevokeUserTokens revokes all tokens for a user
func (m *JWTManager) RevokeUserTokens(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// In a real implementation, you would track user tokens
	// For now, we just mark the revocation time
	m.revokedTokens[userID] = time.Now()
}

// CleanupRevokedTokens removes expired revoked tokens
func (m *JWTManager) CleanupRevokedTokens() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for tokenID, revocationTime := range m.revokedTokens {
		// Remove tokens that were revoked longer than the refresh duration ago
		if now.Sub(revocationTime) > m.refreshDuration {
			delete(m.revokedTokens, tokenID)
		}
	}
}

// StartCleanupRoutine starts a goroutine that periodically cleans up revoked tokens
func (m *JWTManager) StartCleanupRoutine(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.CleanupRevokedTokens()
			case <-stop:
				return
			}
		}
	}()

	return stop
}

// generateTokenID generates a unique token ID
func generateTokenID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ExtractTokenFromHeader extracts bearer token from authorization header
func ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is empty")
	}

	const prefix = "Bearer "
	if len(authHeader) < len(prefix) {
		return "", errors.New("invalid authorization header format")
	}

	if authHeader[:len(prefix)] != prefix {
		return "", errors.New("authorization header must start with 'Bearer '")
	}

	return authHeader[len(prefix):], nil
}
