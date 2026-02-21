package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/services/flows"
	"mooc-manus/internal/infra/external/sse"
	"mooc-manus/pkg/logger"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BaseFlowApplicationService interface {
	Run(dtos.ChatClientRequest, http.ResponseWriter)
}

type BaseFlowApplicationServiceImpl struct {
	flowDomainSvc flows.BaseFlowDomainService
}

func NewFlowApplicationService(flowDomainSvc flows.BaseFlowDomainService) BaseFlowApplicationService {
	return &BaseFlowApplicationServiceImpl{flowDomainSvc: flowDomainSvc}
}

func (s *BaseFlowApplicationServiceImpl) Run(clientRequest dtos.ChatClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
		logger.Info("start new flow run", zap.String("conversationId", clientRequest.ConversationId))
	}
	request := dtos.ConvertChatClientRequest2Request(clientRequest)
	messageId := sse.StartChat(writer)
	logger.Info("start flow run instance", zap.String("messageId", messageId))
	defer func() {
		logger.Info("stop flow run instance", zap.String("messageId", messageId))
		sse.CloseChat(messageId)
	}()

	flowEventCh := make(chan events.AgentEvent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		s.flowDomainSvc.Run(request, flowEventCh)
		wg.Done()
	}()
	for event := range flowEventCh {
		event.SaveConversationId(clientRequest.ConversationId)
		sse.SendEvent(event, messageId)
	}
	wg.Wait()
}
