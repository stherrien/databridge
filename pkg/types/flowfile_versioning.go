package types

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FlowFileVersion represents a versioned snapshot of a FlowFile
type FlowFileVersion struct {
	Version      int               `json:"version"`
	FlowFileID   uuid.UUID         `json:"flowFileId"`
	Timestamp    time.Time         `json:"timestamp"`
	Attributes   map[string]string `json:"attributes"`
	Size         int64             `json:"size"`
	ContentClaim *ContentClaim     `json:"contentClaim,omitempty"`
	ChangeType   string            `json:"changeType"` // "CREATE", "UPDATE", "CLONE"
	ChangedBy    string            `json:"changedBy"`  // Processor ID or user
	ChangeReason string            `json:"changeReason,omitempty"`
}

// FlowFileVersionHistory tracks all versions of a FlowFile
type FlowFileVersionHistory struct {
	FlowFileID      uuid.UUID          `json:"flowFileId"`
	CurrentVersion  int                `json:"currentVersion"`
	Versions        []FlowFileVersion  `json:"versions"`
	MaxVersions     int                `json:"maxVersions"` // 0 = unlimited
	RetentionPolicy RetentionPolicy    `json:"retentionPolicy"`
}

// RetentionPolicy defines version retention rules
type RetentionPolicy struct {
	MaxAge      time.Duration `json:"maxAge"`      // Keep versions for this duration
	MaxVersions int           `json:"maxVersions"` // Maximum number of versions to keep
	KeepFirst   bool          `json:"keepFirst"`   // Always keep the first version
	KeepLatest  int           `json:"keepLatest"`  // Always keep N latest versions
}

// FlowFileVersionManager manages FlowFile versioning
type FlowFileVersionManager struct {
	histories map[uuid.UUID]*FlowFileVersionHistory
	policy    RetentionPolicy
}

// NewFlowFileVersionManager creates a new version manager
func NewFlowFileVersionManager() *FlowFileVersionManager {
	return &FlowFileVersionManager{
		histories: make(map[uuid.UUID]*FlowFileVersionHistory),
		policy: RetentionPolicy{
			MaxAge:      24 * time.Hour * 30, // 30 days default
			MaxVersions: 100,                 // Keep last 100 versions
			KeepFirst:   true,
			KeepLatest:  10,
		},
	}
}

// SetRetentionPolicy sets the retention policy
func (m *FlowFileVersionManager) SetRetentionPolicy(policy RetentionPolicy) {
	m.policy = policy
}

// CreateVersion creates a new version of a FlowFile
func (m *FlowFileVersionManager) CreateVersion(ff *FlowFile, changeType, changedBy, reason string) (*FlowFileVersion, error) {
	history, exists := m.histories[ff.ID]
	if !exists {
		// Create new history
		history = &FlowFileVersionHistory{
			FlowFileID:      ff.ID,
			CurrentVersion:  0,
			Versions:        make([]FlowFileVersion, 0),
			RetentionPolicy: m.policy,
		}
		m.histories[ff.ID] = history
	}

	// Create version snapshot
	version := FlowFileVersion{
		Version:      history.CurrentVersion + 1,
		FlowFileID:   ff.ID,
		Timestamp:    time.Now(),
		Size:         ff.Size,
		ChangeType:   changeType,
		ChangedBy:    changedBy,
		ChangeReason: reason,
	}

	// Deep copy attributes
	version.Attributes = make(map[string]string)
	for k, v := range ff.Attributes {
		version.Attributes[k] = v
	}

	// Copy content claim
	if ff.ContentClaim != nil {
		version.ContentClaim = &ContentClaim{
			ID:        ff.ContentClaim.ID,
			Container: ff.ContentClaim.Container,
			Section:   ff.ContentClaim.Section,
			Offset:    ff.ContentClaim.Offset,
			Length:    ff.ContentClaim.Length,
			RefCount:  ff.ContentClaim.RefCount,
		}
	}

	// Add version to history
	history.Versions = append(history.Versions, version)
	history.CurrentVersion = version.Version

	// Apply retention policy
	m.applyRetentionPolicy(history)

	return &version, nil
}

