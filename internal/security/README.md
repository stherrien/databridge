# DataBridge Security Framework

Comprehensive security and authentication framework for DataBridge.

## Overview

The DataBridge security framework provides:

- **Multi-provider Authentication** - Basic, JWT, API Keys, and certificate-based auth
- **Role-Based Access Control (RBAC)** - Flexible permission system with predefined roles
- **JWT Token Management** - Access and refresh tokens with revocation support
- **API Key Management** - Service-to-service authentication
- **Encryption** - AES-256-GCM encryption for sensitive data
- **Audit Logging** - Comprehensive security event tracking
- **Security Middleware** - Gin middleware for authentication, authorization, and rate limiting

## Quick Start

### 1. Basic Setup

```go
package main

import (
    "context"
    "github.com/gin-gonic/gin"
    "github.com/sirupsen/logrus"
    "github.com/shawntherrien/databridge/internal/security"
    "github.com/shawntherrien/databridge/internal/security/handlers"
)

func main() {
    logger := logrus.New()
    ctx := context.Background()

    // Configure security
    config := security.DefaultSecurityConfig()
    config.Enabled = true
    config.JWTSecret = "your-secret-key-min-32-chars-long"
    config.EncryptionKey = "your-encryption-key-16-chars"

    // Create security system
    authManager, err := security.CreateDefaultSecuritySystem(config, logger)
    if err != nil {
        logger.Fatal(err)
    }

    // Initialize (creates default admin user)
    if err := security.InitializeSecurity(ctx, authManager, logger); err != nil {
        logger.Fatal(err)
    }

    // Create Gin router
    router := gin.Default()

    // Register security routes and middleware
    handlers.RegisterSecurityMiddleware(router, authManager)
    handlers.RegisterSecurityRoutes(router, authManager)

    // Start server
    router.Run(":8080")
}
```

### 2. Default Admin User

On first run, a default admin user is created:
- **Username:** `admin`
- **Password:** `admin123`

**IMPORTANT:** Change this password immediately in production!

## Architecture

### Core Components

```
internal/security/
├── auth.go              # Authentication providers
├── rbac.go              # Role-based access control
├── jwt.go               # JWT token management
├── apikey.go            # API key management
├── encryption.go        # Data encryption
├── audit.go             # Audit logging
├── config.go            # Security configuration
├── manager.go           # Main authentication manager
├── middleware.go        # Gin middleware
├── repository.go        # Data repositories
├── init.go              # Initialization helpers
└── handlers/            # HTTP handlers
    ├── auth_handler.go
    ├── user_handler.go
    ├── role_handler.go
    ├── apikey_handler.go
    ├── audit_handler.go
    └── router.go
```

## Authentication

### Authentication Types

#### 1. Basic Authentication

Username and password authentication.

```go
credentials := security.Credentials{
    Type:     security.AuthTypeBasic,
    Username: "admin",
    Password: "admin123",
}

user, err := authManager.Authenticate(ctx, credentials)
```

#### 2. JWT Bearer Token

Token-based authentication for web and mobile clients.

```go
// Login to get tokens
accessToken, refreshToken, user, err := authManager.Login(ctx, "admin", "admin123")

// Use token in requests
credentials := security.Credentials{
    Type:  security.AuthTypeBearer,
    Token: accessToken,
}
user, err := authManager.Authenticate(ctx, credentials)

// Refresh token when expired
newToken, err := authManager.RefreshToken(ctx, refreshToken)
```

#### 3. API Key

Service-to-service authentication.

```go
// Generate API key
apiKey, rawKey, err := authManager.GetAPIKeyManager().GenerateAPIKey(
    ctx,
    userID,
    "Service Key",
    "For automation",
    []string{"*"},
    nil, // No expiration
)

// Authenticate with API key
credentials := security.Credentials{
    Type:   security.AuthTypeAPIKey,
    APIKey: rawKey,
}
user, err := authManager.Authenticate(ctx, credentials)
```

## Authorization (RBAC)

### Predefined Roles

1. **Admin** - Full system access
   - Resource: `*`, Action: `admin`, Scope: `*`

2. **Operator** - Manage flows and processors
   - Can read/write/execute/delete flows and processors
   - Can read monitoring data

