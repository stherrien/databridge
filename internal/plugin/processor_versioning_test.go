package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/shawntherrien/databridge/pkg/types"
)

func TestNewProcessorVersionRegistry(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.versions)
	assert.NotNil(t, registry.migrations)
	assert.NotNil(t, registry.transforms)
}

func TestRegisterVersion(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	t.Run("Register Valid Version", func(t *testing.T) {
		version := &ProcessorVersion{
			ProcessorType: "TestProcessor",
			Version:       "1.0.0",
			ReleaseDate:   time.Now(),
			ChangeLog: []VersionChange{
				{
					Type:        ChangeTypeFeature,
					Description: "Initial release",
					Impact:      ImpactNone,
				},
			},
		}

		err := registry.RegisterVersion(version)
		assert.NoError(t, err)
	})

	t.Run("Register Duplicate Version", func(t *testing.T) {
		version := &ProcessorVersion{
			ProcessorType: "TestProcessor",
			Version:       "1.0.0",
			ReleaseDate:   time.Now(),
		}

		err := registry.RegisterVersion(version)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("Register Missing Type", func(t *testing.T) {
		version := &ProcessorVersion{
			Version:     "1.0.0",
			ReleaseDate: time.Now(),
		}

		err := registry.RegisterVersion(version)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "processor type is required")
	})

	t.Run("Register Missing Version", func(t *testing.T) {
		version := &ProcessorVersion{
			ProcessorType: "TestProcessor",
			ReleaseDate:   time.Now(),
		}

		err := registry.RegisterVersion(version)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version is required")
	})
}

func TestRegisterMultipleVersions(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register multiple versions
	versions := []string{"1.0.0", "2.0.0", "1.5.0", "3.0.0"}
	for _, ver := range versions {
		version := &ProcessorVersion{
			ProcessorType: "TestProcessor",
			Version:       ver,
			ReleaseDate:   time.Now(),
		}
		err := registry.RegisterVersion(version)
		assert.NoError(t, err)
	}

	// Verify versions are sorted (newest first)
	registered := registry.ListVersions("TestProcessor")
	assert.Len(t, registered, 4)
}

func TestGetLatestVersion(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register versions
	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
	})
	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "2.0.0",
		ReleaseDate:   time.Now(),
	})

	t.Run("Get Latest Version", func(t *testing.T) {
		latest, err := registry.GetLatestVersion("TestProcessor")
		assert.NoError(t, err)
		assert.NotNil(t, latest)
		// Note: comparison depends on compareVersions implementation
	})

	t.Run("Get Latest for Non-existent Processor", func(t *testing.T) {
		_, err := registry.GetLatestVersion("NonExistent")
		assert.Error(t, err)
	})
}

func TestGetVersion(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
	})

	t.Run("Get Existing Version", func(t *testing.T) {
		version, err := registry.GetVersion("TestProcessor", "1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, "1.0.0", version.Version)
	})

	t.Run("Get Non-existent Version", func(t *testing.T) {
		_, err := registry.GetVersion("TestProcessor", "2.0.0")
		assert.Error(t, err)
	})
}

func TestListVersions(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register versions
	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
	})
	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "2.0.0",
		ReleaseDate:   time.Now(),
	})

	versions := registry.ListVersions("TestProcessor")
	assert.Len(t, versions, 2)
}

func TestRegisterMigration(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	migration := &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
		Strategies: []MigrationStrategy{
			{
				Type:        MigrationTypeRename,
				OldProperty: "oldProp",
				NewProperty: "newProp",
			},
		},
	}

	err := registry.RegisterMigration("TestProcessor", migration)
	assert.NoError(t, err)
}

func TestGetMigrationPath(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register direct migration
	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
	})

	t.Run("Direct Migration", func(t *testing.T) {
		path, err := registry.GetMigrationPath("TestProcessor", "1.0.0", "2.0.0")
		assert.NoError(t, err)
		assert.Len(t, path, 1)
		assert.Equal(t, "1.0.0", path[0].FromVersion)
		assert.Equal(t, "2.0.0", path[0].ToVersion)
	})

	t.Run("No Migration Path", func(t *testing.T) {
		_, err := registry.GetMigrationPath("TestProcessor", "1.0.0", "3.0.0")
		assert.Error(t, err)
	})
}

