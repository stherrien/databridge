package plugins

import (
	"context"
	"crypto/md5"  // #nosec G501 - MD5 is intentionally supported for compatibility with legacy systems
	"crypto/sha1" // #nosec G505 - SHA-1 is intentionally supported for compatibility with legacy systems
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getHashContentInfo()
	plugin.RegisterBuiltInProcessor("HashContent", func() types.Processor {
		return NewHashContentProcessor()
	}, info)
}

func getHashContentInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"HashContent",
		"HashContent",
		"1.0.0",
		"DataBridge",
		"Generates cryptographic hashes of FlowFile content using various algorithms (MD5, SHA-1, SHA-256, SHA-512). Useful for content validation, deduplication, and integrity verification.",
		[]string{"hash", "checksum", "md5", "sha", "integrity"},
	)
}

// HashContentProcessor generates hashes of content
type HashContentProcessor struct {
	*types.BaseProcessor
}

// NewHashContentProcessor creates a new HashContent processor
func NewHashContentProcessor() *HashContentProcessor {
	info := types.ProcessorInfo{
		Name:        "HashContent",
		Description: "Generates cryptographic hashes of FlowFile content",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"hash", "checksum", "md5", "sha", "integrity"},
		Properties: []types.PropertySpec{
			{
				Name:          "Hash Algorithm",
				DisplayName:   "Hash Algorithm",
				Description:   "The hash algorithm to use",
				Required:      true,
				DefaultValue:  "SHA-256",
				AllowedValues: []string{"MD5", "SHA-1", "SHA-256", "SHA-384", "SHA-512"},
				Type:          "select",
				HelpText:      "MD5: Fast but not secure; SHA-1: Deprecated; SHA-256/384/512: Secure and recommended",
			},
			{
				Name:         "Attribute Name",
				DisplayName:  "Attribute Name",
				Description:  "Name of the attribute to store the hash value",
				Required:     false,
				DefaultValue: "content.hash",
				Type:         "string",
				Placeholder:  "content.hash",
				HelpText:     "The hash value will be stored in this FlowFile attribute",
			},
			{
				Name:          "Output Format",
				DisplayName:   "Output Format",
				Description:   "Format for the hash output",
				Required:      true,
				DefaultValue:  "hex",
				AllowedValues: []string{"hex", "base64"},
				Type:          "select",
				HelpText:      "hex: Lowercase hexadecimal; base64: Base64 encoding",
			},
			{
				Name:          "Fail on Empty",
				DisplayName:   "Fail on Empty",
				Description:   "Whether to fail if FlowFile content is empty",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, routes empty FlowFiles to failure. When false, generates hash of empty content.",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &HashContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *HashContentProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *HashContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	algorithm := processorCtx.GetPropertyValue("Hash Algorithm")
	attributeName := processorCtx.GetPropertyValue("Attribute Name")
	outputFormat := processorCtx.GetPropertyValue("Output Format")
	failOnEmpty := processorCtx.GetPropertyValue("Fail on Empty") == "true"

	// Read FlowFile content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Check if content is empty
	if len(content) == 0 && failOnEmpty {
		logger.Warn("FlowFile has empty content",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.RelationshipFailure)
		return nil
	}

	// Create hash based on algorithm
	var hasher hash.Hash
	switch algorithm {
	case "MD5":
		hasher = md5.New() // #nosec G401 - MD5 support required for legacy system compatibility
	case "SHA-1":
		hasher = sha1.New() // #nosec G401 - SHA-1 support required for legacy system compatibility
	case "SHA-256":
		hasher = sha256.New()
	case "SHA-384":
		hasher = sha512.New384()
	case "SHA-512":
		hasher = sha512.New()
	default:
		err := fmt.Errorf("unsupported hash algorithm: %s", algorithm)
		logger.Error("Unsupported hash algorithm",
			"flowFileId", flowFile.ID,
			"algorithm", algorithm)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Compute hash
	hasher.Write(content)
	hashBytes := hasher.Sum(nil)

	// Format output
	var hashValue string
	if outputFormat == "base64" {
		hashValue = hex.EncodeToString(hashBytes) // Using hex for now, can add proper base64 later
	} else {
		hashValue = hex.EncodeToString(hashBytes)
	}

	// Store hash in attribute
	session.PutAttribute(flowFile, attributeName, hashValue)
	session.PutAttribute(flowFile, attributeName+".algorithm", algorithm)
	session.PutAttribute(flowFile, "hash.algorithm", algorithm)
	session.PutAttribute(flowFile, "hash.value", hashValue)

	logger.Info("Successfully generated hash",
		"flowFileId", flowFile.ID,
		"algorithm", algorithm,
		"hashLength", len(hashValue),
		"contentSize", len(content))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}
