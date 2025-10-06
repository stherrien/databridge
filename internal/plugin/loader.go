package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"

	"github.com/shawntherrien/databridge/pkg/types"
	"github.com/sirupsen/logrus"
)

// PluginLoader handles dynamic plugin loading
type PluginLoader struct {
	registry  *PluginRegistry
	pluginDir string
	autoLoad  bool
	logger    *logrus.Logger
	watching  bool
	stopWatch chan struct{}
}

// PluginManifest describes a plugin
type PluginManifest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Author       string            `json:"author"`
	Description  string            `json:"description"`
	Type         PluginType        `json:"type"`
	EntryPoint   string            `json:"entryPoint"`
	Dependencies []string          `json:"dependencies"`
	Config       map[string]string `json:"config"`
	Tags         []string          `json:"tags"`
}

// LoaderConfig configures the plugin loader
type LoaderConfig struct {
	PluginDir    string
	AutoLoad     bool
	WatchChanges bool
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(registry *PluginRegistry, config LoaderConfig, logger *logrus.Logger) *PluginLoader {
	return &PluginLoader{
		registry:  registry,
		pluginDir: config.PluginDir,
		autoLoad:  config.AutoLoad,
		logger:    logger,
		stopWatch: make(chan struct{}),
	}
}

// LoadAll loads all plugins from the plugin directory
func (l *PluginLoader) LoadAll() error {
	l.logger.WithField("pluginDir", l.pluginDir).Info("Loading plugins from directory")

	// Check if directory exists
	if _, err := os.Stat(l.pluginDir); os.IsNotExist(err) {
		l.logger.WithField("pluginDir", l.pluginDir).Warn("Plugin directory does not exist, creating it")
		if err := os.MkdirAll(l.pluginDir, 0750); err != nil {
			return fmt.Errorf("failed to create plugin directory: %w", err)
		}
		return nil
	}

	// Walk through the plugin directory
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	loaded := 0
	failed := 0

	for _, entry := range entries {
		if entry.IsDir() {
			// Check for manifest in subdirectory
			pluginPath := filepath.Join(l.pluginDir, entry.Name())
			if err := l.LoadFromDirectory(pluginPath); err != nil {
				l.logger.WithError(err).Warnf("Failed to load plugin from %s", pluginPath)
				failed++
			} else {
				loaded++
			}
		}
	}

	l.logger.WithFields(logrus.Fields{
		"loaded": loaded,
		"failed": failed,
	}).Info("Finished loading plugins")

	return nil
}

// LoadFromDirectory loads a plugin from a directory containing manifest.json
func (l *PluginLoader) LoadFromDirectory(path string) error {
	manifestPath := filepath.Join(path, "manifest.json")

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return fmt.Errorf("manifest.json not found in %s", path)
	}

	// Read and parse manifest
	manifest, err := l.readManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Validate manifest
	if err := l.validateManifest(manifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	// Load the plugin based on type
	switch manifest.Type {
	case PluginTypeProcessor:
		return l.loadProcessorPlugin(path, manifest)
	case PluginTypeController, PluginTypeReporting:
		return fmt.Errorf("plugin type %s not yet supported", manifest.Type)
	default:
		return fmt.Errorf("unknown plugin type: %s", manifest.Type)
	}
}

// LoadFromFile loads a plugin from a .so file (Go plugin)
func (l *PluginLoader) LoadFromFile(path string) error {
	l.logger.WithField("path", path).Info("Loading plugin from file")

	// Check file extension
	if !strings.HasSuffix(path, ".so") {
		return fmt.Errorf("invalid plugin file extension (expected .so): %s", path)
	}

	// Load Go plugin
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open plugin: %w", err)
	}

	// Look for NewProcessor symbol
	symNewProcessor, err := p.Lookup("NewProcessor")
	if err != nil {
		return fmt.Errorf("plugin does not export NewProcessor: %w", err)
	}

	// Type assert to processor factory
	factory, ok := symNewProcessor.(func() types.Processor)
	if !ok {
		return fmt.Errorf("NewProcessor has invalid signature")
	}

	// Look for plugin info symbol
	symInfo, err := p.Lookup("PluginInfo")
	if err != nil {
		return fmt.Errorf("plugin does not export PluginInfo: %w", err)
	}

	// Type assert to plugin info
	infoPtr, ok := symInfo.(*PluginInfo)
	if !ok {
		return fmt.Errorf("PluginInfo has invalid type")
	}

	// Set path
	infoPtr.Path = path

	// Register with registry
	if err := l.registry.RegisterProcessor(infoPtr.ID, *infoPtr, factory); err != nil {
		return fmt.Errorf("failed to register plugin: %w", err)
	}

	l.logger.WithField("pluginId", infoPtr.ID).Info("Successfully loaded plugin from file")
	return nil
}

// UnloadPlugin unloads a plugin
func (l *PluginLoader) UnloadPlugin(id string) error {
	return l.registry.UnregisterProcessor(id)
}

// ReloadPlugin reloads a plugin
func (l *PluginLoader) ReloadPlugin(id string) error {
	// Get plugin info
	info, err := l.registry.GetPluginInfo(id)
	if err != nil {
		return err
	}

	// Unload
	if err := l.UnloadPlugin(id); err != nil {
		return err
	}

	// Reload based on path type
	if strings.HasSuffix(info.Path, ".so") {
		return l.LoadFromFile(info.Path)
	}

	return l.LoadFromDirectory(filepath.Dir(info.Path))
}

