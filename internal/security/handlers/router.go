package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/databridge/internal/security"
)

// RegisterSecurityRoutes registers all security-related routes
func RegisterSecurityRoutes(router *gin.Engine, authManager *security.AuthManager) {
	// Create handlers
	authHandler := NewAuthHandler(authManager)
	userHandler := NewUserHandler(authManager)
	roleHandler := NewRoleHandler(authManager)
	apiKeyHandler := NewAPIKeyHandler(authManager)
	auditHandler := NewAuditHandler(authManager)

	// Get managers
	rbacManager := authManager.GetRBACManager()

	// API group
	api := router.Group("/api")

	// Authentication endpoints (public)
	auth := api.Group("/auth")
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
	}

	// Protected authentication endpoints
	authProtected := api.Group("/auth")
	authProtected.Use(security.AuthMiddleware(authManager))
	{
		authProtected.POST("/logout", authHandler.Logout)
		authProtected.GET("/me", authHandler.Me)
	}

	// User management endpoints (admin only)
	users := api.Group("/users")
	users.Use(security.AuthMiddleware(authManager))
	users.Use(security.RequireRole(rbacManager, security.RoleAdmin))
	{
		users.GET("", userHandler.ListUsers)
		users.POST("", userHandler.CreateUser)
		users.GET("/:id", userHandler.GetUser)
		users.PUT("/:id", userHandler.UpdateUser)
		users.DELETE("/:id", userHandler.DeleteUser)
		users.POST("/:id/roles", userHandler.AssignRole)
		users.DELETE("/:id/roles/:roleId", userHandler.RemoveRole)
	}

	// Role management endpoints (admin only)
	roles := api.Group("/roles")
	roles.Use(security.AuthMiddleware(authManager))
	{
		// List and get are available to all authenticated users
		roles.GET("", roleHandler.ListRoles)
		roles.GET("/:id", roleHandler.GetRole)

		// Create, update, delete require admin
		adminRoles := roles.Group("")
		adminRoles.Use(security.RequireRole(rbacManager, security.RoleAdmin))
		{
			adminRoles.POST("", roleHandler.CreateRole)
			adminRoles.PUT("/:id", roleHandler.UpdateRole)
			adminRoles.DELETE("/:id", roleHandler.DeleteRole)
		}
	}

	// API key management endpoints (authenticated users)
	apiKeys := api.Group("/apikeys")
	apiKeys.Use(security.AuthMiddleware(authManager))
	{
		apiKeys.GET("", apiKeyHandler.ListAPIKeys)
		apiKeys.POST("", apiKeyHandler.CreateAPIKey)
		apiKeys.DELETE("/:id", apiKeyHandler.RevokeAPIKey)
	}

	// Audit log endpoints (admin only)
	audit := api.Group("/audit")
	audit.Use(security.AuthMiddleware(authManager))
	audit.Use(security.RequireRole(rbacManager, security.RoleAdmin))
	{
		audit.GET("", auditHandler.QueryAudit)
	}
}

// RegisterSecurityMiddleware registers global security middleware
func RegisterSecurityMiddleware(router *gin.Engine, authManager *security.AuthManager) {
	config := authManager.GetConfig()

	// Add security headers
	router.Use(security.SecurityHeadersMiddleware())

	// Add audit logging
	if config.AuditEnabled {
		router.Use(security.AuditMiddleware(authManager.GetAuditLogger()))
	}

	// Add rate limiting
	if config.RateLimitEnabled {
		rateLimitConfig := security.RateLimitConfig{
			RequestsPerSecond: config.RateLimitRPS,
			Burst:             config.RateLimitBurst,
			KeyFunc: func(c *gin.Context) string {
				return c.ClientIP()
			},
		}
		router.Use(security.RateLimitMiddleware(rateLimitConfig))
	}
}
