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
	//clientRequest.SystemPrompt = prompts.GetSRESystemPrompt()
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

func (h *AgentHandler) StopMessage(c *gin.Context) {
	clientRequest := dtos.StopMessageClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := h.baseAgentAppSvc.StopMessage(clientRequest.MessageId)
	c.JSON(http.StatusOK, result)
}

func (h *AgentHandler) StopConversation(c *gin.Context) {
	clientRequest := dtos.StopConversationClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := h.baseAgentAppSvc.StopConversation(clientRequest.ConversationId)
	c.JSON(http.StatusOK, result)
}

// Resume 处理 HITL 决策回投
// 200 accepted / 409 already_decided / 404 not_found / 400 参数错误
func (h *AgentHandler) Resume(c *gin.Context) {
	clientRequest := dtos.ResumeClientRequest{}
	if err := c.ShouldBindJSON(&clientRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result := h.baseAgentAppSvc.Resume(clientRequest)
	switch result.Status {
	case "accepted":
		c.JSON(http.StatusOK, result)
	case "already_decided":
		c.JSON(http.StatusConflict, result)
	case "not_found":
		c.JSON(http.StatusNotFound, result)
	default:
		c.JSON(http.StatusInternalServerError, result)
	}
}
