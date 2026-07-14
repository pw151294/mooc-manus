package agents

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaptinlin/jsonrepair"
	"go.uber.org/zap"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/circuitbreaker"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/interrupt"
	"mooc-manus/internal/domains/models/invoker"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/pkg/logger"
)

type BaseAgent struct {
	name           string
	systemPrompt   string
	retryInterval  int
	agentConfig    models.AgentConfig
	invoker        invoker.Invoker
	memory         *memory.ChatMemory
	tools          []tools.Tool
	circuitBreaker *circuitbreaker.ToolCallCounter
	pendingSink    interrupt.PendingSink // HITL 审批管理器
	messageId      string                // HITL 用于 RegisterInterrupt 定位 slot
}

// BaseAgentOption 为 NewBaseAgent 的可选参数
type BaseAgentOption func(*BaseAgent)

// WithPendingSink 注入 HITL 审批管理器
func WithPendingSink(sink interrupt.PendingSink) BaseAgentOption {
	return func(a *BaseAgent) { a.pendingSink = sink }
}

// WithMessageId 注入 messageId，供 HITL Register 使用
func WithMessageId(mid string) BaseAgentOption {
	return func(a *BaseAgent) { a.messageId = mid }
}

func NewBaseAgent(agentConfig models.AgentConfig, inv invoker.Invoker, mem *memory.ChatMemory, ts []tools.Tool, systemPrompt string, opts ...BaseAgentOption) *BaseAgent {
	a := &BaseAgent{
		agentConfig:    agentConfig,
		invoker:        inv,
		memory:         mem,
		tools:          ts,
		systemPrompt:   systemPrompt,
		retryInterval:  5,
		circuitBreaker: circuitbreaker.NewToolCallCounter(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *BaseAgent) GetAvailableTools() []llm.Tool {
	out := make([]llm.Tool, 0)
	for _, t := range a.tools {
		out = append(out, t.GetTools()...)
	}
	return out
}

func (a *BaseAgent) GetMessages() []llm.Message {
	return a.memory.GetMessages()
}

func (a *BaseAgent) GetTool(functionName string) tools.Tool {
	for _, tool := range a.tools {
		if tool.HasTool(functionName) {
			return tool
		}
	}
	return nil
}

func (a *BaseAgent) AddToMemory(messages []llm.Message) {
	if a.memory.IsEmpty() {
		a.memory.AddMessage(llm.Message{Role: llm.RoleSystem, Content: a.systemPrompt})
	}
	a.memory.AddMessages(messages)
}

// injectEventChToSubagent 将 eventCh 注入到 SubagentTool（如果存在）
// 用于支持子智能体事件透传到主智能体事件流
func (a *BaseAgent) injectEventChToSubagent(eventCh chan events.AgentEvent) {
	for _, tool := range a.tools {
		if subagentTool, ok := tool.(*tools.SubagentTool); ok {
			subagentTool.SetParentEventCh(eventCh)
			logger.Info("injected eventCh to SubagentTool")
			return
		}
	}
}

// InvokeToolCalls 执行工具调用并采集ToolMessage 注：该方法不管在流式还是非流式的场景下都是阻塞调用的
func (a *BaseAgent) InvokeToolCalls(ctx context.Context, toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message {
	ctx, batchSpan := a.startToolBatchSpan(ctx, len(toolCalls))
	defer batchSpan.End()
	successCount := 0
	failCount := 0
	defer func() {
		a.finalizeToolBatchSpan(batchSpan, successCount, failCount)
	}()

	toolMessages := make([]llm.Message, 0, len(toolCalls))
	currentRoundKeys := make([]string, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		// 检查 context 是否已被 cancel（停止按钮 / 超时兜底）
		select {
		case <-ctx.Done():
			logger.Info("InvokeToolCalls cancelled by context", zap.Error(ctx.Err()))
			return toolMessages
		default:
		}

		// abort：需要外层 return toolMessages 的信号（HITL 拒绝/超时/取消 / RegisterInterrupt 冲突 / ctx.Done）
		abort := false

		func() {
			toolSpan := a.startToolCallSpan(ctx, toolCall)
			defer toolSpan.End()

			toolCallID := toolCall.ID
			funcName := toolCall.Name
			funcArgs := toolCall.Arguments
			// 使用jsonrepair修复funcArgs
			repairedArgs, err := jsonrepair.JSONRepair(funcArgs)
			if err != nil {
				logger.Error("repair tool call args failed", zap.Error(err), zap.String("function args", funcArgs))
				errMsg := fmt.Sprintf("工具调用参数不符合规范，修复失败：%v", err)
				toolMessages = append(toolMessages, llm.Message{
					Role:       llm.RoleTool,
					Content:    errMsg,
					ToolCallID: toolCallID,
				})
				result := models.ToolCallResult{
					Success: false,
					Message: errMsg,
				}
				// 修复失败，发送失败事件并继续
				eventCh <- events.OnToolCallFail(toolCall, "", &result)
				a.recordToolRepairFailed(toolSpan, err, errMsg)
				failCount++
				return
			}
			funcArgs = repairedArgs // 使用修复后的参数
			// 熔断机制：生成工具调用 Key，用于失败计数
			key, err := circuitbreaker.GenerateKey(funcName, funcArgs)
			if err != nil {
				logger.Warn("生成工具调用 Key 失败，跳过计数",
					zap.String("tool", funcName),
					zap.Error(err))
				key = ""
			} else {
				currentRoundKeys = append(currentRoundKeys, key)
			}

			// 查询Agent中对应的工具
			tool := a.GetTool(funcName)
			if tool == nil {
				errMsg := fmt.Sprintf("找不到工具%s对应的工具集", funcName)
				toolMessages = append(toolMessages, llm.Message{
					Role:       llm.RoleTool,
					Content:    errMsg,
					ToolCallID: toolCallID,
				})
				result := models.ToolCallResult{
					Success: false,
					Message: errMsg,
				}
				// 工具不存在也计入熔断
				a.recordToolFailure(key, funcName, funcArgs)
				eventCh <- events.OnToolCallFail(toolCall, "", &result)
				a.recordToolNotFound(toolSpan, errMsg)
				failCount++
				return
			}

			// ===== 【HITL】风险审批闸门 =====
			if tool.SupportsRiskAssessment() {
				risk, reason, perr := interrupt.ParseRiskFromArgs(funcArgs)
				if perr != nil {
					logger.Warn("HITL 风险字段解析失败，降级为直接执行",
						zap.String("component", "hitl"),
						zap.String("tool", funcName),
						zap.Error(perr))
				} else if risk == "dangerous" {
					a.recordToolHitlRequested(toolSpan, risk, reason)
					snap := interrupt.Snapshot{
						ToolCallID:   toolCallID,
						FunctionName: funcName,
						FunctionArgs: funcArgs,
						RiskLevel:    risk,
						RiskReason:   reason,
						RegisteredAt: time.Now(),
					}
					ch, regErr := a.pendingSink.RegisterInterrupt(a.messageId, snap)
					if regErr != nil {
						logger.Error("HITL Register 撞已有 pending，视为拒绝",
							zap.String("component", "hitl"),
							zap.String("mid", a.messageId),
							zap.Error(regErr))
						toolMessages = append(toolMessages,
							buildRejectMessage(toolCall, "系统内部错误，拒绝执行"))
						toolMessages = appendSiblingSkipped(toolMessages, toolCalls, toolCall.ID)
						a.circuitBreaker.StartNewRound(currentRoundKeys)
						a.recordToolHitlRegisterFailed(toolSpan, regErr)
						failCount++
						abort = true
						return
					}
					eventCh <- events.OnToolCallInterrupt(toolCall, tool.ProviderName(), risk, reason)

					var decision interrupt.Decision
					select {
					case decision = <-ch:
					case <-time.After(a.pendingSink.WaitTimeout()):
						decision = interrupt.Decision{Kind: interrupt.DecisionTimeout}
					case <-ctx.Done():
						a.recordToolHitlCtxCancelled(toolSpan)
						abort = true
						return
					}

					switch decision.Kind {
					case interrupt.DecisionApprove:
						a.recordToolHitlDecided(toolSpan, "approve")
						// 落地：继续走原 InvokeTool 分支（下方无需 continue）
					case interrupt.DecisionReject:
						content := interrupt.MsgUserReject
						if decision.Feedback != "" {
							content = fmt.Sprintf(interrupt.MsgUserRejectWithFeedbackTpl, decision.Feedback)
						}
						toolMessages = append(toolMessages, buildRejectMessage(toolCall, content))
						toolMessages = appendSiblingSkipped(toolMessages, toolCalls, toolCall.ID)
						a.circuitBreaker.StartNewRound(currentRoundKeys)
						a.recordToolHitlDecided(toolSpan, "reject")
						failCount++
						abort = true
						return
					case interrupt.DecisionTimeout:
						toolMessages = append(toolMessages,
							buildRejectMessage(toolCall, interrupt.MsgTimeout))
						toolMessages = appendSiblingSkipped(toolMessages, toolCalls, toolCall.ID)
						a.circuitBreaker.StartNewRound(currentRoundKeys)
						a.recordToolHitlDecided(toolSpan, "timeout")
						failCount++
						abort = true
						return
					case interrupt.DecisionCancel:
						a.recordToolHitlDecided(toolSpan, "cancel")
						// Stop 路径接管清理，直接 return
						abort = true
						return
					}
				}
			}
			// ===== 中断闸门结束 =====

			// 开始工具调用
			eventCh <- events.OnToolCallStart(toolCall, tool.ProviderName())
			result := a.InvokeTool(tool, funcName, funcArgs)
			eventCh <- events.OnToolCallComplete(toolCall, tool.ProviderName(), &result)
			if !result.Success {
				a.recordToolFailure(key, funcName, funcArgs)
				eventCh <- events.OnToolCallFail(toolCall, tool.ProviderName(), &result)
				toolMessages = append(toolMessages, llm.Message{
					Role:       llm.RoleTool,
					Content:    "工具调用失败：" + result.Message,
					ToolCallID: toolCallID,
				})
				a.recordToolInvokeFailed(toolSpan, tool.ProviderName(), result.Message)
				failCount++
			} else {
				content := models.ConvertToolCallResult2Text(result)
				toolMessages = append(toolMessages, llm.Message{
					Role:       llm.RoleTool,
					Content:    content,
					ToolCallID: toolCallID,
				})
				a.recordToolInvokeSuccess(toolSpan, tool.ProviderName(), content)
				successCount++
			}
			a.recordToolInvokeCompleted(toolSpan, result.Success)
		}()

		if abort {
			return toolMessages
		}
	}

	// 熔断机制：本轮工具调用结束，清零未出现在本轮的历史计数
	a.circuitBreaker.StartNewRound(currentRoundKeys)

	return toolMessages
}

func (a *BaseAgent) InvokeTool(tool tools.Tool, funcName, funcArgs string) models.ToolCallResult {
	return tool.Invoke(funcName, funcArgs)
}

// recordToolFailure 记录一次工具调用失败到熔断计数器。key 为空时直接跳过（GenerateKey 曾失败）。
func (a *BaseAgent) recordToolFailure(key, funcName, funcArgs string) {
	if key == "" {
		return
	}
	metadata := circuitbreaker.ToolCallMetadata{
		ToolName:      funcName,
		ParamsPreview: circuitbreaker.GenerateParamsPreview(funcName, funcArgs),
	}
	failCount := a.circuitBreaker.RecordFailure(key, metadata)
	logger.Info("工具调用失败，更新计数器",
		zap.String("tool", funcName),
		zap.String("key", key),
		zap.Int("failCount", failCount))
}

// buildRejectMessage 构造一条"工具未执行"的 tool result 消息，供 HITL 拒绝/超时/中止路径使用
func buildRejectMessage(toolCall llm.ToolCall, content string) llm.Message {
	return llm.Message{
		Role:       llm.RoleTool,
		Content:    content,
		ToolCallID: toolCall.ID,
	}
}

// appendSiblingSkipped 为 abortedToolCallID 之后（不含）的所有 toolCall 追加"因用户拒绝而未执行"占位消息。
// 保证同一轮内 assistant.tool_calls 中每条 ID 都能配到一条 tool result，避免 memory 里孤儿。
func appendSiblingSkipped(msgs []llm.Message, toolCalls []llm.ToolCall, abortedToolCallID string) []llm.Message {
	seen := false
	for _, tc := range toolCalls {
		if tc.ID == abortedToolCallID {
			seen = true
			continue
		}
		if !seen {
			continue
		}
		msgs = append(msgs, buildRejectMessage(tc, interrupt.MsgSiblingSkipped))
	}
	return msgs
}

// startToolBatchSpan 起 TOOL_BATCH span 并写入批次维度初始 tag
func (a *BaseAgent) startToolBatchSpan(ctx context.Context, toolCallsCount int) (context.Context, *tracing.Span) {
	ctx, batchSpan := tracing.StartSpanFromContext(ctx, tracing.SpanTypeToolBatch, "")
	batchSpan.SetTag("batch.tool_calls_count", toolCallsCount)
	batchSpan.SetTag("batch.parallel", false)
	return ctx, batchSpan
}

// finalizeToolBatchSpan 批次收尾：回填成功/失败计数
func (a *BaseAgent) finalizeToolBatchSpan(batchSpan *tracing.Span, successCount, failCount int) {
	batchSpan.SetTag("batch.success_count", successCount)
	batchSpan.SetTag("batch.fail_count", failCount)
}

// startToolCallSpan 起单个工具调用 span（SubagentTool 走 SUBAGENT_CALL），并写入基础 tag / 请求发送日志
func (a *BaseAgent) startToolCallSpan(ctx context.Context, toolCall llm.ToolCall) *tracing.Span {
	spanType := tracing.SpanTypeToolCall
	if t := a.GetTool(toolCall.Name); t != nil {
		if _, ok := t.(*tools.SubagentTool); ok {
			spanType = tracing.SpanTypeSubagentCall
		}
	}
	_, toolSpan := tracing.StartSpanFromContext(ctx, spanType, toolCall.Name)
	toolSpan.SetTag("tool.name", toolCall.Name)
	toolSpan.SetTag("tool.tool_call_id", toolCall.ID)
	toolSpan.SetTag("tool.arguments", toolCall.Arguments) // 自动 2048 字节截断
	toolSpan.AddLog("INFO", "tool.invoke.start", nil)
	return toolSpan
}

// recordToolRepairFailed jsonrepair 修复失败的埋点
func (a *BaseAgent) recordToolRepairFailed(toolSpan *tracing.Span, err error, errMsg string) {
	toolSpan.SetError(err)
	toolSpan.AddLog("ERROR", "tool.invoke.repair_failed", map[string]interface{}{"message": errMsg})
}

// recordToolNotFound 找不到匹配 tool 时的埋点
func (a *BaseAgent) recordToolNotFound(toolSpan *tracing.Span, errMsg string) {
	toolSpan.SetError(errors.New(errMsg))
	toolSpan.AddLog("ERROR", "tool.not_found", map[string]interface{}{"message": errMsg})
}

// recordToolHitlRequested HITL 风险审批闸门触发
func (a *BaseAgent) recordToolHitlRequested(toolSpan *tracing.Span, risk, reason string) {
	toolSpan.SetTag("tool.hitl.required", true)
	toolSpan.SetTag("tool.risk_level", risk)
	toolSpan.AddLog("INFO", "tool.hitl.requested", map[string]interface{}{"reason": reason})
}

// recordToolHitlRegisterFailed HITL Register 撞已有 pending 视为拒绝
func (a *BaseAgent) recordToolHitlRegisterFailed(toolSpan *tracing.Span, err error) {
	toolSpan.SetError(err)
	toolSpan.AddLog("ERROR", "tool.hitl.register_failed", map[string]interface{}{"message": err.Error()})
}

// recordToolHitlCtxCancelled HITL 等待期间 ctx 被取消
func (a *BaseAgent) recordToolHitlCtxCancelled(toolSpan *tracing.Span) {
	toolSpan.AddLog("WARN", "tool.hitl.ctx_cancelled", nil)
}

// recordToolHitlDecided HITL 决策落地（approve/reject/timeout/cancel）
func (a *BaseAgent) recordToolHitlDecided(toolSpan *tracing.Span, decision string) {
	toolSpan.SetTag("tool.hitl.decision", decision)
	toolSpan.AddLog("INFO", "tool.hitl.decided", map[string]interface{}{"decision": decision})
}

// recordToolInvokeSuccess 工具调用成功：provider + result 预览 tag
func (a *BaseAgent) recordToolInvokeSuccess(toolSpan *tracing.Span, providerName, content string) {
	toolSpan.SetTag("tool.provider", providerName)
	toolSpan.SetTag("tool.result_size", len(content))
	toolSpan.SetTag("tool.result_preview", content) // 自动 512 字节截断
}

// recordToolInvokeFailed 工具调用失败：provider + 错误消息
func (a *BaseAgent) recordToolInvokeFailed(toolSpan *tracing.Span, providerName, message string) {
	toolSpan.SetTag("tool.provider", providerName)
	toolSpan.AddLog("ERROR", "tool.invoke.failed", map[string]interface{}{"message": message})
}

// recordToolInvokeCompleted 一次工具调用最终结束：统一写完成日志
func (a *BaseAgent) recordToolInvokeCompleted(toolSpan *tracing.Span, success bool) {
	toolSpan.AddLog("INFO", "tool.invoke.completed", map[string]interface{}{"success": success})
}

// initRootTracingTags 在会话入口把 domain 层信息回填到 root span
func (a *BaseAgent) initRootTracingTags(rootSpan *tracing.Span) {
	rootSpan.SetTag("agent.name", a.name)
	rootSpan.SetTag("agent.max_iterations", a.agentConfig.MaxIterations)
	rootSpan.SetTag("agent.tools_count", len(a.GetAvailableTools()))
	rootSpan.SetTag("system_prompt.hash", tracing.Sha256Prefix(a.systemPrompt, 16))
	rootSpan.SetAgentName(a.name)
}

// startRoundSpan 起一轮 AGENT_ROUND span，并回填基础 tag
func (a *BaseAgent) startRoundSpan(ctx context.Context, round int) (context.Context, *tracing.Span) {
	roundCtx, roundSpan := tracing.StartSpanFromContext(ctx, tracing.SpanTypeAgentRound, "")
	roundSpan.SetTag("round.index", round)
	roundSpan.SetTag("round.messages_count", len(a.GetMessages()))
	return roundCtx, roundSpan
}

// recordCtxCancelled 在 ctx cancel 分支记录里程碑；cancel 属于会话级异常，root span 标红
func (a *BaseAgent) recordCtxCancelled(rootSpan *tracing.Span, round int, err error) {
	rootSpan.AddLog("WARN", "agent.context_cancelled", map[string]interface{}{
		"round": round,
		"err":   err.Error(),
	})
	rootSpan.SetError(fmt.Errorf("agent context cancelled at round %d: %v", round, err))
}

// recordMaxIterationsExceeded 循环耗尽时记录里程碑并标错到 root span
func (a *BaseAgent) recordMaxIterationsExceeded(rootSpan *tracing.Span) {
	rootSpan.AddLog("ERROR", "agent.max_iterations_exceeded", map[string]interface{}{
		"max_iterations": a.agentConfig.MaxIterations,
	})
	rootSpan.SetError(fmt.Errorf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
}

// startLLMCallSpan 起 LLM_CALL span 并写入初始 tag / 请求发送日志
func (a *BaseAgent) startLLMCallSpan(ctx context.Context) *tracing.Span {
	_, llmSpan := tracing.StartSpanFromContext(ctx, tracing.SpanTypeLLMCall, "")
	llmSpan.SetTag("llm.messages_count", len(a.GetMessages()))
	llmSpan.SetTag("llm.tools_count", len(a.GetAvailableTools()))
	llmSpan.AddLog("INFO", "llm.request.sent", nil)
	return llmSpan
}

// recordLLMFirstToken 在流式模式下记录首个事件到达的里程碑
func (a *BaseAgent) recordLLMFirstToken(llmSpan *tracing.Span, eventType string) {
	llmSpan.AddLog("INFO", "llm.stream.first_token", map[string]interface{}{"event_type": eventType})
}

// recordLLMStreamError LLM 流式调用中途出错：从 error 事件中抽错并标到 span
func (a *BaseAgent) recordLLMStreamError(llmSpan *tracing.Span, errMsg string) {
	llmSpan.SetError(fmt.Errorf("llm stream error: %s", errMsg))
	llmSpan.AddLog("ERROR", "llm.stream.error", map[string]interface{}{"message": errMsg})
}

// finalizeLLMSpanSuccess LLM 调用成功收尾：补 tool_calls_count tag + 完成日志
func (a *BaseAgent) finalizeLLMSpanSuccess(llmSpan *tracing.Span, toolCallsCount int) {
	llmSpan.SetTag("llm.tool_calls_count", toolCallsCount)
	llmSpan.AddLog("INFO", "llm.stream.completed", nil)
}

// recordLLMRetry 记录一次 LLM 调用重试的里程碑
func (a *BaseAgent) recordLLMRetry(llmSpan *tracing.Span, attempt int, err error) {
	llmSpan.AddLog("WARN", "llm.retry", map[string]interface{}{
		"attempt": attempt,
		"err":     err.Error(),
	})
}

// finalizeLLMSpanFailed LLM 调用最终失败：标错到 span
func (a *BaseAgent) finalizeLLMSpanFailed(llmSpan *tracing.Span, err error) {
	llmSpan.SetError(err)
}

// injectInterventionIfNeeded 在进入 LLM 前检查是否有工具触发熔断阈值；若有则向 messages 追加一条用户角色的干预提示并返回新 slice。
func (a *BaseAgent) injectInterventionIfNeeded(messages []llm.Message) []llm.Message {
	triggeredRecords := a.circuitBreaker.GetTriggeredRecords(3)
	if len(triggeredRecords) == 0 {
		return messages
	}
	interventionMsg := circuitbreaker.BuildInterventionPrompt(triggeredRecords)
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: interventionMsg,
	})
	toolNames := make([]string, 0, len(triggeredRecords))
	for _, r := range triggeredRecords {
		toolNames = append(toolNames, r.ToolName)
	}
	logger.Warn("检测到工具调用死循环，注入干预提示",
		zap.Int("triggeredCount", len(triggeredRecords)),
		zap.Strings("tools", toolNames))
	return messages
}

// StreamingInvokeLLM 在一个round内调用大模型不管在流式/非流式场景下都是阻塞调用的
func (a *BaseAgent) StreamingInvokeLLM(ctx context.Context, messages []llm.Message, eventCh chan<- events.AgentEvent) llm.Message {
	llmSpan := a.startLLMCallSpan(ctx)
	defer llmSpan.End()

	messages = a.injectInterventionIfNeeded(messages)
	a.AddToMemory(messages)
	availableTools := a.GetAvailableTools()
	messagesToAdd := make([]llm.Message, 0)
	llmEventCh := make(chan events.AgentEvent)
	var message llm.Message
	logger.Info("begin llm streaming chat", zap.Any("messages", a.GetMessages()), zap.Any("available tools", availableTools))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		message = a.invoker.StreamingInvoke(a.GetMessages(), availableTools, llmEventCh)
		content := message.Content
		toolCalls := message.ToolCalls
		if content == "" && len(toolCalls) == 0 {
			logger.Info("content and toolCalls are both empty")
			messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleAssistant, Content: content})
			messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleUser, Content: "AI无响应内容，请继续"})
		} else if len(toolCalls) > 0 {
			logger.Info("tool calls returned", zap.Any("toolCalls", toolCalls))
			messagesToAdd = append(messagesToAdd, message)
		} else {
			logger.Info("content returned", zap.Any("content", content))
			messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleAssistant, Content: content})
		}
		a.AddToMemory(messagesToAdd)
		wg.Done()
	}()

	firstTokenSeen := false
	streamErr := ""
	for event := range llmEventCh {
		if !firstTokenSeen {
			a.recordLLMFirstToken(llmSpan, event.EventType())
			firstTokenSeen = true
		}
		if event.EventType() == events.EventTypeError {
			if ee, ok := event.(*events.ErrorEvent); ok {
				streamErr = ee.Error
			}
		}
		eventCh <- event
	}
	wg.Wait()

	if streamErr != "" {
		a.recordLLMStreamError(llmSpan, streamErr)
	} else {
		a.finalizeLLMSpanSuccess(llmSpan, len(message.ToolCalls))
	}

	close(eventCh)
	return message
}

