package plugins

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	// Register this processor as a built-in
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
		"Merges multiple FlowFiles into a single FlowFile based on configurable merge strategies",
		[]string{"merge", "aggregate", "combine", "content"},
	)
}

// MergeContentProcessor merges multiple FlowFiles into a single FlowFile
type MergeContentProcessor struct {
	*types.BaseProcessor
	mergeQueue []*types.FlowFile
}

// Define merge relationship
var (
	RelationshipMerged = types.Relationship{
		Name:        "merged",
		Description: "Merged FlowFiles",
	}
)

// NewMergeContentProcessor creates a new MergeContent processor
func NewMergeContentProcessor() *MergeContentProcessor {
	info := types.ProcessorInfo{
		Name:        "MergeContent",
		Description: "Merges multiple FlowFiles into a single FlowFile based on configurable merge strategies",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"merge", "aggregate", "combine", "content"},
		Properties: []types.PropertySpec{
			{
				Name:        "Merge Strategy",
				Description: "Strategy for merging FlowFiles",
				Required:    false,
				DefaultValue: "Bin-Packing",
				AllowedValues: []string{
					"Bin-Packing",      // Group by size/count limits
					"Defragment",       // Group by fragment.identifier
					"Attribute-Based",  // Group by attribute value
				},
			},
			{
				Name:         "Merge Format",
				Description:  "Format for merged content",
				Required:     false,
				DefaultValue: "Binary Concatenation",
				AllowedValues: []string{
					"Binary Concatenation", // Simple concatenation
					"Text Concatenation",   // With delimiters
					"TAR",                  // TAR archive (future)
					"ZIP",                  // ZIP archive (future)
				},
			},
			{
				Name:         "Minimum Number of Entries",
				Description:  "Minimum number of FlowFiles to merge",
				Required:     false,
				DefaultValue: "1",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Maximum Number of Entries",
				Description:  "Maximum number of FlowFiles to merge (0 = unlimited)",
				Required:     false,
				DefaultValue: "100",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Minimum Group Size",
				Description:  "Minimum total size in bytes (0 = no minimum)",
				Required:     false,
				DefaultValue: "0",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Maximum Group Size",
				Description:  "Maximum total size in bytes (0 = no maximum)",
				Required:     false,
				DefaultValue: "0",
				Pattern:      `^\d+$`,
			},
			{
				Name:         "Max Bin Age",
				Description:  "Maximum time to wait for bin to fill (e.g., 30s, 5m)",
				Required:     false,
				DefaultValue: "0s",
			},
			{
				Name:         "Correlation Attribute Name",
				Description:  "Attribute name for Attribute-Based merge strategy",
				Required:     false,
				DefaultValue: "merge.group",
			},
			{
				Name:         "Delimiter Strategy",
				Description:  "Delimiter between merged content for Text Concatenation",
				Required:     false,
				DefaultValue: "Newline",
				AllowedValues: []string{
					"None",
					"Newline",
					"Tab",
					"Custom",
				},
			},
			{
				Name:         "Delimiter Text",
				Description:  "Custom delimiter text when Delimiter Strategy is Custom",
				Required:     false,
				DefaultValue: "",
			},
			{
				Name:         "Keep Path",
				Description:  "Whether to keep path attribute from original FlowFiles",
				Required:     false,
				DefaultValue: "false",
				AllowedValues: []string{"true", "false"},
			},
		},
		Relationships: []types.Relationship{
			RelationshipMerged,
			types.RelationshipOriginal,
			types.RelationshipFailure,
		},
	}

	return &MergeContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		mergeQueue:    make([]*types.FlowFile, 0),
	}
}

// Initialize initializes the processor
func (p *MergeContentProcessor) Initialize(ctx types.ProcessorContext) error {
	logger := ctx.GetLogger()
	logger.Info("Initializing MergeContent processor")

	// Validate minimum/maximum entries
	minEntries := ctx.GetPropertyValue("Minimum Number of Entries")
	if minEntries != "" {
		val, err := strconv.Atoi(minEntries)
		if err != nil || val < 1 {
			return fmt.Errorf("Minimum Number of Entries must be a positive integer")
		}
	}

	maxEntries := ctx.GetPropertyValue("Maximum Number of Entries")
	if maxEntries != "" {
		val, err := strconv.Atoi(maxEntries)
		if err != nil || val < 0 {
			return fmt.Errorf("Maximum Number of Entries must be a non-negative integer")
		}
	}

	return nil
}