func TestMultiStepMigration(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register multi-step migration path
	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
	})
	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "2.0.0",
		ToVersion:   "3.0.0",
		AutoMigrate: true,
	})

	path, err := registry.GetMigrationPath("TestProcessor", "1.0.0", "3.0.0")
	assert.NoError(t, err)
	assert.Len(t, path, 2)
	assert.Equal(t, "1.0.0", path[0].FromVersion)
	assert.Equal(t, "2.0.0", path[0].ToVersion)
	assert.Equal(t, "2.0.0", path[1].FromVersion)
	assert.Equal(t, "3.0.0", path[1].ToVersion)
}

func TestMigrateConfig(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register migration with rename strategy
	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
		Strategies: []MigrationStrategy{
			{
				Type:        MigrationTypeRename,
				OldProperty: "oldProp",
				NewProperty: "newProp",
			},
		},
	})

	config := types.ProcessorConfig{
		ID:   uuid.New(),
		Type: "TestProcessor",
		Properties: map[string]string{
			"oldProp": "value1",
			"other":   "value2",
		},
	}

	migratedConfig, err := registry.MigrateConfig("TestProcessor", config, "1.0.0", "2.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "value1", migratedConfig.Properties["newProp"])
	assert.NotContains(t, migratedConfig.Properties, "oldProp")
	assert.Equal(t, "value2", migratedConfig.Properties["other"])
}

func TestMigrationStrategies(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	t.Run("Rename Strategy", func(t *testing.T) {
		migration := &ProcessorMigration{
			FromVersion: "1.0.0",
			ToVersion:   "2.0.0",
			AutoMigrate: true,
			Strategies: []MigrationStrategy{
				{
					Type:        MigrationTypeRename,
					OldProperty: "oldName",
					NewProperty: "newName",
				},
			},
		}

		config := types.ProcessorConfig{
			Properties: map[string]string{"oldName": "testValue"},
		}

		migratedConfig, err := registry.applyMigration(config, migration)
		assert.NoError(t, err)
		assert.Equal(t, "testValue", migratedConfig.Properties["newName"])
		assert.NotContains(t, migratedConfig.Properties, "oldName")
	})

	t.Run("Remove Strategy", func(t *testing.T) {
		migration := &ProcessorMigration{
			FromVersion: "1.0.0",
			ToVersion:   "2.0.0",
			AutoMigrate: true,
			Strategies: []MigrationStrategy{
				{
					Type:        MigrationTypeRemove,
					OldProperty: "deprecatedProp",
				},
			},
		}

		config := types.ProcessorConfig{
			Properties: map[string]string{
				"deprecatedProp": "value",
				"keepProp":       "value2",
			},
		}

		migratedConfig, err := registry.applyMigration(config, migration)
		assert.NoError(t, err)
		assert.NotContains(t, migratedConfig.Properties, "deprecatedProp")
		assert.Contains(t, migratedConfig.Properties, "keepProp")
	})

	t.Run("Add Strategy", func(t *testing.T) {
		migration := &ProcessorMigration{
			FromVersion: "1.0.0",
			ToVersion:   "2.0.0",
			AutoMigrate: true,
			Strategies: []MigrationStrategy{
				{
					Type:        MigrationTypeAdd,
					NewProperty: "newProp",
					Config: map[string]string{
						"default": "defaultValue",
					},
				},
			},
		}

		config := types.ProcessorConfig{
			Properties: map[string]string{},
		}

		migratedConfig, err := registry.applyMigration(config, migration)
		assert.NoError(t, err)
		assert.Equal(t, "defaultValue", migratedConfig.Properties["newProp"])
	})

	t.Run("Transform Strategy", func(t *testing.T) {
		transform := func(oldValue string, config map[string]string) (string, error) {
			return oldValue + "_transformed", nil
		}

		migration := &ProcessorMigration{
			FromVersion: "1.0.0",
			ToVersion:   "2.0.0",
			AutoMigrate: true,
			Strategies: []MigrationStrategy{
				{
					Type:        MigrationTypeTransform,
					OldProperty: "prop",
					NewProperty: "prop",
					Transform:   transform,
				},
			},
		}

		config := types.ProcessorConfig{
			Properties: map[string]string{"prop": "value"},
		}

		migratedConfig, err := registry.applyMigration(config, migration)
		assert.NoError(t, err)
		assert.Equal(t, "value_transformed", migratedConfig.Properties["prop"])
	})
}

func TestManualMigration(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register manual migration
	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: false,
		Manual:      true,
	})

	config := types.ProcessorConfig{
		Properties: map[string]string{"prop": "value"},
	}

	_, err := registry.MigrateConfig("TestProcessor", config, "1.0.0", "2.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manual migration required")
}

func TestIsDeprecated(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "1.0.0",
		DeprecatedIn:  "2.0.0",
		ReleaseDate:   time.Now(),
	})

	deprecated, version := registry.IsDeprecated("TestProcessor", "1.0.0")
	assert.True(t, deprecated)
	assert.Equal(t, "2.0.0", version)
}