func (a *BaseAgent) InvokeLLM(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	llmSpan := a.startLLMCallSpan(ctx)
	defer llmSpan.End()

	messages = a.injectInterventionIfNeeded(messages)
	a.AddToMemory(messages)
	availableTools := a.GetAvailableTools()
	logger.Info("begin llm chat", zap.Any("messages", a.GetMessages()), zap.Any("available tools", availableTools))

	attempt := 0
	errs := make([]error, 0)
	for attempt < a.agentConfig.MaxRetries {
		messagesToAdd := make([]llm.Message, 0)
		message, err := a.invoker.Invoke(a.GetMessages(), availableTools)
		if err == nil {
			content := message.Content
			toolCalls := message.ToolCalls
			if content == "" && len(toolCalls) == 0 {
				logger.Info("content and tool calls are both empty")
				messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleAssistant, Content: content})
				messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleUser, Content: "AI无响应内容，请继续"})
			} else if len(toolCalls) > 0 {
				logger.Info("tool calls returned", zap.Any("toolCalls", toolCalls))
				messagesToAdd = append(messagesToAdd, message)
			} else {
				logger.Info("content returned", zap.Any("content", content))
				messagesToAdd = append(messagesToAdd, llm.Message{Role: llm.RoleAssistant, Content: content})
			}
			a.AddToMemory(messagesToAdd)
			a.finalizeLLMSpanSuccess(llmSpan, len(message.ToolCalls))
			return message, nil
		} else {
			// 对话出现异常 等待间隔之后重试
			attempt++
			errs = append(errs, fmt.Errorf("第%d次对话失败：%v", attempt, err))
			a.recordLLMRetry(llmSpan, attempt, err)
			time.Sleep(time.Duration(a.retryInterval) * time.Second)
			logger.Info("retry llm chat", zap.Int("attempt", attempt), zap.Error(err))
		}
	}

	finalErr := fmt.Errorf("对话重试次数达到最大值：%v", errors.Join(errs...))
	a.finalizeLLMSpanFailed(llmSpan, finalErr)
	return llm.Message{}, finalErr
}