// OnTrigger processes the trigger event
func (p *MergeContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	logger := session.GetLogger()

	// Get processor context to access properties
	processorCtx, ok := ctx.Value("processorContext").(types.ProcessorContext)
	if !ok {
		return fmt.Errorf("processor context not available")
	}

	// Read configuration properties
	mergeStrategy := processorCtx.GetPropertyValue("Merge Strategy")
	if mergeStrategy == "" {
		mergeStrategy = "Bin-Packing"
	}

	mergeFormat := processorCtx.GetPropertyValue("Merge Format")
	if mergeFormat == "" {
		mergeFormat = "Binary Concatenation"
	}

	minEntries := p.getIntProperty(processorCtx, "Minimum Number of Entries", 1)
	maxEntries := p.getIntProperty(processorCtx, "Maximum Number of Entries", 100)
	minSize := p.getIntProperty(processorCtx, "Minimum Group Size", 0)
	maxSize := p.getIntProperty(processorCtx, "Maximum Group Size", 0)

	maxBinAge := processorCtx.GetPropertyValue("Max Bin Age")
	maxBinDuration, _ := time.ParseDuration(maxBinAge)

	correlationAttr := processorCtx.GetPropertyValue("Correlation Attribute Name")
	if correlationAttr == "" {
		correlationAttr = "merge.group"
	}

	delimiterStrategy := processorCtx.GetPropertyValue("Delimiter Strategy")
	if delimiterStrategy == "" {
		delimiterStrategy = "Newline"
	}

	delimiterText := processorCtx.GetPropertyValue("Delimiter Text")
	keepPath := processorCtx.GetPropertyValue("Keep Path") == "true"

	// Get FlowFiles to merge
	var flowFiles []*types.FlowFile
	if maxEntries > 0 {
		flowFiles = session.GetBatch(maxEntries)
	} else {
		// Get available FlowFiles (reasonable batch size)
		flowFiles = session.GetBatch(1000)
	}

	if len(flowFiles) == 0 {
		logger.Debug("No FlowFiles available to merge")
		return nil
	}

	logger.Debug("Retrieved FlowFiles for merging", "count", len(flowFiles))

	// Group FlowFiles based on merge strategy
	var groups [][]*types.FlowFile

	switch mergeStrategy {
	case "Bin-Packing":
		groups = p.binPackingStrategy(flowFiles, minEntries, maxEntries, minSize, maxSize)
	case "Defragment":
		groups = p.defragmentStrategy(flowFiles)
	case "Attribute-Based":
		groups = p.attributeBasedStrategy(flowFiles, correlationAttr)
	default:
		logger.Error("Unknown merge strategy", "strategy", mergeStrategy)
		for _, ff := range flowFiles {
			session.Transfer(ff, types.RelationshipFailure)
		}
		return nil
	}

	logger.Debug("Grouped FlowFiles", "groupCount", len(groups))

	// Process each group
	for groupIdx, group := range groups {
		// Check minimum entries requirement
		if len(group) < minEntries {
			logger.Debug("Group has fewer than minimum entries, routing to original",
				"groupSize", len(group), "minEntries", minEntries)
			for _, ff := range group {
				session.Transfer(ff, types.RelationshipOriginal)
			}
			continue
		}

		// Check max bin age if configured
		if maxBinDuration > 0 {
			oldestTime := p.getOldestFlowFileTime(group)
			if time.Since(oldestTime) < maxBinDuration {
				logger.Debug("Group not old enough, routing to original",
					"age", time.Since(oldestTime), "maxAge", maxBinDuration)
				for _, ff := range group {
					session.Transfer(ff, types.RelationshipOriginal)
				}
				continue
			}
		}

		// Merge the group
		mergedFlowFile, err := p.mergeGroup(session, group, mergeFormat, delimiterStrategy, delimiterText, keepPath)
		if err != nil {
			logger.Error("Failed to merge group", "groupIndex", groupIdx, "error", err)
			for _, ff := range group {
				session.Transfer(ff, types.RelationshipFailure)
			}
			continue
		}

		// Add merge metadata attributes
		session.PutAttribute(mergedFlowFile, "merge.count", strconv.Itoa(len(group)))
		session.PutAttribute(mergedFlowFile, "merge.strategy", mergeStrategy)
		session.PutAttribute(mergedFlowFile, "merge.format", mergeFormat)

		// Transfer merged FlowFile to success
		session.Transfer(mergedFlowFile, RelationshipMerged)

		// Remove original FlowFiles
		for _, ff := range group {
			session.Remove(ff)
		}

		logger.Info("Successfully merged FlowFiles",
			"groupIndex", groupIdx,
			"flowFileCount", len(group),
			"mergedFlowFileId", mergedFlowFile.ID)
	}

	return nil
}

