package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/shawntherrien/databridge/pkg/types"
)

// MockProcessor for testing
type MockDocProcessor struct {
	*types.BaseProcessor
}

func NewMockDocProcessor() *MockDocProcessor {
	info := types.ProcessorInfo{
		Name:        "MockProcessor",
		Description: "A mock processor for testing documentation",
		Version:     "1.0.0",
		Author:      "Test Author",
		Tags:        []string{"test", "mock", "utility"},
		Properties: []types.PropertySpec{
			{
				Name:         "TestProperty",
				Description:  "A test property",
				Required:     true,
				DefaultValue: "default",
				AllowedValues: []string{"value1", "value2"},
			},
			{
				Name:        "SensitiveProperty",
				Description: "A sensitive test property",
				Sensitive:   true,
				Required:    false,
			},
		},
		Relationships: []types.Relationship{
			{
				Name:        "success",
				Description: "Successful processing",
			},
			{
				Name:        "failure",
				Description: "Failed processing",
			},
		},
	}

	return &MockDocProcessor{
		BaseProcessor: types.NewBaseProcessor(info),
	}
}

func (p *MockDocProcessor) Initialize(ctx types.ProcessorContext) error {
	return nil
}

func (p *MockDocProcessor) OnTrigger(ctx context.Context, session types.ProcessSession) error {
	return nil
}

func TestNewDocumentationGenerator(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})
	gen := NewDocumentationGenerator(registry, versionRegistry)

	assert.NotNil(t, gen)
	assert.NotNil(t, gen.outputFormats)
	assert.Contains(t, gen.outputFormats, "markdown")
	assert.Contains(t, gen.outputFormats, "html")
	assert.Contains(t, gen.outputFormats, "json")
}

func TestGenerateDocumentation(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	// Register mock processor
	registry.Register("MockProcessor", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)

	t.Run("Generate Valid Documentation", func(t *testing.T) {
		doc, err := gen.GenerateDocumentation("MockProcessor")
		assert.NoError(t, err)
		assert.NotNil(t, doc)
		assert.Equal(t, "MockProcessor", doc.Name)
		assert.Equal(t, "MockProcessor", doc.Type)
		assert.Equal(t, "1.0.0", doc.Version)
		assert.Equal(t, "Test Author", doc.Author)
		assert.Len(t, doc.Properties, 2)
		assert.Len(t, doc.Relationships, 2)
		assert.NotZero(t, doc.GeneratedAt)
	})

	t.Run("Generate for Non-existent Processor", func(t *testing.T) {
		_, err := gen.GenerateDocumentation("NonExistent")
		assert.Error(t, err)
	})
}

func TestGenerateAllDocumentation(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	// Register multiple processors
	registry.Register("MockProcessor1", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})
	registry.Register("MockProcessor2", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)

	docs, err := gen.GenerateAllDocumentation()
	assert.NoError(t, err)
	assert.Len(t, docs, 2)
	assert.Contains(t, docs, "MockProcessor1")
	assert.Contains(t, docs, "MockProcessor2")
}

func TestDocumentationWithVersionHistory(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	// Register processor
	registry.Register("MockProcessor", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	// Register version history
	versionRegistry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "MockProcessor",
		Version:       "1.0.0",
		ReleaseDate:   time.Now().Add(-30 * 24 * time.Hour),
		ChangeLog: []VersionChange{
			{
				Type:        ChangeTypeFeature,
				Description: "Initial release",
				Impact:      ImpactNone,
			},
		},
	})

	versionRegistry.RegisterVersion(&ProcessorVersion{
		ProcessorType: "MockProcessor",
		Version:       "1.1.0",
		ReleaseDate:   time.Now(),
		ChangeLog: []VersionChange{
			{
				Type:        ChangeTypeImprovement,
				Description: "Performance improvements",
				Impact:      ImpactLow,
			},
		},
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)
	doc, err := gen.GenerateDocumentation("MockProcessor")

	assert.NoError(t, err)
	assert.Len(t, doc.VersionHistory, 2)
}

