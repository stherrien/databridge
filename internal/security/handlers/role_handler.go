package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/databridge/internal/security"
)

// RoleHandler handles role management endpoints
type RoleHandler struct {
	authManager *security.AuthManager
}

// NewRoleHandler creates a new role handler
func NewRoleHandler(authManager *security.AuthManager) *RoleHandler {
	return &RoleHandler{
		authManager: authManager,
	}
}

// RoleDTO represents a role data transfer object
type RoleDTO struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Permissions []security.Permission `json:"permissions"`
}

// CreateRoleRequest represents a role creation request
type CreateRoleRequest struct {
	Name        string                `json:"name" binding:"required"`
	Description string                `json:"description"`
	Permissions []security.Permission `json:"permissions"`
}

// ListRoles lists all roles
func (h *RoleHandler) ListRoles(c *gin.Context) {
	rbacManager := h.authManager.GetRBACManager()

	roles, err := rbacManager.ListRoles(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list roles"})
		return
	}

	dtos := make([]RoleDTO, len(roles))
	for i, role := range roles {
		dtos[i] = RoleDTO{
			ID:          role.ID,
			Name:        role.Name,
			Description: role.Description,
			Permissions: role.Permissions,
		}
	}

	c.JSON(http.StatusOK, gin.H{"roles": dtos})
}

// GetRole retrieves a role by ID
func (h *RoleHandler) GetRole(c *gin.Context) {
	roleID := c.Param("id")
	rbacManager := h.authManager.GetRBACManager()

	role, err := rbacManager.GetRole(c.Request.Context(), roleID)
	if err != nil {
		if err == security.ErrRoleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get role"})
		}
		return
	}

	c.JSON(http.StatusOK, RoleDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: role.Permissions,
	})
}

// CreateRole creates a new role
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rbacManager := h.authManager.GetRBACManager()

	role := &security.Role{
		ID:          "role_" + req.Name,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	if err := rbacManager.CreateRole(c.Request.Context(), role); err != nil {
		if err == security.ErrRoleExists {
			c.JSON(http.StatusConflict, gin.H{"error": "Role already exists"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create role"})
		}
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogRoleAction(
		c.Request.Context(),
		actor,
		security.AuditActionRoleCreate,
		role.ID,
		c.ClientIP(),
	)

	c.JSON(http.StatusCreated, RoleDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: role.Permissions,
	})
}

// UpdateRole updates a role
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	roleID := c.Param("id")

	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rbacManager := h.authManager.GetRBACManager()

	role, err := rbacManager.GetRole(c.Request.Context(), roleID)
	if err != nil {
		if err == security.ErrRoleNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Role not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get role"})
		}
		return
	}

	// Update role
	role.Description = req.Description
	role.Permissions = req.Permissions

	if err := rbacManager.UpdateRole(c.Request.Context(), role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogRoleAction(
		c.Request.Context(),
		actor,
		security.AuditActionRoleUpdate,
		role.ID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, RoleDTO{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		Permissions: role.Permissions,
	})
}

// DeleteRole deletes a role
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	roleID := c.Param("id")
	rbacManager := h.authManager.GetRBACManager()

	if err := rbacManager.DeleteRole(c.Request.Context(), roleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Log audit event
	actor := security.GetUserFromContext(c)
	h.authManager.GetAuditLogger().LogRoleAction(
		c.Request.Context(),
		actor,
		security.AuditActionRoleDelete,
		roleID,
		c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "Role deleted successfully"})
}
