package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/databridge/internal/security"
)

// UserHandler handles user management endpoints
type UserHandler struct {
	authManager *security.AuthManager
}

// NewUserHandler creates a new user handler
func NewUserHandler(authManager *security.AuthManager) *UserHandler {
	return &UserHandler{
		authManager: authManager,
	}
}

// CreateUserRequest represents a user creation request
type CreateUserRequest struct {
	Username   string            `json:"username" binding:"required"`
	Email      string            `json:"email"`
	Password   string            `json:"password" binding:"required"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Email      string            `json:"email"`
	Password   string            `json:"password"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes"`
	Enabled    *bool             `json:"enabled"`
}

// ListUsers lists all users
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.authManager.GetUserRepository().List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list users"})
		return
	}

	dtos := make([]UserDTO, len(users))
	for i, user := range users {
		dtos[i] = UserDTO{
			ID:         user.ID,
			Username:   user.Username,
			Email:      user.Email,
			Roles:      user.Roles,
			Attributes: user.Attributes,
		}
	}

	c.JSON(http.StatusOK, gin.H{"users": dtos})
}

// CreateUser creates a new user
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate password against policy
	policy := h.authManager.GetConfig().PasswordPolicy
	if err := policy.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password
	passwordHash, err := security.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create user
	user := &security.User{
		ID:           fmt.Sprintf("user_%d", time.Now().UnixNano()),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Roles:        req.Roles,
		Attributes:   req.Attributes,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Enabled:      true,
	}

	if err := h.authManager.GetUserRepository().Create(c.Request.Context(), user); err != nil {
		if err == security.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": "User already exists"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		}
		return
	}

	// Assign roles
	rbacManager := h.authManager.GetRBACManager()
	for _, roleName := range req.Roles {
		role, err := rbacManager.GetRoleByName(c.Request.Context(), roleName)
		if err == nil {
			_ = rbacManager.AssignRole(c.Request.Context(), user.ID, role.ID)
		}
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogUserAction(
		c.Request.Context(),
		actor,
		security.AuditActionUserCreate,
		user.ID,
		c.ClientIP(),
	)

	c.JSON(http.StatusCreated, UserDTO{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		Roles:      user.Roles,
		Attributes: user.Attributes,
	})
}

// GetUser retrieves a user by ID
func (h *UserHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.authManager.GetUserRepository().GetByID(c.Request.Context(), userID)
	if err != nil {
		if err == security.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		}
		return
	}

	c.JSON(http.StatusOK, UserDTO{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		Roles:      user.Roles,
		Attributes: user.Attributes,
	})
}

// UpdateUser updates a user
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.authManager.GetUserRepository().GetByID(c.Request.Context(), userID)
	if err != nil {
		if err == security.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		}
		return
	}

	// Update fields
	if req.Email != "" {
		user.Email = req.Email
	}

	if req.Password != "" {
		// Validate password against policy
		policy := h.authManager.GetConfig().PasswordPolicy
		if err := policy.ValidatePassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Hash password
		passwordHash, err := security.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
			return
		}
		user.PasswordHash = passwordHash
	}

	if req.Roles != nil {
		user.Roles = req.Roles
	}

	if req.Attributes != nil {
		user.Attributes = req.Attributes
	}

	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}

	user.UpdatedAt = time.Now()

	if err := h.authManager.GetUserRepository().Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogUserAction(
		c.Request.Context(),
		actor,
		security.AuditActionUserUpdate,
		user.ID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, UserDTO{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		Roles:      user.Roles,
		Attributes: user.Attributes,
	})
}

// DeleteUser deletes a user
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	// Check if user exists
	_, err := h.authManager.GetUserRepository().GetByID(c.Request.Context(), userID)
	if err != nil {
		if err == security.ErrUserNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		}
		return
	}

	// Delete user
	if err := h.authManager.GetUserRepository().Delete(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogUserAction(
		c.Request.Context(),
		actor,
		security.AuditActionUserDelete,
		userID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}

// AssignRole assigns a role to a user
func (h *UserHandler) AssignRole(c *gin.Context) {
	userID := c.Param("id")
	roleID := c.Param("roleId")

	rbacManager := h.authManager.GetRBACManager()

	if err := rbacManager.AssignRole(c.Request.Context(), userID, roleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign role"})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogRoleAction(
		c.Request.Context(),
		actor,
		security.AuditActionRoleAssign,
		roleID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "Role assigned successfully"})
}

// RemoveRole removes a role from a user
func (h *UserHandler) RemoveRole(c *gin.Context) {
	userID := c.Param("id")
	roleID := c.Param("roleId")

	rbacManager := h.authManager.GetRBACManager()

	if err := rbacManager.RemoveRole(c.Request.Context(), userID, roleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove role"})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogRoleAction(
		c.Request.Context(),
		actor,
		security.AuditActionRoleRemove,
		roleID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "Role removed successfully"})
}
