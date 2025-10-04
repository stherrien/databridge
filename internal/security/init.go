package security

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// InitializeSecurity initializes the security system with default admin user
func InitializeSecurity(ctx context.Context, authManager *AuthManager, logger *logrus.Logger) error {
	logger.Info("Initializing security system...")

	// Initialize RBAC (creates predefined roles)
	if err := authManager.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize auth manager: %w", err)
	}

	// Create default admin user if no users exist
	userRepo := authManager.GetUserRepository()
	users, err := userRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	if len(users) == 0 {
		logger.Info("No users found, creating default admin user")

		// Create default admin user
		// WARNING: Change this password immediately in production!
		passwordHash, err := HashPassword("admin123")
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		adminUser := &User{
			ID:           "user_admin",
			Username:     "admin",
			Email:        "admin@databridge.local",
			PasswordHash: passwordHash,
			Roles:        []string{RoleAdmin},
			Attributes:   map[string]string{},
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			Enabled:      true,
		}

		if err := userRepo.Create(ctx, adminUser); err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}

		// Assign admin role
		rbacManager := authManager.GetRBACManager()
		adminRole, err := rbacManager.GetRoleByName(ctx, RoleAdmin)
		if err != nil {
			return fmt.Errorf("failed to get admin role: %w", err)
		}

		if err := rbacManager.AssignRole(ctx, adminUser.ID, adminRole.ID); err != nil {
			return fmt.Errorf("failed to assign admin role: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"username": adminUser.Username,
			"password": "admin123",
		}).Warn("Default admin user created - CHANGE PASSWORD IMMEDIATELY!")
	}

	logger.Info("Security system initialized successfully")
	return nil
}

// CreateDefaultSecuritySystem creates a complete security system with in-memory repositories
func CreateDefaultSecuritySystem(config SecurityConfig, logger *logrus.Logger) (*AuthManager, error) {
	// Create repositories
	userRepo := NewInMemoryUserRepository()
	roleRepo := NewInMemoryRoleRepository()
	apiKeyRepo := NewInMemoryAPIKeyRepository()
	auditRepo := NewInMemoryAuditRepository()

	// Create auth manager
	authManager, err := NewAuthManager(
		config,
		userRepo,
		roleRepo,
		apiKeyRepo,
		auditRepo,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth manager: %w", err)
	}

	return authManager, nil
}
