package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/infra/external/sse"
	"mooc-manus/pkg/logger"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type A2AApplicationService interface {
	A2AChat(dtos.ChatClientRequest, http.ResponseWriter)
}

type A2AApplicationServiceImpl struct {
	a2aDomainSvc agents.A2ADomainService
}

func NewA2AApplicationService(a2aDomainSvc agents.A2ADomainService) A2AApplicationService {
	return &A2AApplicationServiceImpl{
		a2aDomainSvc: a2aDomainSvc,
	}
}

func (s *A2AApplicationServiceImpl) A2AChat(clientRequest dtos.ChatClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
	}
	request := dtos.ConvertChatClientRequest2Request(clientRequest)
	messageId := sse.StartChat(writer)
	logger.Info("start new  a2a chat", zap.String("messageId", messageId))
	defer func() {
		sse.CloseChat(messageId)
		logger.Info("close a2a chat", zap.String("messageId", messageId))
	}()

	eventCh := make(chan events.AgentEvent)
	logger.Info("begin a2a chat in domain service")
	s.a2aDomainSvc.A2AChat(request, eventCh)
	for event := range eventCh {
		logger.Debug("event from agent", zap.String("type", event.EventType()), zap.Any("data", event))
		event.SaveConversationId(clientRequest.ConversationId)
		sse.SendEvent(event, messageId)
	}
}