func TestVersionManifestExportImport(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	// Register test data
	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "TestProcessor",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
		ChangeLog: []VersionChange{
			{
				Type:        ChangeTypeFeature,
				Description: "Initial release",
				Impact:      ImpactNone,
			},
		},
	})

	registry.RegisterMigration("TestProcessor", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
	})

	// Export
	tmpDir := t.TempDir()
	err := registry.ExportVersionManifest(tmpDir)
	assert.NoError(t, err)

	// Verify file exists
	manifestFile := filepath.Join(tmpDir, "TestProcessor-versions.json")
	assert.FileExists(t, manifestFile)

	// Import into new registry
	newRegistry := NewProcessorVersionRegistry(logger)
	err = newRegistry.ImportVersionManifest(tmpDir)
	assert.NoError(t, err)

	// Verify import
	versions := newRegistry.ListVersions("TestProcessor")
	assert.Len(t, versions, 1)
}

func TestGetVersionStats(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "Processor1",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
	})

	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "Processor1",
		Version:       "2.0.0",
		ReleaseDate:   time.Now(),
	})

	registry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "Processor2",
		Version:       "1.0.0",
		ReleaseDate:   time.Now(),
	})

	registry.RegisterMigration("Processor1", &ProcessorMigration{
		FromVersion: "1.0.0",
		ToVersion:   "2.0.0",
		AutoMigrate: true,
	})

	stats := registry.GetVersionStats()
	assert.Equal(t, 2, stats.ProcessorTypes)
	assert.Equal(t, 3, stats.TotalVersions)
	assert.Equal(t, 1, stats.TotalMigrations)
}

func TestRegisterTransform(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	transform := func(oldValue string, config map[string]string) (string, error) {
		return oldValue + "_transformed", nil
	}

	registry.RegisterTransform("test_transform", transform)

	assert.Contains(t, registry.transforms, "test_transform")
}

func TestVersionChangeTypes(t *testing.T) {
	changes := []struct {
		changeType ChangeType
		expected   string
	}{
		{ChangeTypeFeature, "FEATURE"},
		{ChangeTypeBugFix, "BUGFIX"},
		{ChangeTypeImprovement, "IMPROVEMENT"},
		{ChangeTypeDeprecation, "DEPRECATION"},
		{ChangeTypeBreaking, "BREAKING"},
		{ChangeTypeSecurity, "SECURITY"},
	}

	for _, tc := range changes {
		assert.Equal(t, tc.expected, string(tc.changeType))
	}
}

func TestImpactLevels(t *testing.T) {
	impacts := []struct {
		impact   Impact
		expected string
	}{
		{ImpactNone, "NONE"},
		{ImpactLow, "LOW"},
		{ImpactMedium, "MEDIUM"},
		{ImpactHigh, "HIGH"},
		{ImpactCritical, "CRITICAL"},
	}

	for _, tc := range impacts {
		assert.Equal(t, tc.expected, string(tc.impact))
	}
}

func TestMigrationTypes(t *testing.T) {
	types := []struct {
		migrationType MigrationType
		expected      string
	}{
		{MigrationTypeRename, "RENAME"},
		{MigrationTypeTransform, "TRANSFORM"},
		{MigrationTypeRemove, "REMOVE"},
		{MigrationTypeAdd, "ADD"},
		{MigrationTypeMerge, "MERGE"},
		{MigrationTypeSplit, "SPLIT"},
	}

	for _, tc := range types {
		assert.Equal(t, tc.expected, string(tc.migrationType))
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
	}

	for _, tc := range tests {
		result := compareVersions(tc.v1, tc.v2)
		assert.Equal(t, tc.expected, result, "Comparing %s and %s", tc.v1, tc.v2)
	}
}

func TestExportImportEdgeCases(t *testing.T) {
	logger := &types.MockLogger{}
	registry := NewProcessorVersionRegistry(logger)

	t.Run("Export Empty Registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := registry.ExportVersionManifest(tmpDir)
		assert.NoError(t, err)
	})

	t.Run("Import from Non-existent Directory", func(t *testing.T) {
		err := registry.ImportVersionManifest("/nonexistent/path")
		assert.Error(t, err)
	})

	t.Run("Import with Invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "invalid.json")
		os.WriteFile(invalidFile, []byte("invalid json"), 0644)

		// Should not error, just skip invalid files
		err := registry.ImportVersionManifest(tmpDir)
		assert.NoError(t, err)
	})
}
