package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/shawntherrien/databridge/internal/api"
	"github.com/shawntherrien/databridge/internal/cluster"
	"github.com/shawntherrien/databridge/internal/core"
	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
	_ "github.com/shawntherrien/databridge/plugins" // Import to register built-in processors
)

var (
	cfgFile string
	logger  *logrus.Logger
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "databridge",
	Short: "DataBridge - Modern Data Flow Processing Engine",
	Long: `DataBridge is a modern, Go-based data flow processing engine
inspired by Apache NiFi. It provides a visual interface for designing
data flows and supports distributed processing.`,
	Run: runDataBridge,
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./databridge.yaml)")
	rootCmd.PersistentFlags().String("data-dir", "./data", "data directory for repositories")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Int("port", 8080, "HTTP server port")

	// Plugin flags
	rootCmd.PersistentFlags().String("plugin-dir", "./plugins", "plugin directory")
	rootCmd.PersistentFlags().Bool("auto-load-plugins", true, "automatically load plugins on startup")

	// Cluster flags
	rootCmd.PersistentFlags().Bool("cluster-enabled", false, "enable clustering")
	rootCmd.PersistentFlags().String("cluster-node-id", "", "cluster node ID (auto-generated if empty)")
	rootCmd.PersistentFlags().String("cluster-bind-address", "127.0.0.1", "cluster bind address")
	rootCmd.PersistentFlags().Int("cluster-bind-port", 8090, "cluster bind port")
	rootCmd.PersistentFlags().StringSlice("cluster-seeds", []string{}, "cluster seed nodes")
	rootCmd.PersistentFlags().String("cluster-discovery", "static", "cluster discovery method (static, multicast, dns, etcd)")
	rootCmd.PersistentFlags().Int("cluster-replica-count", 3, "number of replicas for state replication")

	// Bind flags to viper
	viper.BindPFlag("dataDir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
	viper.BindPFlag("pluginDir", rootCmd.PersistentFlags().Lookup("plugin-dir"))
	viper.BindPFlag("autoLoadPlugins", rootCmd.PersistentFlags().Lookup("auto-load-plugins"))
	viper.BindPFlag("cluster.enabled", rootCmd.PersistentFlags().Lookup("cluster-enabled"))
	viper.BindPFlag("cluster.nodeId", rootCmd.PersistentFlags().Lookup("cluster-node-id"))
	viper.BindPFlag("cluster.bindAddress", rootCmd.PersistentFlags().Lookup("cluster-bind-address"))
	viper.BindPFlag("cluster.bindPort", rootCmd.PersistentFlags().Lookup("cluster-bind-port"))
	viper.BindPFlag("cluster.seeds", rootCmd.PersistentFlags().Lookup("cluster-seeds"))
	viper.BindPFlag("cluster.discovery", rootCmd.PersistentFlags().Lookup("cluster-discovery"))
	viper.BindPFlag("cluster.replicaCount", rootCmd.PersistentFlags().Lookup("cluster-replica-count"))

	// Initialize logger
	logger = logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("databridge")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		logger.Info("Using config file:", viper.ConfigFileUsed())
	}

	// Set log level
	logLevel := viper.GetString("logLevel")
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logger.Warn("Invalid log level, using info", "level", logLevel)
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
}

func runDataBridge(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	if err := run(ctx, logger); err != nil {
		logger.WithError(err).Fatal("DataBridge failed")
	}
}

