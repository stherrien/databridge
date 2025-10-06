package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/shawntherrien/databridge/internal/core"
)

// Server represents the REST API server
type Server struct {
	httpServer *http.Server
	router     *mux.Router
	logger     *logrus.Logger
	collector  *MetricsCollector
	sseHandler *SSEHandler
	port       int
}

// Config represents server configuration
type Config struct {
	Port                 int
	MetricsCacheInterval time.Duration
	SSEUpdateInterval    time.Duration
}

// DefaultConfig returns default server configuration
func DefaultConfig() Config {
	return Config{
		Port:                 8080,
		MetricsCacheInterval: 5 * time.Second,
		SSEUpdateInterval:    2 * time.Second,
	}
}

// NewServer creates a new API server
func NewServer(flowController *core.FlowController, scheduler *core.ProcessScheduler, logger *logrus.Logger, config Config) *Server {
	// Create metrics collector
	collector := NewMetricsCollector(flowController, scheduler, config.MetricsCacheInterval)

	// Create SSE handler
	sseHandler := NewSSEHandler(collector, logger, config.SSEUpdateInterval)

	// Create monitoring handlers
	monitoringHandlers := NewMonitoringHandlers(collector)

	// Create flow handlers
	flowHandlers := NewFlowHandlers(flowController)

	// Create template handlers
	templateHandlers := NewTemplateHandlers(logger)

	// Create provenance handlers
	provenanceHandlers := NewProvenanceHandlers(logger)

	// Create settings handlers
	settingsHandlers := NewSettingsHandlers(logger)

	// Create filesystem handlers
	filesystemHandlers := NewFilesystemHandlers()

	// Create router
	router := mux.NewRouter()

	// Register middleware
	router.Use(loggingMiddleware(logger))
	router.Use(corsMiddleware)

	// Register system endpoints
	api := router.PathPrefix("/api").Subrouter()

	// System status endpoints
	api.HandleFunc("/system/status", monitoringHandlers.HandleSystemStatus).Methods("GET", "OPTIONS")
	api.HandleFunc("/system/health", monitoringHandlers.HandleHealth).Methods("GET", "OPTIONS")
	api.HandleFunc("/system/metrics", monitoringHandlers.HandleSystemMetrics).Methods("GET", "OPTIONS")
	api.HandleFunc("/system/stats", monitoringHandlers.HandleSystemStats).Methods("GET", "OPTIONS")

	// Component monitoring endpoints
	api.HandleFunc("/monitoring/processors", monitoringHandlers.HandleProcessors).Methods("GET", "OPTIONS")
	api.HandleFunc("/monitoring/processors/{id}", monitoringHandlers.HandleProcessor).Methods("GET", "OPTIONS")
	api.HandleFunc("/monitoring/connections", monitoringHandlers.HandleConnections).Methods("GET", "OPTIONS")
	api.HandleFunc("/monitoring/connections/{id}", monitoringHandlers.HandleConnection).Methods("GET", "OPTIONS")
	api.HandleFunc("/monitoring/queues", monitoringHandlers.HandleQueues).Methods("GET", "OPTIONS")

	// Real-time streaming endpoint
	api.HandleFunc("/monitoring/stream", sseHandler.HandleStream).Methods("GET")

	// Flow management endpoints
	api.HandleFunc("/flows", flowHandlers.HandleListFlows).Methods("GET", "OPTIONS")
	api.HandleFunc("/flows", flowHandlers.HandleCreateFlow).Methods("POST", "OPTIONS")
	api.HandleFunc("/flows/{id}", flowHandlers.HandleGetFlow).Methods("GET", "OPTIONS")
	api.HandleFunc("/flows/{id}", flowHandlers.HandleUpdateFlow).Methods("PUT", "OPTIONS")
	api.HandleFunc("/flows/{id}", flowHandlers.HandleDeleteFlow).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/flows/{id}/start", flowHandlers.HandleStartFlow).Methods("POST", "OPTIONS")
	api.HandleFunc("/flows/{id}/stop", flowHandlers.HandleStopFlow).Methods("POST", "OPTIONS")

	// Processor endpoints
	api.HandleFunc("/processors/types", flowHandlers.HandleGetProcessorTypes).Methods("GET", "OPTIONS")
	api.HandleFunc("/processors/types/{type}", flowHandlers.HandleGetProcessorMetadata).Methods("GET", "OPTIONS")
	api.HandleFunc("/flows/{id}/processors", flowHandlers.HandleCreateProcessor).Methods("POST", "OPTIONS")
	api.HandleFunc("/flows/{flowId}/processors/{processorId}", flowHandlers.HandleUpdateProcessor).Methods("PUT", "OPTIONS")
	api.HandleFunc("/flows/{flowId}/processors/{processorId}", flowHandlers.HandleDeleteProcessor).Methods("DELETE", "OPTIONS")

	// Connection endpoints
	api.HandleFunc("/flows/{id}/connections", flowHandlers.HandleCreateConnection).Methods("POST", "OPTIONS")
	api.HandleFunc("/flows/{flowId}/connections/{connectionId}", flowHandlers.HandleDeleteConnection).Methods("DELETE", "OPTIONS")

	// Flow import/export and validation endpoints
	api.HandleFunc("/flows/{id}/export", flowHandlers.HandleExportFlow).Methods("GET", "OPTIONS")
	api.HandleFunc("/flows/import", flowHandlers.HandleImportFlow).Methods("POST", "OPTIONS")
	api.HandleFunc("/flows/{id}/validate", flowHandlers.HandleValidateFlow).Methods("POST", "OPTIONS")

	// Template endpoints
	api.HandleFunc("/templates", templateHandlers.HandleListTemplates).Methods("GET", "OPTIONS")
	api.HandleFunc("/templates", templateHandlers.HandleCreateTemplate).Methods("POST", "OPTIONS")
	api.HandleFunc("/templates/search", templateHandlers.HandleSearchTemplates).Methods("GET", "OPTIONS")
	api.HandleFunc("/templates/{id}", templateHandlers.HandleGetTemplate).Methods("GET", "OPTIONS")
	api.HandleFunc("/templates/{id}", templateHandlers.HandleUpdateTemplate).Methods("PUT", "OPTIONS")
	api.HandleFunc("/templates/{id}", templateHandlers.HandleDeleteTemplate).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/templates/{id}/instantiate", templateHandlers.HandleInstantiateTemplate).Methods("POST", "OPTIONS")

	// Provenance endpoints
	api.HandleFunc("/provenance/events", provenanceHandlers.HandleGetEvents).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/events/search", provenanceHandlers.HandleSearchEvents).Methods("POST", "OPTIONS")
	api.HandleFunc("/provenance/events/{id}", provenanceHandlers.HandleGetEvent).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/lineage/{flowFileId}", provenanceHandlers.HandleGetLineage).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/timeline", provenanceHandlers.HandleGetTimeline).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/stats", provenanceHandlers.HandleGetStatistics).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/stats/processors", provenanceHandlers.HandleGetProcessorStatistics).Methods("GET", "OPTIONS")
	api.HandleFunc("/provenance/stats/event-types", provenanceHandlers.HandleGetEventTypeStatistics).Methods("GET", "OPTIONS")

	// Settings endpoints
	api.HandleFunc("/settings", settingsHandlers.HandleGetSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings", settingsHandlers.HandleUpdateSettings).Methods("PUT", "OPTIONS")
	api.HandleFunc("/settings/reset", settingsHandlers.HandleResetSettings).Methods("POST", "OPTIONS")
	api.HandleFunc("/settings/general", settingsHandlers.HandleGetGeneralSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/general", settingsHandlers.HandleUpdateGeneralSettings).Methods("PUT", "OPTIONS")
	api.HandleFunc("/settings/flow", settingsHandlers.HandleGetFlowSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/flow", settingsHandlers.HandleUpdateFlowSettings).Methods("PUT", "OPTIONS")
	api.HandleFunc("/settings/storage", settingsHandlers.HandleGetStorageSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/storage", settingsHandlers.HandleUpdateStorageSettings).Methods("PUT", "OPTIONS")
	api.HandleFunc("/settings/monitoring", settingsHandlers.HandleGetMonitoringSettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/monitoring", settingsHandlers.HandleUpdateMonitoringSettings).Methods("PUT", "OPTIONS")
	api.HandleFunc("/settings/security", settingsHandlers.HandleGetSecuritySettings).Methods("GET", "OPTIONS")
	api.HandleFunc("/settings/security", settingsHandlers.HandleUpdateSecuritySettings).Methods("PUT", "OPTIONS")

	// Filesystem endpoints
	api.HandleFunc("/filesystem/browse", filesystemHandlers.HandleBrowseFilesystem).Methods("GET", "OPTIONS")
	api.HandleFunc("/filesystem/validate", filesystemHandlers.HandleValidatePath).Methods("POST", "OPTIONS")

	// FlowFile inspection and queue management endpoints
	flowFileHandlers := NewFlowFileHandlers(flowController)
	api.HandleFunc("/flowfiles/{id}", flowFileHandlers.HandleGetFlowFile).Methods("GET", "OPTIONS")
	api.HandleFunc("/flowfiles/{id}/content", flowFileHandlers.HandleGetFlowFileContent).Methods("GET", "OPTIONS")
	api.HandleFunc("/connections/{id}/queue", flowFileHandlers.HandleGetConnectionQueue).Methods("GET", "OPTIONS")
	api.HandleFunc("/connections/{id}/queue", flowFileHandlers.HandleClearConnectionQueue).Methods("DELETE", "OPTIONS")

	// Health check endpoint (simple version without /api prefix)
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}).Methods("GET")

	server := &Server{
		router:     router,
		logger:     logger,
		collector:  collector,
		sseHandler: sseHandler,
		port:       config.Port,
	}

	return server
}

// Start starts the API server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.WithField("port", s.port).Info("Starting API server")

	// Start server in goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("API server error")
		}
	}()

	return nil
}

// Stop stops the API server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping API server")

	// Stop SSE handler
	s.sseHandler.Stop()

	// Shutdown HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down API server: %w", err)
		}
	}

	s.logger.Info("API server stopped")
	return nil
}

// GetRouter returns the mux router for registering additional routes
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// Middleware

func loggingMiddleware(logger *logrus.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response wrapper to capture status code
			wrapper := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call next handler
			next.ServeHTTP(wrapper, r)

			// Log request
			logger.WithFields(logrus.Fields{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     wrapper.statusCode,
				"duration":   time.Since(start),
				"remoteAddr": r.RemoteAddr,
			}).Info("HTTP request")
		})
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
