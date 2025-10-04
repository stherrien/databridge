package security

// Example of how to integrate security with DataBridge
//
// This file provides example code for integrating the security framework
// with a Gin-based HTTP server.

/*
Example integration in main.go:

package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/internal/security"
)

func main() {
	logger := logrus.New()
	ctx := context.Background()

	// Configure security
	securityConfig := security.DefaultSecurityConfig()
	securityConfig.Enabled = true
	securityConfig.JWTSecret = "your-secret-key-min-32-chars-long-change-in-production"
	securityConfig.EncryptionKey = "your-encryption-key-change-in-production"

	// Create security system
	authManager, err := security.CreateDefaultSecuritySystem(securityConfig, logger)
	if err != nil {
		log.Fatal("Failed to create security system:", err)
	}

	// Initialize security (creates default admin user)
	if err := security.InitializeSecurity(ctx, authManager, logger); err != nil {
		log.Fatal("Failed to initialize security:", err)
	}

	// Create Gin router
	router := gin.Default()

	// Register security middleware
	handlers.RegisterSecurityMiddleware(router, authManager)

	// Register security routes
	handlers.RegisterSecurityRoutes(router, authManager)

	// Add protected endpoints
	api := router.Group("/api")
	api.Use(security.AuthMiddleware(authManager))
	{
		// These endpoints require authentication
		api.GET("/protected", func(c *gin.Context) {
			user := security.GetUserFromContext(c)
			c.JSON(200, gin.H{"user": user.Username})
		})

		// Require specific permission
		api.GET("/admin",
			security.RequirePermission(authManager.GetRBACManager(), "system", security.ActionAdmin),
			func(c *gin.Context) {
				c.JSON(200, gin.H{"message": "Admin access"})
			},
		)
	}

	// Start server
	router.Run(":8080")
}

Example API Usage:

1. Login to get token:
   curl -X POST http://localhost:8080/api/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"admin123"}'

   Response:
   {
     "access_token": "eyJhbGc...",
     "refresh_token": "eyJhbGc...",
     "token_type": "Bearer",
     "expires_in": 86400,
     "user": {
       "id": "user_admin",
       "username": "admin",
       "email": "admin@databridge.local",
       "roles": ["admin"]
     }
   }

2. Use token for authenticated requests:
   curl -X GET http://localhost:8080/api/auth/me \
     -H "Authorization: Bearer eyJhbGc..."

3. Create a new user:
   curl -X POST http://localhost:8080/api/users \
     -H "Authorization: Bearer eyJhbGc..." \
     -H "Content-Type: application/json" \
     -d '{
       "username": "operator",
       "email": "operator@example.com",
       "password": "SecurePass123!",
       "roles": ["operator"]
     }'

4. Create an API key:
   curl -X POST http://localhost:8080/api/apikeys \
     -H "Authorization: Bearer eyJhbGc..." \
     -H "Content-Type: application/json" \
     -d '{
       "name": "My API Key",
       "description": "For automation",
       "scopes": ["*"]
     }'

5. Use API key:
   curl -X GET http://localhost:8080/api/protected \
     -H "X-API-Key: <api-key>"

6. Query audit logs:
   curl -X GET "http://localhost:8080/api/audit?action=login" \
     -H "Authorization: Bearer eyJhbGc..."

Environment Variables:

Set these environment variables for production:

export DATABRIDGE_JWT_SECRET="your-very-secure-jwt-secret-at-least-32-characters"
export DATABRIDGE_ENCRYPTION_KEY="your-very-secure-encryption-key-at-least-16-chars"
export DATABRIDGE_SECURITY_ENABLED="true"
export DATABRIDGE_REQUIRE_HTTPS="true"

Configuration File (databridge.yaml):

security:
  enabled: true
  auth_providers:
    - jwt
    - apikey
  jwt_secret: "${DATABRIDGE_JWT_SECRET}"
  jwt_issuer: "databridge"
  token_duration: "24h"
  refresh_duration: "168h"  # 7 days
  require_https: true
  session_timeout: "30m"
  max_login_attempts: 5
  lockout_duration: "15m"
  encryption_key: "${DATABRIDGE_ENCRYPTION_KEY}"
  audit_enabled: true
  rate_limit_enabled: true
  rate_limit_rps: 10
  rate_limit_burst: 20
  api_keys_enabled: true
  password_policy:
    min_length: 8
    require_upper: true
    require_lower: true
    require_number: true
    require_special: true

*/