func (a *BaseAgent) Invoke(ctx context.Context, query string, eventCh chan events.AgentEvent) {
	// 注入 eventCh 到 SubagentTool（如果存在），以支持子智能体事件透传
	a.injectEventChToSubagent(eventCh)

	rootSpan := tracing.SpanFromContext(ctx)
	a.initRootTracingTags(rootSpan)

	messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
	logger.Info("begin invoke llm", zap.Any("query", query))
	message, err := a.InvokeLLM(ctx, messages)
	if err != nil {
		logger.Warn("invoke llm failed", zap.Any("query", query))
		eventCh <- events.OnError(fmt.Sprintf("Agent对话失败：%s", err.Error()))
		close(eventCh)
		return
	}

	// 循环遍历直到最大的迭代次数
	round := 0
	for round < a.agentConfig.MaxIterations {
		// 检查 context 是否已被 cancel（停止按钮 / 超时兜底）
		select {
		case <-ctx.Done():
			a.recordCtxCancelled(rootSpan, round, ctx.Err())
			logger.Info("Invoke cancelled by context", zap.Error(ctx.Err()), zap.Int("round", round))
			eventCh <- events.OnError("对话已被中止")
			close(eventCh)
			return
		default:
		}
		round++

		// 每轮用匿名函数隔离 defer 作用域，保证 roundSpan.End() 逐轮触发
		shouldReturn := func() bool {
			roundCtx, roundSpan := a.startRoundSpan(ctx, round)
			defer roundSpan.End()

			toolCalls := message.ToolCalls
			if len(toolCalls) == 0 {
				// 如果响应的内容无工具调用则表示LLM生成了文本回答 这时候就是最终答案
				logger.Info("end invoke llm", zap.String("text", message.Content))
				eventCh <- events.OnMessage(message.Content, nil)
				return true
			}
			logger.Info("begin invoke tool calls", zap.Any("toolCalls", toolCalls))
			toolMessages := a.InvokeToolCalls(roundCtx, toolCalls, eventCh)

			// 所有的工具都执行完成之后 调用LLM获取汇总消息二次提问
			logger.Info("invoke llm", zap.Int("round", round), zap.Any("tool messages", toolMessages))
			message, err = a.InvokeLLM(roundCtx, toolMessages)
			return false
		}()

		if shouldReturn {
			close(eventCh)
			return
		}
	}

	a.recordMaxIterationsExceeded(rootSpan)
	// 超过最大迭代次数 抛出ErrorEvent
	eventCh <- events.OnError(fmt.Sprintf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
	close(eventCh)
}

// StreamingInvoke 在流式/非流式场景下该方法都是阻塞调用的
func (a *BaseAgent) StreamingInvoke(ctx context.Context, query string, eventCh chan events.AgentEvent) {
	// 注入 eventCh 到 SubagentTool（如果存在），以支持子智能体事件透传
	a.injectEventChToSubagent(eventCh)

	rootSpan := tracing.SpanFromContext(ctx)
	a.initRootTracingTags(rootSpan)

	messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
	var wg sync.WaitGroup
	var shouldEnd atomic.Bool
	shouldEnd.Store(false)

	round := 0
	for round < a.agentConfig.MaxIterations {
		// 检查 context 是否已被 cancel（停止按钮 / 超时兜底）
		select {
		case <-ctx.Done():
			a.recordCtxCancelled(rootSpan, round, ctx.Err())
			logger.Info("StreamingInvoke cancelled by context", zap.Error(ctx.Err()), zap.Int("round", round))
			eventCh <- events.OnError("对话已被中止")
			close(eventCh)
			return
		default:
		}
		round++

		// 每轮用匿名函数隔离 defer 作用域，保证 roundSpan.End() 逐轮触发
		shouldClose := func() bool {
			roundCtx, roundSpan := a.startRoundSpan(ctx, round)
			defer roundSpan.End()

			wg.Add(1)
			llmEventCh := make(chan events.AgentEvent)
			go func() {
				defer wg.Done()
				message := a.StreamingInvokeLLM(roundCtx, messages, llmEventCh) //  这一步是阻塞调用 需要持续监听上报的Event
				toolCalls := message.ToolCalls
				if len(toolCalls) == 0 {
					logger.Info("end invoke llm", zap.Int("round", round), zap.Any("text", message.Content))
					eventCh <- events.OnMessageEnd()
					shouldEnd.CompareAndSwap(false, true)
					return
				}
				messages = a.InvokeToolCalls(roundCtx, toolCalls, eventCh) // 传递 roundCtx 给工具调用层
			}()
			for event := range llmEventCh {
				eventCh <- event
			}
			wg.Wait()
			return shouldEnd.Load()
		}()

		if shouldClose {
			close(eventCh)
			return
		}
	}

	a.recordMaxIterationsExceeded(rootSpan)
	eventCh <- events.OnError(fmt.Sprintf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
	close(eventCh)
}
