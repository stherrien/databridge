package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

// ExecuteSQLProcessor executes SQL queries against databases
type ExecuteSQLProcessor struct {
	*types.BaseProcessor
	db            *sql.DB
	processorCtx  types.ProcessorContext
	isInitialized bool
	mu            sync.Mutex
}

// SQLResult represents a query result
type SQLResult struct {
	Columns []string                 `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
	Count   int                      `json:"count"`
}

// NewExecuteSQLProcessor creates a new ExecuteSQL processor
func NewExecuteSQLProcessor() *ExecuteSQLProcessor {
	info := types.ProcessorInfo{
		Name:        "ExecuteSQL",
		Description: "Executes SQL queries against relational databases and creates FlowFiles with results",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"sql", "database", "query", "rdbms", "data"},
		Properties: []types.PropertySpec{
			{
				Name:          "Database Type",
				Description:   "Type of database to connect to",
				Required:      true,
				DefaultValue:  "postgres",
				AllowedValues: []string{"postgres", "mysql", "sqlite3"},
			},
			{
				Name:         "Connection String",
				Description:  "Database connection string (format depends on database type)",
				Required:     true,
				DefaultValue: "",
				Sensitive:    true,
			},
			{
				Name:         "SQL Query",
				Description:  "SQL query to execute (supports FlowFile attribute expressions with ${attr})",
				Required:     true,
				DefaultValue: "",
			},
			{
				Name:         "Query Timeout",
				Description:  "Timeout for query execution (e.g., 30s, 1m)",
				Required:     false,
				DefaultValue: "30s",
			},
			{
				Name:         "Max Rows",
				Description:  "Maximum number of rows to return (0 = unlimited)",
				Required:     false,
				DefaultValue: "0",
			},
			{
				Name:          "Result Format",
				Description:   "Format for query results",
				Required:      false,
				DefaultValue:  "json",
				AllowedValues: []string{"json", "csv", "avro"},
			},
			{
				Name:         "Fetch Size",
				Description:  "Number of rows to fetch at a time",
				Required:     false,
				DefaultValue: "1000",
			},
			{
				Name:         "Max Connection Pool Size",
				Description:  "Maximum number of database connections",
				Required:     false,
				DefaultValue: "10",
			},
			{
				Name:         "Connection Max Lifetime",
				Description:  "Maximum lifetime of a connection (e.g., 30m, 1h)",
				Required:     false,
				DefaultValue: "30m",
			},
			{
				Name:          "Normalize Column Names",
				Description:   "Convert column names to lowercase",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:          "Output Empty Results",
				Description:   "Create FlowFile even if query returns no rows",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
			},
			{
				Name:          "Query Mode",
				Description:   "How to execute the query",
				Required:      false,
				DefaultValue:  "select",
				AllowedValues: []string{"select", "update", "transaction"},
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &ExecuteSQLProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the database connection
func (p *ExecuteSQLProcessor) Initialize(ctx types.ProcessorContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isInitialized {
		return nil
	}

	logger := ctx.GetLogger()
	logger.Info("Initializing ExecuteSQL processor")

	p.processorCtx = ctx

	// Validate required properties
	if !ctx.HasProperty("Database Type") {
		return fmt.Errorf("Database Type property is required")
	}

	if !ctx.HasProperty("Connection String") {
		return fmt.Errorf("Connection String property is required")
	}

	if !ctx.HasProperty("SQL Query") {
		return fmt.Errorf("SQL Query property is required")
	}

	dbType := ctx.GetPropertyValue("Database Type")
	connStr := ctx.GetPropertyValue("Connection String")

	if connStr == "" {
		return fmt.Errorf("Connection String cannot be empty")
	}

	// Open database connection
	db, err := sql.Open(dbType, connStr)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Configure connection pool
	maxPoolSizeStr := ctx.GetPropertyValue("Max Connection Pool Size")
	if maxPoolSizeStr != "" {
		maxPoolSize, err := strconv.Atoi(maxPoolSizeStr)
		if err != nil {
			db.Close()
			return fmt.Errorf("invalid Max Connection Pool Size: %w", err)
		}
		db.SetMaxOpenConns(maxPoolSize)
		db.SetMaxIdleConns(maxPoolSize / 2)
	}

	// Configure connection lifetime
	connLifetimeStr := ctx.GetPropertyValue("Connection Max Lifetime")
	if connLifetimeStr != "" {
		connLifetime, err := time.ParseDuration(connLifetimeStr)
		if err != nil {
			db.Close()
			return fmt.Errorf("invalid Connection Max Lifetime: %w", err)
		}
		db.SetConnMaxLifetime(connLifetime)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.db = db
	p.isInitialized = true

	logger.Info("Database connection established successfully")

	return nil
}

// OnTrigger executes SQL queries
func (p *ExecuteSQLProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	if !p.isInitialized {
		return fmt.Errorf("processor not initialized")
	}

	logger := p.processorCtx.GetLogger()

	// Get FlowFile from session (optional - can generate results without input)
	flowFile := session.Get()

	// Get SQL query with attribute replacement
	sqlQuery := p.processorCtx.GetPropertyValue("SQL Query")
	if flowFile != nil && strings.Contains(sqlQuery, "${") {
		// Replace attribute expressions
		for key, value := range flowFile.Attributes {
			placeholder := "${" + key + "}"
			sqlQuery = strings.ReplaceAll(sqlQuery, placeholder, value)
		}
	}

	// Get query timeout
	timeoutStr := p.processorCtx.GetPropertyValue("Query Timeout")
	timeout := 30 * time.Second
	if timeoutStr != "" {
		var err error
		timeout, err = time.ParseDuration(timeoutStr)
		if err != nil {
			logger.Warn("Invalid Query Timeout, using default 30s")
		}
	}

	// Create query context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute query based on mode
	queryMode := p.processorCtx.GetPropertyValue("Query Mode")
	var result *SQLResult
	var err error

	switch queryMode {
	case "update":
		result, err = p.executeUpdate(queryCtx, sqlQuery)
	case "transaction":
		result, err = p.executeTransaction(queryCtx, sqlQuery)
	default:
		result, err = p.executeSelect(queryCtx, sqlQuery)
	}

	if err != nil {
		logger.Error("Failed to execute SQL query: " + err.Error())
		if flowFile != nil {
			session.Transfer(flowFile, types.RelationshipFailure)
		}
		return err
	}

	// Check if we should output empty results
	outputEmpty := p.processorCtx.GetPropertyValue("Output Empty Results") == "true"
	if result.Count == 0 && !outputEmpty {
		logger.Debug("Query returned no results, not creating FlowFile")
		if flowFile != nil {
			session.Remove(flowFile)
		}
		return nil
	}

	// Format results
	resultFormat := p.processorCtx.GetPropertyValue("Result Format")
	var content []byte
	var mimeType string

	switch resultFormat {
	case "csv":
		content, err = p.formatCSV(result)
		mimeType = "text/csv"
	case "avro":
		content, err = p.formatAvro(result)
		mimeType = "application/avro"
	default:
		content, err = p.formatJSON(result)
		mimeType = "application/json"
	}

	if err != nil {
		logger.Error("Failed to format query results: " + err.Error())
		if flowFile != nil {
			session.Transfer(flowFile, types.RelationshipFailure)
		}
		return err
	}

	// Create or update FlowFile
	var outputFlowFile *types.FlowFile
	if flowFile != nil {
		outputFlowFile = flowFile
	} else {
		outputFlowFile = session.Create()
	}

	// Write content
	if err := session.Write(outputFlowFile, content); err != nil {
		logger.Error("Failed to write query results: " + err.Error())
		session.Remove(outputFlowFile)
		return err
	}

	// Add attributes
	attributes := map[string]string{
		"sql.result.count":   strconv.Itoa(result.Count),
		"sql.result.format":  resultFormat,
		"mime.type":          mimeType,
		"sql.query":          sqlQuery,
		"sql.execution.time": time.Now().Format(time.RFC3339),
	}

	session.PutAllAttributes(outputFlowFile, attributes)

	// Transfer to success
	session.Transfer(outputFlowFile, types.RelationshipSuccess)

	logger.Debug("Successfully executed SQL query")

	return nil
}

// executeSelect executes a SELECT query
func (p *ExecuteSQLProcessor) executeSelect(ctx context.Context, query string) (*SQLResult, error) {
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Normalize column names if requested
	if p.processorCtx.GetPropertyValue("Normalize Column Names") == "true" {
		for i, col := range columns {
			columns[i] = strings.ToLower(col)
		}
	}

	// Get max rows limit
	maxRowsStr := p.processorCtx.GetPropertyValue("Max Rows")
	maxRows := 0
	if maxRowsStr != "" {
		maxRows, _ = strconv.Atoi(maxRowsStr)
	}

	// Fetch results
	result := &SQLResult{
		Columns: columns,
		Rows:    make([]map[string]interface{}, 0),
	}

	rowCount := 0
	for rows.Next() {
		if maxRows > 0 && rowCount >= maxRows {
			break
		}

		// Create slice for column values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan row
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}

		result.Rows = append(result.Rows, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	result.Count = len(result.Rows)
	return result, nil
}

// executeUpdate executes an UPDATE/INSERT/DELETE query
func (p *ExecuteSQLProcessor) executeUpdate(ctx context.Context, query string) (*SQLResult, error) {
	result, err := p.db.ExecContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("update execution failed: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return &SQLResult{
		Columns: []string{"rows_affected"},
		Rows: []map[string]interface{}{
			{"rows_affected": rowsAffected},
		},
		Count: 1,
	}, nil
}

// executeTransaction executes multiple queries in a transaction
func (p *ExecuteSQLProcessor) executeTransaction(ctx context.Context, queries string) (*SQLResult, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Split queries by semicolon
	queryList := strings.Split(queries, ";")
	totalAffected := int64(0)

	for _, query := range queryList {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}

		result, err := tx.ExecContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("transaction query failed: %w", err)
		}

		affected, _ := result.RowsAffected()
		totalAffected += affected
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &SQLResult{
		Columns: []string{"rows_affected"},
		Rows: []map[string]interface{}{
			{"rows_affected": totalAffected},
		},
		Count: 1,
	}, nil
}

// formatJSON formats results as JSON
func (p *ExecuteSQLProcessor) formatJSON(result *SQLResult) ([]byte, error) {
	return json.Marshal(result)
}

// formatCSV formats results as CSV
func (p *ExecuteSQLProcessor) formatCSV(result *SQLResult) ([]byte, error) {
	var builder strings.Builder

	// Write header
	builder.WriteString(strings.Join(result.Columns, ","))
	builder.WriteString("\n")

	// Write rows
	for _, row := range result.Rows {
		values := make([]string, len(result.Columns))
		for i, col := range result.Columns {
			if val, ok := row[col]; ok {
				values[i] = fmt.Sprintf("%v", val)
			}
		}
		builder.WriteString(strings.Join(values, ","))
		builder.WriteString("\n")
	}

	return []byte(builder.String()), nil
}

// formatAvro formats results as Avro (simplified, would need proper Avro library)
func (p *ExecuteSQLProcessor) formatAvro(result *SQLResult) ([]byte, error) {
	// For now, just return JSON (proper Avro would require additional dependencies)
	return p.formatJSON(result)
}

// Cleanup cleans up resources
func (p *ExecuteSQLProcessor) Cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isInitialized {
		return nil
	}

	logger := p.processorCtx.GetLogger()
	logger.Info("Cleaning up ExecuteSQL processor")

	if p.db != nil {
		if err := p.db.Close(); err != nil {
			logger.Error("Error closing database connection: " + err.Error())
			return err
		}
	}

	p.isInitialized = false
	logger.Info("ExecuteSQL processor cleaned up successfully")

	return nil
}

// getExecuteSQLInfo returns processor metadata
func getExecuteSQLInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ExecuteSQL",
		"ExecuteSQL",
		"1.0.0",
		"DataBridge",
		"Executes SQL queries against databases",
		[]string{"database", "sql", "query"},
	)
}

// Register the processor
func init() {
	plugin.RegisterBuiltInProcessor("ExecuteSQL", func() types.Processor {
		return NewExecuteSQLProcessor()
	}, getExecuteSQLInfo())
}
