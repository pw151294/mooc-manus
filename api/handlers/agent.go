package handlers

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/applications/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	baseAgentAppSvc services.BaseAgentApplicationService
	a2aAppSvc       services.A2AApplicationService
}

func NewAgentHandler(baseAgentAppSvc services.BaseAgentApplicationService,
	a2aAppSvc services.A2AApplicationService) *AgentHandler {
	return &AgentHandler{
		baseAgentAppSvc: baseAgentAppSvc,
		a2aAppSvc:       a2aAppSvc,
	}
}

func (h *AgentHandler) Chat(c *gin.Context) {
	clientRequest := dtos.ChatClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.baseAgentAppSvc.Chat(clientRequest, c.Writer)
}

func (h *AgentHandler) A2AChat(c *gin.Context) {
	clientRequest := dtos.ChatClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.a2aAppSvc.A2AChat(clientRequest, c.Writer)
}

func (h *AgentHandler) CreatePlan(c *gin.Context) {
	clientRequest := dtos.AgentPlanCreateClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.baseAgentAppSvc.CreatePlan(clientRequest, c.Writer)
}

func (h *AgentHandler) UpdatePlan(c *gin.Context) {
	clientRequest := dtos.AgentPlanUpdateClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.baseAgentAppSvc.UpdatePlan(clientRequest, c.Writer)
}
