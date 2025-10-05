package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// PluginAPIHandler handles REST API requests for plugin management
type PluginAPIHandler struct {
	pluginManager *PluginManager
	logger        *logrus.Logger
}

// NewPluginAPIHandler creates a new plugin API handler
func NewPluginAPIHandler(manager *PluginManager, logger *logrus.Logger) *PluginAPIHandler {
	return &PluginAPIHandler{
		pluginManager: manager,
		logger:        logger,
	}
}

// RegisterRoutes registers plugin API routes
func (h *PluginAPIHandler) RegisterRoutes(router *mux.Router) {
	api := router.PathPrefix("/api/plugins").Subrouter()

	// Plugin management
	api.HandleFunc("", h.HandleListPlugins).Methods("GET", "OPTIONS")
	api.HandleFunc("/{id}", h.HandleGetPlugin).Methods("GET", "OPTIONS")
	api.HandleFunc("/{id}/enable", h.HandleEnablePlugin).Methods("PUT", "OPTIONS")
	api.HandleFunc("/{id}/disable", h.HandleDisablePlugin).Methods("PUT", "OPTIONS")
	api.HandleFunc("/{id}/reload", h.HandleReloadPlugin).Methods("POST", "OPTIONS")
	api.HandleFunc("/{id}", h.HandleUninstallPlugin).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/{id}/status", h.HandleGetPluginStatus).Methods("GET", "OPTIONS")

	// Plugin installation
	api.HandleFunc("/install", h.HandleInstallPlugin).Methods("POST", "OPTIONS")
	api.HandleFunc("/upload", h.HandleUploadPlugin).Methods("POST", "OPTIONS")

	// Plugin search and discovery
	api.HandleFunc("/search", h.HandleSearchPlugins).Methods("GET", "OPTIONS")

	// Plugin statistics
	api.HandleFunc("/stats", h.HandleGetStats).Methods("GET", "OPTIONS")

	// Built-in processors
	api.HandleFunc("/builtin", h.HandleListBuiltInProcessors).Methods("GET", "OPTIONS")

	h.logger.Info("Registered plugin API routes")
}

// HandleListPlugins returns all plugins
// GET /api/plugins
func (h *PluginAPIHandler) HandleListPlugins(w http.ResponseWriter, r *http.Request) {
	plugins := h.pluginManager.ListAllPlugins()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"plugins": plugins,
		"count":   len(plugins),
	})
}

// HandleGetPlugin returns plugin details
// GET /api/plugins/{id}
func (h *PluginAPIHandler) HandleGetPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	info, err := h.pluginManager.GetPluginInfo(id)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Plugin not found: %s", id))
		return
	}

	respondJSON(w, http.StatusOK, info)
}

// HandleEnablePlugin enables a plugin
// PUT /api/plugins/{id}/enable
func (h *PluginAPIHandler) HandleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.pluginManager.Registry.EnablePlugin(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to enable plugin: %v", err))
		return
	}

	info, _ := h.pluginManager.GetPluginInfo(id)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Plugin enabled successfully",
		"plugin":  info,
	})
}

// HandleDisablePlugin disables a plugin
// PUT /api/plugins/{id}/disable
func (h *PluginAPIHandler) HandleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.pluginManager.Registry.DisablePlugin(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to disable plugin: %v", err))
		return
	}

	info, _ := h.pluginManager.GetPluginInfo(id)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Plugin disabled successfully",
		"plugin":  info,
	})
}

// HandleReloadPlugin reloads a plugin
// POST /api/plugins/{id}/reload
func (h *PluginAPIHandler) HandleReloadPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Check if it's a built-in
	if h.pluginManager.BuiltIns.IsBuiltIn(id) {
		respondError(w, http.StatusBadRequest, "Cannot reload built-in processors")
		return
	}

	if err := h.pluginManager.Loader.ReloadPlugin(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reload plugin: %v", err))
		return
	}

	info, _ := h.pluginManager.GetPluginInfo(id)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Plugin reloaded successfully",
		"plugin":  info,
	})
}

// HandleUninstallPlugin uninstalls a plugin
// DELETE /api/plugins/{id}
func (h *PluginAPIHandler) HandleUninstallPlugin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Check if it's a built-in
	if h.pluginManager.BuiltIns.IsBuiltIn(id) {
		respondError(w, http.StatusBadRequest, "Cannot uninstall built-in processors")
		return
	}

	// Get plugin info before uninstalling
	info, err := h.pluginManager.GetPluginInfo(id)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Plugin not found: %s", id))
		return
	}

	// Unload from registry
	if err := h.pluginManager.Loader.UnloadPlugin(id); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to uninstall plugin: %v", err))
		return
	}

	// Optionally delete plugin files
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"
	if deleteFiles && info.Path != "built-in" {
		pluginDir := filepath.Dir(info.Path)
		if err := os.RemoveAll(pluginDir); err != nil {
			h.logger.WithError(err).Warnf("Failed to delete plugin files: %s", pluginDir)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Plugin uninstalled successfully",
		"id":      id,
	})
}