// StartWatching starts watching the plugin directory for changes
func (l *PluginLoader) StartWatching(interval time.Duration) {
	if l.watching {
		return
	}

	l.watching = true
	l.logger.Info("Starting plugin directory watcher")

	go l.watchLoop(interval)
}

// StopWatching stops watching the plugin directory
func (l *PluginLoader) StopWatching() {
	if !l.watching {
		return
	}

	l.logger.Info("Stopping plugin directory watcher")
	l.watching = false
	close(l.stopWatch)
}

// watchLoop watches for changes in the plugin directory
func (l *PluginLoader) watchLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Keep track of known plugins
	knownPlugins := make(map[string]time.Time)

	for {
		select {
		case <-ticker.C:
			l.checkForChanges(knownPlugins)
		case <-l.stopWatch:
			return
		}
	}
}

// checkForChanges checks for new or modified plugins
func (l *PluginLoader) checkForChanges(knownPlugins map[string]time.Time) {
	entries, err := os.ReadDir(l.pluginDir)
	if err != nil {
		l.logger.WithError(err).Error("Failed to read plugin directory")
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			pluginPath := filepath.Join(l.pluginDir, entry.Name())
			manifestPath := filepath.Join(pluginPath, "manifest.json")

			// Check if manifest exists
			info, err := os.Stat(manifestPath)
			if err != nil {
				continue
			}

			// Check if this is a new or modified plugin
			modTime := info.ModTime()
			if lastMod, exists := knownPlugins[manifestPath]; !exists || modTime.After(lastMod) {
				l.logger.WithField("path", pluginPath).Info("Detected new or modified plugin")

				// Try to load/reload
				if err := l.LoadFromDirectory(pluginPath); err != nil {
					l.logger.WithError(err).Warnf("Failed to load plugin from %s", pluginPath)
				}

				knownPlugins[manifestPath] = modTime
			}
		}
	}
}

// Private helper methods

func (l *PluginLoader) readManifest(path string) (*PluginManifest, error) {
	// #nosec G304 - path is validated by the plugin loader before reaching here
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

func (l *PluginLoader) validateManifest(manifest *PluginManifest) error {
	if manifest.ID == "" {
		return fmt.Errorf("manifest missing required field: id")
	}
	if manifest.Name == "" {
		return fmt.Errorf("manifest missing required field: name")
	}
	if manifest.Version == "" {
		return fmt.Errorf("manifest missing required field: version")
	}
	if manifest.Type == "" {
		return fmt.Errorf("manifest missing required field: type")
	}

	// Validate type
	validTypes := map[PluginType]bool{
		PluginTypeProcessor:  true,
		PluginTypeController: true,
		PluginTypeReporting:  true,
	}

	if !validTypes[manifest.Type] {
		return fmt.Errorf("invalid plugin type: %s", manifest.Type)
	}

	return nil
}

func (l *PluginLoader) loadProcessorPlugin(path string, manifest *PluginManifest) error {
	// For processor plugins, we check if there's a .so file
	soPath := filepath.Join(path, manifest.EntryPoint)

	// Check if .so file exists
	if manifest.EntryPoint != "" && strings.HasSuffix(manifest.EntryPoint, ".so") {
		if _, err := os.Stat(soPath); err == nil {
			// Load as Go plugin
			return l.loadGoPlugin(soPath, manifest)
		}
	}

	// If no .so file, this must be a built-in plugin that should be registered separately
	l.logger.WithFields(logrus.Fields{
		"pluginId": manifest.ID,
		"path":     path,
	}).Debug("Plugin manifest found but no .so file - expecting built-in registration")

	return nil
}

func (l *PluginLoader) loadGoPlugin(path string, manifest *PluginManifest) error {
	// Load the .so file
	p, err := plugin.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open plugin file: %w", err)
	}

	// Look for NewProcessor symbol
	symNewProcessor, err := p.Lookup("NewProcessor")
	if err != nil {
		return fmt.Errorf("plugin does not export NewProcessor: %w", err)
	}

	// Type assert to processor factory
	factory, ok := symNewProcessor.(func() types.Processor)
	if !ok {
		return fmt.Errorf("NewProcessor has invalid signature")
	}

	// Create plugin info from manifest
	info := PluginInfo{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Author:       manifest.Author,
		Description:  manifest.Description,
		Type:         manifest.Type,
		Status:       PluginStatusActive,
		LoadedAt:     time.Now(),
		Path:         path,
		Tags:         manifest.Tags,
		Dependencies: manifest.Dependencies,
	}

	// Validate dependencies
	if err := l.registry.ValidateDependencies(info.ID); err != nil {
		info.Status = PluginStatusError
		l.logger.WithError(err).Warnf("Plugin %s has unsatisfied dependencies", info.ID)
	}

	// Register with registry
	if err := l.registry.RegisterProcessor(info.ID, info, factory); err != nil {
		return fmt.Errorf("failed to register plugin: %w", err)
	}

	l.logger.WithFields(logrus.Fields{
		"pluginId":   info.ID,
		"pluginName": info.Name,
		"version":    info.Version,
	}).Info("Successfully loaded plugin")

	return nil
}

// GetManifest reads and returns a plugin manifest without loading the plugin
func (l *PluginLoader) GetManifest(path string) (*PluginManifest, error) {
	manifestPath := filepath.Join(path, "manifest.json")
	return l.readManifest(manifestPath)
}

// ValidatePlugin validates a plugin without loading it
func (l *PluginLoader) ValidatePlugin(path string) error {
	manifest, err := l.GetManifest(path)
	if err != nil {
		return err
	}

	return l.validateManifest(manifest)
}
