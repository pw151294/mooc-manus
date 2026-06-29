package services

import (
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/sse"
	"mooc-manus/pkg/logger"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BaseAgentApplicationService interface {
	Chat(dtos.ChatClientRequest, http.ResponseWriter)
	CreatePlan(dtos.AgentPlanCreateClientRequest, http.ResponseWriter)
	UpdatePlan(dtos.AgentPlanUpdateClientRequest, http.ResponseWriter)
}

type BaseAgentApplicationServiceImpl struct {
	agentDomainSvc  agents.BaseAgentDomainService
	skillExecutor   tools.SkillExecutor      // 用于 SSE 流结束时清理 skill 容器（D7=A）
	nativeWorkspace *tools.NativeWorkspace   // 用于 SSE 流结束时清理 NATIVE workspace 目录
}

func NewBaseAgentApplicationService(
	agentDomainSvc agents.BaseAgentDomainService,
	skillExecutor tools.SkillExecutor,
	nativeWorkspace *tools.NativeWorkspace,
) BaseAgentApplicationService {
	return &BaseAgentApplicationServiceImpl{
		agentDomainSvc:  agentDomainSvc,
		skillExecutor:   skillExecutor,
		nativeWorkspace: nativeWorkspace,
	}
}

// cleanupSkillByMessageID 在 SSE 流关闭前清理 skill 容器与工作目录
// 与 sse.CloseChat 配对，确保容器和消息生命周期对齐
func (s *BaseAgentApplicationServiceImpl) cleanupSkillByMessageID(messageId string) {
	if s.skillExecutor == nil || messageId == "" {
		return
	}
	if err := s.skillExecutor.CleanupMessage(messageId); err != nil {
		logger.Warn("cleanup skill executor failed",
			zap.Error(err), zap.String("messageId", messageId))
	}
}

// cleanupNativeWorkspaceByMessageID 在 SSE 流关闭前清理 NATIVE 工具的 workspace 目录
// 与 cleanupSkillByMessageID 并列，消息生命周期结束即回收 fileEdit 写入的临时文件
func (s *BaseAgentApplicationServiceImpl) cleanupNativeWorkspaceByMessageID(messageId string) {
	if s.nativeWorkspace == nil || messageId == "" {
		return
	}
	if err := s.nativeWorkspace.Cleanup(messageId); err != nil {
		logger.Warn("cleanup native workspace failed",
			zap.Error(err), zap.String("messageId", messageId))
	}
}

func (s *BaseAgentApplicationServiceImpl) Chat(clientRequest dtos.ChatClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
		logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))
	}
	request := dtos.ConvertChatClientRequest2Request(clientRequest)
	messageId := sse.StartChat(writer)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start new chat", zap.String("messageId", messageId))
	defer func() {
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeWorkspaceByMessageID(messageId)
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

	request := dtos.ConvertPlanCreateClientRequest2DORequest(clientRequest)
	messageId := sse.StartChat(writer)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start create plans", zap.String("messageId", messageId))
	defer func() {
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeWorkspaceByMessageID(messageId)
		sse.CloseChat(messageId)
		logger.Info("end create plans", zap.String("messageId", messageId))
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	eventCh := make(chan events.AgentEvent)
	logger.Info("begin create plans in domain service")
	go func() {
		s.agentDomainSvc.CreatePlan(request, eventCh)
		wg.Done()
	}()

	for event := range eventCh {
		event.SaveConversationId(clientRequest.ConversationId)
		switch event.EventType() {
		case events.EventTypePlanCreateSuccess:
			// todo 计划创建成功
			planEvent := event.(*events.PlanEvent)
			logger.Info("create plans success", zap.Any("plans", planEvent.Plan))
		case events.EventTypeError:
			logger.Info("create plans failed", zap.Any("data", event))
		// todo 计划创建失败
		default:
			logger.Info("receive event during plans creating", zap.String("type", event.EventType()), zap.Any("data", event))
			// todo 计划创建期间上报的事件
		}
		sse.SendEvent(event, messageId)
	}
}

func (s *BaseAgentApplicationServiceImpl) UpdatePlan(clientRequest dtos.AgentPlanUpdateClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
	}
	logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))

	request := dtos.ConvertPlanUpdateClientRequest2DORequest(clientRequest)
	messageId := sse.StartChat(writer)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start update plans", zap.String("messageId", messageId))
	defer func() {
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeWorkspaceByMessageID(messageId)
		sse.CloseChat(messageId)
		logger.Info("end update plans", zap.String("messageId", messageId))
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	eventCh := make(chan events.AgentEvent)
	logger.Info("begin update plans in domain service")
	go func() {
		s.agentDomainSvc.UpdatePlan(request, eventCh)
		wg.Done()
	}()

	for event := range eventCh {
		event.SaveConversationId(clientRequest.ConversationId)
		switch event.EventType() {
		case events.EventTypePlanUpdateSuccess:
			planEvent := event.(*events.PlanEvent)
			logger.Info("update plans success", zap.Any("plans", planEvent.Plan))
		case events.EventTypePlanUpdateFailed:
			planEvent := event.(*events.PlanEvent)
			logger.Info("update plans failed", zap.Any("plans", planEvent.Plan))
		case events.EventTypeError:
			errorEvent := event.(*events.ErrorEvent)
			logger.Info("update plans error", zap.Any("error", errorEvent.Error))
		default:
			logger.Info("receive event during plans updating", zap.Any("data", event))
		}
		sse.SendEvent(event, messageId)
	}
}