// HandleGetPluginStatus returns plugin status
// GET /api/plugins/{id}/status
func (h *PluginAPIHandler) HandleGetPluginStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	info, err := h.pluginManager.GetPluginInfo(id)
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("Plugin not found: %s", id))
		return
	}

	enabled := h.pluginManager.Registry.IsEnabled(id)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":      info.ID,
		"name":    info.Name,
		"status":  info.Status,
		"enabled": enabled,
	})
}

// InstallPluginRequest represents a plugin installation request
type InstallPluginRequest struct {
	Source   string `json:"source"`   // URL or path to plugin
	Validate bool   `json:"validate"` // Whether to validate before installing
}

// HandleInstallPlugin installs a plugin from URL or path
// POST /api/plugins/install
func (h *PluginAPIHandler) HandleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	var req InstallPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if req.Source == "" {
		respondError(w, http.StatusBadRequest, "Source is required")
		return
	}

	// For now, only support local paths
	// In production, you'd want to support downloading from URLs
	if !filepath.IsAbs(req.Source) {
		respondError(w, http.StatusBadRequest, "Source must be an absolute path")
		return
	}

	// Validate plugin if requested
	if req.Validate {
		result := h.pluginManager.Validator.Validate(req.Source)
		if !result.Valid {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{
				"message": "Plugin validation failed",
				"result":  result,
			})
			return
		}
	}

	// Load plugin
	if err := h.pluginManager.Loader.LoadFromDirectory(req.Source); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to install plugin: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Plugin installed successfully",
		"source":  req.Source,
	})
}

// HandleUploadPlugin handles plugin upload
// POST /api/plugins/upload
func (h *PluginAPIHandler) HandleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB limit
		respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get file from form
	file, header, err := r.FormFile("plugin")
	if err != nil {
		respondError(w, http.StatusBadRequest, "No plugin file provided")
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			h.logger.WithError(err).Warn("Failed to close uploaded file")
		}
	}()

	// Create temporary directory for plugin
	tempDir, err := os.MkdirTemp(h.pluginManager.Registry.pluginDir, "upload-*")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create temporary directory")
		return
	}

	// Save uploaded file
	destPath := filepath.Join(tempDir, header.Filename)
	destFile, err := os.Create(destPath)
	if err != nil {
		_ = os.RemoveAll(tempDir) // Best effort cleanup
		respondError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			h.logger.WithError(err).Warn("Failed to close destination file")
		}
	}()

	if _, err := io.Copy(destFile, file); err != nil {
		_ = os.RemoveAll(tempDir) // Best effort cleanup
		respondError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	// Validate if requested
	validate := r.FormValue("validate") == "true"
	if validate {
		result := h.pluginManager.Validator.Validate(tempDir)
		if !result.Valid {
			_ = os.RemoveAll(tempDir) // Best effort cleanup
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{
				"message": "Plugin validation failed",
				"result":  result,
			})
			return
		}
	}

	// Load plugin
	if err := h.pluginManager.Loader.LoadFromDirectory(tempDir); err != nil {
		_ = os.RemoveAll(tempDir) // Best effort cleanup
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load plugin: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Plugin uploaded and installed successfully",
		"path":    tempDir,
	})
}

// HandleSearchPlugins searches for plugins
// GET /api/plugins/search?q=query&type=processor&tags=tag1,tag2
func (h *PluginAPIHandler) HandleSearchPlugins(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	pluginTypeStr := r.URL.Query().Get("type")
	tagsStr := r.URL.Query().Get("tags")

	var pluginType PluginType
	if pluginTypeStr != "" {
		pluginType = PluginType(pluginTypeStr)
	}

	var tags []string
	if tagsStr != "" {
		// Simple comma-separated parsing
		for _, tag := range splitString(tagsStr, ',') {
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	results := h.pluginManager.Registry.SearchPlugins(query, pluginType, tags)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"plugins": results,
		"count":   len(results),
		"query":   query,
		"type":    pluginType,
		"tags":    tags,
	})
}

// HandleGetStats returns plugin statistics
// GET /api/plugins/stats
func (h *PluginAPIHandler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	stats := h.pluginManager.GetStats()
	respondJSON(w, http.StatusOK, stats)
}

// HandleListBuiltInProcessors returns all built-in processors
// GET /api/plugins/builtin
func (h *PluginAPIHandler) HandleListBuiltInProcessors(w http.ResponseWriter, r *http.Request) {
	processors := h.pluginManager.BuiltIns.ListBuiltInProcessors()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"processors": processors,
		"count":      len(processors),
	})
}

// Helper functions

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) // Best effort encoding
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]interface{}{
		"error":   message,
		"success": false,
	})
}

func splitString(s string, sep rune) []string {
	var result []string
	var current []rune

	for _, r := range s {
		if r == sep {
			if len(current) > 0 {
				result = append(result, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		result = append(result, string(current))
	}

	return result
}