// GetVersion retrieves a specific version
func (m *FlowFileVersionManager) GetVersion(flowFileID uuid.UUID, version int) (*FlowFileVersion, error) {
	history, exists := m.histories[flowFileID]
	if !exists {
		return nil, fmt.Errorf("no version history found for FlowFile %s", flowFileID)
	}

	for _, v := range history.Versions {
		if v.Version == version {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("version %d not found for FlowFile %s", version, flowFileID)
}

// GetLatestVersion retrieves the most recent version
func (m *FlowFileVersionManager) GetLatestVersion(flowFileID uuid.UUID) (*FlowFileVersion, error) {
	history, exists := m.histories[flowFileID]
	if !exists || len(history.Versions) == 0 {
		return nil, fmt.Errorf("no versions found for FlowFile %s", flowFileID)
	}

	return &history.Versions[len(history.Versions)-1], nil
}

// GetHistory retrieves the complete version history
func (m *FlowFileVersionManager) GetHistory(flowFileID uuid.UUID) (*FlowFileVersionHistory, error) {
	history, exists := m.histories[flowFileID]
	if !exists {
		return nil, fmt.Errorf("no version history found for FlowFile %s", flowFileID)
	}

	return history, nil
}

// ListVersions lists all version numbers for a FlowFile
func (m *FlowFileVersionManager) ListVersions(flowFileID uuid.UUID) ([]int, error) {
	history, exists := m.histories[flowFileID]
	if !exists {
		return nil, fmt.Errorf("no version history found for FlowFile %s", flowFileID)
	}

	versions := make([]int, len(history.Versions))
	for i, v := range history.Versions {
		versions[i] = v.Version
	}

	return versions, nil
}

// RollbackToVersion rolls back a FlowFile to a specific version
func (m *FlowFileVersionManager) RollbackToVersion(ff *FlowFile, version int) error {
	targetVersion, err := m.GetVersion(ff.ID, version)
	if err != nil {
		return err
	}

	// Restore attributes
	ff.Attributes = make(map[string]string)
	for k, v := range targetVersion.Attributes {
		ff.Attributes[k] = v
	}

	// Restore size
	ff.Size = targetVersion.Size

	// Restore content claim
	if targetVersion.ContentClaim != nil {
		ff.ContentClaim = &ContentClaim{
			ID:        targetVersion.ContentClaim.ID,
			Container: targetVersion.ContentClaim.Container,
			Section:   targetVersion.ContentClaim.Section,
			Offset:    targetVersion.ContentClaim.Offset,
			Length:    targetVersion.ContentClaim.Length,
			RefCount:  targetVersion.ContentClaim.RefCount,
		}
	}

	ff.UpdatedAt = time.Now()

	// Create a new version recording the rollback
	_, err = m.CreateVersion(ff, "ROLLBACK", "system", fmt.Sprintf("Rolled back to version %d", version))
	return err
}

// CompareVersions compares two versions and returns the differences
func (m *FlowFileVersionManager) CompareVersions(flowFileID uuid.UUID, v1, v2 int) (map[string]interface{}, error) {
	version1, err := m.GetVersion(flowFileID, v1)
	if err != nil {
		return nil, err
	}

	version2, err := m.GetVersion(flowFileID, v2)
	if err != nil {
		return nil, err
	}

	diff := make(map[string]interface{})

	// Compare attributes
	attrDiff := make(map[string]map[string]string)
	for k, v1Val := range version1.Attributes {
		if v2Val, exists := version2.Attributes[k]; !exists || v1Val != v2Val {
			attrDiff[k] = map[string]string{
				"v1": v1Val,
				"v2": v2Val,
			}
		}
	}
	for k, v2Val := range version2.Attributes {
		if _, exists := version1.Attributes[k]; !exists {
			attrDiff[k] = map[string]string{
				"v1": "",
				"v2": v2Val,
			}
		}
	}

	if len(attrDiff) > 0 {
		diff["attributes"] = attrDiff
	}

	// Compare size
	if version1.Size != version2.Size {
		diff["size"] = map[string]int64{
			"v1": version1.Size,
			"v2": version2.Size,
		}
	}

	// Compare content claim
	if !contentClaimsEqual(version1.ContentClaim, version2.ContentClaim) {
		diff["contentClaim"] = map[string]*ContentClaim{
			"v1": version1.ContentClaim,
			"v2": version2.ContentClaim,
		}
	}

	return diff, nil
}

// applyRetentionPolicy removes old versions based on retention policy
func (m *FlowFileVersionManager) applyRetentionPolicy(history *FlowFileVersionHistory) {
	policy := history.RetentionPolicy
	now := time.Now()

	// Find versions to keep
	keepVersions := make([]FlowFileVersion, 0)

	for i, version := range history.Versions {
		keep := false

		// Always keep first version if policy says so
		if policy.KeepFirst && i == 0 {
			keep = true
		}

		// Keep if within max age
		if policy.MaxAge > 0 && now.Sub(version.Timestamp) <= policy.MaxAge {
			keep = true
		}

		// Keep latest N versions
		if policy.KeepLatest > 0 && i >= len(history.Versions)-policy.KeepLatest {
			keep = true
		}

		if keep {
			keepVersions = append(keepVersions, version)
		}
	}

	// Apply max versions limit if specified
	if policy.MaxVersions > 0 && len(keepVersions) > policy.MaxVersions {
		// Keep most recent versions within limit
		start := len(keepVersions) - policy.MaxVersions
		keepVersions = keepVersions[start:]
	}

	history.Versions = keepVersions
}

// contentClaimsEqual compares two content claims
func contentClaimsEqual(c1, c2 *ContentClaim) bool {
	if c1 == nil && c2 == nil {
		return true
	}
	if c1 == nil || c2 == nil {
		return false
	}
	return c1.ID == c2.ID &&
		c1.Container == c2.Container &&
		c1.Section == c2.Section &&
		c1.Offset == c2.Offset &&
		c1.Length == c2.Length
}

// ExportHistory exports version history to JSON
func (m *FlowFileVersionManager) ExportHistory(flowFileID uuid.UUID) ([]byte, error) {
	history, err := m.GetHistory(flowFileID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(history, "", "  ")
}

// ImportHistory imports version history from JSON
func (m *FlowFileVersionManager) ImportHistory(data []byte) error {
	var history FlowFileVersionHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return fmt.Errorf("failed to unmarshal history: %w", err)
	}

	m.histories[history.FlowFileID] = &history
	return nil
}

// PurgeHistory removes all version history for a FlowFile
func (m *FlowFileVersionManager) PurgeHistory(flowFileID uuid.UUID) {
	delete(m.histories, flowFileID)
}

// GetStats returns statistics about version management
func (m *FlowFileVersionManager) GetStats() map[string]interface{} {
	totalFlowFiles := len(m.histories)
	totalVersions := 0
	avgVersionsPerFlowFile := 0.0

	for _, history := range m.histories {
		totalVersions += len(history.Versions)
	}

	if totalFlowFiles > 0 {
		avgVersionsPerFlowFile = float64(totalVersions) / float64(totalFlowFiles)
	}

	return map[string]interface{}{
		"totalFlowFiles":         totalFlowFiles,
		"totalVersions":          totalVersions,
		"avgVersionsPerFlowFile": avgVersionsPerFlowFile,
		"retentionPolicy":        m.policy,
	}
}
