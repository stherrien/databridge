package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
)

// ProcessorVersion represents a specific version of a processor
type ProcessorVersion struct {
	ProcessorType   string                 `json:"processorType"`
	Version         string                 `json:"version"`
	MinimumVersion  string                 `json:"minimumVersion,omitempty"`
	DeprecatedIn    string                 `json:"deprecatedIn,omitempty"`
	RemovedIn       string                 `json:"removedIn,omitempty"`
	ReleaseDate     time.Time              `json:"releaseDate"`
	ChangeLog       []VersionChange        `json:"changeLog"`
	Properties      []types.PropertySpec   `json:"properties"`
	MigrationGuide  string                 `json:"migrationGuide,omitempty"`
	BreakingChanges []string               `json:"breakingChanges,omitempty"`
	Factory         ProcessorFactory       `json:"-"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// VersionChange describes a change in a version
type VersionChange struct {
	Type        ChangeType `json:"type"`
	Description string     `json:"description"`
	Property    string     `json:"property,omitempty"`
	Impact      Impact     `json:"impact"`
}

// ChangeType categorizes version changes
type ChangeType string

const (
	ChangeTypeFeature     ChangeType = "FEATURE"
	ChangeTypeBugFix      ChangeType = "BUGFIX"
	ChangeTypeImprovement ChangeType = "IMPROVEMENT"
	ChangeTypeDeprecation ChangeType = "DEPRECATION"
	ChangeTypeBreaking    ChangeType = "BREAKING"
	ChangeTypeSecurity    ChangeType = "SECURITY"
)

// Impact describes the impact of a change
type Impact string

const (
	ImpactNone     Impact = "NONE"
	ImpactLow      Impact = "LOW"
	ImpactMedium   Impact = "MEDIUM"
	ImpactHigh     Impact = "HIGH"
	ImpactCritical Impact = "CRITICAL"
)

// ProcessorMigration defines how to migrate from one version to another
type ProcessorMigration struct {
	FromVersion string              `json:"fromVersion"`
	ToVersion   string              `json:"toVersion"`
	Strategies  []MigrationStrategy `json:"strategies"`
	AutoMigrate bool                `json:"autoMigrate"`
	Manual      bool                `json:"manual"`
	Script      string              `json:"script,omitempty"`
}

// MigrationStrategy defines a specific migration operation
type MigrationStrategy struct {
	Type        MigrationType     `json:"type"`
	Description string            `json:"description"`
	OldProperty string            `json:"oldProperty,omitempty"`
	NewProperty string            `json:"newProperty,omitempty"`
	Transform   PropertyTransform `json:"-"`
	Config      map[string]string `json:"config,omitempty"`
}

// MigrationType categorizes migration strategies
type MigrationType string

const (
	MigrationTypeRename    MigrationType = "RENAME"
	MigrationTypeTransform MigrationType = "TRANSFORM"
	MigrationTypeRemove    MigrationType = "REMOVE"
	MigrationTypeAdd       MigrationType = "ADD"
	MigrationTypeMerge     MigrationType = "MERGE"
	MigrationTypeSplit     MigrationType = "SPLIT"
)

// PropertyTransform transforms property values during migration
type PropertyTransform func(oldValue string, config map[string]string) (string, error)

// ProcessorVersionRegistry manages processor versions and migrations
type ProcessorVersionRegistry struct {
	mu         sync.RWMutex
	versions   map[string][]*ProcessorVersion   // processorType -> versions
	migrations map[string][]*ProcessorMigration // processorType -> migrations
	transforms map[string]PropertyTransform     // transform name -> function
	logger     types.Logger
}

// NewProcessorVersionRegistry creates a new version registry
func NewProcessorVersionRegistry(logger types.Logger) *ProcessorVersionRegistry {
	return &ProcessorVersionRegistry{
		versions:   make(map[string][]*ProcessorVersion),
		migrations: make(map[string][]*ProcessorMigration),
		transforms: make(map[string]PropertyTransform),
		logger:     logger,
	}
}

// RegisterVersion registers a processor version
func (r *ProcessorVersionRegistry) RegisterVersion(version *ProcessorVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if version.ProcessorType == "" {
		return fmt.Errorf("processor type is required")
	}

	if version.Version == "" {
		return fmt.Errorf("version is required")
	}

	// Check for duplicate version
	versions := r.versions[version.ProcessorType]
	for _, v := range versions {
		if v.Version == version.Version {
			return fmt.Errorf("version %s already registered for processor %s",
				version.Version, version.ProcessorType)
		}
	}

	r.versions[version.ProcessorType] = append(r.versions[version.ProcessorType], version)

	// Sort versions in descending order (newest first)
	sort.Slice(r.versions[version.ProcessorType], func(i, j int) bool {
		return compareVersions(r.versions[version.ProcessorType][i].Version,
			r.versions[version.ProcessorType][j].Version) > 0
	})

	r.logger.Info("Registered processor version",
		"type", version.ProcessorType,
		"version", version.Version)

	return nil
}

// RegisterMigration registers a migration path
func (r *ProcessorVersionRegistry) RegisterMigration(processorType string, migration *ProcessorMigration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if processorType == "" {
		return fmt.Errorf("processor type is required")
	}

	r.migrations[processorType] = append(r.migrations[processorType], migration)

	r.logger.Info("Registered processor migration",
		"type", processorType,
		"from", migration.FromVersion,
		"to", migration.ToVersion)

	return nil
}

// RegisterTransform registers a property transform function
func (r *ProcessorVersionRegistry) RegisterTransform(name string, transform PropertyTransform) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transforms[name] = transform
}

// GetLatestVersion returns the latest version of a processor
func (r *ProcessorVersionRegistry) GetLatestVersion(processorType string) (*ProcessorVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := r.versions[processorType]
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for processor %s", processorType)
	}

	return versions[0], nil
}

// GetVersion returns a specific version of a processor
func (r *ProcessorVersionRegistry) GetVersion(processorType, version string) (*ProcessorVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := r.versions[processorType]
	for _, v := range versions {
		if v.Version == version {
			return v, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for processor %s", version, processorType)
}

// ListVersions returns all versions of a processor
func (r *ProcessorVersionRegistry) ListVersions(processorType string) []*ProcessorVersion {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := r.versions[processorType]
	result := make([]*ProcessorVersion, len(versions))
	copy(result, versions)
	return result
}

// GetMigrationPath finds a migration path between two versions
func (r *ProcessorVersionRegistry) GetMigrationPath(processorType, fromVersion, toVersion string) ([]*ProcessorMigration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	migrations := r.migrations[processorType]

	// Direct migration
	for _, m := range migrations {
		if m.FromVersion == fromVersion && m.ToVersion == toVersion {
			return []*ProcessorMigration{m}, nil
		}
	}

	// Multi-step migration (simplified - could be improved with graph search)
	var path []*ProcessorMigration
	currentVersion := fromVersion

	for currentVersion != toVersion {
		found := false
		for _, m := range migrations {
			if m.FromVersion == currentVersion {
				path = append(path, m)
				currentVersion = m.ToVersion
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no migration path found from %s to %s",
				fromVersion, toVersion)
		}

		// Prevent infinite loops
		if len(path) > 10 {
			return nil, fmt.Errorf("migration path too long (possible cycle)")
		}
	}

	return path, nil
}

// MigrateConfig migrates processor configuration from one version to another
func (r *ProcessorVersionRegistry) MigrateConfig(
	processorType string,
	config types.ProcessorConfig,
	fromVersion, toVersion string,
) (types.ProcessorConfig, error) {
	// Get migration path
	path, err := r.GetMigrationPath(processorType, fromVersion, toVersion)
	if err != nil {
		return config, err
	}

	r.logger.Info("Migrating processor configuration",
		"type", processorType,
		"from", fromVersion,
		"to", toVersion,
		"steps", len(path))

	// Apply each migration in the path
	migratedConfig := config
	for _, migration := range path {
		migratedConfig, err = r.applyMigration(migratedConfig, migration)
		if err != nil {
			return config, fmt.Errorf("migration failed from %s to %s: %w",
				migration.FromVersion, migration.ToVersion, err)
		}
	}

	return migratedConfig, nil
}

// applyMigration applies a single migration
func (r *ProcessorVersionRegistry) applyMigration(
	config types.ProcessorConfig,
	migration *ProcessorMigration,
) (types.ProcessorConfig, error) {
	if !migration.AutoMigrate {
		return config, fmt.Errorf("manual migration required from %s to %s",
			migration.FromVersion, migration.ToVersion)
	}

	newConfig := config
	newProperties := make(map[string]string)
	for k, v := range config.Properties {
		newProperties[k] = v
	}

	for _, strategy := range migration.Strategies {
		switch strategy.Type {
		case MigrationTypeRename:
			if value, exists := newProperties[strategy.OldProperty]; exists {
				newProperties[strategy.NewProperty] = value
				delete(newProperties, strategy.OldProperty)
			}

		case MigrationTypeTransform:
			if value, exists := newProperties[strategy.OldProperty]; exists {
				if strategy.Transform != nil {
					transformed, err := strategy.Transform(value, strategy.Config)
					if err != nil {
						return config, fmt.Errorf("transform failed for %s: %w",
							strategy.OldProperty, err)
					}
					newProperties[strategy.NewProperty] = transformed
					if strategy.NewProperty != strategy.OldProperty {
						delete(newProperties, strategy.OldProperty)
					}
				}
			}

		case MigrationTypeRemove:
			delete(newProperties, strategy.OldProperty)

		case MigrationTypeAdd:
			if _, exists := newProperties[strategy.NewProperty]; !exists {
				if defaultValue, ok := strategy.Config["default"]; ok {
					newProperties[strategy.NewProperty] = defaultValue
				}
			}
		}
	}

	newConfig.Properties = newProperties
	return newConfig, nil
}

// IsDeprecated checks if a processor version is deprecated
func (r *ProcessorVersionRegistry) IsDeprecated(processorType, version string) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, err := r.GetVersion(processorType, version)
	if err != nil {
		return false, ""
	}

	return v.DeprecatedIn != "", v.DeprecatedIn
}

// ExportVersionManifest exports version information to a file
func (r *ProcessorVersionRegistry) ExportVersionManifest(outputDir string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for processorType, versions := range r.versions {
		manifest := VersionManifest{
			ProcessorType: processorType,
			Versions:      versions,
			Migrations:    r.migrations[processorType],
			ExportedAt:    time.Now(),
		}

		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal manifest for %s: %w", processorType, err)
		}

		filename := filepath.Join(outputDir, fmt.Sprintf("%s-versions.json", processorType))
		if err := os.WriteFile(filename, data, 0600); err != nil {
			return fmt.Errorf("failed to write manifest for %s: %w", processorType, err)
		}

		r.logger.Info("Exported version manifest", "type", processorType, "file", filename)
	}

	return nil
}

// ImportVersionManifest imports version information from a directory
func (r *ProcessorVersionRegistry) ImportVersionManifest(inputDir string) error {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("failed to read input directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filename := filepath.Join(inputDir, entry.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			r.logger.Warn("Failed to read manifest file", "file", filename, "error", err)
			continue
		}

		var manifest VersionManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			r.logger.Warn("Failed to unmarshal manifest", "file", filename, "error", err)
			continue
		}

		// Register versions
		for _, version := range manifest.Versions {
			if err := r.RegisterVersion(version); err != nil {
				r.logger.Warn("Failed to register version",
					"type", manifest.ProcessorType,
					"version", version.Version,
					"error", err)
			}
		}

		// Register migrations
		for _, migration := range manifest.Migrations {
			if err := r.RegisterMigration(manifest.ProcessorType, migration); err != nil {
				r.logger.Warn("Failed to register migration",
					"type", manifest.ProcessorType,
					"error", err)
			}
		}
	}

	return nil
}

// VersionManifest represents exported version information
type VersionManifest struct {
	ProcessorType string                `json:"processorType"`
	Versions      []*ProcessorVersion   `json:"versions"`
	Migrations    []*ProcessorMigration `json:"migrations"`
	ExportedAt    time.Time             `json:"exportedAt"`
}

// GetVersionStats returns statistics about registered versions
func (r *ProcessorVersionRegistry) GetVersionStats() VersionStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := VersionStats{
		ProcessorTypes:  len(r.versions),
		TotalVersions:   0,
		TotalMigrations: 0,
	}

	for _, versions := range r.versions {
		stats.TotalVersions += len(versions)
	}

	for _, migrations := range r.migrations {
		stats.TotalMigrations += len(migrations)
	}

	return stats
}

// VersionStats provides version registry statistics
type VersionStats struct {
	ProcessorTypes  int
	TotalVersions   int
	TotalMigrations int
}

// compareVersions compares two version strings (simplified semantic versioning)
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	if v1 == v2 {
		return 0
	}

	// Simplified comparison - in production would use proper semver library
	if v1 > v2 {
		return 1
	}
	return -1
}
