package plugin

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/shawntherrien/databridge/pkg/types"
)

// PluginType defines plugin types
type PluginType string

const (
	PluginTypeProcessor  PluginType = "processor"
	PluginTypeController PluginType = "controller"
	PluginTypeReporting  PluginType = "reporting"
)

// PluginStatus tracks plugin state
type PluginStatus string

const (
	PluginStatusActive   PluginStatus = "active"
	PluginStatusInactive PluginStatus = "inactive"
	PluginStatusError    PluginStatus = "error"
	PluginStatusLoading  PluginStatus = "loading"
)

// PluginInfo contains plugin metadata
type PluginInfo struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Author      string       `json:"author"`
	Description string       `json:"description"`
	Type        PluginType   `json:"type"`
	Status      PluginStatus `json:"status"`
	LoadedAt    time.Time    `json:"loadedAt"`
	Path        string       `json:"path"`
	Tags        []string     `json:"tags"`
	Dependencies []string    `json:"dependencies"`
}

// ProcessorFactory creates processor instances
type ProcessorFactory func() types.Processor

// PluginRegistry manages all plugins
type PluginRegistry struct {
	plugins           map[string]*PluginInfo
	processors        map[string]ProcessorFactory
	pluginDir         string
	mu                sync.RWMutex
	logger            *logrus.Logger
	enabledPlugins    map[string]bool
}

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry(pluginDir string, logger *logrus.Logger) *PluginRegistry {
	return &PluginRegistry{
		plugins:        make(map[string]*PluginInfo),
		processors:     make(map[string]ProcessorFactory),
		pluginDir:      pluginDir,
		logger:         logger,
		enabledPlugins: make(map[string]bool),
	}
}

// RegisterProcessor registers a processor factory
func (r *PluginRegistry) RegisterProcessor(id string, info PluginInfo, factory ProcessorFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.processors[id]; exists {
		return fmt.Errorf("processor %s is already registered", id)
	}

	// Set plugin type if not specified
	if info.Type == "" {
		info.Type = PluginTypeProcessor
	}

	// Set loaded time
	if info.LoadedAt.IsZero() {
		info.LoadedAt = time.Now()
	}

	// Set ID if not specified
	if info.ID == "" {
		info.ID = id
	}

	// Set initial status
	if info.Status == "" {
		info.Status = PluginStatusActive
	}

	r.plugins[id] = &info
	r.processors[id] = factory
	r.enabledPlugins[id] = true

	r.logger.WithFields(logrus.Fields{
		"pluginId":   id,
		"pluginName": info.Name,
		"pluginType": info.Type,
		"version":    info.Version,
	}).Info("Registered plugin")

	return nil
}

// UnregisterProcessor unregisters a processor
func (r *PluginRegistry) UnregisterProcessor(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.processors[id]; !exists {
		return fmt.Errorf("processor %s is not registered", id)
	}

	delete(r.plugins, id)
	delete(r.processors, id)
	delete(r.enabledPlugins, id)

	r.logger.WithField("pluginId", id).Info("Unregistered plugin")
	return nil
}

// GetProcessor creates a new instance of a processor
func (r *PluginRegistry) GetProcessor(id string) (types.Processor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.processors[id]
	if !exists {
		return nil, fmt.Errorf("processor %s not found", id)
	}

	// Check if plugin is enabled
	if enabled, ok := r.enabledPlugins[id]; ok && !enabled {
		return nil, fmt.Errorf("processor %s is disabled", id)
	}

	// Create new instance
	processor := factory()
	if processor == nil {
		return nil, fmt.Errorf("factory for processor %s returned nil", id)
	}

	return processor, nil
}

// GetPluginInfo retrieves plugin information
func (r *PluginRegistry) GetPluginInfo(id string) (*PluginInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.plugins[id]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", id)
	}

	// Return a copy to prevent external modifications
	infoCopy := *info
	return &infoCopy, nil
}

// ListPlugins returns all registered plugins
func (r *PluginRegistry) ListPlugins() []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]*PluginInfo, 0, len(r.plugins))
	for _, info := range r.plugins {
		infoCopy := *info
		plugins = append(plugins, &infoCopy)
	}

	return plugins
}

// ListProcessors returns all registered processor IDs
func (r *PluginRegistry) ListProcessors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors := make([]string, 0, len(r.processors))
	for id := range r.processors {
		processors = append(processors, id)
	}

	return processors
}

