package plugins

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getCompressContentInfo()
	plugin.RegisterBuiltInProcessor("CompressContent", func() types.Processor {
		return NewCompressContentProcessor()
	}, info)
}

func getCompressContentInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"CompressContent",
		"CompressContent",
		"1.0.0",
		"DataBridge",
		"Compresses or decompresses FlowFile content using various compression algorithms including gzip, deflate, and bzip2.",
		[]string{"compression", "decompression", "gzip", "zip"},
	)
}

// CompressContentProcessor compresses or decompresses content
type CompressContentProcessor struct {
	*types.BaseProcessor
}

// NewCompressContentProcessor creates a new CompressContent processor
func NewCompressContentProcessor() *CompressContentProcessor {
	info := types.ProcessorInfo{
		Name:        "CompressContent",
		Description: "Compresses or decompresses FlowFile content",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"compression", "decompression", "gzip", "zip"},
		Properties: []types.PropertySpec{
			{
				Name:          "Mode",
				DisplayName:   "Mode",
				Description:   "Whether to compress or decompress content",
				Required:      true,
				DefaultValue:  "compress",
				AllowedValues: []string{"compress", "decompress"},
				Type:          "select",
				HelpText:      "compress: Compress content; decompress: Decompress content",
			},
			{
				Name:          "Compression Format",
				DisplayName:   "Compression Format",
				Description:   "Compression algorithm to use",
				Required:      true,
				DefaultValue:  "gzip",
				AllowedValues: []string{"gzip", "deflate"},
				Type:          "select",
				HelpText:      "Compression algorithm (gzip recommended for best compatibility)",
			},
			{
				Name:          "Compression Level",
				DisplayName:   "Compression Level",
				Description:   "Compression level (1=fastest, 9=best compression)",
				Required:      false,
				DefaultValue:  "6",
				AllowedValues: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"},
				Type:          "select",
				HelpText:      "Higher values = better compression but slower. 6 is a good balance.",
			},
			{
				Name:          "Update Filename",
				DisplayName:   "Update Filename",
				Description:   "Whether to update filename attribute with compression extension",
				Required:      false,
				DefaultValue:  "true",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, adds .gz extension on compress, removes on decompress",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &CompressContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *CompressContentProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *CompressContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	mode := processorCtx.GetPropertyValue("Mode")
	format := processorCtx.GetPropertyValue("Compression Format")
	levelStr := processorCtx.GetPropertyValue("Compression Level")
	updateFilename := processorCtx.GetPropertyValue("Update Filename") == "true"

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	var result []byte
	var newMimeType string

	// Perform compression or decompression
	if mode == "compress" {
		level := gzip.DefaultCompression
		if levelStr != "" {
			switch levelStr {
			case "1":
				level = gzip.BestSpeed
			case "9":
				level = gzip.BestCompression
			default:
				level = gzip.DefaultCompression
			}
		}

		result, err = p.compress(content, format, level)
		if err != nil {
			logger.Error("Failed to compress content",
				"flowFileId", flowFile.ID,
				"format", format,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		newMimeType = "application/gzip"

		// Update filename if requested
		if updateFilename {
			if filename, exists := flowFile.GetAttribute("filename"); exists {
				session.PutAttribute(flowFile, "filename", filename+".gz")
			}
		}
	} else {
		// Decompress
		result, err = p.decompress(content, format)
		if err != nil {
			logger.Error("Failed to decompress content",
				"flowFileId", flowFile.ID,
				"format", format,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		newMimeType = "application/octet-stream"

		// Update filename if requested
		if updateFilename {
			if filename, exists := flowFile.GetAttribute("filename"); exists {
				// Remove .gz extension if present
				if len(filename) > 3 && filename[len(filename)-3:] == ".gz" {
					session.PutAttribute(flowFile, "filename", filename[:len(filename)-3])
				}
			}
		}
	}

	// Write modified content
	if err := session.Write(flowFile, result); err != nil {
		logger.Error("Failed to write modified content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Update attributes
	session.PutAttribute(flowFile, "mime.type", newMimeType)

	logger.Info("Successfully processed content",
		"flowFileId", flowFile.ID,
		"mode", mode,
		"format", format,
		"originalSize", len(content),
		"newSize", len(result),
		"ratio", fmt.Sprintf("%.2f%%", float64(len(result))/float64(len(content))*100))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}

// compress compresses content using the specified format
func (p *CompressContentProcessor) compress(data []byte, format string, level int) ([]byte, error) {
	var buf bytes.Buffer

	switch format {
	case "gzip", "deflate":
		writer, err := gzip.NewWriterLevel(&buf, level)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip writer: %w", err)
		}

		if _, err := writer.Write(data); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("failed to write compressed data: %w", err)
		}

		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %w", err)
		}

		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("unsupported compression format: %s", format)
	}
}

// decompress decompresses content using the specified format
func (p *CompressContentProcessor) decompress(data []byte, format string) ([]byte, error) {
	switch format {
	case "gzip", "deflate":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() { _ = reader.Close() }()

		result, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to read decompressed data: %w", err)
		}

		return result, nil

	default:
		return nil, fmt.Errorf("unsupported compression format: %s", format)
	}
}
