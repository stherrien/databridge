package security

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user
	UserContextKey ContextKey = "user"
)

// AuthMiddleware validates requests using the authentication manager
func AuthMiddleware(authManager *AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract authentication credentials from request
		authHeader := c.GetHeader("Authorization")
		apiKey := c.GetHeader("X-API-Key")

		var user *User
		var err error

		// Try API key authentication first
		if apiKey != "" {
			user, err = authManager.Authenticate(c.Request.Context(), Credentials{
				Type:   AuthTypeAPIKey,
				APIKey: apiKey,
			})
		} else if authHeader != "" {
			// Try Bearer token authentication
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != authHeader {
				user, err = authManager.Authenticate(c.Request.Context(), Credentials{
					Type:  AuthTypeBearer,
					Token: token,
				})
			} else if strings.HasPrefix(authHeader, "Basic ") {
				// Basic auth not recommended for API, but supported
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Basic authentication not supported. Use Bearer token or API key",
				})
				c.Abort()
				return
			}
		}

		if err != nil || user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		// Store user in context
		c.Set(string(UserContextKey), user)
		c.Next()
	}
}

// OptionalAuthMiddleware validates authentication if present, but allows unauthenticated requests
func OptionalAuthMiddleware(authManager *AuthManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		apiKey := c.GetHeader("X-API-Key")

		var user *User

		if apiKey != "" {
			user, _ = authManager.Authenticate(c.Request.Context(), Credentials{
				Type:   AuthTypeAPIKey,
				APIKey: apiKey,
			})
		} else if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != authHeader {
				user, _ = authManager.Authenticate(c.Request.Context(), Credentials{
					Type:  AuthTypeBearer,
					Token: token,
				})
			}
		}

		if user != nil {
			c.Set(string(UserContextKey), user)
		}
		c.Next()
	}
}

// RequirePermission checks if the user has a specific permission
func RequirePermission(rbacManager *RBACManager, resource string, action Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		permission := Permission{
			Resource: resource,
			Action:   action,
			Scope:    "*",
		}

		if !rbacManager.HasPermission(c.Request.Context(), user, permission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireResourcePermission checks permission for a specific resource instance
func RequireResourcePermission(rbacManager *RBACManager, resource string, action Action, scopeParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		scope := c.Param(scopeParam)
		permission := Permission{
			Resource: resource,
			Action:   action,
			Scope:    scope,
		}

		if !rbacManager.HasPermission(c.Request.Context(), user, permission) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireRole checks if the user has one of the specified roles
func RequireRole(rbacManager *RBACManager, roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetUserFromContext(c)
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Unauthorized",
			})
			c.Abort()
			return
		}

		userRoles, err := rbacManager.GetUserRoles(c.Request.Context(), user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to check user roles",
			})
			c.Abort()
			return
		}

		hasRole := false
		for _, userRole := range userRoles {
			for _, requiredRole := range roles {
				if userRole.Name == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Forbidden: insufficient role",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAdmin checks if the user is an admin
func RequireAdmin(rbacManager *RBACManager) gin.HandlerFunc {
	return RequireRole(rbacManager, RoleAdmin)
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
	KeyFunc           func(*gin.Context) string
}

// DefaultRateLimitConfig returns default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 10,
		Burst:             20,
		KeyFunc: func(c *gin.Context) string {
			// Rate limit by IP address
			return c.ClientIP()
		},
	}
}

// RateLimitMiddleware implements rate limiting
func RateLimitMiddleware(config RateLimitConfig) gin.HandlerFunc {
	limiters := &sync.Map{}

	return func(c *gin.Context) {
		key := config.KeyFunc(c)

		limiterInterface, _ := limiters.LoadOrStore(key, rate.NewLimiter(
			rate.Limit(config.RequestsPerSecond),
			config.Burst,
		))

		limiter := limiterInterface.(*rate.Limiter)

		if !limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// AuditMiddleware logs all requests for audit purposes
func AuditMiddleware(auditLogger *AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Log audit event
		user := GetUserFromContext(c)
		userID := ""
		username := ""
		if user != nil {
			userID = user.ID
			username = user.Username
		}

		result := AuditSuccess
		if c.Writer.Status() >= 400 {
			if c.Writer.Status() == http.StatusForbidden {
				result = AuditDenied
			} else {
				result = AuditFailure
			}
		}

		event := AuditEvent{
			Timestamp: start,
			UserID:    userID,
			Username:  username,
			Action:    c.Request.Method,
			Resource:  c.Request.URL.Path,
			Result:    result,
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Details: map[string]interface{}{
				"status":   c.Writer.Status(),
				"duration": time.Since(start).Milliseconds(),
			},
		}

		auditLogger.Log(c.Request.Context(), event)
	}
}

// SecurityHeadersMiddleware adds security headers to responses
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// GetUserFromContext retrieves the authenticated user from the Gin context
func GetUserFromContext(c *gin.Context) *User {
	userInterface, exists := c.Get(string(UserContextKey))
	if !exists {
		return nil
	}

	user, ok := userInterface.(*User)
	if !ok {
		return nil
	}

	return user
}

// GetUserFromStdContext retrieves the authenticated user from standard context
func GetUserFromStdContext(ctx context.Context) *User {
	userInterface := ctx.Value(UserContextKey)
	if userInterface == nil {
		return nil
	}

	user, ok := userInterface.(*User)
	if !ok {
		return nil
	}

	return user
}

// SetUserInContext sets the authenticated user in standard context
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, UserContextKey, user)
}
