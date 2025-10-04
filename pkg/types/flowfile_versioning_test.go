package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewFlowFileVersionManager(t *testing.T) {
	manager := NewFlowFileVersionManager()
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.histories)
	assert.NotZero(t, manager.policy.MaxAge)
	assert.NotZero(t, manager.policy.MaxVersions)
}

func TestCreateVersion(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()
	ff.Attributes["test"] = "value"
	ff.Size = 100

	t.Run("Create First Version", func(t *testing.T) {
		version, err := manager.CreateVersion(ff, "CREATE", "test-processor", "initial version")
		assert.NoError(t, err)
		assert.NotNil(t, version)
		assert.Equal(t, 1, version.Version)
		assert.Equal(t, "CREATE", version.ChangeType)
		assert.Equal(t, "test-processor", version.ChangedBy)
		assert.Equal(t, int64(100), version.Size)
		assert.Equal(t, "value", version.Attributes["test"])
	})

	t.Run("Create Multiple Versions", func(t *testing.T) {
		// Modify FlowFile
		ff.Attributes["test"] = "updated"
		ff.Size = 200

		version, err := manager.CreateVersion(ff, "UPDATE", "test-processor", "updated value")
		assert.NoError(t, err)
		assert.Equal(t, 2, version.Version)
		assert.Equal(t, "updated", version.Attributes["test"])
		assert.Equal(t, int64(200), version.Size)
	})
}

func TestGetVersion(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()

	// Create versions
	manager.CreateVersion(ff, "CREATE", "processor", "v1")
	ff.Attributes["data"] = "v2"
	manager.CreateVersion(ff, "UPDATE", "processor", "v2")
	ff.Attributes["data"] = "v3"
	manager.CreateVersion(ff, "UPDATE", "processor", "v3")

	t.Run("Get Existing Version", func(t *testing.T) {
		version, err := manager.GetVersion(ff.ID, 2)
		assert.NoError(t, err)
		assert.NotNil(t, version)
		assert.Equal(t, 2, version.Version)
		assert.Equal(t, "v2", version.Attributes["data"])
	})

	t.Run("Get Non-existent Version", func(t *testing.T) {
		_, err := manager.GetVersion(ff.ID, 999)
		assert.Error(t, err)
	})

	t.Run("Get Version for Unknown FlowFile", func(t *testing.T) {
		unknownFF := NewFlowFile()
		_, err := manager.GetVersion(unknownFF.ID, 1)
		assert.Error(t, err)
	})
}

func TestGetLatestVersion(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()

	manager.CreateVersion(ff, "CREATE", "processor", "v1")
	ff.Attributes["data"] = "latest"
	manager.CreateVersion(ff, "UPDATE", "processor", "v2")

	version, err := manager.GetLatestVersion(ff.ID)
	assert.NoError(t, err)
	assert.Equal(t, 2, version.Version)
	assert.Equal(t, "latest", version.Attributes["data"])
}

func TestListVersions(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()

	manager.CreateVersion(ff, "CREATE", "processor", "v1")
	manager.CreateVersion(ff, "UPDATE", "processor", "v2")
	manager.CreateVersion(ff, "UPDATE", "processor", "v3")

	versions, err := manager.ListVersions(ff.ID)
	assert.NoError(t, err)
	assert.Len(t, versions, 3)
	assert.Equal(t, []int{1, 2, 3}, versions)
}

func TestRollbackToVersion(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()
	ff.Attributes["key"] = "original"
	ff.Size = 100

	// Create versions
	manager.CreateVersion(ff, "CREATE", "processor", "v1")

	ff.Attributes["key"] = "modified"
	ff.Size = 200
	manager.CreateVersion(ff, "UPDATE", "processor", "v2")

	// Rollback to version 1
	err := manager.RollbackToVersion(ff, 1)
	assert.NoError(t, err)
	assert.Equal(t, "original", ff.Attributes["key"])
	assert.Equal(t, int64(100), ff.Size)

	// Verify rollback created a new version
	history, _ := manager.GetHistory(ff.ID)
	assert.Equal(t, 3, history.CurrentVersion)
}

func TestCompareVersions(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()
	ff.Attributes["key1"] = "value1"
	ff.Size = 100

	manager.CreateVersion(ff, "CREATE", "processor", "v1")

	ff.Attributes["key1"] = "updated"
	ff.Attributes["key2"] = "new"
	ff.Size = 200
	manager.CreateVersion(ff, "UPDATE", "processor", "v2")

	diff, err := manager.CompareVersions(ff.ID, 1, 2)
	assert.NoError(t, err)
	assert.NotNil(t, diff)

	// Check attribute differences
	attrDiff := diff["attributes"].(map[string]map[string]string)
	assert.Contains(t, attrDiff, "key1")
	assert.Contains(t, attrDiff, "key2")

	// Check size difference
	sizeDiff := diff["size"].(map[string]int64)
	assert.Equal(t, int64(100), sizeDiff["v1"])
	assert.Equal(t, int64(200), sizeDiff["v2"])
}

