package plugins

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getEncryptContentInfo()
	plugin.RegisterBuiltInProcessor("EncryptContent", func() types.Processor {
		return NewEncryptContentProcessor()
	}, info)
}

func getEncryptContentInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"EncryptContent",
		"EncryptContent",
		"1.0.0",
		"DataBridge",
		"Encrypts or decrypts FlowFile content using various encryption algorithms including AES-128, AES-192, and AES-256. Supports multiple modes of operation.",
		[]string{"encryption", "decryption", "security", "aes", "crypto"},
	)
}

// EncryptContentProcessor encrypts or decrypts content
type EncryptContentProcessor struct {
	*types.BaseProcessor
}

// NewEncryptContentProcessor creates a new EncryptContent processor
func NewEncryptContentProcessor() *EncryptContentProcessor {
	info := types.ProcessorInfo{
		Name:        "EncryptContent",
		Description: "Encrypts or decrypts FlowFile content using various encryption algorithms",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"encryption", "decryption", "security", "aes", "crypto"},
		Properties: []types.PropertySpec{
			{
				Name:          "Mode",
				DisplayName:   "Mode",
				Description:   "Whether to encrypt or decrypt content",
				Required:      true,
				DefaultValue:  "Encrypt",
				AllowedValues: []string{"Encrypt", "Decrypt"},
				Type:          "select",
				HelpText:      "Encrypt: Encrypt FlowFile content; Decrypt: Decrypt FlowFile content",
			},
			{
				Name:          "Encryption Algorithm",
				DisplayName:   "Encryption Algorithm",
				Description:   "The encryption algorithm to use",
				Required:      true,
				DefaultValue:  "AES-256-GCM",
				AllowedValues: []string{"AES-128-GCM", "AES-192-GCM", "AES-256-GCM"},
				Type:          "select",
				HelpText:      "GCM mode provides authenticated encryption. Key sizes: 128-bit (16 bytes), 192-bit (24 bytes), 256-bit (32 bytes)",
			},
			{
				Name:        "Encryption Key",
				DisplayName: "Encryption Key",
				Description: "The encryption key in hexadecimal format",
				Required:    true,
				Type:        "password",
				Placeholder: "Enter key in hex format",
				HelpText:    "AES-128: 32 hex chars (16 bytes); AES-192: 48 hex chars (24 bytes); AES-256: 64 hex chars (32 bytes)",
			},
			{
				Name:         "Store IV in Attribute",
				DisplayName:  "Store IV in Attribute",
				Description:  "Whether to store the initialization vector (IV) in a FlowFile attribute",
				Required:     false,
				DefaultValue: "true",
				AllowedValues: []string{"true", "false"},
				Type:         "boolean",
				HelpText:     "When encrypting, the IV is stored in 'encryption.iv' attribute. Required for decryption.",
			},
			{
				Name:        "IV Attribute Name",
				DisplayName: "IV Attribute Name",
				Description: "Name of the attribute containing the IV (for decryption)",
				Required:    false,
				DefaultValue: "encryption.iv",
				Type:        "string",
				Placeholder: "encryption.iv",
				HelpText:    "When decrypting, the IV is read from this attribute",
			},
		},
		Relationships: []types.Relationship{
			types.RelationshipSuccess,
			types.RelationshipFailure,
		},
	}

	return &EncryptContentProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *EncryptContentProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *EncryptContentProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	algorithm := processorCtx.GetPropertyValue("Encryption Algorithm")
	keyHex := processorCtx.GetPropertyValue("Encryption Key")
	storeIV := processorCtx.GetPropertyValue("Store IV in Attribute") == "true"
	ivAttributeName := processorCtx.GetPropertyValue("IV Attribute Name")

	// Parse encryption key
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		logger.Error("Invalid encryption key format",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("invalid encryption key: must be hex format")
	}

	// Validate key size for algorithm
	expectedKeySize := 0
	switch algorithm {
	case "AES-128-GCM":
		expectedKeySize = 16
	case "AES-192-GCM":
		expectedKeySize = 24
	case "AES-256-GCM":
		expectedKeySize = 32
	default:
		logger.Error("Unsupported algorithm",
			"flowFileId", flowFile.ID,
			"algorithm", algorithm)
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("unsupported algorithm: %s", algorithm)
	}

	if len(key) != expectedKeySize {
		logger.Error("Invalid key size",
			"flowFileId", flowFile.ID,
			"expected", expectedKeySize,
			"actual", len(key))
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("invalid key size: expected %d bytes, got %d bytes", expectedKeySize, len(key))
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

	var result []byte
	if mode == "Encrypt" {
		result, err = p.encrypt(content, key, algorithm)
		if err != nil {
			logger.Error("Failed to encrypt content",
				"flowFileId", flowFile.ID,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}

		// Extract and store IV from encrypted content (first 12 bytes for GCM)
		if storeIV && len(result) >= 12 {
			iv := result[:12]
			session.PutAttribute(flowFile, ivAttributeName, hex.EncodeToString(iv))
		}

		session.PutAttribute(flowFile, "encryption.algorithm", algorithm)
	} else {
		// Decrypt mode
		result, err = p.decrypt(content, key, algorithm)
		if err != nil {
			logger.Error("Failed to decrypt content",
				"flowFileId", flowFile.ID,
				"error", err)
			session.Transfer(flowFile, types.RelationshipFailure)
			return err
		}
	}

	// Write result
	if err := session.Write(flowFile, result); err != nil {
		logger.Error("Failed to write content",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	logger.Info("Successfully processed content",
		"flowFileId", flowFile.ID,
		"mode", mode,
		"algorithm", algorithm,
		"originalSize", len(content),
		"resultSize", len(result))

	session.Transfer(flowFile, types.RelationshipSuccess)
	return nil
}

// encrypt encrypts content using AES-GCM
func (p *EncryptContentProcessor) encrypt(plaintext []byte, key []byte, algorithm string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts content using AES-GCM
func (p *EncryptContentProcessor) decrypt(ciphertext []byte, key []byte, algorithm string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
