package services

import (
	"context"
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/prompts"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/sse"
	"mooc-manus/pkg/logger"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BaseAgentApplicationService interface {
	Chat(dtos.ChatClientRequest, http.ResponseWriter)
	CreatePlan(dtos.AgentPlanCreateClientRequest, http.ResponseWriter)
	UpdatePlan(dtos.AgentPlanUpdateClientRequest, http.ResponseWriter)
	StopMessage(messageId string) dtos.StopMessageResult
	StopConversation(conversationId string) dtos.StopConversationResult
}

type BaseAgentApplicationServiceImpl struct {
	agentDomainSvc      agents.BaseAgentDomainService
	skillExecutor       tools.SkillExecutor           // 用于 SSE 流结束时清理 skill 容器（D7=A）
	nativeToolsProvider tools.NativeToolsProvider     // 用于 SSE 流结束时清理 NATIVE workspace 目录
	cancelFuncs         map[string]context.CancelFunc // messageId -> context cancel 函数映射，用于停止按钮主动中止对话
	mu                  sync.Mutex
}

func NewBaseAgentApplicationService(
	agentDomainSvc agents.BaseAgentDomainService,
	skillExecutor tools.SkillExecutor,
	nativeToolsProvider tools.NativeToolsProvider,
) BaseAgentApplicationService {
	return &BaseAgentApplicationServiceImpl{
		agentDomainSvc:      agentDomainSvc,
		skillExecutor:       skillExecutor,
		nativeToolsProvider: nativeToolsProvider,
		cancelFuncs:         make(map[string]context.CancelFunc),
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

// cleanupNativeToolsByMessageID 在 SSE 流关闭前清理 NATIVE 工具关联资源
// 与 cleanupSkillByMessageID 并列，消息生命周期结束即回收 fileEdit 写入的工作区文件
func (s *BaseAgentApplicationServiceImpl) cleanupNativeToolsByMessageID(messageId string) {
	if s.nativeToolsProvider == nil || messageId == "" {
		return
	}
	if err := s.nativeToolsProvider.Cleanup(messageId); err != nil {
		logger.Warn("cleanup native tools failed",
			zap.Error(err), zap.String("messageId", messageId))
	}
}

func (s *BaseAgentApplicationServiceImpl) Chat(clientRequest dtos.ChatClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
		logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))
	}
	request := dtos.ConvertChatClientRequest2Request(clientRequest)

	// PlanMode：注入规划提示词 + 断点续跑自动恢复
	if clientRequest.PlanMode && s.nativeToolsProvider != nil {
		planDir := s.nativeToolsProvider.ConversationPlanDir(clientRequest.ConversationId)
		planPrompt := strings.ReplaceAll(prompts.GetPlanModePrompt(), "{{PLAN_DIR}}", planDir)
		request.SystemPrompt = request.SystemPrompt + "\n\n" + planPrompt
		logger.Info("plan mode enabled, injected plan mode prompt",
			zap.String("conversationId", clientRequest.ConversationId),
			zap.String("planDir", planDir),
		)
	}

	messageId := sse.StartChat(writer, clientRequest.ConversationId)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start new chat", zap.String("messageId", messageId))

	// 创建可 cancel 的 context，用于停止按钮和超时兜底
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancelFuncs[messageId] = cancel
	s.mu.Unlock()

	defer func() {
		cancel() // 确保 context 被 cancel
		s.mu.Lock()
		delete(s.cancelFuncs, messageId)
		s.mu.Unlock()
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeToolsByMessageID(messageId)
		sse.CloseChat(messageId)
		logger.Info("close chat", zap.String("messageId", messageId))
	}()

	eventCh := make(chan events.AgentEvent)
	logger.Info("begin chat in domain service")

	// 启动 agent goroutine
	go s.agentDomainSvc.Chat(ctx, request, eventCh)

	// 60s 无事件超时兜底
	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				// eventCh 已关闭，agent 正常结束
				return
			}
			logger.Debug("event from agent", zap.String("type", event.EventType()), zap.Any("data", event))
			event.SaveConversationId(clientRequest.ConversationId)
			sse.SendEvent(event, messageId)
			logger.Debug("send event to http response")
			// 重置超时计时器
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(60 * time.Second)
		case <-timer.C:
			// 60s 无事件，超时兜底
			logger.Warn("chat timeout: no event in 60 seconds", zap.String("messageId", messageId))
			sse.SendEvent(events.OnError("对话超时：60 秒无响应"), messageId)
			cancel() // 通知 agent 中止
			// 等待 eventCh 关闭（agent goroutine 可能还在收尾）
			for range eventCh {
				// 消耗剩余事件避免 goroutine 泄漏
			}
			return
		}
	}
}

