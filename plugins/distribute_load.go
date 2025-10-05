package plugins

import (
	"context"
	"fmt"
	"hash/fnv"
	"sync"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getDistributeLoadInfo()
	plugin.RegisterBuiltInProcessor("DistributeLoad", func() types.Processor {
		return NewDistributeLoadProcessor()
	}, info)
}

func getDistributeLoadInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"DistributeLoad",
		"DistributeLoad",
		"1.0.0",
		"DataBridge",
		"Distributes FlowFiles across multiple relationships for load balancing. Supports round-robin, random, and attribute-based distribution strategies.",
		[]string{"load balancing", "distribution", "parallel", "routing"},
	)
}

// DistributeLoadProcessor distributes FlowFiles for load balancing
type DistributeLoadProcessor struct {
	*types.BaseProcessor
	counter uint64
	mutex   sync.Mutex
}

// NewDistributeLoadProcessor creates a new DistributeLoad processor
func NewDistributeLoadProcessor() *DistributeLoadProcessor {
	info := types.ProcessorInfo{
		Name:        "DistributeLoad",
		Description: "Distributes FlowFiles across multiple relationships for load balancing",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"load balancing", "distribution", "parallel", "routing"},
		Properties: []types.PropertySpec{
			{
				Name:          "Number of Relationships",
				DisplayName:   "Number of Relationships",
				Description:   "Number of distribution relationships to create",
				Required:      true,
				DefaultValue:  "3",
				AllowedValues: []string{"2", "3", "4", "5", "6", "8", "10"},
				Type:          "select",
				HelpText:      "FlowFiles will be distributed across this many numbered relationships (1, 2, 3, etc.)",
			},
			{
				Name:          "Distribution Strategy",
				DisplayName:   "Distribution Strategy",
				Description:   "Strategy for distributing FlowFiles",
				Required:      true,
				DefaultValue:  "round-robin",
				AllowedValues: []string{"round-robin", "load-balanced", "attribute-hash"},
				Type:          "select",
				HelpText:      "round-robin: Sequential distribution; load-balanced: Least loaded; attribute-hash: Hash of attribute value",
			},
			{
				Name:        "Hash Attribute",
				DisplayName: "Hash Attribute",
				Description: "Attribute name to hash for distribution (only for attribute-hash strategy)",
				Required:    false,
				Type:        "string",
				Placeholder: "e.g., 'user.id'",
				HelpText:    "Used only when Distribution Strategy is 'attribute-hash'. FlowFiles with same attribute value go to same relationship.",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipFailure,
			// Dynamic relationships (1, 2, 3, etc.) will be created based on configuration
		},
	}

	return &DistributeLoadProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
		counter:       0,
	}
}

// Initialize initializes the processor
func (p *DistributeLoadProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *DistributeLoadProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	numRelationshipsStr := processorCtx.GetPropertyValue("Number of Relationships")
	strategy := processorCtx.GetPropertyValue("Distribution Strategy")
	hashAttribute := processorCtx.GetPropertyValue("Hash Attribute")

	// Parse number of relationships
	var numRelationships int
	fmt.Sscanf(numRelationshipsStr, "%d", &numRelationships)
	if numRelationships < 2 {
		numRelationships = 2
	}

	// Determine which relationship to use
	var relationshipIndex int
	switch strategy {
	case "round-robin":
		p.mutex.Lock()
		p.counter++
		relationshipIndex = int(p.counter % uint64(numRelationships))
		p.mutex.Unlock()

	case "attribute-hash":
		if hashAttribute == "" {
			logger.Error("Hash attribute not specified",
				"flowFileId", flowFile.ID)
			session.Transfer(flowFile, types.RelationshipFailure)
			return fmt.Errorf("hash attribute required for attribute-hash strategy")
		}

		attrValue, exists := flowFile.GetAttribute(hashAttribute)
		if !exists {
			logger.Warn("Hash attribute not found, using round-robin",
				"flowFileId", flowFile.ID,
				"attribute", hashAttribute)
			p.mutex.Lock()
			p.counter++
			relationshipIndex = int(p.counter % uint64(numRelationships))
			p.mutex.Unlock()
		} else {
			// Hash the attribute value
			h := fnv.New32a()
			h.Write([]byte(attrValue))
			relationshipIndex = int(h.Sum32() % uint32(numRelationships))
		}

	case "load-balanced":
		// For now, use round-robin (would need queue depth info for true load balancing)
		p.mutex.Lock()
		p.counter++
		relationshipIndex = int(p.counter % uint64(numRelationships))
		p.mutex.Unlock()

	default:
		logger.Error("Unknown distribution strategy",
			"flowFileId", flowFile.ID,
			"strategy", strategy)
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("unsupported distribution strategy: %s", strategy)
	}

	// Create relationship name (1-indexed for user friendliness)
	relationshipName := fmt.Sprintf("%d", relationshipIndex+1)

	// Add distribution metadata to FlowFile
	session.PutAttribute(flowFile, "distribute.strategy", strategy)
	session.PutAttribute(flowFile, "distribute.relationship", relationshipName)
	session.PutAttribute(flowFile, "distribute.total", numRelationshipsStr)

	logger.Debug("Distributing FlowFile",
		"flowFileId", flowFile.ID,
		"strategy", strategy,
		"relationship", relationshipName)

	session.Transfer(flowFile, types.Relationship{Name: relationshipName})
	return nil
}
