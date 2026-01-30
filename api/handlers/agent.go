package handlers

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	baseAgentAppSvc services.BaseAgentApplicationService
}

func NewAgentHandler(baseAgentAppSvc services.BaseAgentApplicationService) *AgentHandler {
	return &AgentHandler{
		baseAgentAppSvc: baseAgentAppSvc,
	}
}

func (h *AgentHandler) Chat(c *gin.Context) {
	clientRequest := dtos.AgentChatClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	h.baseAgentAppSvc.Chat(clientRequest, c.Writer)
}

func (h *AgentHandler) CreatePlan(c *gin.Context) {
	clientRequest := dtos.AgentPlanCreateClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	h.baseAgentAppSvc.CreatePlan(clientRequest, c.Writer)
}

func (h *AgentHandler) UpdatePlan(c *gin.Context) {
	clientRequest := dtos.AgentPlanUpdateClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	h.baseAgentAppSvc.UpdatePlan(clientRequest, c.Writer)
}