func (s *BaseAgentApplicationServiceImpl) CreatePlan(clientRequest dtos.AgentPlanCreateClientRequest, writer http.ResponseWriter) {
	if clientRequest.ConversationId == "" {
		clientRequest.ConversationId = uuid.New().String()
	}
	logger.Info("start new conversation", zap.String("conversationId", clientRequest.ConversationId))

	request := dtos.ConvertPlanCreateClientRequest2DORequest(clientRequest)
	messageId := sse.StartChat(writer, clientRequest.ConversationId)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start create plans", zap.String("messageId", messageId))
	defer func() {
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeToolsByMessageID(messageId)
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
	messageId := sse.StartChat(writer, clientRequest.ConversationId)
	request.MessageId = messageId // 注入 messageId 到 domain 层，用于 Skill 容器隔离
	logger.Info("start update plans", zap.String("messageId", messageId))
	defer func() {
		s.cleanupSkillByMessageID(messageId)
		s.cleanupNativeToolsByMessageID(messageId)
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

// stopMessageInternal 终止单条 messageId 关联的运行时资源
// 三段清理彼此独立，单个失败仅告警，不阻塞剩余步骤
// sse=true 当且仅当调用时该 messageId 仍是活跃 SSE 连接（本次真正切断）
func (s *BaseAgentApplicationServiceImpl) stopMessageInternal(messageId string) dtos.StopMessageCleanDetail {
	detail := dtos.StopMessageCleanDetail{}
	if messageId == "" {
		return detail
	}

	// 0) Context cancel：通知 agent goroutine 中止
	s.mu.Lock()
	if cancel, ok := s.cancelFuncs[messageId]; ok {
		cancel()
		delete(s.cancelFuncs, messageId)
		logger.Info("stop message: context cancelled", zap.String("messageId", messageId))
	}
	s.mu.Unlock()

	// 1) SSE：先判断存在性再关闭；HasMessage 与 CloseChat 各自持锁，短时窗内并发也无害（CloseChat 幂等）
	if sse.HasMessage(messageId) {
		sse.CloseChat(messageId)
		detail.SSE = true
	}

	// 2) Skill 容器与 skills 目录
	if s.skillExecutor != nil {
		if err := s.skillExecutor.CleanupMessage(messageId); err != nil {
			logger.Warn("stop message: cleanup skill failed",
				zap.String("messageId", messageId), zap.Error(err))
		} else {
			detail.Skill = true
		}
	}

	// 3) NATIVE workspace
	if s.nativeToolsProvider != nil {
		if err := s.nativeToolsProvider.Cleanup(messageId); err != nil {
			logger.Warn("stop message: cleanup native workspace failed",
				zap.String("messageId", messageId), zap.Error(err))
		} else {
			detail.NativeWorkspace = true
		}
	}

	return detail
}

// StopMessage 终止指定 messageId 的流式对话
// 幂等：messageId 未知时返回 cleaned 全 false 的 200 响应
func (s *BaseAgentApplicationServiceImpl) StopMessage(messageId string) dtos.StopMessageResult {
	logger.Info("stop message requested", zap.String("messageId", messageId))
	detail := s.stopMessageInternal(messageId)
	logger.Info("stop message completed",
		zap.String("messageId", messageId),
		zap.Any("cleaned", detail))
	return dtos.StopMessageResult{
		MessageId: messageId,
		Cleaned:   detail,
	}
}

// StopConversation 终止整个会话的运行时资源
// 步骤：SSE manager 拿活跃 messageIds → 逐个走 stopMessageInternal → 删 chat memory
// 不清 planDir：与 PlanMode 断点续跑语义一致
func (s *BaseAgentApplicationServiceImpl) StopConversation(conversationId string) dtos.StopConversationResult {
	logger.Info("stop conversation requested", zap.String("conversationId", conversationId))
	result := dtos.StopConversationResult{
		ConversationId: conversationId,
		Cleaned:        dtos.StopConversationCleanDetail{Messages: []string{}},
	}
	if conversationId == "" {
		return result
	}

	messageIds := sse.MessageIdsOf(conversationId)
	for _, mid := range messageIds {
		s.stopMessageInternal(mid)
		result.Cleaned.Messages = append(result.Cleaned.Messages, mid)
	}

	// memory.DeleteMemory 内部对 conversationId 不存在场景 no-op
	// 这里通过 goroutine-safe 的方式：deleted 语义上代表"至少调用过一次删除"，
	// 目前实现没暴露"是否真的删过"的返回值，先按 true 上报；如需精确可下钻改 Memory API
	memory.DeleteMemory(conversationId)
	result.Cleaned.Memory = true

	logger.Info("stop conversation completed",
		zap.String("conversationId", conversationId),
		zap.Int("messages", len(messageIds)))
	return result
}
