package evaluation

import (
	"context"
	"errors"
	"time"

	"mooc-manus/internal/domains/models/agents"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/tracing"
	domagents "mooc-manus/internal/domains/services/agents"
)

// InternalChatReq 评测系统内部触发的单次 Chat 请求。
// 通过 Snapshot 冻结 AppConfig，驱动 BaseAgent 独立执行一次对话（不再受源 AppConfig 修改影响）。
type InternalChatReq struct {
	Snapshot       *ev.AgentSnapshot
	ConversationID string
	MessageID      string
	Query          string
	TotalTimeout   time.Duration
}

// InternalChatResult 单次 Chat 的收敛结果。
// spec §3.5：错误（ErrorEvent / 智能体自身报错）通过 Error 返回；超时通过 DidTimeout 标识。
type InternalChatResult struct {
	Error            error  // 智能体流内 ErrorEvent 汇总（若发生）
	LastAssistantMsg string // 最后一条 assistant 消息文本（用于评测）
	DidTimeout       bool   // 是否因 TotalTimeout 而中断
}

// InternalChatRunner 评测系统内部对 BaseAgent 的一次性 Chat 收敛封装。
// 相比生产链路的 sse.StartChat：不做 SSE 推送，直接同步等待事件流关闭。
type InternalChatRunner interface {
	Run(ctx context.Context, req InternalChatReq) (InternalChatResult, error)
}

type internalChatRunnerImpl struct {
	baseAgent domagents.BaseAgentDomainService
}

// NewInternalChatRunner 构造评测专用 Chat 执行器；
// 复用生产 BaseAgentDomainService.Chat，保持行为一致。
func NewInternalChatRunner(baseAgent domagents.BaseAgentDomainService) InternalChatRunner {
	return &internalChatRunnerImpl{baseAgent: baseAgent}
}

// startEvalRootSpan 为评测链路开启 AGENT_ROOT span 并注入 ctx。
func startEvalRootSpan(ctx context.Context, req InternalChatReq) (context.Context, *tracing.Span) {
	tracer := tracing.Global()
	if tracer == nil {
		return ctx, nil
	}
	ctx, root := tracer.StartRootSpan(ctx, req.MessageID)
	root.SetConversationID(req.ConversationID)
	root.SetTag("user.query", req.Query)
	if req.Snapshot != nil {
		root.SetTag("evaluation.source_app_config_id", req.Snapshot.SourceAppConfigID)
	}
	return ctx, root
}

func (r *internalChatRunnerImpl) Run(ctx context.Context, req InternalChatReq) (InternalChatResult, error) {
	if req.Snapshot == nil {
		return InternalChatResult{}, errors.New("snapshot 不能为空")
	}

	// 评测收敛：总超时优先由调用方传入；未传时保底给一个较宽松值避免永久阻塞。
	timeout := req.TotalTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 开启评测链路 root span：traceID 复用 messageID，保持与生产 Chat 同构。
	// tracer 未初始化时 root 为 nil，后续调用都走 nil-safe 分支。
	ctx2, rootSpan := startEvalRootSpan(ctx2, req)
	defer rootSpan.End()

	// 事件通道由本函数创建，BaseAgent.Chat 内部负责关闭（生产链路契约一致）
	eventCh := make(chan events.AgentEvent, 64)
	override := req.Snapshot.ToAppConfig()
	chatReq := agents.ChatRequest{
		Streaming:      true,
		SystemPrompt:   req.Snapshot.SystemPrompt,
		ConversationId: req.ConversationID,
		MessageId:      req.MessageID,
		Query:          req.Query,
		AppConfigId:    req.Snapshot.SourceAppConfigID,
		ConfigOverride: override,
	}

	// BaseAgent.Chat 是同步方法，内部会 close(eventCh)；用 goroutine 启动使主循环可 select 超时。
	go r.baseAgent.Chat(ctx2, chatReq, eventCh)

	var lastMsg string
	var errFromStream error
	for {
		select {
		case <-ctx2.Done():
			// 超时/取消：不再阻塞等待 chan drain（BaseAgent 内部 goroutine 因 ctx 取消会自行退出并 close chan）
			didTimeout := errors.Is(ctx2.Err(), context.DeadlineExceeded)
			if didTimeout {
				rootSpan.AddLog("ERROR", "eval.timeout", map[string]interface{}{
					"timeout_ms": timeout.Milliseconds(),
				})
				rootSpan.MarkError()
			}
			return InternalChatResult{
				DidTimeout:       didTimeout,
				LastAssistantMsg: lastMsg,
				Error:            errFromStream,
			}, nil
		case e, ok := <-eventCh:
			if !ok {
				// 事件流关闭，正常收敛
				return InternalChatResult{
					Error:            errFromStream,
					LastAssistantMsg: lastMsg,
				}, nil
			}
			switch v := e.(type) {
			case *events.MessageEvent:
				if v.Role == "assistant" && v.Message != "" {
					lastMsg = v.Message
				}
			case *events.ErrorEvent:
				// 记录第一条错误即可；后续错误保持首个非空原因方便定位
				if errFromStream == nil {
					errFromStream = errors.New(v.Error)
					rootSpan.AddLog("ERROR", "eval.stream_error", map[string]interface{}{"error": v.Error})
					rootSpan.MarkError()
				}
			}
		}
	}
}
