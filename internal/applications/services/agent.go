package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/infra/sse"
	"mooc-manus/pkg/logger"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BaseAgentApplicationService interface {
	Chat(dtos.AgentChatClientRequest, http.ResponseWriter)
	CreatePlan(dtos.AgentPlanCreateClientRequest, http.ResponseWriter)
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

func (s *BaseAgentApplicationServiceImpl) CreatePlan(clientRequest dtos.AgentPlanCreateClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
	}
	logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))

	request := dtos.ConvertAgentPlanCreateClientRequest2DORequest(clientRequest)
	messageId := sse.StartChat(writer)
	logger.Info("start create plan", zap.String("messageId", messageId))
	defer func() {
		sse.CloseChat(messageId)
		logger.Info("end create plan", zap.String("messageId", messageId))
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	eventCh := make(chan events.AgentEvent)
	logger.Info("begin create plan in domain service")
	go func() {
		s.agentDomainSvc.CreatePlan(request, eventCh)
		wg.Done()
	}()

	for event := range eventCh {
		switch event.EventType() {
		case events.EventTypePlanCreateSuccess:
			// todo 计划创建成功
			planEvent := event.(*events.PlanEvent)
			logger.Info("create plan success", zap.Any("plan", planEvent.Plan))
		case events.EventTypeError:
			logger.Info("create plan failed", zap.Any("data", event))
		// todo 计划创建失败
		default:
			logger.Info("receive event during plan creating", zap.String("type", event.EventType()), zap.Any("data", event))
			// todo 计划创建期间上报的事件
		}
		sse.SendEvent(event, messageId)
	}
}
