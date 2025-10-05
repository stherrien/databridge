package plugins

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"

	"github.com/shawntherrien/databridge/internal/plugin"
	"github.com/shawntherrien/databridge/pkg/types"
)

func init() {
	info := getSplitXMLInfo()
	plugin.RegisterBuiltInProcessor("SplitXML", func() types.Processor {
		return NewSplitXMLProcessor()
	}, info)
}

func getSplitXMLInfo() plugin.PluginInfo {
	return plugin.NewProcessorInfo(
		"SplitXML",
		"SplitXML",
		"1.0.0",
		"DataBridge",
		"Splits XML documents into individual FlowFiles based on a specified element path. Each matching element becomes a separate FlowFile with its own XML document.",
		[]string{"xml", "split", "parse", "element"},
	)
}

// SplitXMLProcessor splits XML documents by element
type SplitXMLProcessor struct {
	*types.BaseProcessor
}

// NewSplitXMLProcessor creates a new SplitXML processor
func NewSplitXMLProcessor() *SplitXMLProcessor {
	info := types.ProcessorInfo{
		Name:        "SplitXML",
		Description: "Splits XML documents into individual FlowFiles based on element path",
		Version:     "1.0.0",
		Author:      "DataBridge",
		Tags:        []string{"xml", "split", "parse", "element"},
		Properties: []types.PropertySpec{
			{
				Name:        "Split Element",
				DisplayName: "Split Element",
				Description: "Name of the XML element to split on",
				Required:    true,
				Type:        "string",
				Placeholder: "e.g., 'item', 'record', 'entry'",
				HelpText:    "Each occurrence of this element will become a separate FlowFile",
			},
			{
				Name:          "Include Parent Wrapper",
				DisplayName:   "Include Parent Wrapper",
				Description:   "Whether to include parent element wrapper in split FlowFiles",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, each split includes the parent element structure",
			},
			{
				Name:          "Keep Original",
				DisplayName:   "Keep Original",
				Description:   "Whether to keep the original FlowFile",
				Required:      false,
				DefaultValue:  "false",
				AllowedValues: []string{"true", "false"},
				Type:          "boolean",
				HelpText:      "When true, the original FlowFile is routed to 'original'. When false, only splits are kept.",
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "splits",
				Description: "Split FlowFiles are routed to this relationship",
			},
			types.RelationshipOriginal,
			types.RelationshipFailure,
		},
	}

	return &SplitXMLProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

// Initialize initializes the processor
func (p *SplitXMLProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

// OnTrigger processes FlowFiles
func (p *SplitXMLProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
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
	splitElement := processorCtx.GetPropertyValue("Split Element")
	includeWrapper := processorCtx.GetPropertyValue("Include Parent Wrapper") == "true"
	keepOriginal := processorCtx.GetPropertyValue("Keep Original") == "true"

	if splitElement == "" {
		logger.Error("Split element not specified",
			"flowFileId", flowFile.ID)
		session.Transfer(flowFile, types.RelationshipFailure)
		return fmt.Errorf("split element is required")
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

	// Parse and split XML
	splits, err := p.splitXML(content, splitElement, includeWrapper)
	if err != nil {
		logger.Error("Failed to split XML",
			"flowFileId", flowFile.ID,
			"error", err)
		session.Transfer(flowFile, types.RelationshipFailure)
		return err
	}

	// Create FlowFiles for each split
	for i, splitContent := range splits {
		split := session.Clone(flowFile)
		if err := session.Write(split, splitContent); err != nil {
			logger.Error("Failed to write split content",
				"flowFileId", flowFile.ID,
				"split", i,
				"error", err)
			session.Remove(split)
			continue
		}

		// Add split attributes
		session.PutAttribute(split, "split.index", strconv.Itoa(i))
		session.PutAttribute(split, "split.count", strconv.Itoa(len(splits)))
		session.PutAttribute(split, "split.element", splitElement)
		session.PutAttribute(split, "fragment.identifier", flowFile.ID.String())
		session.PutAttribute(split, "fragment.index", strconv.Itoa(i))
		session.PutAttribute(split, "fragment.count", strconv.Itoa(len(splits)))

		// Update filename for split
		if filename, exists := flowFile.GetAttribute("filename"); exists {
			session.PutAttribute(split, "filename", fmt.Sprintf("%s.split.%d.xml", filename, i))
		}

		session.Transfer(split, types.Relationship{Name: "splits"})
	}

	logger.Info("Successfully split XML",
		"flowFileId", flowFile.ID,
		"element", splitElement,
		"splitCount", len(splits))

	// Handle original FlowFile
	if keepOriginal {
		session.Transfer(flowFile, types.RelationshipOriginal)
	} else {
		session.Remove(flowFile)
	}

	return nil
}

// splitXML splits XML content by element
func (p *SplitXMLProcessor) splitXML(content []byte, elementName string, includeWrapper bool) ([][]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var splits [][]byte
	var currentElement *bytes.Buffer
	var depth int
	var capturing bool
	var parentElements []xml.StartElement

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse XML: %w", err)
		}

		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == elementName && depth == 0 {
				// Start capturing this element
				capturing = true
				currentElement = &bytes.Buffer{}

				if includeWrapper && len(parentElements) > 0 {
					// Write parent wrapper start tags
					for _, parent := range parentElements {
						if err := xml.NewEncoder(currentElement).EncodeToken(parent); err != nil {
							return nil, err
						}
					}
				}
			}

			if capturing {
				if err := xml.NewEncoder(currentElement).EncodeToken(t); err != nil {
					return nil, err
				}
				depth++
			} else {
				// Track parent elements for wrapper
				parentElements = append(parentElements, t)
			}

		case xml.EndElement:
			if capturing {
				if err := xml.NewEncoder(currentElement).EncodeToken(t); err != nil {
					return nil, err
				}
				depth--

				if depth == 0 {
					// Finished capturing this element
					if includeWrapper && len(parentElements) > 0 {
						// Write parent wrapper end tags
						for i := len(parentElements) - 1; i >= 0; i-- {
							endToken := xml.EndElement{Name: parentElements[i].Name}
							if err := xml.NewEncoder(currentElement).EncodeToken(endToken); err != nil {
								return nil, err
							}
						}
					}

					// Add XML declaration
					xmlContent := []byte(xml.Header + currentElement.String())
					splits = append(splits, xmlContent)
					capturing = false
					currentElement = nil
				}
			} else if len(parentElements) > 0 {
				// Pop parent element
				parentElements = parentElements[:len(parentElements)-1]
			}

		case xml.CharData:
			if capturing {
				if err := xml.NewEncoder(currentElement).EncodeToken(token); err != nil {
					return nil, err
				}
			}

		case xml.Comment:
			if capturing {
				if err := xml.NewEncoder(currentElement).EncodeToken(token); err != nil {
					return nil, err
				}
			}
		}
	}

	return splits, nil
}
