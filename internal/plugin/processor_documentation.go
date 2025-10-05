package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

// DocumentationGenerator generates processor documentation
type DocumentationGenerator struct {
	registry        *PluginRegistry
	versionRegistry *ProcessorVersionRegistry
	outputFormats   map[string]DocumentFormatter
	templates       map[string]*template.Template
}

// DocumentFormatter formats documentation in a specific format
type DocumentFormatter interface {
	Format(doc *ProcessorDocumentation) (string, error)
	FileExtension() string
}

// ProcessorDocumentation contains complete processor documentation
type ProcessorDocumentation struct {
	Name            string                      `json:"name"`
	Type            string                      `json:"type"`
	Description     string                      `json:"description"`
	Version         string                      `json:"version"`
	Author          string                      `json:"author"`
	Tags            []string                    `json:"tags"`
	Category        string                      `json:"category"`
	Properties      []PropertyDocumentation     `json:"properties"`
	Relationships   []RelationshipDocumentation `json:"relationships"`
	Examples        []UsageExample              `json:"examples"`
	SeeAlso         []string                    `json:"seeAlso,omitempty"`
	AdditionalNotes string                      `json:"additionalNotes,omitempty"`
	VersionHistory  []VersionInfo               `json:"versionHistory,omitempty"`
	GeneratedAt     time.Time                   `json:"generatedAt"`
}

