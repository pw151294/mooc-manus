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
	pendingSink    PendingSink // 可为 nil，nil 时 InvokeToolCalls 跳过 HITL 闸门（A2A 场景）
	messageId      string      // HITL 用于 RegisterInterrupt 定位 slot
}

// BaseAgentOption 为 NewBaseAgent 的可选参数
type BaseAgentOption func(*BaseAgent)

// WithPendingSink 注入 HITL 审批管理器；nil 或不传则不启用 HITL
func WithPendingSink(sink PendingSink) BaseAgentOption {
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

// InvokeToolCalls 执行工具调用并采集ToolMessage 注：该方法不管在流式还是非流式的场景下都是阻塞调用的
func (a *BaseAgent) InvokeToolCalls(ctx context.Context, toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message {
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
			continue
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
			continue
		}

		// ===== 【HITL 新增】风险审批闸门 =====
		if tool.SupportsRiskAssessment() && a.pendingSink != nil {
			risk, reason, perr := interrupt.ParseRiskFromArgs(funcArgs)
			if perr != nil {
				logger.Warn("HITL 风险字段解析失败，降级为直接执行",
					zap.String("component", "hitl"),
					zap.String("tool", funcName),
					zap.Error(perr))
			} else if risk == "dangerous" {
				snap := InterruptSnapshot{
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
					return toolMessages
				}
				eventCh <- events.OnToolCallInterrupt(toolCall, tool.ProviderName(), risk, reason)

				var decision InterruptDecision
				select {
				case decision = <-ch:
				case <-time.After(a.pendingSink.WaitTimeout()):
					decision = InterruptDecision{Kind: DecisionTimeout}
				case <-ctx.Done():
					return toolMessages
				}

				switch decision.Kind {
				case DecisionApprove:
					// 落地：继续走原 InvokeTool 分支（下方无需 continue）
				case DecisionReject:
					content := interrupt.MsgUserReject
					if decision.Feedback != "" {
						content = fmt.Sprintf(interrupt.MsgUserRejectWithFeedbackTpl, decision.Feedback)
					}
					toolMessages = append(toolMessages, buildRejectMessage(toolCall, content))
					toolMessages = appendSiblingSkipped(toolMessages, toolCalls, toolCall.ID)
					a.circuitBreaker.StartNewRound(currentRoundKeys)
					return toolMessages
				case DecisionTimeout:
					toolMessages = append(toolMessages,
						buildRejectMessage(toolCall, interrupt.MsgTimeout))
					toolMessages = appendSiblingSkipped(toolMessages, toolCalls, toolCall.ID)
					a.circuitBreaker.StartNewRound(currentRoundKeys)
					return toolMessages
				case DecisionCancel:
					// Stop 路径接管清理，直接 return
					return toolMessages
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
		} else {
			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Content:    models.ConvertToolCallResult2Text(result),
				ToolCallID: toolCallID,
			})
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
func (a *BaseAgent) StreamingInvokeLLM(messages []llm.Message, eventCh chan<- events.AgentEvent) llm.Message {
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

	for event := range llmEventCh {
		eventCh <- event
	}
	wg.Wait()
	close(eventCh)
	return message
}

func (a *BaseAgent) InvokeLLM(messages []llm.Message) (llm.Message, error) {
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
			return message, nil
		} else {
			// 对话出现异常 等待间隔之后重试
			attempt++
			errs = append(errs, fmt.Errorf("第%d次对话失败：%v", attempt, err))
			time.Sleep(time.Duration(a.retryInterval) * time.Second)
			logger.Info("retry llm chat", zap.Int("attempt", attempt), zap.Error(err))
		}
	}

	return llm.Message{}, fmt.Errorf("对话重试次数达到最大值：%v", errors.Join(errs...))
}

func (a *BaseAgent) Invoke(ctx context.Context, query string, eventCh chan events.AgentEvent) {
	messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
	logger.Info("begin invoke llm", zap.Any("query", query))
	message, err := a.InvokeLLM(messages)
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
			logger.Info("Invoke cancelled by context", zap.Error(ctx.Err()), zap.Int("round", round))
			eventCh <- events.OnError("对话已被中止")
			close(eventCh)
			return
		default:
		}
		round++
		toolCalls := message.ToolCalls
		if len(toolCalls) == 0 {
			// 如果响应的内容无工具调用则表示LLM生成了文本回答 这时候就是最终答案
			logger.Info("end invoke llm", zap.String("text", message.Content))
			eventCh <- events.OnMessage(message.Content, nil)
			close(eventCh)
			return
		}
		logger.Info("begin invoke tool calls", zap.Any("toolCalls", toolCalls))
		toolMessages := a.InvokeToolCalls(ctx, toolCalls, eventCh)

		// 所有的工具都执行完成之后 调用LLM获取汇总消息二次提问
		logger.Info("invoke llm", zap.Int("round", round), zap.Any("tool messages", toolMessages))
		message, err = a.InvokeLLM(toolMessages)
	}

	// 超过最大迭代次数 抛出ErrorEvent
	eventCh <- events.OnError(fmt.Sprintf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
	close(eventCh)
}

// StreamingInvoke 在流式/非流式场景下该方法都是阻塞调用的
func (a *BaseAgent) StreamingInvoke(ctx context.Context, query string, eventCh chan events.AgentEvent) {
	messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
	var wg sync.WaitGroup
	var shouldEnd atomic.Bool
	shouldEnd.Store(false)

	round := 0
	for round < a.agentConfig.MaxIterations {
		// 检查 context 是否已被 cancel（停止按钮 / 超时兜底）
		select {
		case <-ctx.Done():
			logger.Info("StreamingInvoke cancelled by context", zap.Error(ctx.Err()), zap.Int("round", round))
			eventCh <- events.OnError("对话已被中止")
			close(eventCh)
			return
		default:
		}
		round++
		wg.Add(1)
		llmEventCh := make(chan events.AgentEvent)
		go func() {
			defer wg.Done()
			message := a.StreamingInvokeLLM(messages, llmEventCh) //  这一步是阻塞调用 需要持续监听上报的Event
			toolCalls := message.ToolCalls
			if len(toolCalls) == 0 {
				logger.Info("end invoke llm", zap.Int("round", round), zap.Any("text", message.Content))
				eventCh <- events.OnMessageEnd()
				shouldEnd.CompareAndSwap(false, true)
				return
			}
			messages = a.InvokeToolCalls(ctx, toolCalls, eventCh) // 传递 context 给工具调用层
		}()
		for event := range llmEventCh {
			eventCh <- event
		}
		wg.Wait()
		if shouldEnd.Load() {
			close(eventCh)
			return
		}
	}
	eventCh <- events.OnError(fmt.Sprintf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
	close(eventCh)
}
