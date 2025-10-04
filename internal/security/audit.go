package security

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// AuditResult indicates event outcome
type AuditResult string

const (
	AuditSuccess AuditResult = "success"
	AuditFailure AuditResult = "failure"
	AuditDenied  AuditResult = "denied"
)

// AuditAction defines common audit actions
type AuditAction string

const (
	// Authentication actions
	AuditActionLogin         AuditAction = "login"
	AuditActionLogout        AuditAction = "logout"
	AuditActionRefreshToken  AuditAction = "refresh_token"
	AuditActionLoginFailed   AuditAction = "login_failed"

	// User management actions
	AuditActionUserCreate    AuditAction = "user_create"
	AuditActionUserUpdate    AuditAction = "user_update"
	AuditActionUserDelete    AuditAction = "user_delete"
	AuditActionUserDisable   AuditAction = "user_disable"
	AuditActionUserEnable    AuditAction = "user_enable"

	// Role management actions
	AuditActionRoleCreate    AuditAction = "role_create"
	AuditActionRoleUpdate    AuditAction = "role_update"
	AuditActionRoleDelete    AuditAction = "role_delete"
	AuditActionRoleAssign    AuditAction = "role_assign"
	AuditActionRoleRemove    AuditAction = "role_remove"

	// Permission actions
	AuditActionPermissionGrant  AuditAction = "permission_grant"
	AuditActionPermissionRevoke AuditAction = "permission_revoke"

	// API key actions
	AuditActionAPIKeyCreate  AuditAction = "apikey_create"
	AuditActionAPIKeyRevoke  AuditAction = "apikey_revoke"
	AuditActionAPIKeyDelete  AuditAction = "apikey_delete"
	AuditActionAPIKeyUsed    AuditAction = "apikey_used"

	// Flow management actions
	AuditActionFlowCreate    AuditAction = "flow_create"
	AuditActionFlowUpdate    AuditAction = "flow_update"
	AuditActionFlowDelete    AuditAction = "flow_delete"
	AuditActionFlowStart     AuditAction = "flow_start"
	AuditActionFlowStop      AuditAction = "flow_stop"

	// Processor actions
	AuditActionProcessorCreate AuditAction = "processor_create"
	AuditActionProcessorUpdate AuditAction = "processor_update"
	AuditActionProcessorDelete AuditAction = "processor_delete"
	AuditActionProcessorStart  AuditAction = "processor_start"
	AuditActionProcessorStop   AuditAction = "processor_stop"

	// Configuration actions
	AuditActionConfigUpdate AuditAction = "config_update"
	AuditActionConfigView   AuditAction = "config_view"
)

// AuditEvent records security events
type AuditEvent struct {
	ID         string
	Timestamp  time.Time
	UserID     string
	Username   string
	Action     string
	Resource   string
	ResourceID string
	Result     AuditResult
	IPAddress  string
	UserAgent  string
	Details    map[string]interface{}
}

// String returns a string representation of the audit event
func (e AuditEvent) String() string {
	return fmt.Sprintf("[%s] %s %s %s on %s/%s from %s - %s",
		e.Timestamp.Format(time.RFC3339),
		e.Username,
		e.Action,
		e.Result,
		e.Resource,
		e.ResourceID,
		e.IPAddress,
		e.Result)
}