func TestExportDocumentation(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	registry.Register("MockProcessor", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)
	doc, _ := gen.GenerateDocumentation("MockProcessor")

	t.Run("Export as Markdown", func(t *testing.T) {
		output, err := gen.ExportDocumentation(doc, "markdown")
		assert.NoError(t, err)
		assert.Contains(t, output, "# MockProcessor")
		assert.Contains(t, output, "## Properties")
		assert.Contains(t, output, "TestProperty")
	})

	t.Run("Export as HTML", func(t *testing.T) {
		output, err := gen.ExportDocumentation(doc, "html")
		assert.NoError(t, err)
		assert.Contains(t, output, "<html>")
		assert.Contains(t, output, "<h1>MockProcessor</h1>")
		assert.Contains(t, output, "TestProperty")
	})

	t.Run("Export as JSON", func(t *testing.T) {
		output, err := gen.ExportDocumentation(doc, "json")
		assert.NoError(t, err)
		assert.Contains(t, output, "\"name\": \"MockProcessor\"")
		assert.Contains(t, output, "TestProperty")
	})

	t.Run("Export Unknown Format", func(t *testing.T) {
		_, err := gen.ExportDocumentation(doc, "unknown")
		assert.Error(t, err)
	})
}

func TestExportAllDocumentation(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	registry.Register("MockProcessor", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)
	tmpDir := t.TempDir()

	t.Run("Export All as Markdown", func(t *testing.T) {
		err := gen.ExportAllDocumentation(tmpDir, "markdown")
		assert.NoError(t, err)

		// Verify files exist
		assert.FileExists(t, filepath.Join(tmpDir, "MockProcessor.md"))
		assert.FileExists(t, filepath.Join(tmpDir, "index.md"))

		// Verify content
		content, _ := os.ReadFile(filepath.Join(tmpDir, "MockProcessor.md"))
		assert.Contains(t, string(content), "# MockProcessor")
	})

	t.Run("Export All as HTML", func(t *testing.T) {
		htmlDir := filepath.Join(tmpDir, "html")
		err := gen.ExportAllDocumentation(htmlDir, "html")
		assert.NoError(t, err)

		assert.FileExists(t, filepath.Join(htmlDir, "MockProcessor.html"))
		assert.FileExists(t, filepath.Join(htmlDir, "index.html"))
	})

	t.Run("Export Unknown Format", func(t *testing.T) {
		err := gen.ExportAllDocumentation(tmpDir, "unknown")
		assert.Error(t, err)
	})
}

func TestCategorizeProcessor(t *testing.T) {
	tests := []struct {
		tags     []string
		expected string
	}{
		{[]string{"input", "source"}, "Data Ingestion"},
		{[]string{"output", "sink"}, "Data Egress"},
		{[]string{"transform", "convert"}, "Transformation"},
		{[]string{"kafka", "messaging"}, "Messaging"},
		{[]string{"database", "sql"}, "Database"},
		{[]string{"file", "directory"}, "File Operations"},
		{[]string{"http", "rest"}, "HTTP"},
		{[]string{"utility"}, "Utility"},
		{[]string{"unknown"}, "Other"},
	}

	for _, tc := range tests {
		result := categorizeProcessor(tc.tags)
		assert.Equal(t, tc.expected, result, "Tags: %v", tc.tags)
	}
}

func TestMarkdownFormatter(t *testing.T) {
	formatter := &MarkdownFormatter{}

	assert.Equal(t, "md", formatter.FileExtension())

	doc := &ProcessorDocumentation{
		Name:        "TestProcessor",
		Description: "Test description",
		Version:     "1.0.0",
		Author:      "Test Author",
		Category:    "Utility",
		Properties: []PropertyDocumentation{
			{
				Name:         "Prop1",
				Description:  "Property 1",
				Required:     true,
				DefaultValue: "default",
			},
		},
		Relationships: []RelationshipDocumentation{
			{
				Name:        "success",
				Description: "Success relationship",
			},
		},
		GeneratedAt: time.Now(),
	}

	output, err := formatter.Format(doc)
	assert.NoError(t, err)
	assert.Contains(t, output, "# TestProcessor")
	assert.Contains(t, output, "**Version**: 1.0.0")
	assert.Contains(t, output, "## Properties")
	assert.Contains(t, output, "### Prop1")
	assert.Contains(t, output, "**(Required)**")
}