// PropertyDocumentation contains detailed property documentation
type PropertyDocumentation struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"displayName"`
	Description   string   `json:"description"`
	Required      bool     `json:"required"`
	Sensitive     bool     `json:"sensitive"`
	DefaultValue  string   `json:"defaultValue"`
	AllowedValues []string `json:"allowedValues,omitempty"`
	Pattern       string   `json:"pattern,omitempty"`
	ExampleValue  string   `json:"exampleValue,omitempty"`
	SupportsEL    bool     `json:"supportsExpressionLanguage"`
	Dependencies  []string `json:"dependencies,omitempty"`
}

// RelationshipDocumentation contains relationship documentation
type RelationshipDocumentation struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	AutoTerminate bool   `json:"autoTerminate"`
}

// UsageExample provides a usage example
type UsageExample struct {
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Configuration map[string]string `json:"configuration"`
	InputData     string            `json:"inputData,omitempty"`
	OutputData    string            `json:"outputData,omitempty"`
	Notes         string            `json:"notes,omitempty"`
}

// VersionInfo contains version information
type VersionInfo struct {
	Version     string    `json:"version"`
	ReleaseDate time.Time `json:"releaseDate"`
	Changes     []string  `json:"changes"`
}

// NewDocumentationGenerator creates a new documentation generator
func NewDocumentationGenerator(
	registry *PluginRegistry,
	versionRegistry *ProcessorVersionRegistry,
) *DocumentationGenerator {
	gen := &DocumentationGenerator{
		registry:        registry,
		versionRegistry: versionRegistry,
		outputFormats:   make(map[string]DocumentFormatter),
		templates:       make(map[string]*template.Template),
	}

	// Register default formatters
	gen.RegisterFormatter("markdown", &MarkdownFormatter{})
	gen.RegisterFormatter("html", &HTMLFormatter{})
	gen.RegisterFormatter("json", &JSONFormatter{})

	// Load default templates
	gen.loadDefaultTemplates()

	return gen
}

// RegisterFormatter registers a documentation formatter
func (g *DocumentationGenerator) RegisterFormatter(name string, formatter DocumentFormatter) {
	g.outputFormats[name] = formatter
}

// GenerateDocumentation generates documentation for a processor
func (g *DocumentationGenerator) GenerateDocumentation(processorType string) (*ProcessorDocumentation, error) {
	// Get processor from registry
	processor, err := g.registry.GetProcessor(processorType)
	if err != nil {
		return nil, fmt.Errorf("processor type %s not found: %w", processorType, err)
	}

	info := processor.GetInfo()

	doc := &ProcessorDocumentation{
		Name:        info.Name,
		Type:        processorType,
		Description: info.Description,
		Version:     info.Version,
		Author:      info.Author,
		Tags:        info.Tags,
		Category:    categorizeProcessor(info.Tags),
		GeneratedAt: time.Now(),
	}

	// Convert properties
	doc.Properties = make([]PropertyDocumentation, len(info.Properties))
	for i, prop := range info.Properties {
		doc.Properties[i] = PropertyDocumentation{
			Name:          prop.Name,
			DisplayName:   prop.Name,
			Description:   prop.Description,
			Required:      prop.Required,
			Sensitive:     prop.Sensitive,
			DefaultValue:  prop.DefaultValue,
			AllowedValues: prop.AllowedValues,
			Pattern:       prop.Pattern,
		}
	}

	// Convert relationships
	doc.Relationships = make([]RelationshipDocumentation, len(info.Relationships))
	for i, rel := range info.Relationships {
		doc.Relationships[i] = RelationshipDocumentation{
			Name:          rel.Name,
			Description:   rel.Description,
			AutoTerminate: rel.AutoTerminate,
		}
	}

	// Add version history if available
	if g.versionRegistry != nil {
		versions := g.versionRegistry.ListVersions(processorType)
		doc.VersionHistory = make([]VersionInfo, len(versions))
		for i, ver := range versions {
			changes := make([]string, len(ver.ChangeLog))
			for j, change := range ver.ChangeLog {
				changes[j] = fmt.Sprintf("[%s] %s", change.Type, change.Description)
			}
			doc.VersionHistory[i] = VersionInfo{
				Version:     ver.Version,
				ReleaseDate: ver.ReleaseDate,
				Changes:     changes,
			}
		}
	}

	return doc, nil
}

// GenerateAllDocumentation generates documentation for all registered processors
func (g *DocumentationGenerator) GenerateAllDocumentation() (map[string]*ProcessorDocumentation, error) {
	processors := g.registry.ListProcessors()
	docs := make(map[string]*ProcessorDocumentation)

	for _, processorType := range processors {
		doc, err := g.GenerateDocumentation(processorType)
		if err != nil {
			return nil, fmt.Errorf("failed to generate docs for %s: %w", processorType, err)
		}
		docs[processorType] = doc
	}

	return docs, nil
}

// ExportDocumentation exports documentation in the specified format
func (g *DocumentationGenerator) ExportDocumentation(
	doc *ProcessorDocumentation,
	format string,
) (string, error) {
	formatter, exists := g.outputFormats[format]
	if !exists {
		return "", fmt.Errorf("unknown format: %s", format)
	}

	return formatter.Format(doc)
}

// ExportAllDocumentation exports all documentation to files
func (g *DocumentationGenerator) ExportAllDocumentation(outputDir, format string) error {
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	docs, err := g.GenerateAllDocumentation()
	if err != nil {
		return err
	}

	formatter, exists := g.outputFormats[format]
	if !exists {
		return fmt.Errorf("unknown format: %s", format)
	}

	for processorType, doc := range docs {
		content, err := formatter.Format(doc)
		if err != nil {
			return fmt.Errorf("failed to format docs for %s: %w", processorType, err)
		}

		filename := filepath.Join(outputDir, fmt.Sprintf("%s.%s", processorType, formatter.FileExtension()))
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filename, err)
		}
	}

	// Generate index file
	if err := g.generateIndex(outputDir, docs, format); err != nil {
		return fmt.Errorf("failed to generate index: %w", err)
	}

	return nil
}

// categoryDocs groups processors by category
type categoryDocs struct {
	Category   string
	Processors []*ProcessorDocumentation
}

// generateIndex generates an index file for all processors
func (g *DocumentationGenerator) generateIndex(
	outputDir string,
	docs map[string]*ProcessorDocumentation,
	format string,
) error {
	// Sort processors by category and name

	categorized := make(map[string][]*ProcessorDocumentation)
	for _, doc := range docs {
		categorized[doc.Category] = append(categorized[doc.Category], doc)
	}

	var categories []categoryDocs
	for cat, procs := range categorized {
		sort.Slice(procs, func(i, j int) bool {
			return procs[i].Name < procs[j].Name
		})
		categories = append(categories, categoryDocs{
			Category:   cat,
			Processors: procs,
		})
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Category < categories[j].Category
	})

	var content string
	switch format {
	case "markdown":
		content = g.generateMarkdownIndex(categories)
	case "html":
		content = g.generateHTMLIndex(categories)
	default:
		return nil
	}

	filename := filepath.Join(outputDir, fmt.Sprintf("index.%s", g.outputFormats[format].FileExtension()))
	return os.WriteFile(filename, []byte(content), 0600)
}

func (g *DocumentationGenerator) generateMarkdownIndex(categories []categoryDocs) string {
	var buf bytes.Buffer
	buf.WriteString("# Processor Documentation Index\n\n")
	buf.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	for _, cat := range categories {
		buf.WriteString(fmt.Sprintf("## %s\n\n", cat.Category))
		for _, proc := range cat.Processors {
			buf.WriteString(fmt.Sprintf("- [%s](%s.md) - %s\n", proc.Name, proc.Type, proc.Description))
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func (g *DocumentationGenerator) generateHTMLIndex(categories []categoryDocs) string {
	var buf bytes.Buffer
	buf.WriteString("<html><head><title>Processor Documentation Index</title></head><body>\n")
	buf.WriteString("<h1>Processor Documentation Index</h1>\n")
	buf.WriteString(fmt.Sprintf("<p>Generated: %s</p>\n", time.Now().Format(time.RFC3339)))

	for _, cat := range categories {
		buf.WriteString(fmt.Sprintf("<h2>%s</h2>\n<ul>\n", cat.Category))
		for _, proc := range cat.Processors {
			buf.WriteString(fmt.Sprintf("<li><a href=\"%s.html\">%s</a> - %s</li>\n",
				proc.Type, proc.Name, proc.Description))
		}
		buf.WriteString("</ul>\n")
	}

	buf.WriteString("</body></html>")
	return buf.String()
}

func (g *DocumentationGenerator) loadDefaultTemplates() {
	// Markdown template
	markdownTemplate := `# {{.Name}}

