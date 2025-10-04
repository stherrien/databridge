package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shawntherrien/databridge/internal/security"
)

// AuditHandler handles audit log endpoints
type AuditHandler struct {
	authManager *security.AuthManager
}

// NewAuditHandler creates a new audit handler
func NewAuditHandler(authManager *security.AuthManager) *AuditHandler {
	return &AuditHandler{
		authManager: authManager,
	}
}

// AuditEventDTO represents an audit event data transfer object
type AuditEventDTO struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	UserID     string                 `json:"user_id"`
	Username   string                 `json:"username"`
	Action     string                 `json:"action"`
	Resource   string                 `json:"resource"`
	ResourceID string                 `json:"resource_id"`
	Result     security.AuditResult   `json:"result"`
	IPAddress  string                 `json:"ip_address"`
	UserAgent  string                 `json:"user_agent"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// QueryAudit queries audit logs
func (h *AuditHandler) QueryAudit(c *gin.Context) {
	auditLogger := h.authManager.GetAuditLogger()

	// Parse query parameters
	filter := security.AuditFilter{
		UserID:   c.Query("user_id"),
		Action:   c.Query("action"),
		Resource: c.Query("resource"),
		Result:   security.AuditResult(c.Query("result")),
		Limit:    100, // Default limit
		Offset:   0,
	}

	// Parse time range
	if startTimeStr := c.Query("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			filter.StartTime = &startTime
		}
	}

	if endTimeStr := c.Query("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			filter.EndTime = &endTime
		}
	}

	events, err := auditLogger.Query(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query audit logs"})
		return
	}

	dtos := make([]AuditEventDTO, len(events))
	for i, event := range events {
		dtos[i] = AuditEventDTO{
			ID:         event.ID,
			Timestamp:  event.Timestamp,
			UserID:     event.UserID,
			Username:   event.Username,
			Action:     event.Action,
			Resource:   event.Resource,
			ResourceID: event.ResourceID,
			Result:     event.Result,
			IPAddress:  event.IPAddress,
			UserAgent:  event.UserAgent,
			Details:    event.Details,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"events": dtos,
		"count":  len(dtos),
	})
}