// run contains the main application logic and is testable
func run(ctx context.Context, log *logrus.Logger) error {
	log.Info("Starting DataBridge")

	// Create data directories
	dataDir := viper.GetString("dataDir")
	if err := createDirectories(dataDir); err != nil {
		return fmt.Errorf("failed to create data directories: %w", err)
	}

	// Initialize repositories
	flowFileRepo, err := core.NewBadgerFlowFileRepository(filepath.Join(dataDir, "flowfiles"))
	if err != nil {
		return fmt.Errorf("failed to initialize FlowFile repository: %w", err)
	}
	defer flowFileRepo.Close()

	contentRepo, err := core.NewFileSystemContentRepository(filepath.Join(dataDir, "content"))
	if err != nil {
		return fmt.Errorf("failed to initialize Content repository: %w", err)
	}
	defer contentRepo.Close()

	// For now, use a simple in-memory provenance repository
	provenanceRepo := core.NewInMemoryProvenanceRepository()

	// Initialize plugin manager
	pluginConfig := plugin.DefaultPluginManagerConfig()
	pluginConfig.PluginDir = viper.GetString("pluginDir")
	pluginConfig.AutoLoad = viper.GetBool("autoLoadPlugins")

	pluginManager, err := plugin.NewPluginManager(pluginConfig, log)
	if err != nil {
		return fmt.Errorf("failed to create plugin manager: %w", err)
	}

	// Initialize plugin manager (registers built-ins and loads external plugins)
	if err := pluginManager.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize plugin manager: %w", err)
	}
	defer pluginManager.Shutdown()

	log.WithFields(logrus.Fields{
		"totalPlugins": len(pluginManager.ListAllPlugins()),
		"builtIns":     pluginManager.BuiltIns.GetBuiltInCount(),
	}).Info("Plugin manager initialized")

	// Create FlowController with plugin support
	flowController := core.NewFlowControllerWithPlugins(
		flowFileRepo,
		contentRepo,
		provenanceRepo,
		pluginManager,
		log,
	)

	// Initialize cluster if enabled
	var clusterManager *cluster.ClusterManager
	if viper.GetBool("cluster.enabled") {
		clusterConfig := cluster.ClusterConfig{
			Enabled:          true,
			NodeID:           viper.GetString("cluster.nodeId"),
			BindAddress:      viper.GetString("cluster.bindAddress"),
			BindPort:         viper.GetInt("cluster.bindPort"),
			SeedNodes:        viper.GetStringSlice("cluster.seeds"),
			Discovery:        viper.GetString("cluster.discovery"),
			ReplicaCount:     viper.GetInt("cluster.replicaCount"),
			HeartbeatTimeout: 10 * time.Second,
			ElectionTimeout:  30 * time.Second,
		}

		cm, err := cluster.NewClusterManager(clusterConfig, log)
		if err != nil {
			return fmt.Errorf("failed to create cluster manager: %w", err)
		}
		clusterManager = cm

		// Start cluster manager
		if err := clusterManager.Start(); err != nil {
			return fmt.Errorf("failed to start cluster manager: %w", err)
		}
		defer func() {
			if err := clusterManager.Stop(); err != nil {
				log.WithError(err).Error("Error stopping cluster manager")
			}
		}()

		// Set cluster manager in flow controller
		flowController.SetClusterManager(clusterManager)

		log.WithFields(logrus.Fields{
			"nodeId":      clusterConfig.NodeID,
			"bindAddress": clusterConfig.BindAddress,
			"bindPort":    clusterConfig.BindPort,
		}).Info("Cluster mode enabled")
	}

	// Start FlowController
	if err := flowController.Start(); err != nil {
		return fmt.Errorf("failed to start FlowController: %w", err)
	}
	defer func() {
		if err := flowController.Stop(); err != nil {
			log.WithError(err).Error("Error stopping FlowController")
		}
	}()

	// Create default flow for UI
	if err := setupDefaultFlow(flowController, log); err != nil {
		return fmt.Errorf("failed to setup default flow: %w", err)
	}

	// Add example processors for demonstration
	if err := setupExampleFlow(flowController, log); err != nil {
		return fmt.Errorf("failed to add example flow: %w", err)
	}

	// Create and start monitoring API server
	apiConfig := api.Config{
		Port:                 viper.GetInt("port"),
		MetricsCacheInterval: 5 * time.Second,
		SSEUpdateInterval:    2 * time.Second,
	}

	apiServer := api.NewServer(flowController, flowController.GetScheduler(), log, apiConfig)

	// Register plugin API routes
	pluginAPIHandler := plugin.NewPluginAPIHandler(pluginManager, log)
	pluginAPIHandler.RegisterRoutes(apiServer.GetRouter())

	if err := apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := apiServer.Stop(shutdownCtx); err != nil {
			log.WithError(err).Error("Error stopping API server")
		}
	}()

	log.Info("DataBridge started successfully")

	// Wait for interrupt signal
	return waitForShutdownSignal(ctx, log)
}