3. **Monitor** - Read-only access
   - Can view flows, processors, connections
   - Can access monitoring endpoints

4. **Developer** - Create and test flows
   - Can create/modify flows and processors
   - Cannot deploy to production

### Permission System

```go
// Define a permission
permission := security.Permission{
    Resource: "flow",      // Resource type
    Action:   security.ActionWrite, // read, write, execute, delete, admin
    Scope:    "*",         // Resource ID or "*" for all
}

// Check permission
rbacManager := authManager.GetRBACManager()
hasPermission := rbacManager.HasPermission(ctx, user, permission)

// Assign role to user
err := rbacManager.AssignRole(ctx, userID, roleID)
```

### Custom Roles

```go
// Create custom role
role := &security.Role{
    ID:          "role_custom",
    Name:        "custom",
    Description: "Custom role",
    Permissions: []security.Permission{
        {Resource: "flow", Action: security.ActionRead, Scope: "*"},
        {Resource: "processor", Action: security.ActionRead, Scope: "proc123"},
    },
}

err := rbacManager.CreateRole(ctx, role)
```

## Middleware

### Authentication Middleware

Requires valid authentication:

```go
api := router.Group("/api")
api.Use(security.AuthMiddleware(authManager))
{
    api.GET("/protected", handler)
}
```

### Permission Middleware

Requires specific permission:

```go
api.Use(security.RequirePermission(rbacManager, "flow", security.ActionWrite))
{
    api.POST("/flows", createFlowHandler)
}
```

### Role Middleware

Requires specific role:

```go
api.Use(security.RequireRole(rbacManager, security.RoleAdmin))
{
    api.GET("/admin", adminHandler)
}
```

### Rate Limiting

```go
config := security.RateLimitConfig{
    RequestsPerSecond: 10,
    Burst:             20,
    KeyFunc: func(c *gin.Context) string {
        return c.ClientIP()
    },
}
router.Use(security.RateLimitMiddleware(config))
```

### Audit Logging

```go
router.Use(security.AuditMiddleware(authManager.GetAuditLogger()))
```

## Encryption

### Encrypt Sensitive Data

```go
encryptor := authManager.GetEncryptor()

// Encrypt string
ciphertext, err := encryptor.EncryptString("sensitive data")

// Decrypt string
plaintext, err := encryptor.DecryptString(ciphertext)
```

### Encrypt Processor Properties

```go
propEncryptor := security.NewPropertyEncryptor(
    encryptor,
    security.DefaultSensitiveFields(),
)

// Encrypt properties
properties := map[string]string{
    "username": "user",
    "password": "secret123",  // Will be encrypted
    "api_key":  "key123",     // Will be encrypted
}

encrypted, err := propEncryptor.EncryptProperties(properties)

// Decrypt properties
decrypted, err := propEncryptor.DecryptProperties(encrypted)
```

## REST API

### Authentication Endpoints

#### Login
```bash
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}

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
```

#### Refresh Token
```bash
POST /api/auth/refresh
Content-Type: application/json

{
  "refresh_token": "eyJhbGc..."
}
```

#### Get Current User
```bash
GET /api/auth/me
Authorization: Bearer eyJhbGc...
```

#### Logout
```bash
POST /api/auth/logout
Authorization: Bearer eyJhbGc...
```

### User Management (Admin Only)

```bash
GET    /api/users              # List users
POST   /api/users              # Create user
GET    /api/users/:id          # Get user
PUT    /api/users/:id          # Update user
DELETE /api/users/:id          # Delete user
POST   /api/users/:id/roles    # Assign role
DELETE /api/users/:id/roles/:roleId  # Remove role
```

### Role Management

```bash
GET    /api/roles              # List roles (authenticated)
GET    /api/roles/:id          # Get role (authenticated)
POST   /api/roles              # Create role (admin)
PUT    /api/roles/:id          # Update role (admin)
DELETE /api/roles/:id          # Delete role (admin)
```

### API Key Management

```bash
GET    /api/apikeys            # List API keys
POST   /api/apikeys            # Create API key
DELETE /api/apikeys/:id        # Revoke API key
```

### Audit Logs (Admin Only)

```bash
GET /api/audit?user_id=xxx&action=login&start_time=2024-01-01T00:00:00Z
```

