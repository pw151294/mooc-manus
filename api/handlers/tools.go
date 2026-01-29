package handlers

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ToolHandler struct {
	providerAppSvc services.ToolProviderApplicationService
	functionAppSvc services.ToolFunctionApplicationService
}

func NewToolHandler(providerAppSvc services.ToolProviderApplicationService, functionAppSvc services.ToolFunctionApplicationService) *ToolHandler {
	return &ToolHandler{providerAppSvc: providerAppSvc, functionAppSvc: functionAppSvc}
}

func (h *ToolHandler) AddProvider(c *gin.Context) {
	var req dtos.AddToolProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.providerAppSvc.Add(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *ToolHandler) UpdateProvider(c *gin.Context) {
	var req dtos.UpdateToolProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ProviderID = c.Param("id")
	if err := h.providerAppSvc.Update(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *ToolHandler) DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if err := h.providerAppSvc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *ToolHandler) ListProviders(c *gin.Context) {
	providers, err := h.providerAppSvc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, providers)
}

func (h *ToolHandler) AddFunction(c *gin.Context) {
	var req dtos.AddToolFunctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.providerAppSvc.Exist(req.ProviderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
		return
	}
	if err := h.functionAppSvc.Add(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *ToolHandler) UpdateFunction(c *gin.Context) {
	var req dtos.UpdateToolFunctionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.FunctionID = c.Param("id")
	if !h.providerAppSvc.Exist(req.ProviderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
		return
	}
	if err := h.functionAppSvc.Update(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// DeleteFunction 根据ID删除Function
func (h *ToolHandler) DeleteFunction(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	h.functionAppSvc.DeleteById(id)
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// ListFunctionsByProvider 根据providerId查询对应的function列表
func (h *ToolHandler) ListFunctionsByProvider(c *gin.Context) {
	providerId := c.Query("providerId")
	if providerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "providerId is required"})
		return
	}
	functions, err := h.functionAppSvc.ListByProviderId(providerId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, functions)
}

// AddMcpFunctions 新增mcp工具
func (h *ToolHandler) AddMcpFunctions(c *gin.Context) {
	request := dtos.AddMcpFunctionsRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.functionAppSvc.AddMcpFunctions(request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