// SearchPlugins searches for plugins matching criteria
func (r *PluginRegistry) SearchPlugins(query string, pluginType PluginType, tags []string) []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*PluginInfo

	for _, info := range r.plugins {
		// Match type if specified
		if pluginType != "" && info.Type != pluginType {
			continue
		}

		// Match query in name, description, or ID
		if query != "" {
			matched := false
			if matchString(info.Name, query) ||
				matchString(info.Description, query) ||
				matchString(info.ID, query) ||
				matchString(info.Author, query) {
				matched = true
			}
			if !matched {
				continue
			}
		}

		// Match tags if specified
		if len(tags) > 0 {
			if !hasAnyTag(info.Tags, tags) {
				continue
			}
		}

		// Add to results
		infoCopy := *info
		results = append(results, &infoCopy)
	}

	return results
}

// EnablePlugin enables a plugin
func (r *PluginRegistry) EnablePlugin(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.plugins[id]
	if !exists {
		return fmt.Errorf("plugin %s not found", id)
	}

	r.enabledPlugins[id] = true
	info.Status = PluginStatusActive

	r.logger.WithField("pluginId", id).Info("Enabled plugin")
	return nil
}

// DisablePlugin disables a plugin
func (r *PluginRegistry) DisablePlugin(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.plugins[id]
	if !exists {
		return fmt.Errorf("plugin %s not found", id)
	}

	r.enabledPlugins[id] = false
	info.Status = PluginStatusInactive

	r.logger.WithField("pluginId", id).Info("Disabled plugin")
	return nil
}

// IsEnabled checks if a plugin is enabled
func (r *PluginRegistry) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	enabled, exists := r.enabledPlugins[id]
	return exists && enabled
}

// UpdatePluginStatus updates a plugin's status
func (r *PluginRegistry) UpdatePluginStatus(id string, status PluginStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, exists := r.plugins[id]
	if !exists {
		return fmt.Errorf("plugin %s not found", id)
	}

	info.Status = status

	r.logger.WithFields(logrus.Fields{
		"pluginId": id,
		"status":   status,
	}).Debug("Updated plugin status")

	return nil
}

// GetPluginsByType returns all plugins of a specific type
func (r *PluginRegistry) GetPluginsByType(pluginType PluginType) []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*PluginInfo
	for _, info := range r.plugins {
		if info.Type == pluginType {
			infoCopy := *info
			results = append(results, &infoCopy)
		}
	}

	return results
}

// GetPluginsByStatus returns all plugins with a specific status
func (r *PluginRegistry) GetPluginsByStatus(status PluginStatus) []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*PluginInfo
	for _, info := range r.plugins {
		if info.Status == status {
			infoCopy := *info
			results = append(results, &infoCopy)
		}
	}

	return results
}

// ValidateDependencies checks if all dependencies are satisfied
func (r *PluginRegistry) ValidateDependencies(id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, exists := r.plugins[id]
	if !exists {
		return fmt.Errorf("plugin %s not found", id)
	}

	for _, depID := range info.Dependencies {
		depInfo, exists := r.plugins[depID]
		if !exists {
			return fmt.Errorf("dependency %s not found", depID)
		}
		if depInfo.Status != PluginStatusActive {
			return fmt.Errorf("dependency %s is not active (status: %s)", depID, depInfo.Status)
		}
	}

	return nil
}

// GetPluginCount returns the total number of registered plugins
func (r *PluginRegistry) GetPluginCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}

// GetPluginStats returns statistics about registered plugins
func (r *PluginRegistry) GetPluginStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]interface{})

	// Count by type
	typeCount := make(map[PluginType]int)
	statusCount := make(map[PluginStatus]int)

	for _, info := range r.plugins {
		typeCount[info.Type]++
		statusCount[info.Status]++
	}

	stats["total"] = len(r.plugins)
	stats["byType"] = typeCount
	stats["byStatus"] = statusCount
	stats["enabled"] = len(r.enabledPlugins)

	return stats
}

// Helper functions

func matchString(s, query string) bool {
	// Simple case-insensitive substring match
	// In production, you might want to use a more sophisticated matching algorithm
	sLower := toLower(s)
	queryLower := toLower(query)
	return contains(sLower, queryLower)
}

func toLower(s string) string {
	result := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r = r + 32
		}
		result = append(result, r)
	}
	return string(result)
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasAnyTag(pluginTags, searchTags []string) bool {
	for _, searchTag := range searchTags {
		for _, pluginTag := range pluginTags {
			if matchString(pluginTag, searchTag) {
				return true
			}
		}
	}
	return false
}