## Configuration

### Environment Variables

```bash
export DATABRIDGE_JWT_SECRET="your-very-secure-jwt-secret-at-least-32-characters"
export DATABRIDGE_ENCRYPTION_KEY="your-very-secure-encryption-key"
export DATABRIDGE_SECURITY_ENABLED="true"
export DATABRIDGE_REQUIRE_HTTPS="true"
```

### Configuration File (databridge.yaml)

```yaml
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
    max_age: "90d"
    prevent_reuse: 3
```

### Programmatic Configuration

```go
config := security.SecurityConfig{
    Enabled:          true,
    AuthProviders:    []string{"jwt", "apikey"},
    JWTSecret:        "your-secret-key-min-32-chars-long",
    JWTIssuer:        "databridge",
    TokenDuration:    24 * time.Hour,
    RefreshDuration:  7 * 24 * time.Hour,
    RequireHTTPS:     true,
    SessionTimeout:   30 * time.Minute,
    MaxLoginAttempts: 5,
    LockoutDuration:  15 * time.Minute,
    EncryptionKey:    "your-encryption-key-16-chars",
    AuditEnabled:     true,
    RateLimitEnabled: true,
    RateLimitRPS:     10,
    RateLimitBurst:   20,
    APIKeysEnabled:   true,
    PasswordPolicy:   security.DefaultPasswordPolicy(),
}
```

## Testing

Run the comprehensive test suite:

```bash
# Run all security tests
go test ./internal/security -v

# Run with coverage
go test ./internal/security -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Security Best Practices

1. **Secrets Management**
   - Never hardcode secrets in code
   - Use environment variables or secure vaults
   - Rotate secrets regularly
   - Use different secrets for dev/staging/prod

2. **JWT Tokens**
   - Keep secret keys secure and long (32+ chars)
   - Set appropriate token expiration times
   - Use HTTPS to prevent token interception
   - Implement token revocation for logout
   - Store refresh tokens securely

3. **Passwords**
   - Enforce strong password policies
   - Use bcrypt for password hashing
   - Implement rate limiting on login attempts
   - Lock accounts after failed attempts
   - Force password changes periodically

4. **API Keys**
   - Scope API keys to minimum required permissions
   - Set expiration dates on API keys
   - Monitor and log API key usage
   - Revoke unused or compromised keys immediately

5. **HTTPS**
   - Always use HTTPS in production
   - Set `RequireHTTPS: true` in config
   - Use TLS 1.2 or higher
   - Configure proper certificates

6. **Audit Logging**
   - Enable audit logging in production
   - Review audit logs regularly
   - Monitor for suspicious activity
   - Retain logs per compliance requirements

7. **Rate Limiting**
   - Enable rate limiting to prevent abuse
   - Set appropriate limits per endpoint
   - Monitor rate limit violations
   - Implement graduated response (warn, throttle, block)

## Production Deployment Checklist

- [ ] Change default admin password
- [ ] Set secure JWT secret (32+ characters)
- [ ] Set secure encryption key (16+ characters)
- [ ] Enable HTTPS (`RequireHTTPS: true`)
- [ ] Configure appropriate token durations
- [ ] Enable audit logging
- [ ] Enable rate limiting
- [ ] Set up secure secret storage (Vault, AWS Secrets Manager, etc.)
- [ ] Configure password policy
- [ ] Set up monitoring and alerting
- [ ] Test authentication and authorization flows
- [ ] Review and test backup/recovery procedures
- [ ] Document API key management procedures
- [ ] Set up log aggregation and analysis
- [ ] Configure firewall rules
- [ ] Enable security headers middleware

## Troubleshooting

### Common Issues

**Issue: "JWT secret is required when security is enabled"**
- Solution: Set `JWTSecret` in configuration (min 32 characters)

**Issue: "Invalid credentials" even with correct password**
- Solution: Check user is enabled, not locked out, and password is correct

**Issue: "Token expired"**
- Solution: Use refresh token to get new access token

**Issue: "Permission denied"**
- Solution: Check user has correct roles and permissions for the resource

**Issue: "Rate limit exceeded"**
- Solution: Reduce request rate or increase rate limit configuration

## Support

For issues, questions, or contributions, please refer to the main DataBridge documentation.
