package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/databridge/internal/security"
)

// APIKeyHandler handles API key management endpoints
type APIKeyHandler struct {
	authManager *security.AuthManager
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(authManager *security.AuthManager) *APIKeyHandler {
	return &APIKeyHandler{
		authManager: authManager,
	}
}

// APIKeyDTO represents an API key data transfer object
type APIKeyDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Enabled     bool       `json:"enabled"`
}

// CreateAPIKeyRequest represents an API key creation request
type CreateAPIKeyRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
	ExpiresIn   *int     `json:"expires_in"` // seconds
}

// CreateAPIKeyResponse includes the generated key
type CreateAPIKeyResponse struct {
	APIKey APIKeyDTO `json:"api_key"`
	Key    string    `json:"key"` // Only returned on creation
}

// ListAPIKeys lists all API keys for the current user
func (h *APIKeyHandler) ListAPIKeys(c *gin.Context) {
	user := security.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	apiKeyManager := h.authManager.GetAPIKeyManager()

	// Check if user is admin - they can see all keys
	rbacManager := h.authManager.GetRBACManager()
	isAdmin := rbacManager.IsAdmin(c.Request.Context(), user)

	var keys []*security.APIKey
	var err error

	if isAdmin {
		keys, err = apiKeyManager.ListAPIKeys(c.Request.Context())
	} else {
		keys, err = apiKeyManager.GetUserAPIKeys(c.Request.Context(), user.ID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list API keys"})
		return
	}

	dtos := make([]APIKeyDTO, len(keys))
	for i, key := range keys {
		dtos[i] = APIKeyDTO{
			ID:          key.ID,
			Name:        key.Name,
			Description: key.Description,
			Scopes:      key.Scopes,
			ExpiresAt:   key.ExpiresAt,
			CreatedAt:   key.CreatedAt,
			LastUsedAt:  key.LastUsedAt,
			Enabled:     key.Enabled,
		}
	}

	c.JSON(http.StatusOK, gin.H{"api_keys": dtos})
}

// CreateAPIKey creates a new API key
func (h *APIKeyHandler) CreateAPIKey(c *gin.Context) {
	user := security.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiKeyManager := h.authManager.GetAPIKeyManager()

	var expiresIn *time.Duration
	if req.ExpiresIn != nil {
		duration := time.Duration(*req.ExpiresIn) * time.Second
		expiresIn = &duration
	}

	apiKey, key, err := apiKeyManager.GenerateAPIKey(
		c.Request.Context(),
		user.ID,
		req.Name,
		req.Description,
		req.Scopes,
		expiresIn,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create API key"})
		return
	}

	// Log audit event
	h.authManager.GetAuditLogger().LogAPIKeyAction(
		c.Request.Context(),
		user,
		security.AuditActionAPIKeyCreate,
		apiKey.ID,
		c.ClientIP(),
	)

	c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		APIKey: APIKeyDTO{
			ID:          apiKey.ID,
			Name:        apiKey.Name,
			Description: apiKey.Description,
			Scopes:      apiKey.Scopes,
			ExpiresAt:   apiKey.ExpiresAt,
			CreatedAt:   apiKey.CreatedAt,
			LastUsedAt:  apiKey.LastUsedAt,
			Enabled:     apiKey.Enabled,
		},
		Key: key, // Only time the key is visible
	})
}

// RevokeAPIKey revokes an API key
func (h *APIKeyHandler) RevokeAPIKey(c *gin.Context) {
	user := security.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	keyID := c.Param("id")
	apiKeyManager := h.authManager.GetAPIKeyManager()

	// Get the key to verify ownership
	key, err := apiKeyManager.GetAPIKey(c.Request.Context(), keyID)
	if err != nil {
		if err == security.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get API key"})
		}
		return
	}

	// Check if user owns the key or is admin
	rbacManager := h.authManager.GetRBACManager()
	isAdmin := rbacManager.IsAdmin(c.Request.Context(), user)

	if key.UserID != user.ID && !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	if err := apiKeyManager.RevokeAPIKey(c.Request.Context(), keyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke API key"})
		return
	}

	// Log audit event
	h.authManager.GetAuditLogger().LogAPIKeyAction(
		c.Request.Context(),
		user,
		security.AuditActionAPIKeyRevoke,
		keyID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked successfully"})
}

// DeleteAPIKey deletes an API key
func (h *APIKeyHandler) DeleteAPIKey(c *gin.Context) {
	user := security.GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	keyID := c.Param("id")
	apiKeyManager := h.authManager.GetAPIKeyManager()

	// Get the key to verify ownership
	key, err := apiKeyManager.GetAPIKey(c.Request.Context(), keyID)
	if err != nil {
		if err == security.ErrAPIKeyNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get API key"})
		}
		return
	}

	// Check if user owns the key or is admin
	rbacManager := h.authManager.GetRBACManager()
	isAdmin := rbacManager.IsAdmin(c.Request.Context(), user)

	if key.UserID != user.ID && !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	if err := apiKeyManager.DeleteAPIKey(c.Request.Context(), keyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete API key"})
		return
	}

	// Log audit event
	h.authManager.GetAuditLogger().LogAPIKeyAction(
		c.Request.Context(),
		user,
		security.AuditActionAPIKeyDelete,
		keyID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "API key deleted successfully"})
}