func createDirectories(dataDir string) error {
	dirs := []string{
		filepath.Join(dataDir, "flowfiles"),
		filepath.Join(dataDir, "content"),
		filepath.Join(dataDir, "provenance"),
		filepath.Join(dataDir, "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// waitForShutdownSignal waits for SIGINT or SIGTERM and returns
func waitForShutdownSignal(ctx context.Context, log *logrus.Logger) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.WithField("signal", sig).Info("Received shutdown signal")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// setupDefaultFlow creates a default process group for the UI
func setupDefaultFlow(flowController *core.FlowController, log *logrus.Logger) error {
	log.Info("Setting up default flow for UI")

	// Create a default process group for the frontend to use
	flow, err := flowController.CreateProcessGroup("My Flow", nil)
	if err != nil {
		return fmt.Errorf("failed to create default flow: %w", err)
	}

	log.WithField("flowId", flow.ID).Info("Default flow created successfully - use this ID in frontend")
	return nil
}

// setupExampleFlow adds and configures example processors for demonstration
func setupExampleFlow(flowController *core.FlowController, log *logrus.Logger) error {
	log.Info("Adding example flow")

	// Create a GenerateFlowFile processor using plugin manager
	generateConfig := types.ProcessorConfig{
		ID:           uuid.New(),
		Name:         "Generate Sample Data",
		Type:         "GenerateFlowFile",
		ScheduleType: types.ScheduleTypeTimer,
		ScheduleValue: "5s", // Generate every 5 seconds
		Concurrency:  1,
		Properties: map[string]string{
			"File Size":        "512",
			"Content":          "Sample data from DataBridge",
			"Unique FlowFiles": "true",
			"Custom Text":      "Demo Flow",
		},
		AutoTerminate: map[string]bool{
			"success": false, // Don't auto-terminate, we'll connect to another processor
		},
	}

	generateNode, err := flowController.CreateProcessorByType("GenerateFlowFile", generateConfig)
	if err != nil {
		return fmt.Errorf("failed to add GenerateFlowFile processor: %w", err)
	}

	// Create a LogAttribute processor (we'll implement this as a simple example)
	logProcessor := &SimpleLogProcessor{}
	logConfig := types.ProcessorConfig{
		ID:           uuid.New(),
		Name:         "Log Attributes",
		Type:         "LogAttribute",
		ScheduleType: types.ScheduleTypeEvent, // Event-driven
		ScheduleValue: "0s",
		Concurrency:  1,
		Properties: map[string]string{
			"Log Level": "INFO",
		},
		AutoTerminate: map[string]bool{
			"success": true, // Auto-terminate success relationship
		},
	}

	logNode, err := flowController.AddProcessor(logProcessor, logConfig)
	if err != nil {
		return fmt.Errorf("failed to add LogAttribute processor: %w", err)
	}

	// Connect the processors
	_, err = flowController.AddConnection(
		generateNode.ID,
		logNode.ID,
		types.RelationshipSuccess,
	)
	if err != nil {
		return fmt.Errorf("failed to connect processors: %w", err)
	}

	// Start the processors
	if err := flowController.StartProcessor(generateNode.ID); err != nil {
		return fmt.Errorf("failed to start GenerateFlowFile processor: %w", err)
	}

	if err := flowController.StartProcessor(logNode.ID); err != nil {
		return fmt.Errorf("failed to start LogAttribute processor: %w", err)
	}

	log.Info("Example flow added and started successfully")
	return nil
}

// SimpleLogProcessor is a basic processor that logs FlowFile attributes
type SimpleLogProcessor struct {
	*types.BaseProcessor
}

func init() {
	// Initialize the SimpleLogProcessor
	info := types.ProcessorInfo{
		Name:        "LogAttribute",
		Description: "Logs FlowFile attributes and content information",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"logging", "debug", "attributes"},
		Properties: []types.PropertySpec{
			{
				Name:         "Log Level",
				Description:  "Log level for output",
				Required:     false,
				DefaultValue: "INFO",
				AllowedValues: []string{"DEBUG", "INFO", "WARN", "ERROR"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}
	// This would be set in a proper initialization
	_ = info
}

// Initialize initializes the processor
func (p *SimpleLogProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// GetInfo returns processor metadata
func (p *SimpleLogProcessor) GetInfo() types.ProcessorInfo {
	return types.ProcessorInfo{
		Name:        "LogAttribute",
		Description: "Logs FlowFile attributes and content information",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"logging", "debug", "attributes"},
		Properties: []types.PropertySpec{
			{
				Name:         "Log Level",
				Description:  "Log level for output",
				Required:     false,
				DefaultValue: "INFO",
				AllowedValues: []string{"DEBUG", "INFO", "WARN", "ERROR"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}
}

// OnTrigger processes FlowFiles
func (p *SimpleLogProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get FlowFile from input
	flowFile := session.Get()
	if flowFile == nil {
		// No FlowFile available
		return nil
	}

	// Log FlowFile information
	logger.Info("Processing FlowFile",
		"flowFileId", flowFile.ID,
		"size", flowFile.Size,
		"attributeCount", len(flowFile.Attributes))

	// Log all attributes
	for key, value := range flowFile.Attributes {
		logger.Info("FlowFile Attribute",
			"flowFileId", flowFile.ID,
			"key", key,
			"value", value)
	}

	// Read and log content size (don't log actual content to avoid spam)
	if flowFile.ContentClaim != nil {
		content, err := session.Read(flowFile)
		if err != nil {
			logger.Error("Failed to read FlowFile content", "error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
		} else {
			logger.Info("FlowFile Content",
				"flowFileId", flowFile.ID,
				"contentLength", len(content),
				"preview", string(content[:min(100, len(content))]))

			session.Transfer(flowFile, types.RelationshipSuccess)
		}
	} else {
		logger.Info("FlowFile has no content claim", "flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.RelationshipSuccess)
	}

	return nil
}

// Validate validates the processor configuration
func (p *SimpleLogProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	var results []types.ValidationResult

	if logLevel, exists := config.Properties["Log Level"]; exists {
		validLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
		valid := false
		for _, level := range validLevels {
			if logLevel == level {
				valid = true
				break
			}
		}
		if !valid {
			results = append(results, types.ValidationResult{
				Property: "Log Level",
				Valid:    false,
				Message:  "Log Level must be one of: DEBUG, INFO, WARN, ERROR",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *SimpleLogProcessor) OnStopped(ctx context.Context) {
	// No cleanup needed
}


// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