**Version**: {{.Version}}
**Author**: {{.Author}}
**Category**: {{.Category}}

## Description

{{.Description}}

## Properties

{{range .Properties}}
### {{.Name}}{{if .Required}} *{{end}}

{{.Description}}

- **Required**: {{if .Required}}Yes{{else}}No{{end}}
- **Sensitive**: {{if .Sensitive}}Yes{{else}}No{{end}}
{{if .DefaultValue}}- **Default**: {{.DefaultValue}}{{end}}
{{if .AllowedValues}}- **Allowed Values**: {{join .AllowedValues ", "}}{{end}}

{{end}}

## Relationships

{{range .Relationships}}
### {{.Name}}

{{.Description}}

{{end}}

---
*Generated: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}*
`

	tmpl, _ := template.New("markdown").Funcs(template.FuncMap{
		"join": strings.Join,
	}).Parse(markdownTemplate)

	g.templates["markdown"] = tmpl
}

// categorizeProcessor determines the category based on tags
func categorizeProcessor(tags []string) string {
	categories := map[string][]string{
		"Data Ingestion":  {"input", "source", "consume", "fetch", "get"},
		"Data Egress":     {"output", "sink", "publish", "send", "put"},
		"Transformation":  {"transform", "convert", "modify", "update"},
		"Routing":         {"route", "split", "merge", "filter"},
		"Database":        {"database", "sql", "query"},
		"Messaging":       {"kafka", "mqtt", "amqp", "jms"},
		"File Operations": {"file", "directory"},
		"HTTP":            {"http", "https", "rest", "api"},
		"Utility":         {"utility", "generate", "log"},
	}

	for _, tag := range tags {
		tagLower := strings.ToLower(tag)
		for category, keywords := range categories {
			for _, keyword := range keywords {
				if strings.Contains(tagLower, keyword) {
					return category
				}
			}
		}
	}

	return "Other"
}

// MarkdownFormatter formats documentation as Markdown
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Format(doc *ProcessorDocumentation) (string, error) {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("# %s\n\n", doc.Name))
	buf.WriteString(fmt.Sprintf("**Version**: %s  \n", doc.Version))
	buf.WriteString(fmt.Sprintf("**Author**: %s  \n", doc.Author))
	buf.WriteString(fmt.Sprintf("**Category**: %s  \n\n", doc.Category))

	buf.WriteString("## Description\n\n")
	buf.WriteString(fmt.Sprintf("%s\n\n", doc.Description))

	if len(doc.Tags) > 0 {
		buf.WriteString("**Tags**: ")
		buf.WriteString(strings.Join(doc.Tags, ", "))
		buf.WriteString("\n\n")
	}

	buf.WriteString("## Properties\n\n")
	for _, prop := range doc.Properties {
		req := ""
		if prop.Required {
			req = " **(Required)**"
		}
		buf.WriteString(fmt.Sprintf("### %s%s\n\n", prop.Name, req))
		buf.WriteString(fmt.Sprintf("%s\n\n", prop.Description))

		if prop.DefaultValue != "" {
			buf.WriteString(fmt.Sprintf("- **Default**: `%s`\n", prop.DefaultValue))
		}
		if len(prop.AllowedValues) > 0 {
			buf.WriteString(fmt.Sprintf("- **Allowed Values**: %s\n", strings.Join(prop.AllowedValues, ", ")))
		}
		if prop.Sensitive {
			buf.WriteString("- **Sensitive**: Yes\n")
		}
		buf.WriteString("\n")
	}

	buf.WriteString("## Relationships\n\n")
	for _, rel := range doc.Relationships {
		buf.WriteString(fmt.Sprintf("### %s\n\n", rel.Name))
		buf.WriteString(fmt.Sprintf("%s\n\n", rel.Description))
	}

	if len(doc.VersionHistory) > 0 {
		buf.WriteString("## Version History\n\n")
		for _, ver := range doc.VersionHistory {
			buf.WriteString(fmt.Sprintf("### %s\n", ver.Version))
			buf.WriteString(fmt.Sprintf("Released: %s\n\n", ver.ReleaseDate.Format("2006-01-02")))
			for _, change := range ver.Changes {
				buf.WriteString(fmt.Sprintf("- %s\n", change))
			}
			buf.WriteString("\n")
		}
	}

	buf.WriteString(fmt.Sprintf("\n---\n*Generated: %s*\n", doc.GeneratedAt.Format("2006-01-02 15:04:05")))

	return buf.String(), nil
}

func (f *MarkdownFormatter) FileExtension() string {
	return "md"
}

// HTMLFormatter formats documentation as HTML
type HTMLFormatter struct{}

func (f *HTMLFormatter) Format(doc *ProcessorDocumentation) (string, error) {
	var buf bytes.Buffer

	buf.WriteString("<html><head><title>" + doc.Name + "</title>")
	buf.WriteString("<style>body{font-family:Arial,sans-serif;max-width:800px;margin:40px auto;padding:20px;}")
	buf.WriteString("h1{color:#333;}h2{color:#666;border-bottom:2px solid #ddd;padding-bottom:5px;}")
	buf.WriteString("h3{color:#888;}code{background:#f4f4f4;padding:2px 5px;border-radius:3px;}</style>")
	buf.WriteString("</head><body>")

	buf.WriteString(fmt.Sprintf("<h1>%s</h1>", doc.Name))
	buf.WriteString(fmt.Sprintf("<p><strong>Version</strong>: %s<br>", doc.Version))
	buf.WriteString(fmt.Sprintf("<strong>Author</strong>: %s<br>", doc.Author))
	buf.WriteString(fmt.Sprintf("<strong>Category</strong>: %s</p>", doc.Category))

	buf.WriteString("<h2>Description</h2>")
	buf.WriteString(fmt.Sprintf("<p>%s</p>", doc.Description))

	buf.WriteString("<h2>Properties</h2>")
	for _, prop := range doc.Properties {
		req := ""
		if prop.Required {
			req = " <strong>(Required)</strong>"
		}
		buf.WriteString(fmt.Sprintf("<h3>%s%s</h3>", prop.Name, req))
		buf.WriteString(fmt.Sprintf("<p>%s</p>", prop.Description))
		if prop.DefaultValue != "" {
			buf.WriteString(fmt.Sprintf("<p><strong>Default</strong>: <code>%s</code></p>", prop.DefaultValue))
		}
	}

	buf.WriteString("<h2>Relationships</h2>")
	for _, rel := range doc.Relationships {
		buf.WriteString(fmt.Sprintf("<h3>%s</h3>", rel.Name))
		buf.WriteString(fmt.Sprintf("<p>%s</p>", rel.Description))
	}

	buf.WriteString(fmt.Sprintf("<hr><p><em>Generated: %s</em></p>", doc.GeneratedAt.Format("2006-01-02 15:04:05")))
	buf.WriteString("</body></html>")

	return buf.String(), nil
}

func (f *HTMLFormatter) FileExtension() string {
	return "html"
}

// JSONFormatter formats documentation as JSON
type JSONFormatter struct{}

func (f *JSONFormatter) Format(doc *ProcessorDocumentation) (string, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f *JSONFormatter) FileExtension() string {
	return "json"
}
