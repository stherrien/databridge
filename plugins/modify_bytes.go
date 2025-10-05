package plugins

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getModifyBytesInfo()
	plugin.RegisterBuiltInProcessor("ModifyBytes", func() types.Processor {
		return NewModifyBytesProcessor()
	}, info)
}

func getModifyBytesInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"ModifyBytes",
		"ModifyBytes",
		"1.0.0",
		"DataBridge",
		"Modifies binary content by prepending, appending, or replacing bytes. Useful for adding headers, footers, or transforming binary data.",
		[]string{"binary", "bytes", "modification", "prepend", "append"},
	)
}

// ModifyBytesProcessor modifies binary content
type ModifyBytesProcessor struct {
	*types.BaseProcessor
}

// NewModifyBytesProcessor creates a new ModifyBytes processor
func NewModifyBytesProcessor() *ModifyBytesProcessor {
	info := types.ProcessorInfo{
		Name:        "ModifyBytes",
		Description: "Modifies binary content by prepending, appending, or replacing bytes",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"binary", "bytes", "modification", "prepend", "append"},
		Properties: []types.PropertySpec{
			{
				Name:          "Operation",
				DisplayName:   "Operation",
				Description:   "The type of modification to perform",
				Required:      true,
				DefaultValue:  "prepend",
				AllowedValues: []string{"prepend", "append", "replace", "insert"},
				Type:          "select",
				HelpText:      "prepend: Add bytes at start; append: Add bytes at end; replace: Replace existing bytes; insert: Insert bytes at position",
			},
			{
				Name:        "Byte Value",
				DisplayName: "Byte Value",
				Description: "The bytes to add or use for replacement (hex format: e.g., '0A 1B 2C' or text)",
				Required:    true,
				Type:        "string",
				Placeholder: "e.g., '0A 1B 2C' or 'Hello'",
				HelpText:    "Enter bytes in hex format (space/comma separated) or plain text. Hex: '0A 1B', Text: 'Hello'",
			},
			{
				Name:          "Input Format",
				DisplayName:   "Input Format",
				Description:   "Format of the Byte Value input",
				Required:      true,
				DefaultValue:  "text",
				AllowedValues: []string{"text", "hex"},
				Type:          "select",
				HelpText:      "text: Plain text string; hex: Hexadecimal byte values",
			},
			{
				Name:         "Start Position",
				DisplayName:  "Start Position",
				Description:  "Starting position for replace/insert operations (0-based)",
				Required:     false,
				DefaultValue: "0",
				Type:         "string",
				Placeholder:  "0",
				HelpText:     "Only used for 'replace' and 'insert' operations. Position starts at 0.",
			},
			{
				Name:        "Length",
				DisplayName: "Length",
				Description: "Number of bytes to replace (only for replace operation)",
				Required:    false,
				Type:        "string",
				Placeholder: "10",
				HelpText:    "Only used for 'replace' operation. If empty, replaces from start position to end.",
			},
			{
				Name:          "Remove Original",
				DisplayName:   "Remove Original",
				Description:   "Whether to remove the original FlowFile after modification",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, only the modified FlowFile is kept. When false, both original and modified are output.",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
			types.RelationshipOriginal,
		},
	}

	return &ModifyBytesProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *ModifyBytesProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *ModifyBytesProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	operation := processorCtx.GetPropertyValue("Operation")
	byteValue := processorCtx.GetPropertyValue("Byte Value")
	inputFormat := processorCtx.GetPropertyValue("Input Format")
	startPosStr := processorCtx.GetPropertyValue("Start Position")
	lengthStr := processorCtx.GetPropertyValue("Length")
	removeOriginal := processorCtx.GetPropertyValue("Remove Original") == "true"

	// Parse byte value based on format
	var bytesToAdd []byte
	var err error

	if inputFormat == "hex" {
		bytesToAdd, err = parseHexBytes(byteValue)
		if err != nil {
			logger.Error("Failed to parse hex bytes",
				"flowFileId", flowFile.ID,
				"byteValue", byteValue,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}
	} else {
		bytesToAdd = []byte(byteValue)
	}

	// Read original content
	content, err := session.Read(flowFile)
	if err != nil {
		logger.Error("Failed to read FlowFile content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Perform the operation
	var result []byte
	switch operation {
	case "prepend":
		result = append(bytesToAdd, content...)

	case "append":
		result = append(content, bytesToAdd...)

	case "insert":
		startPos, err := parsePosition(startPosStr, len(content))
		if err != nil {
			logger.Error("Invalid start position",
				"flowFileId", flowFile.ID,
				"startPos", startPosStr,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		result = make([]byte, 0, len(content)+len(bytesToAdd))
		result = append(result, content[:startPos]...)
		result = append(result, bytesToAdd...)
		result = append(result, content[startPos:]...)

	case "replace":
		startPos, err := parsePosition(startPosStr, len(content))
		if err != nil {
			logger.Error("Invalid start position",
				"flowFileId", flowFile.ID,
				"startPos", startPosStr,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		// Determine length to replace
		length := len(content) - startPos // Default: replace to end
		if lengthStr != "" {
			length, err = strconv.Atoi(lengthStr)
			if err != nil {
				logger.Error("Invalid length value",
					"flowFileId", flowFile.ID,
					"length", lengthStr,
					"error", err)
				session.Transfer(flowFile, types.RelationshipFailure)
				return err
			}
		}

		// Validate bounds
		endPos := startPos + length
		if endPos > len(content) {
			endPos = len(content)
		}

		result = make([]byte, 0, startPos+len(bytesToAdd)+(len(content)-endPos))
		result = append(result, content[:startPos]...)
		result = append(result, bytesToAdd...)
		result = append(result, content[endPos:]...)

	default:
		err := fmt.Errorf("unsupported operation: %s", operation)
		logger.Error("Unsupported operation",
			"flowFileId", flowFile.ID,
			"operation", operation)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Create modified FlowFile
	modified := session.Clone(flowFile)
	if err := session.Write(modified, result); err != nil {
		logger.Error("Failed to write modified content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		session.Remove(modified)
		return err
	}

	// Update attributes
	session.PutAttribute(modified, "modify_bytes.operation", operation)
	session.PutAttribute(modified, "modify_bytes.original_size", strconv.Itoa(len(content)))
	session.PutAttribute(modified, "modify_bytes.modified_size", strconv.Itoa(len(result)))
	session.PutAttribute(modified, "modify_bytes.bytes_added", strconv.Itoa(len(bytesToAdd)))

	logger.Info("Successfully modified bytes",
		"flowFileId", flowFile.ID,
		"operation", operation,
		"originalSize", len(content),
		"modifiedSize", len(result),
		"bytesAdded", len(bytesToAdd))

	// Route FlowFiles
	session.Transfer(modified, types.RelationshipSuccess)

	if removeOriginal {
		session.Remove(flowFile)
	} else {
		session.Transfer(flowFile, types.RelationshipOriginal)
	}

	return nil
}

// parseHexBytes converts hex string to bytes
// Accepts formats: "0A 1B 2C", "0A,1B,2C", "0A1B2C", "0a 1b 2c"
func parseHexBytes(hexStr string) ([]byte, error) {
	// Remove common separators and whitespace
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, ",", "")
	hexStr = strings.ReplaceAll(hexStr, ":", "")
	hexStr = strings.TrimSpace(hexStr)

	if len(hexStr)%2 != 0 {
		return nil, fmt.Errorf("hex string must have even number of characters")
	}

	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}

	return bytes, nil
}

// parsePosition parses position string and validates it
func parsePosition(posStr string, maxLen int) (int, error) {
	if posStr == "" {
		return 0, nil
	}

	pos, err := strconv.Atoi(posStr)
	if err != nil {
		return 0, fmt.Errorf("invalid position: %w", err)
	}

	if pos < 0 {
		return 0, fmt.Errorf("position cannot be negative")
	}

	if pos > maxLen {
		return 0, fmt.Errorf("position %d exceeds content length %d", pos, maxLen)
	}

	return pos, nil
}
