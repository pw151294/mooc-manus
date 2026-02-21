package handlers

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type FlowHandler struct {
	baseFlowAppSvc services.BaseFlowApplicationService
}

func NewFlowHandler(baseFlowAppSvc services.BaseFlowApplicationService) *FlowHandler {
	return &FlowHandler{
		baseFlowAppSvc: baseFlowAppSvc,
	}
}

func (h *FlowHandler) Run(c *gin.Context) {
	clientRequest := dtos.ChatClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	h.baseFlowAppSvc.Run(clientRequest, c.Writer)
}