// ToJSON converts the event to JSON
func (e AuditEvent) ToJSON() (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AuditLogger logs security events
type AuditLogger struct {
	logger *logrus.Logger
	repo   AuditRepository
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *logrus.Logger, repo AuditRepository) *AuditLogger {
	return &AuditLogger{
		logger: logger,
		repo:   repo,
	}
}

// Log logs an audit event
func (al *AuditLogger) Log(ctx context.Context, event AuditEvent) {
	// Generate ID if not provided
	if event.ID == "" {
		event.ID = fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Log to structured logger
	fields := logrus.Fields{
		"audit_id":    event.ID,
		"user_id":     event.UserID,
		"username":    event.Username,
		"action":      event.Action,
		"resource":    event.Resource,
		"resource_id": event.ResourceID,
		"result":      event.Result,
		"ip_address":  event.IPAddress,
		"user_agent":  event.UserAgent,
	}

	// Add details
	for key, value := range event.Details {
		fields[fmt.Sprintf("detail_%s", key)] = value
	}

	// Log based on result
	switch event.Result {
	case AuditSuccess:
		al.logger.WithFields(fields).Info("Audit event")
	case AuditFailure:
		al.logger.WithFields(fields).Warn("Audit event - failure")
	case AuditDenied:
		al.logger.WithFields(fields).Warn("Audit event - denied")
	default:
		al.logger.WithFields(fields).Info("Audit event")
	}

	// Store in repository (fire and forget)
	if al.repo != nil {
		go func() {
			if err := al.repo.Create(ctx, &event); err != nil {
				al.logger.WithError(err).Error("Failed to store audit event")
			}
		}()
	}
}

// LogLogin logs a login event
func (al *AuditLogger) LogLogin(ctx context.Context, user *User, ipAddress, userAgent string, success bool) {
	result := AuditSuccess
	action := string(AuditActionLogin)
	if !success {
		result = AuditFailure
		action = string(AuditActionLoginFailed)
	}

	userID := ""
	username := ""
	if user != nil {
		userID = user.ID
		username = user.Username
	}

	al.Log(ctx, AuditEvent{
		UserID:    userID,
		Username:  username,
		Action:    action,
		Resource:  "authentication",
		Result:    result,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	})
}

// LogLogout logs a logout event
func (al *AuditLogger) LogLogout(ctx context.Context, user *User, ipAddress, userAgent string) {
	al.Log(ctx, AuditEvent{
		UserID:    user.ID,
		Username:  user.Username,
		Action:    string(AuditActionLogout),
		Resource:  "authentication",
		Result:    AuditSuccess,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	})
}

// LogPermissionDenied logs a permission denied event
func (al *AuditLogger) LogPermissionDenied(ctx context.Context, user *User, resource, action, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:    user.ID,
		Username:  user.Username,
		Action:    action,
		Resource:  resource,
		Result:    AuditDenied,
		IPAddress: ipAddress,
	})
}

// LogUserAction logs a user management action
func (al *AuditLogger) LogUserAction(ctx context.Context, actor *User, action AuditAction, targetUserID string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "user",
		ResourceID: targetUserID,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// LogRoleAction logs a role management action
func (al *AuditLogger) LogRoleAction(ctx context.Context, actor *User, action AuditAction, roleID string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "role",
		ResourceID: roleID,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// LogAPIKeyAction logs an API key action
func (al *AuditLogger) LogAPIKeyAction(ctx context.Context, actor *User, action AuditAction, keyID string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "apikey",
		ResourceID: keyID,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// LogFlowAction logs a flow management action
func (al *AuditLogger) LogFlowAction(ctx context.Context, actor *User, action AuditAction, flowID string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "flow",
		ResourceID: flowID,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// LogProcessorAction logs a processor action
func (al *AuditLogger) LogProcessorAction(ctx context.Context, actor *User, action AuditAction, processorID string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "processor",
		ResourceID: processorID,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// LogConfigAction logs a configuration action
func (al *AuditLogger) LogConfigAction(ctx context.Context, actor *User, action AuditAction, configKey string, ipAddress string) {
	al.Log(ctx, AuditEvent{
		UserID:     actor.ID,
		Username:   actor.Username,
		Action:     string(action),
		Resource:   "config",
		ResourceID: configKey,
		Result:     AuditSuccess,
		IPAddress:  ipAddress,
	})
}

// Query queries audit events
func (al *AuditLogger) Query(ctx context.Context, filter AuditFilter) ([]*AuditEvent, error) {
	if al.repo == nil {
		return nil, fmt.Errorf("audit repository not configured")
	}
	return al.repo.Query(ctx, filter)
}

// AuditFilter defines filters for querying audit events
type AuditFilter struct {
	UserID     string
	Action     string
	Resource   string
	Result     AuditResult
	StartTime  *time.Time
	EndTime    *time.Time
	Limit      int
	Offset     int
}
