package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

// Settings represents system settings
type Settings struct {
	General    GeneralSettings    `json:"general"`
	Flow       FlowSettings       `json:"flow"`
	Storage    StorageSettings    `json:"storage"`
	Monitoring MonitoringSettings `json:"monitoring"`
	Security   SecuritySettings   `json:"security"`
}

// GeneralSettings represents general system settings
type GeneralSettings struct {
	SystemName        string `json:"systemName"`
	MaxConcurrentTasks int    `json:"maxConcurrentTasks"`
	DefaultSchedule   string `json:"defaultSchedule"`
	TimeZone          string `json:"timeZone"`
	LogLevel          string `json:"logLevel"`
}

// FlowSettings represents flow-related settings
type FlowSettings struct {
	AutoSave              bool `json:"autoSave"`
	AutoSaveInterval      int  `json:"autoSaveInterval"` // seconds
	MaxFlowFileSize       int64 `json:"maxFlowFileSize"` // bytes
	DefaultBackPressure   int  `json:"defaultBackPressure"`
	EnableCompression     bool `json:"enableCompression"`
}

// StorageSettings represents storage-related settings
type StorageSettings struct {
	DataDirectory         string `json:"dataDirectory"`
	ContentRepository     string `json:"contentRepository"`
	FlowFileRepository    string `json:"flowFileRepository"`
	ProvenanceRepository  string `json:"provenanceRepository"`
	MaxStorageSize        int64  `json:"maxStorageSize"` // bytes
	CleanupOlderThan      int    `json:"cleanupOlderThan"` // days
	EnableAutoCleanup     bool   `json:"enableAutoCleanup"`
}

// MonitoringSettings represents monitoring-related settings
type MonitoringSettings struct {
	EnableMetrics         bool `json:"enableMetrics"`
	MetricsRetention      int  `json:"metricsRetention"` // days
	EnableProvenance      bool `json:"enableProvenance"`
	ProvenanceRetention   int  `json:"provenanceRetention"` // days
	EnableHealthChecks    bool `json:"enableHealthChecks"`
	HealthCheckInterval   int  `json:"healthCheckInterval"` // seconds
}

// SecuritySettings represents security-related settings
type SecuritySettings struct {
	EnableAuthentication  bool   `json:"enableAuthentication"`
	EnableAuthorization   bool   `json:"enableAuthorization"`
	SessionTimeout        int    `json:"sessionTimeout"` // minutes
	PasswordMinLength     int    `json:"passwordMinLength"`
	RequireStrongPassword bool   `json:"requireStrongPassword"`
	EnableAuditLog        bool   `json:"enableAuditLog"`
}

// SettingsHandlers handles settings-related HTTP requests
type SettingsHandlers struct {
	log      *logrus.Logger
	settings *Settings
	mu       sync.RWMutex
}

// NewSettingsHandlers creates a new settings handler
func NewSettingsHandlers(log *logrus.Logger) *SettingsHandlers {
	return &SettingsHandlers{
		log:      log,
		settings: getDefaultSettings(),
	}
}

// getDefaultSettings returns default system settings
func getDefaultSettings() *Settings {
	return &Settings{
		General: GeneralSettings{
			SystemName:         "DataBridge",
			MaxConcurrentTasks: 10,
			DefaultSchedule:    "0 * * * *",
			TimeZone:           "America/New_York",
			LogLevel:           "INFO",
		},
		Flow: FlowSettings{
			AutoSave:            true,
			AutoSaveInterval:    30,
			MaxFlowFileSize:     10 * 1024 * 1024, // 10MB
			DefaultBackPressure: 10000,
			EnableCompression:   false,
		},
		Storage: StorageSettings{
			DataDirectory:        "./data",
			ContentRepository:    "./data/content",
			FlowFileRepository:   "./data/flowfiles",
			ProvenanceRepository: "./data/provenance",
			MaxStorageSize:       100 * 1024 * 1024 * 1024, // 100GB
			CleanupOlderThan:     30,
			EnableAutoCleanup:    true,
		},
		Monitoring: MonitoringSettings{
			EnableMetrics:       true,
			MetricsRetention:    7,
			EnableProvenance:    true,
			ProvenanceRetention: 30,
			EnableHealthChecks:  true,
			HealthCheckInterval: 60,
		},
		Security: SecuritySettings{
			EnableAuthentication:  false,
			EnableAuthorization:   false,
			SessionTimeout:        60,
			PasswordMinLength:     8,
			RequireStrongPassword: false,
			EnableAuditLog:        true,
		},
	}
}

