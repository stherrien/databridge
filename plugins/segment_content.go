package plugins

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getSegmentContentInfo()
	plugin.RegisterBuiltInProcessor("SegmentContent", func() types.Processor {
		return NewSegmentContentProcessor()
	}, info)
}

func getSegmentContentInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"SegmentContent",
		"SegmentContent",
		"1.0.0",
		"DataBridge",
		"Segments FlowFile content into fixed-size chunks. Each segment becomes a separate FlowFile, useful for processing large files in smaller pieces or preparing data for parallel processing.",
		[]string{"segment", "chunk", "split", "partition"},
	)
}

// SegmentContentProcessor segments content into fixed-size chunks
type SegmentContentProcessor struct {
	*types.BaseProcessor
}

// NewSegmentContentProcessor creates a new SegmentContent processor
func NewSegmentContentProcessor() *SegmentContentProcessor {
	info := types.ProcessorInfo{
		Name:        "SegmentContent",
		Description: "Segments FlowFile content into fixed-size chunks",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"segment", "chunk", "split", "partition"},
		Properties: []types.PropertySpec{
			{
				Name:         "Segment Size",
				DisplayName:  "Segment Size",
				Description:  "The size of each segment in bytes",
				Required:     true,
				DefaultValue: "1048576",
				Type:         "string",
				Placeholder:  "1048576",
				HelpText:     "Size in bytes. Examples: 1024 (1KB), 1048576 (1MB), 10485760 (10MB)",
			},
			{
				Name:          "Keep Original",
				DisplayName:   "Keep Original",
				Description:   "Whether to keep the original FlowFile",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, the original FlowFile is routed to 'original'. When false, only segments are kept.",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "segments",
				Description: "Segment FlowFiles are routed to this relationship",
			},
			types.RelationshipOriginal,
			types.RelationshipFailure,
		},
	}

	return &SegmentContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *SegmentContentProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *SegmentContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	segmentSizeStr := processorCtx.GetPropertyValue("Segment Size")
	keepOriginal := processorCtx.GetPropertyValue("Keep Original") == "true"

	// Parse segment size
	segmentSize, err := strconv.Atoi(segmentSizeStr)
	if err != nil || segmentSize <= 0 {
		logger.Error("Invalid segment size",
			"flowFileId", flowFile.ID,
			"segmentSize", segmentSizeStr,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("invalid segment size: must be a positive integer")
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

	contentLen := len(content)

	// If content is smaller than segment size, just pass through
	if contentLen <= segmentSize {
		logger.Info("Content smaller than segment size, passing through",
			"flowFileId", flowFile.ID,
			"contentSize", contentLen,
			"segmentSize", segmentSize)
		session.Transfer(flowFile, types.Relationship{Name: "segments"})
		return nil
	}

	// Calculate number of segments
	numSegments := (contentLen + segmentSize - 1) / segmentSize

	// Create segments
	for i := 0; i < numSegments; i++ {
		start := i * segmentSize
		end := start + segmentSize
		if end > contentLen {
			end = contentLen
		}

		segmentData := content[start:end]

		// Create segment FlowFile
		segment := session.Clone(flowFile)
		if err := session.Write(segment, segmentData); err != nil {
			logger.Error("Failed to write segment",
				"flowFileId", flowFile.ID,
				"segment", i,
				"error", err)
			session.Remove(segment)
			continue
		}

		// Add segment attributes
		origFilename, _ := flowFile.GetAttribute("filename")
		session.PutAttribute(segment, "segment.original.filename", origFilename)
		session.PutAttribute(segment, "segment.index", strconv.Itoa(i))
		session.PutAttribute(segment, "segment.count", strconv.Itoa(numSegments))
		session.PutAttribute(segment, "segment.offset", strconv.Itoa(start))
		session.PutAttribute(segment, "segment.size", strconv.Itoa(len(segmentData)))
		session.PutAttribute(segment, "segment.original.size", strconv.Itoa(contentLen))

		// Update filename for segment
		if filename, exists := flowFile.GetAttribute("filename"); exists {
			session.PutAttribute(segment, "filename", fmt.Sprintf("%s.segment.%d", filename, i))
		}

		logger.Debug("Created segment",
			"flowFileId", flowFile.ID,
			"segmentId", segment.ID,
			"index", i,
			"offset", start,
			"size", len(segmentData))

		session.Transfer(segment, types.Relationship{Name: "segments"})
	}

	logger.Info("Successfully segmented content",
		"flowFileId", flowFile.ID,
		"contentSize", contentLen,
		"segmentSize", segmentSize,
		"numSegments", numSegments)

	// Handle original FlowFile
	if keepOriginal {
		session.Transfer(flowFile, types.RelationshipOriginal)
	} else {
		session.Remove(flowFile)
	}

	return nil
}