func TestHTMLFormatter(t *testing.T) {
	formatter := &HTMLFormatter{}

	assert.Equal(t, "html", formatter.FileExtension())

	doc := &ProcessorDocumentation{
		Name:        "TestProcessor",
		Description: "Test description",
		Version:     "1.0.0",
		Author:      "Test Author",
		Category:    "Utility",
		Properties: []PropertyDocumentation{
			{
				Name:         "Prop1",
				Description:  "Property 1",
				Required:     true,
				DefaultValue: "default",
			},
		},
		Relationships: []RelationshipDocumentation{
			{
				Name:        "success",
				Description: "Success relationship",
			},
		},
		GeneratedAt: time.Now(),
	}

	output, err := formatter.Format(doc)
	assert.NoError(t, err)
	assert.Contains(t, output, "<html>")
	assert.Contains(t, output, "<h1>TestProcessor</h1>")
	assert.Contains(t, output, "<h2>Properties</h2>")
	assert.Contains(t, output, "<h3>Prop1")
}

func TestJSONFormatter(t *testing.T) {
	formatter := &JSONFormatter{}

	assert.Equal(t, "json", formatter.FileExtension())

	doc := &ProcessorDocumentation{
		Name:        "TestProcessor",
		Description: "Test description",
		Version:     "1.0.0",
		GeneratedAt: time.Now(),
	}

	output, err := formatter.Format(doc)
	assert.NoError(t, err)
	assert.Contains(t, output, "\"name\": \"TestProcessor\"")
	assert.Contains(t, output, "\"version\": \"1.0.0\"")
}

func TestRegisterFormatter(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})
	gen := NewDocumentationGenerator(registry, versionRegistry)

	// Custom formatter
	type CustomFormatter struct{}
	func (f *CustomFormatter) Format(doc *ProcessorDocumentation) (string, error) {
		return "custom format", nil
	}
	func (f *CustomFormatter) FileExtension() string {
		return "custom"
	}

	gen.RegisterFormatter("custom", &CustomFormatter{})
	assert.Contains(t, gen.outputFormats, "custom")
}

func TestPropertyDocumentation(t *testing.T) {
	prop := PropertyDocumentation{
		Name:            "TestProp",
		DisplayName:     "Test Property",
		Description:     "A test property",
		Required:        true,
		Sensitive:       false,
		DefaultValue:    "default",
		AllowedValues:   []string{"val1", "val2"},
		Pattern:         "^[a-z]+$",
		ExampleValue:    "example",
		SupportsEL:      true,
		Dependencies:    []string{"OtherProp"},
	}

	assert.Equal(t, "TestProp", prop.Name)
	assert.True(t, prop.Required)
	assert.True(t, prop.SupportsEL)
	assert.Len(t, prop.AllowedValues, 2)
	assert.Len(t, prop.Dependencies, 1)
}

func TestUsageExample(t *testing.T) {
	example := UsageExample{
		Title:       "Basic Usage",
		Description: "Shows basic usage",
		Configuration: map[string]string{
			"prop1": "value1",
			"prop2": "value2",
		},
		InputData:  "input data",
		OutputData: "output data",
		Notes:      "Some notes",
	}

	assert.Equal(t, "Basic Usage", example.Title)
	assert.Len(t, example.Configuration, 2)
	assert.NotEmpty(t, example.Notes)
}

func TestVersionInfo(t *testing.T) {
	info := VersionInfo{
		Version:     "1.0.0",
		ReleaseDate: time.Now(),
		Changes:     []string{"Feature 1", "Bug fix 2"},
	}

	assert.Equal(t, "1.0.0", info.Version)
	assert.Len(t, info.Changes, 2)
}

func TestGenerateIndex(t *testing.T) {
	registry := NewPluginRegistry()
	versionRegistry := NewProcessorVersionRegistry(&types.MockLogger{})

	registry.Register("Processor1", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})
	registry.Register("Processor2", func() (types.Processor, error) {
		return NewMockDocProcessor(), nil
	})

	gen := NewDocumentationGenerator(registry, versionRegistry)
	tmpDir := t.TempDir()

	err := gen.ExportAllDocumentation(tmpDir, "markdown")
	assert.NoError(t, err)

	// Verify index exists and contains processor links
	indexContent, err := os.ReadFile(filepath.Join(tmpDir, "index.md"))
	assert.NoError(t, err)
	assert.Contains(t, string(indexContent), "Processor Documentation Index")
	assert.Contains(t, string(indexContent), "Processor1")
	assert.Contains(t, string(indexContent), "Processor2")
}
