package handlers

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AppConfigHandler struct {
	appConfigAppSvc services.AppConfigApplicationService
}

func NewAppConfigHandler(appConfigAppSvc services.AppConfigApplicationService) *AppConfigHandler {
	return &AppConfigHandler{appConfigAppSvc: appConfigAppSvc}
}

func (h *AppConfigHandler) Add(c *gin.Context) {
	var req dtos.AppConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := h.appConfigAppSvc.CreateAppConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *AppConfigHandler) Update(c *gin.Context) {
	var req dtos.AppConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从 URL 路径中获取 id
	req.AppConfigID = c.Param("id")

	err := h.appConfigAppSvc.UpdateAppConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *AppConfigHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	dto, err := h.appConfigAppSvc.LoadAppConfig(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto)
}

func (h *AppConfigHandler) List(c *gin.Context) {
	dtos, err := h.appConfigAppSvc.GetAllAppConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dtos)
}

func (h *AppConfigHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	err := h.appConfigAppSvc.DeleteAppConfig(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