// binPackingStrategy groups FlowFiles based on size and count constraints
func (p *MergeContentProcessor) binPackingStrategy(flowFiles []*types.FlowFile, minEntries, maxEntries, minSize, maxSize int) [][]*types.FlowFile {
	var groups [][]*types.FlowFile
	var currentGroup []*types.FlowFile
	var currentSize int

	for _, ff := range flowFiles {
		ffSize := ff.Size

		// Check if adding this FlowFile would exceed limits
		wouldExceedCount := maxEntries > 0 && len(currentGroup) >= maxEntries
		wouldExceedSize := maxSize > 0 && currentSize+int(ffSize) > maxSize

		if wouldExceedCount || wouldExceedSize {
			// Start a new group
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
			}
			currentGroup = []*types.FlowFile{ff}
			currentSize = int(ffSize)
		} else {
			// Add to current group
			currentGroup = append(currentGroup, ff)
			currentSize += int(ffSize)
		}
	}

	// Add final group
	if len(currentGroup) > 0 {
		groups = append(groups, currentGroup)
	}

	return groups
}

// defragmentStrategy groups FlowFiles by fragment.identifier attribute
func (p *MergeContentProcessor) defragmentStrategy(flowFiles []*types.FlowFile) [][]*types.FlowFile {
	fragmentMap := make(map[string][]*types.FlowFile)

	for _, ff := range flowFiles {
		identifier, exists := ff.GetAttribute("fragment.identifier")
		if !exists {
			// No fragment identifier, treat as singleton group
			identifier = ff.ID.String()
		}

		fragmentMap[identifier] = append(fragmentMap[identifier], ff)
	}

	// Sort each group by fragment.index
	var groups [][]*types.FlowFile
	for _, group := range fragmentMap {
		sort.Slice(group, func(i, j int) bool {
			iIndex, iExists := group[i].GetAttribute("fragment.index")
			jIndex, jExists := group[j].GetAttribute("fragment.index")

			if !iExists || !jExists {
				return false
			}

			iVal, _ := strconv.Atoi(iIndex)
			jVal, _ := strconv.Atoi(jIndex)
			return iVal < jVal
		})

		groups = append(groups, group)
	}

	return groups
}

// attributeBasedStrategy groups FlowFiles by a correlation attribute
func (p *MergeContentProcessor) attributeBasedStrategy(flowFiles []*types.FlowFile, correlationAttr string) [][]*types.FlowFile {
	attrMap := make(map[string][]*types.FlowFile)

	for _, ff := range flowFiles {
		attrValue, exists := ff.GetAttribute(correlationAttr)
		if !exists {
			// No correlation attribute, treat as singleton group
			attrValue = ff.ID.String()
		}

		attrMap[attrValue] = append(attrMap[attrValue], ff)
	}

	var groups [][]*types.FlowFile
	for _, group := range attrMap {
		groups = append(groups, group)
	}

	return groups
}

// mergeGroup merges a group of FlowFiles into a single FlowFile
func (p *MergeContentProcessor) mergeGroup(
	session types.ProcessSession,
	group []*types.FlowFile,
	mergeFormat, delimiterStrategy, delimiterText string,
	keepPath bool,
) (*types.FlowFile, error) {
	if len(group) == 0 {
		return nil, fmt.Errorf("cannot merge empty group")
	}

	// Create merged FlowFile from first FlowFile in group
	mergedFlowFile := session.Clone(group[0])

	// Get delimiter based on strategy
	delimiter := p.getDelimiter(delimiterStrategy, delimiterText)

	// Merge content based on format
	var mergedContent []byte

	switch mergeFormat {
	case "Binary Concatenation":
		mergedContent = p.binaryConcatenation(session, group)
	case "Text Concatenation":
		mergedContent = p.textConcatenation(session, group, delimiter)
	case "TAR", "ZIP":
		return nil, fmt.Errorf("merge format %s not yet implemented", mergeFormat)
	default:
		return nil, fmt.Errorf("unknown merge format: %s", mergeFormat)
	}

	// Write merged content
	if err := session.Write(mergedFlowFile, mergedContent); err != nil {
		return nil, fmt.Errorf("failed to write merged content: %w", err)
	}

	// Optionally preserve path attributes
	if keepPath {
		var paths []string
		for _, ff := range group {
			if path, exists := ff.GetAttribute("path"); exists {
				paths = append(paths, path)
			}
		}
		if len(paths) > 0 {
			session.PutAttribute(mergedFlowFile, "merge.paths", strings.Join(paths, ","))
		}
	}

	return mergedFlowFile, nil
}

