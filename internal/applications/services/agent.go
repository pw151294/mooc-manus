package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/infra/sse"
	"mooc-manus/pkg/logger"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BaseAgentApplicationService interface {
	Chat(dtos.AgentChatClientRequest, http.ResponseWriter)
}

type BaseAgentApplicationServiceImpl struct {
	agentDomainSvc agents.BaseAgentDomainService
}

func NewBaseAgentApplicationService(agentDomainSvc agents.BaseAgentDomainService) BaseAgentApplicationService {
	return &BaseAgentApplicationServiceImpl{
		agentDomainSvc: agentDomainSvc,
	}
}

func (s *BaseAgentApplicationServiceImpl) Chat(clientRequest dtos.AgentChatClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
		logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))
	}
	request := dtos.ConvertAgentChatClientRequest2Request(clientRequest)
	messageId := sse.StartChat(writer)
	logger.Info("start new chat", zap.String("messageId", messageId))
	defer func() {
		sse.CloseChat(messageId)
		logger.Info("close chat", zap.String("messageId", messageId))
	}()

	eventCh := make(chan events.AgentEvent)
	logger.Info("begin chat in domain service")
	s.agentDomainSvc.Chat(request, eventCh)
	for event := range eventCh {
		logger.Debug("event from agent", zap.String("type", event.EventType()), zap.Any("data", event))
		event.SaveConversationId(clientRequest.ConversationId)
		sse.SendEvent(event, messageId)
		logger.Debug("send event to http response")
	}
}
