package plugins

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getMergeContentInfo()
	plugin.RegisterBuiltInProcessor("MergeContent", func() types.Processor {
		return NewMergeContentProcessor()
	}, info)
}

func getMergeContentInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"MergeContent",
		"MergeContent",
		"1.0.0",
		"DataBridge",
		"Merges multiple FlowFiles into a single FlowFile. Supports various merge strategies including concatenation, binary concatenation, and custom delimiters.",
		[]string{"merge", "concatenate", "join", "aggregate"},
	)
}

// MergeContentProcessor merges multiple FlowFiles into one
type MergeContentProcessor struct {
	*types.BaseProcessor
	pendingMerges map[string][]*types.FlowFile
}

// NewMergeContentProcessor creates a new MergeContent processor
func NewMergeContentProcessor() *MergeContentProcessor {
	info := types.ProcessorInfo{
		Name:        "MergeContent",
		Description: "Merges multiple FlowFiles into a single FlowFile",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"merge", "concatenate", "join", "aggregate"},
		Properties: []types.PropertySpec{
			{
				Name:         "Merge Strategy",
				DisplayName:  "Merge Strategy",
				Description:  "Strategy for merging FlowFiles",
				Required:     true,
				DefaultValue: "Bin-Packing Algorithm",
				AllowedValues: []string{
					"Defragment",
					"Bin-Packing Algorithm",
				},
				Type:     "select",
				HelpText: "Defragment: Merge based on fragment attributes; Bin-Packing: Merge by accumulating FlowFiles",
			},
			{
				Name:         "Merge Format",
				DisplayName:  "Merge Format",
				Description:  "Format for merged content",
				Required:     true,
				DefaultValue: "Binary Concatenation",
				AllowedValues: []string{
					"Binary Concatenation",
					"Text Concatenation",
				},
				Type:     "select",
				HelpText: "Binary: Concatenate raw bytes; Text: Concatenate with optional delimiters",
			},
			{
				Name:         "Delimiter Strategy",
				DisplayName:  "Delimiter Strategy",
				Description:  "How to add delimiters between merged content",
				Required:     false,
				DefaultValue: "Text",
				AllowedValues: []string{"Filename", "Text"},
				Type:         "select",
				HelpText:     "Filename: Use content from delimiter file; Text: Use specified text delimiter",
			},
			{
				Name:         "Demarcator",
				DisplayName:  "Demarcator",
				Description:  "Text to insert between merged FlowFiles",
				Required:     false,
				DefaultValue: "",
				Type:         "string",
				Placeholder:  "\\n or ,",
				HelpText:     "Delimiter to insert between FlowFiles. Use \\n for newline, \\t for tab.",
			},
			{
				Name:         "Header",
				DisplayName:  "Header",
				Description:  "Text to prepend to merged content",
				Required:     false,
				DefaultValue: "",
				Type:         "multiline",
				Placeholder:  "",
				HelpText:     "Content to add at the beginning of merged FlowFile",
			},
			{
				Name:         "Footer",
				DisplayName:  "Footer",
				Description:  "Text to append to merged content",
				Required:     false,
				DefaultValue: "",
				Type:         "multiline",
				Placeholder:  "",
				HelpText:     "Content to add at the end of merged FlowFile",
			},
			{
				Name:         "Minimum Number of Entries",
				DisplayName:  "Minimum Number of Entries",
				Description:  "Minimum number of FlowFiles to merge",
				Required:     false,
				DefaultValue: "1",
				Type:         "string",
				Placeholder:  "10",
				HelpText:     "Wait until at least this many FlowFiles are available before merging",
			},
			{
				Name:         "Maximum Number of Entries",
				DisplayName:  "Maximum Number of Entries",
				Description:  "Maximum number of FlowFiles to merge in one batch",
				Required:     false,
				DefaultValue: "1000",
				Type:         "string",
				Placeholder:  "1000",
				HelpText:     "Maximum FlowFiles to include in a single merge",
			},
			{
				Name:         "Maximum Bin Age",
				DisplayName:  "Maximum Bin Age",
				Description:  "Maximum time to wait for FlowFiles before merging",
				Required:     false,
				DefaultValue: "5m",
				Type:         "duration",
				Placeholder:  "5m",
				HelpText:     "Duration to wait (e.g., 30s, 5m, 1h)",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "merged",
				Description: "Merged FlowFile",
			},
			{
				Name:        "original",
				Description: "Original FlowFiles that were merged",
			},
			types.RelationshipFailure,
		},
	}

	return &MergeContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		pendingMerges: make(map[string][]*types.FlowFile),
	}
}