func TestRetentionPolicy(t *testing.T) {
	manager := NewFlowFileVersionManager()

	t.Run("MaxVersions Policy", func(t *testing.T) {
		policy := RetentionPolicy{
			MaxVersions: 3,
			KeepLatest:  3,
		}
		manager.SetRetentionPolicy(policy)

		ff := NewFlowFile()
		// Create 5 versions
		for i := 1; i <= 5; i++ {
			manager.CreateVersion(ff, "UPDATE", "processor", "version")
		}

		history, _ := manager.GetHistory(ff.ID)
		assert.LessOrEqual(t, len(history.Versions), 3)
	})

	t.Run("KeepFirst Policy", func(t *testing.T) {
		policy := RetentionPolicy{
			MaxVersions: 2,
			KeepFirst:   true,
			KeepLatest:  1,
		}
		manager.SetRetentionPolicy(policy)

		ff := NewFlowFile()
		for i := 1; i <= 5; i++ {
			manager.CreateVersion(ff, "UPDATE", "processor", "version")
		}

		history, _ := manager.GetHistory(ff.ID)
		// Should keep first and latest
		firstVersion := history.Versions[0]
		assert.Equal(t, 1, firstVersion.Version)
	})

	t.Run("MaxAge Policy", func(t *testing.T) {
		manager := NewFlowFileVersionManager()
		policy := RetentionPolicy{
			MaxAge:     1 * time.Hour,
			KeepLatest: 1,
		}
		manager.SetRetentionPolicy(policy)

		ff := NewFlowFile()
		manager.CreateVersion(ff, "CREATE", "processor", "old version")

		// Manually set old timestamp
		history, _ := manager.GetHistory(ff.ID)
		history.Versions[0].Timestamp = time.Now().Add(-2 * time.Hour)

		// Create new version to trigger retention
		manager.CreateVersion(ff, "UPDATE", "processor", "new version")

		history, _ = manager.GetHistory(ff.ID)
		// Old version should be removed
		assert.LessOrEqual(t, len(history.Versions), 2)
	})
}

func TestExportImportHistory(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()
	ff.Attributes["test"] = "data"

	manager.CreateVersion(ff, "CREATE", "processor", "initial")
	manager.CreateVersion(ff, "UPDATE", "processor", "update")

	// Export
	data, err := manager.ExportHistory(ff.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Clear and import
	manager.PurgeHistory(ff.ID)
	err = manager.ImportHistory(data)
	assert.NoError(t, err)

	// Verify imported data
	history, err := manager.GetHistory(ff.ID)
	assert.NoError(t, err)
	assert.Len(t, history.Versions, 2)
}

func TestPurgeHistory(t *testing.T) {
	manager := NewFlowFileVersionManager()
	ff := NewFlowFile()

	manager.CreateVersion(ff, "CREATE", "processor", "v1")
	_, err := manager.GetHistory(ff.ID)
	assert.NoError(t, err)

	manager.PurgeHistory(ff.ID)
	_, err = manager.GetHistory(ff.ID)
	assert.Error(t, err)
}

func TestGetStats(t *testing.T) {
	manager := NewFlowFileVersionManager()

	ff1 := NewFlowFile()
	ff2 := NewFlowFile()

	manager.CreateVersion(ff1, "CREATE", "processor", "v1")
	manager.CreateVersion(ff1, "UPDATE", "processor", "v2")
	manager.CreateVersion(ff2, "CREATE", "processor", "v1")

	stats := manager.GetStats()
	assert.Equal(t, 2, stats["totalFlowFiles"])
	assert.Equal(t, 3, stats["totalVersions"])
	assert.Equal(t, 1.5, stats["avgVersionsPerFlowFile"])
}

func TestContentClaimsEqual(t *testing.T) {
	t.Run("Both Nil", func(t *testing.T) {
		assert.True(t, contentClaimsEqual(nil, nil))
	})

	t.Run("One Nil", func(t *testing.T) {
		claim := &ContentClaim{Length: 100}
		assert.False(t, contentClaimsEqual(claim, nil))
		assert.False(t, contentClaimsEqual(nil, claim))
	})

	t.Run("Equal Claims", func(t *testing.T) {
		claim1 := &ContentClaim{
			Container: "test",
			Section:   "section1",
			Offset:    0,
			Length:    100,
		}
		claim2 := &ContentClaim{
			Container: "test",
			Section:   "section1",
			Offset:    0,
			Length:    100,
		}
		assert.True(t, contentClaimsEqual(claim1, claim2))
	})

	t.Run("Different Claims", func(t *testing.T) {
		claim1 := &ContentClaim{Length: 100}
		claim2 := &ContentClaim{Length: 200}
		assert.False(t, contentClaimsEqual(claim1, claim2))
	})
}
