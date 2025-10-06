package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shawntherrien/databridge/internal/core"
)

// TemplateHandler handles flow template API requests
type TemplateHandler struct {
	templateManager *core.FlowTemplateManager
}

// NewTemplateHandler creates a new template handler
func NewTemplateHandler(templateManager *core.FlowTemplateManager) *TemplateHandler {
	return &TemplateHandler{
		templateManager: templateManager,
	}
}

// CreateTemplateRequest represents a template creation request
type CreateTemplateRequest struct {
	Name         string      `json:"name" binding:"required"`
	Description  string      `json:"description"`
	Author       string      `json:"author"`
	ProcessorIDs []uuid.UUID `json:"processorIds" binding:"required"`
	Tags         []string    `json:"tags"`
}

// InstantiateTemplateRequest represents a template instantiation request
type InstantiateTemplateRequest struct {
	Variables map[string]string `json:"variables"`
}

// ExportFlowRequest represents a flow export request
type ExportFlowRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Format      string `json:"format"` // JSON, YAML
}

// RegisterRoutes registers template API routes
func (h *TemplateHandler) RegisterRoutes(router *gin.RouterGroup) {
	templates := router.Group("/templates")
	{
		templates.POST("", h.CreateTemplate)
		templates.GET("", h.ListTemplates)
		templates.GET("/:id", h.GetTemplate)
		templates.PUT("/:id", h.UpdateTemplate)
		templates.DELETE("/:id", h.DeleteTemplate)
		templates.POST("/:id/instantiate", h.InstantiateTemplate)
		templates.POST("/:id/save", h.SaveTemplate)
		templates.GET("/search", h.SearchTemplates)
	}

	// Flow import/export endpoints
	flow := router.Group("/flow")
	{
		flow.POST("/export", h.ExportFlow)
		flow.POST("/import", h.ImportFlow)
	}
}

// CreateTemplate creates a new flow template
// @Summary Create flow template
// @Tags Templates
// @Accept json
// @Produce json
// @Param request body CreateTemplateRequest true "Template creation request"
// @Success 201 {object} core.FlowTemplate
// @Router /templates [post]
func (h *TemplateHandler) CreateTemplate(c *gin.Context) {
	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template, err := h.templateManager.CreateTemplate(
		req.Name,
		req.Description,
		req.Author,
		req.ProcessorIDs,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update tags if provided
	if len(req.Tags) > 0 {
		template.Tags = req.Tags
	}

	c.JSON(http.StatusCreated, template)
}

// ListTemplates lists all available templates
// @Summary List templates
// @Tags Templates
// @Produce json
// @Success 200 {array} core.FlowTemplate
// @Router /templates [get]
func (h *TemplateHandler) ListTemplates(c *gin.Context) {
	templates := h.templateManager.ListTemplates()
	c.JSON(http.StatusOK, templates)
}

// GetTemplate retrieves a specific template
// @Summary Get template
// @Tags Templates
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} core.FlowTemplate
// @Router /templates/{id} [get]
func (h *TemplateHandler) GetTemplate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	template, err := h.templateManager.GetTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, template)
}

// UpdateTemplate updates a template
// @Summary Update template
// @Tags Templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param template body core.FlowTemplate true "Updated template"
// @Success 200 {object} core.FlowTemplate
// @Router /templates/{id} [put]
func (h *TemplateHandler) UpdateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	var updates core.FlowTemplate
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.templateManager.UpdateTemplate(templateID, &updates); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	template, _ := h.templateManager.GetTemplate(templateID)
	c.JSON(http.StatusOK, template)
}

// DeleteTemplate deletes a template
// @Summary Delete template
// @Tags Templates
// @Param id path string true "Template ID"
// @Success 204
// @Router /templates/{id} [delete]
func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	if err := h.templateManager.DeleteTemplate(templateID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// InstantiateTemplate creates a new flow from a template
// @Summary Instantiate template
// @Tags Templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param request body InstantiateTemplateRequest true "Instantiation request"
// @Success 201 {object} map[string]interface{}
// @Router /templates/{id}/instantiate [post]
func (h *TemplateHandler) InstantiateTemplate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	var req InstantiateTemplateRequest
	if bindErr := c.ShouldBindJSON(&req); bindErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": bindErr.Error()})
		return
	}

	processorIDs, err := h.templateManager.InstantiateTemplate(templateID, req.Variables)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"processorIds": processorIDs,
		"count":        len(processorIDs),
	})
}

// SaveTemplate saves a template to disk
// @Summary Save template
// @Tags Templates
// @Param id path string true "Template ID"
// @Success 200
// @Router /templates/{id}/save [post]
func (h *TemplateHandler) SaveTemplate(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	if err := h.templateManager.SaveTemplate(templateID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "template saved successfully"})
}

// SearchTemplates searches for templates
// @Summary Search templates
// @Tags Templates
// @Produce json
// @Param q query string true "Search query"
// @Success 200 {array} core.FlowTemplate
// @Router /templates/search [get]
func (h *TemplateHandler) SearchTemplates(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query required"})
		return
	}

	templates := h.templateManager.SearchTemplates(query)
	c.JSON(http.StatusOK, templates)
}

// ExportFlow exports the entire flow configuration
// @Summary Export flow
// @Tags Flow
// @Accept json
// @Produce json
// @Param request body ExportFlowRequest true "Export request"
// @Success 200 {object} core.FlowExport
// @Router /flow/export [post]
func (h *TemplateHandler) ExportFlow(c *gin.Context) {
	var req ExportFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	format := core.ExportFormatJSON
	if req.Format == "YAML" {
		format = core.ExportFormatYAML
	}

	export, err := h.templateManager.ExportFlow(req.Name, req.Description, req.Author, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, export)
}

// ImportFlow imports a flow configuration
// @Summary Import flow
// @Accept json
// @Produce json
// @Param export body core.FlowExport true "Flow export data"
// @Success 201 {object} map[string]interface{}
// @Router /flow/import [post]
func (h *TemplateHandler) ImportFlow(c *gin.Context) {
	var export core.FlowExport
	if err := c.ShouldBindJSON(&export); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processorIDs, err := h.templateManager.ImportFlow(&export)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"processorIds": processorIDs,
		"count":        len(processorIDs),
		"message":      "flow imported successfully",
	})
}

// ExportTemplateToFile exports a template to a downloadable file
func (h *TemplateHandler) ExportTemplateToFile(c *gin.Context) {
	idStr := c.Param("id")
	templateID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template ID"})
		return
	}

	template, err := h.templateManager.GetTemplate(templateID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+template.Name+".json")
	c.Data(http.StatusOK, "application/json", data)
}
