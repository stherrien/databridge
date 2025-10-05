package plugins

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getConvertRecordInfo()
	plugin.RegisterBuiltInProcessor("ConvertRecord", func() types.Processor {
		return NewConvertRecordProcessor()
	}, info)
}

func getConvertRecordInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ConvertRecord",
		"ConvertRecord",
		"1.0.0",
		"DataBridge",
		"Converts records from one data format to another (CSV, JSON, XML). Supports schema inference and format-specific options.",
		[]string{"conversion", "transformation", "csv", "json", "xml"},
	)
}

// ConvertRecordProcessor converts between different data formats
type ConvertRecordProcessor struct {
	*types.BaseProcessor
}

// NewConvertRecordProcessor creates a new ConvertRecord processor
func NewConvertRecordProcessor() *ConvertRecordProcessor {
	info := types.ProcessorInfo{
		Name:        "ConvertRecord",
		Description: "Converts records from one data format to another (CSV, JSON, XML)",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"conversion", "transformation", "csv", "json", "xml"},
		Properties: []types.PropertySpec{
			{
				Name:         "Input Format",
				DisplayName:  "Input Format",
				Description:  "Format of the incoming data",
				Required:     true,
				DefaultValue: "json",
				AllowedValues: []string{"json", "csv", "xml"},
				Type:         "select",
				HelpText:     "Select the format of the input FlowFile content",
			},
			{
				Name:         "Output Format",
				DisplayName:  "Output Format",
				Description:  "Format to convert the data to",
				Required:     true,
				DefaultValue: "json",
				AllowedValues: []string{"json", "csv", "xml"},
				Type:         "select",
				HelpText:     "Select the desired output format",
			},
			{
				Name:         "CSV Delimiter",
				DisplayName:  "CSV Delimiter",
				Description:  "Delimiter character for CSV format (input or output)",
				Required:     false,
				DefaultValue: ",",
				Type:         "string",
				Placeholder:  ",",
				HelpText:     "Single character to use as CSV delimiter (e.g., ',' ';' '\\t')",
			},
			{
				Name:         "CSV Header",
				DisplayName:  "CSV Has Header",
				Description:  "Whether CSV input has a header row",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "Set to true if CSV has header row with column names",
			},
			{
				Name:         "Pretty Print",
				DisplayName:  "Pretty Print JSON",
				Description:  "Whether to format JSON output with indentation",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "Enable for human-readable JSON output (adds whitespace)",
			},
			{
				Name:         "XML Root Element",
				DisplayName:  "XML Root Element",
				Description:  "Name of the root element for XML output",
				Required:     false,
				DefaultValue: "records",
				Type:         "string",
				Placeholder:  "records",
				HelpText:     "Root element name when converting to XML",
			},
			{
				Name:         "XML Record Element",
				DisplayName:  "XML Record Element",
				Description:  "Name of individual record elements in XML",
				Required:     false,
				DefaultValue: "record",
				Type:         "string",
				Placeholder:  "record",
				HelpText:     "Element name for each record when converting to XML",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &ConvertRecordProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *ConvertRecordProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *ConvertRecordProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get FlowFile from input
	flowFile := session.Get()
	if flowFile == nil {
		return nil
	}

	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("failed to get processor context")
	}

	// Get configuration
	inputFormat := processorCtx.GetPropertyValue("Input Format")
	outputFormat := processorCtx.GetPropertyValue("Output Format")

	if inputFormat == outputFormat {
		logger.Warn("Input and output formats are the same, no conversion needed",
			"flowFileId", flowFile.ID,
			"format", inputFormat)
		session.Transfer(flowFile, types.RelationshipSuccess)
		return nil
	}

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Parse input data
	records, err := p.parseInput(content, inputFormat, processorCtx, logger)
	if err != nil {
		logger.Error("Failed to parse input data",
			"flowFileId", flowFile.ID,
			"inputFormat", inputFormat,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Convert to output format
	outputContent, err := p.formatOutput(records, outputFormat, processorCtx, logger)
	if err != nil {
		logger.Error("Failed to format output data",
			"flowFileId", flowFile.ID,
			"outputFormat", outputFormat,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Write converted content
	if err := session.Write(flowFile, outputContent); err != nil {
		logger.Error("Failed to write converted content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Update attributes
	session.PutAttribute(flowFile, "record.format", outputFormat)
	session.PutAttribute(flowFile, "record.count", fmt.Sprintf("%d", len(records)))
	session.PutAttribute(flowFile, "mime.type", getMimeType(outputFormat))

	logger.Info("Successfully converted records",
		"flowFileId", flowFile.ID,
		"inputFormat", inputFormat,
		"outputFormat", outputFormat,
		"recordCount", len(records))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}

// parseInput parses input content based on format
func (p *ConvertRecordProcessor) parseInput(content []byte, format string, ctx types.ProcessorContext, logger types.Logger) ([]map[string]interface{}, error) {
	switch format {
	case "json":
		return p.parseJSON(content)
	case "csv":
		return p.parseCSV(content, ctx)
	case "xml":
		return p.parseXML(content, ctx)
	default:
		return nil, fmt.Errorf("unsupported input format: %s", format)
	}
}

// parseJSON parses JSON input
func (p *ConvertRecordProcessor) parseJSON(content []byte) ([]map[string]interface{}, error) {
	var records []map[string]interface{}

	// Try parsing as array first
	if err := json.Unmarshal(content, &records); err == nil {
		return records, nil
	}

	// Try parsing as single object
	var record map[string]interface{}
	if err := json.Unmarshal(content, &record); err == nil {
		return []map[string]interface{}{record}, nil
	}

	return nil, fmt.Errorf("invalid JSON format: expected array or object")
}

// parseCSV parses CSV input
func (p *ConvertRecordProcessor) parseCSV(content []byte, ctx types.ProcessorContext) ([]map[string]interface{}, error) {
	delimiter := ctx.GetPropertyValue("CSV Delimiter")
	if delimiter == "" {
		delimiter = ","
	}
	hasHeader := ctx.GetPropertyValue("CSV Header") == "true"

	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.Comma = rune(delimiter[0])
	reader.TrimLeadingSpace = true

	// Read all records
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(rows) == 0 {
		return []map[string]interface{}{}, nil
	}

	var headers []string
	startRow := 0

	if hasHeader {
		headers = rows[0]
		startRow = 1
	} else {
		// Generate column names
		for i := 0; i < len(rows[0]); i++ {
			headers = append(headers, fmt.Sprintf("column_%d", i))
		}
	}

	// Convert rows to records
	records := make([]map[string]interface{}, 0, len(rows)-startRow)
	for i := startRow; i < len(rows); i++ {
		record := make(map[string]interface{})
		for j, value := range rows[i] {
			if j < len(headers) {
				record[headers[j]] = value
			}
		}
		records = append(records, record)
	}

	return records, nil
}

// parseXML parses XML input
func (p *ConvertRecordProcessor) parseXML(content []byte, ctx types.ProcessorContext) ([]map[string]interface{}, error) {
	type XMLRecord struct {
		XMLName xml.Name
		Content string `xml:",innerxml"`
	}

	type XMLRoot struct {
		Records []XMLRecord `xml:",any"`
	}

	var root XMLRoot
	if err := xml.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	// Convert XML records to maps
	records := make([]map[string]interface{}, 0, len(root.Records))
	for _, xmlRecord := range root.Records {
		// Simple conversion - in production, would need proper XML to map conversion
		record := map[string]interface{}{
			"element": xmlRecord.XMLName.Local,
			"content": xmlRecord.Content,
		}
		records = append(records, record)
	}

	return records, nil
}

// formatOutput formats records to output format
func (p *ConvertRecordProcessor) formatOutput(records []map[string]interface{}, format string, ctx types.ProcessorContext, logger types.Logger) ([]byte, error) {
	switch format {
	case "json":
		return p.formatJSON(records, ctx)
	case "csv":
		return p.formatCSV(records, ctx)
	case "xml":
		return p.formatXML(records, ctx)
	default:
		return nil, fmt.Errorf("unsupported output format: %s", format)
	}
}

// formatJSON formats records as JSON
func (p *ConvertRecordProcessor) formatJSON(records []map[string]interface{}, ctx types.ProcessorContext) ([]byte, error) {
	prettyPrint := ctx.GetPropertyValue("Pretty Print") == "true"

	if prettyPrint {
		return json.MarshalIndent(records, "", "  ")
	}
	return json.Marshal(records)
}

// formatCSV formats records as CSV
func (p *ConvertRecordProcessor) formatCSV(records []map[string]interface{}, ctx types.ProcessorContext) ([]byte, error) {
	if len(records) == 0 {
		return []byte{}, nil
	}

	delimiter := ctx.GetPropertyValue("CSV Delimiter")
	if delimiter == "" {
		delimiter = ","
	}

	var buf strings.Builder
	writer := csv.NewWriter(&buf)
	writer.Comma = rune(delimiter[0])

	// Get all unique keys for headers
	headers := make([]string, 0)
	headerSet := make(map[string]bool)
	for _, record := range records {
		for key := range record {
			if !headerSet[key] {
				headers = append(headers, key)
				headerSet[key] = true
			}
		}
	}

	// Write header
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write records
	for _, record := range records {
		row := make([]string, len(headers))
		for i, header := range headers {
			if val, ok := record[header]; ok {
				row[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return []byte(buf.String()), nil
}

// formatXML formats records as XML
func (p *ConvertRecordProcessor) formatXML(records []map[string]interface{}, ctx types.ProcessorContext) ([]byte, error) {
	rootElement := ctx.GetPropertyValue("XML Root Element")
	if rootElement == "" {
		rootElement = "records"
	}
	recordElement := ctx.GetPropertyValue("XML Record Element")
	if recordElement == "" {
		recordElement = "record"
	}

	var buf strings.Builder
	buf.WriteString(xml.Header)
	buf.WriteString(fmt.Sprintf("<%s>\n", rootElement))

	for _, record := range records {
		buf.WriteString(fmt.Sprintf("  <%s>\n", recordElement))
		for key, value := range record {
			// Simple XML encoding - in production, would need proper escaping
			buf.WriteString(fmt.Sprintf("    <%s>%v</%s>\n", key, value, key))
		}
		buf.WriteString(fmt.Sprintf("  </%s>\n", recordElement))
	}

	buf.WriteString(fmt.Sprintf("</%s>\n", rootElement))
	return []byte(buf.String()), nil
}

// getMimeType returns MIME type for format
func getMimeType(format string) string {
	switch format {
	case "json":
		return "application/json"
	case "csv":
		return "text/csv"
	case "xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}