// binaryConcatenation performs simple binary concatenation
func (p *MergeContentProcessor) binaryConcatenation(session types.ProcessSession, group []*types.FlowFile) []byte {
	var result []byte

	for _, ff := range group {
		content, err := session.Read(ff)
		if err != nil {
			// Skip FlowFiles that can't be read
			continue
		}
		result = append(result, content...)
	}

	return result
}

// textConcatenation performs text concatenation with delimiters
func (p *MergeContentProcessor) textConcatenation(session types.ProcessSession, group []*types.FlowFile, delimiter string) []byte {
	var parts []string

	for _, ff := range group {
		content, err := session.Read(ff)
		if err != nil {
			// Skip FlowFiles that can't be read
			continue
		}
		parts = append(parts, string(content))
	}

	return []byte(strings.Join(parts, delimiter))
}

// getDelimiter returns the delimiter based on strategy
func (p *MergeContentProcessor) getDelimiter(strategy, customText string) string {
	switch strategy {
	case "None":
		return ""
	case "Newline":
		return "\n"
	case "Tab":
		return "\t"
	case "Custom":
		return customText
	default:
		return "\n"
	}
}

// getIntProperty retrieves an integer property with a default value
func (p *MergeContentProcessor) getIntProperty(ctx types.ProcessorContext, name string, defaultValue int) int {
	valueStr := ctx.GetPropertyValue(name)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// getOldestFlowFileTime returns the oldest entry time from a group
func (p *MergeContentProcessor) getOldestFlowFileTime(group []*types.FlowFile) time.Time {
	if len(group) == 0 {
		return time.Now()
	}

	oldest := group[0].CreatedAt
	for _, ff := range group[1:] {
		if ff.CreatedAt.Before(oldest) {
			oldest = ff.CreatedAt
		}
	}

	return oldest
}

// Validate validates the processor configuration
func (p *MergeContentProcessor) Validate(config types.ProcessorConfig) []types.ValidationResult {
	results := p.BaseProcessor.Validate(config)

	// Validate Minimum Number of Entries
	if minStr, exists := config.Properties["Minimum Number of Entries"]; exists && minStr != "" {
		min, err := strconv.Atoi(minStr)
		if err != nil || min < 1 {
			results = append(results, types.ValidationResult{
				Property: "Minimum Number of Entries",
				Valid:    false,
				Message:  "Minimum Number of Entries must be a positive integer",
			})
		}
	}

	// Validate Maximum Number of Entries
	if maxStr, exists := config.Properties["Maximum Number of Entries"]; exists && maxStr != "" {
		max, err := strconv.Atoi(maxStr)
		if err != nil || max < 0 {
			results = append(results, types.ValidationResult{
				Property: "Maximum Number of Entries",
				Valid:    false,
				Message:  "Maximum Number of Entries must be a non-negative integer",
			})
		}

		// Check that max >= min
		if minStr, minExists := config.Properties["Minimum Number of Entries"]; minExists {
			min, minErr := strconv.Atoi(minStr)
			max, maxErr := strconv.Atoi(maxStr)
			if minErr == nil && maxErr == nil && max > 0 && max < min {
				results = append(results, types.ValidationResult{
					Property: "Maximum Number of Entries",
					Valid:    false,
					Message:  "Maximum Number of Entries must be greater than or equal to Minimum Number of Entries",
				})
			}
		}
	}

	// Validate Group Sizes
	if minStr, exists := config.Properties["Minimum Group Size"]; exists && minStr != "" {
		min, err := strconv.Atoi(minStr)
		if err != nil || min < 0 {
			results = append(results, types.ValidationResult{
				Property: "Minimum Group Size",
				Valid:    false,
				Message:  "Minimum Group Size must be a non-negative integer",
			})
		}
	}

	if maxStr, exists := config.Properties["Maximum Group Size"]; exists && maxStr != "" {
		max, err := strconv.Atoi(maxStr)
		if err != nil || max < 0 {
			results = append(results, types.ValidationResult{
				Property: "Maximum Group Size",
				Valid:    false,
				Message:  "Maximum Group Size must be a non-negative integer",
			})
		}
	}

	// Validate Max Bin Age
	if ageStr, exists := config.Properties["Max Bin Age"]; exists && ageStr != "" {
		if _, err := time.ParseDuration(ageStr); err != nil {
			results = append(results, types.ValidationResult{
				Property: "Max Bin Age",
				Valid:    false,
				Message:  "Max Bin Age must be a valid duration (e.g., 30s, 5m, 1h)",
			})
		}
	}

	return results
}

// OnStopped cleanup when processor is stopped
func (p *MergeContentProcessor) OnStopped(ctx context.Context) {
	p.mergeQueue = nil
}
