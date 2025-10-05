package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FilesystemHandlers handles filesystem browsing API endpoints
type FilesystemHandlers struct{}

// NewFilesystemHandlers creates a new filesystem handlers instance
func NewFilesystemHandlers() *FilesystemHandlers {
	return &FilesystemHandlers{}
}

// FileInfo represents a file or directory in the filesystem
type FileInfo struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	Size        int64  `json:"size"`
	ModTime     string `json:"modTime"`
}

// BrowseRequest represents a request to browse a directory
type BrowseRequest struct {
	Path string `json:"path"`
}

// BrowseResponse represents the response from browsing a directory
type BrowseResponse struct {
	CurrentPath string     `json:"currentPath"`
	Parent      string     `json:"parent,omitempty"`
	Items       []FileInfo `json:"items"`
}

// HandleBrowseFilesystem handles GET /api/filesystem/browse?path=/some/path
func (h *FilesystemHandlers) HandleBrowseFilesystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get path from query parameter
	path := r.URL.Query().Get("path")
	if path == "" {
		// Default to user's home directory or root
		if homeDir, err := os.UserHomeDir(); err == nil {
			path = homeDir
		} else {
			path = "/"
		}
	}

	// Security check: ensure path is absolute
	if !filepath.IsAbs(path) {
		respondError(w, http.StatusBadRequest, "Path must be absolute")
		return
	}

	// Clean the path to prevent directory traversal attacks
	path = filepath.Clean(path)

	// Check if path exists and is accessible
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			respondError(w, http.StatusNotFound, fmt.Sprintf("Path does not exist: %s", path))
		} else if os.IsPermission(err) {
			respondError(w, http.StatusForbidden, fmt.Sprintf("Permission denied: %s", path))
		} else {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("Error accessing path: %v", err))
		}
		return
	}

	// If it's a file, return its parent directory contents
	if !info.IsDir() {
		path = filepath.Dir(path)
	}

	// Read directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Error reading directory: %v", err))
		return
	}

	// Build response
	var items []FileInfo
	for _, entry := range entries {
		// Skip hidden files on Unix systems
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		fullPath := filepath.Join(path, entry.Name())
		items = append(items, FileInfo{
			Name:        entry.Name(),
			Path:        fullPath,
			IsDirectory: entry.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	// Sort: directories first, then files, both alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDirectory != items[j].IsDirectory {
			return items[i].IsDirectory
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	// Get parent directory
	parent := filepath.Dir(path)
	if parent == path {
		parent = "" // We're at the root
	}

	response := BrowseResponse{
		CurrentPath: path,
		Parent:      parent,
		Items:       items,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response) // Best effort encoding
}

// HandleValidatePath handles POST /api/filesystem/validate
func (h *FilesystemHandlers) HandleValidatePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Path string `json:"path"`
		Type string `json:"type"` // "file" or "directory"
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Path == "" {
		respondError(w, http.StatusBadRequest, "Path is required")
		return
	}

	// Security check: ensure path is absolute
	if !filepath.IsAbs(req.Path) {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "Path must be absolute (start with / or drive letter)",
		})
		return
	}

	// Check if path exists
	info, err := os.Stat(req.Path)
	if err != nil {
		if os.IsNotExist(err) {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"valid":   false,
				"message": "Path does not exist",
			})
		} else if os.IsPermission(err) {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"valid":   false,
				"message": "Permission denied",
			})
		} else {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"valid":   false,
				"message": fmt.Sprintf("Error accessing path: %v", err),
			})
		}
		return
	}

	// Validate type matches
	if req.Type == "directory" && !info.IsDir() {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "Path exists but is not a directory",
		})
		return
	}

	if req.Type == "file" && info.IsDir() {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": "Path exists but is a directory, not a file",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"valid":   true,
		"message": "Path is valid and accessible",
	})
}