// Initialize initializes the processor
func (p *MergeContentProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *MergeContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	mergeStrategy := processorCtx.GetPropertyValue("Merge Strategy")
	_ = processorCtx.GetPropertyValue("Merge Format") // mergeFormat - unused for now
	demarcator := processorCtx.GetPropertyValue("Demarcator")
	header := processorCtx.GetPropertyValue("Header")
	footer := processorCtx.GetPropertyValue("Footer")
	minEntriesStr := processorCtx.GetPropertyValue("Minimum Number of Entries")
	maxEntriesStr := processorCtx.GetPropertyValue("Maximum Number of Entries")

	minEntries, _ := strconv.Atoi(minEntriesStr)
	if minEntries < 1 {
		minEntries = 1
	}
	maxEntries, _ := strconv.Atoi(maxEntriesStr)
	if maxEntries < 1 {
		maxEntries = 1000
	}

	// Parse demarcator escape sequences
	demarcator = strings.ReplaceAll(demarcator, "\\n", "\n")
	demarcator = strings.ReplaceAll(demarcator, "\\t", "\t")
	demarcator = strings.ReplaceAll(demarcator, "\\r", "\r")

	// Determine merge bin key
	binKey := "default"
	if mergeStrategy == "Defragment" {
		// Use fragment.identifier as bin key
		if identifier, exists := flowFile.GetAttribute("fragment.identifier"); exists {
			binKey = identifier
		}
	}

	// Add to pending merges
	if _, exists := p.pendingMerges[binKey]; !exists {
		p.pendingMerges[binKey] = make([]*types.FlowFile, 0)
	}
	p.pendingMerges[binKey] = append(p.pendingMerges[binKey], flowFile)

	// Check if we should merge
	shouldMerge := false
	pending := p.pendingMerges[binKey]

	if len(pending) >= maxEntries {
		shouldMerge = true
		logger.Debug("Triggering merge: max entries reached",
			"binKey", binKey,
			"count", len(pending))
	} else if mergeStrategy == "Defragment" {
		// Check if we have all fragments
		expectedCount := p.getExpectedFragmentCount(pending)
		if expectedCount > 0 && len(pending) >= expectedCount {
			shouldMerge = true
			logger.Debug("Triggering merge: all fragments received",
				"binKey", binKey,
				"count", len(pending),
				"expected", expectedCount)
		}
	} else if len(pending) >= minEntries {
		shouldMerge = true
		logger.Debug("Triggering merge: min entries reached",
			"binKey", binKey,
			"count", len(pending))
	}

	if !shouldMerge {
		logger.Debug("Not ready to merge",
			"binKey", binKey,
			"pendingCount", len(pending),
			"minEntries", minEntries,
			"maxEntries", maxEntries)
		return nil
	}

	// Perform merge
	flowFilesToMerge := p.pendingMerges[binKey]
	delete(p.pendingMerges, binKey)

	// Sort FlowFiles if defragmenting
	if mergeStrategy == "Defragment" {
		sort.Slice(flowFilesToMerge, func(i, j int) bool {
			idxI := p.getFragmentIndex(flowFilesToMerge[i])
			idxJ := p.getFragmentIndex(flowFilesToMerge[j])
			return idxI < idxJ
		})
	}

	// Merge content
	var mergedContent []byte

	// Add header
	if header != "" {
		mergedContent = append(mergedContent, []byte(header)...)
	}

	// Merge FlowFile contents
	for i, ff := range flowFilesToMerge {
		content, err := session.Read(ff)
		if err != nil {
			logger.Error("Failed to read FlowFile for merge",
				"flowFileId", ff.ID,
				"error", err)
			continue
		}

		// Add content
		mergedContent = append(mergedContent, content...)

		// Add demarcator between FlowFiles (not after last one)
		if i < len(flowFilesToMerge)-1 && demarcator != "" {
			mergedContent = append(mergedContent, []byte(demarcator)...)
		}
	}

	// Add footer
	if footer != "" {
		mergedContent = append(mergedContent, []byte(footer)...)
	}

	// Create merged FlowFile
	mergedFlowFile := session.Create()

	// Copy attributes from first FlowFile
	for key, value := range flowFilesToMerge[0].Attributes {
		session.PutAttribute(mergedFlowFile, key, value)
	}

	// Add merge metadata
	session.PutAttribute(mergedFlowFile, "merge.count", fmt.Sprintf("%d", len(flowFilesToMerge)))
	session.PutAttribute(mergedFlowFile, "merge.bin.key", binKey)

	// Write merged content
	if err := session.Write(mergedFlowFile, mergedContent); err != nil {
		logger.Error("Failed to write merged content",
			"error", err)
		session.Transfer(mergedFlowFile, types.RelationshipFailure)
		return err
	}

	// Transfer merged FlowFile
	session.Transfer(mergedFlowFile, types.Relationship{Name: "merged", Description: "Merged FlowFile"})

	// Transfer originals
	for _, ff := range flowFilesToMerge {
		session.Transfer(ff, types.Relationship{Name: "original", Description: "Original FlowFile"})
	}

	logger.Info("Successfully merged FlowFiles",
		"mergedFlowFileId", mergedFlowFile.ID,
		"sourceCount", len(flowFilesToMerge),
		"mergedSize", len(mergedContent))

	return nil
}

// getExpectedFragmentCount gets expected fragment count from FlowFiles
func (p *MergeContentProcessor) getExpectedFragmentCount(flowFiles []*types.FlowFile) int {
	for _, ff := range flowFiles {
		if countStr, exists := ff.GetAttribute("fragment.count"); exists {
			if count, err := strconv.Atoi(countStr); err == nil {
				return count
			}
		}
	}
	return 0
}

// getFragmentIndex gets fragment index from FlowFile
func (p *MergeContentProcessor) getFragmentIndex(ff *types.FlowFile) int {
	if idxStr, exists := ff.GetAttribute("fragment.index"); exists {
		if idx, err := strconv.Atoi(idxStr); err == nil {
			return idx
		}
	}
	return 0
}