// HandleGetSettings handles GET /api/settings
func (h *SettingsHandlers) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	settings := h.settings
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, settings)
}

// HandleUpdateSettings handles PUT /api/settings
func (h *SettingsHandlers) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var newSettings Settings
	if err := json.NewDecoder(r.Body).Decode(&newSettings); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings = &newSettings
	h.mu.Unlock()

	h.log.Info("Settings updated")
	respondJSON(w, http.StatusOK, newSettings)
}

// HandleGetGeneralSettings handles GET /api/settings/general
func (h *SettingsHandlers) HandleGetGeneralSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	general := h.settings.General
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, general)
}

// HandleUpdateGeneralSettings handles PUT /api/settings/general
func (h *SettingsHandlers) HandleUpdateGeneralSettings(w http.ResponseWriter, r *http.Request) {
	var general GeneralSettings
	if err := json.NewDecoder(r.Body).Decode(&general); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings.General = general
	h.mu.Unlock()

	h.log.Info("General settings updated")
	respondJSON(w, http.StatusOK, general)
}

// HandleGetFlowSettings handles GET /api/settings/flow
func (h *SettingsHandlers) HandleGetFlowSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	flow := h.settings.Flow
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, flow)
}

// HandleUpdateFlowSettings handles PUT /api/settings/flow
func (h *SettingsHandlers) HandleUpdateFlowSettings(w http.ResponseWriter, r *http.Request) {
	var flow FlowSettings
	if err := json.NewDecoder(r.Body).Decode(&flow); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings.Flow = flow
	h.mu.Unlock()

	h.log.Info("Flow settings updated")
	respondJSON(w, http.StatusOK, flow)
}

// HandleGetStorageSettings handles GET /api/settings/storage
func (h *SettingsHandlers) HandleGetStorageSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	storage := h.settings.Storage
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, storage)
}

// HandleUpdateStorageSettings handles PUT /api/settings/storage
func (h *SettingsHandlers) HandleUpdateStorageSettings(w http.ResponseWriter, r *http.Request) {
	var storage StorageSettings
	if err := json.NewDecoder(r.Body).Decode(&storage); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings.Storage = storage
	h.mu.Unlock()

	h.log.Info("Storage settings updated")
	respondJSON(w, http.StatusOK, storage)
}

// HandleGetMonitoringSettings handles GET /api/settings/monitoring
func (h *SettingsHandlers) HandleGetMonitoringSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	monitoring := h.settings.Monitoring
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, monitoring)
}

// HandleUpdateMonitoringSettings handles PUT /api/settings/monitoring
func (h *SettingsHandlers) HandleUpdateMonitoringSettings(w http.ResponseWriter, r *http.Request) {
	var monitoring MonitoringSettings
	if err := json.NewDecoder(r.Body).Decode(&monitoring); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings.Monitoring = monitoring
	h.mu.Unlock()

	h.log.Info("Monitoring settings updated")
	respondJSON(w, http.StatusOK, monitoring)
}

// HandleGetSecuritySettings handles GET /api/settings/security
func (h *SettingsHandlers) HandleGetSecuritySettings(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	security := h.settings.Security
	h.mu.RUnlock()

	respondJSON(w, http.StatusOK, security)
}

// HandleUpdateSecuritySettings handles PUT /api/settings/security
func (h *SettingsHandlers) HandleUpdateSecuritySettings(w http.ResponseWriter, r *http.Request) {
	var security SecuritySettings
	if err := json.NewDecoder(r.Body).Decode(&security); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	h.mu.Lock()
	h.settings.Security = security
	h.mu.Unlock()

	h.log.Info("Security settings updated")
	respondJSON(w, http.StatusOK, security)
}

// HandleResetSettings handles POST /api/settings/reset
func (h *SettingsHandlers) HandleResetSettings(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.settings = getDefaultSettings()
	h.mu.Unlock()

	h.log.Info("Settings reset to defaults")
	respondJSON(w, http.StatusOK, h.settings)
}
