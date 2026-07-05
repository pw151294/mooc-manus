package agents

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaptinlin/jsonrepair"
	"go.uber.org/zap"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/invoker"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/domains/services/tools/error_recovery"
	"mooc-manus/pkg/logger"
)

type BaseAgent struct {
	name          string
	systemPrompt  string
	retryInterval int
	agentConfig   models.AgentConfig
	invoker       invoker.Invoker
	memory        *memory.ChatMemory
	tools         []tools.Tool
}

func NewBaseAgent(agentConfig models.AgentConfig, inv invoker.Invoker, mem *memory.ChatMemory, ts []tools.Tool, systemPrompt string) *BaseAgent {
	return &BaseAgent{
		agentConfig:   agentConfig,
		invoker:       inv,
		memory:        mem,
		tools:         ts,
		systemPrompt:  systemPrompt,
		retryInterval: 5,
	}
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
// 返回值 abort 为 true 时表示本轮工具调用命中 L3 致命故障,调用方(主循环)应立即终止链路
func (a *BaseAgent) InvokeToolCalls(toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) ([]llm.Message, bool) {
	toolMessages := make([]llm.Message, 0, len(toolCalls))
	abort := false
	for _, toolCall := range toolCalls {
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
			eventCh <- events.OnToolCallFail(toolCall, "", &result)
			continue
		}
		// 开始工具调用
		eventCh <- events.OnToolCallStart(toolCall, tool.ProviderName())
		result := a.InvokeTool(tool, funcName, funcArgs)
		eventCh <- events.OnToolCallComplete(toolCall, tool.ProviderName(), &result)
		if !result.Success {
			eventCh <- events.OnToolCallFail(toolCall, tool.ProviderName(), &result)
			content := "工具调用失败：" + result.Message
			// 4 类 NATIVE 原生工具失败时,按 error_recovery 分类追加修复模板
			if error_recovery.Enabled() && error_recovery.IsNativeTool(funcName) {
				decision := error_recovery.Classify(funcName, result.Message)
				if decision.Level != error_recovery.LevelNone {
					content = content + "\n\n" + decision.Level.Prefix() + decision.Template
					logger.Info("error_recovery hook applied",
						zap.String("tool", funcName),
						zap.Int("level", int(decision.Level)),
						zap.String("template_key", decision.TemplateKey),
					)
					if decision.Level == error_recovery.LevelFatal {
						abort = true
					}
				}
			}
			toolMessages = append(toolMessages, llm.Message{
				Role:       llm.RoleTool,
				Content:    content,
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

	return toolMessages, abort
}

func (a *BaseAgent) InvokeTool(tool tools.Tool, funcName, funcArgs string) models.ToolCallResult {
	attempt := 0
	// 保留最后一次真实 result,避免丢失原始 error 消息影响 error_recovery 分类
	var last models.ToolCallResult
	last.Success = false
	last.Message = "工具调用失败"
	for attempt < a.agentConfig.MaxRetries {
		result := tool.Invoke(funcName, funcArgs)
		if result.Success {
			return result
		}
		last = result
		attempt++
	}
	return last
}

// StreamingInvokeLLM 在一个round内调用大模型不管在流式/非流式场景下都是阻塞调用的
func (a *BaseAgent) StreamingInvokeLLM(messages []llm.Message, eventCh chan<- events.AgentEvent) llm.Message {
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

func (a *BaseAgent) Invoke(query string, eventCh chan events.AgentEvent) {
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
		toolMessages, abort := a.InvokeToolCalls(toolCalls, eventCh)

		if abort {
			logger.Warn("tool call chain aborted by error_recovery L3", zap.Int("round", round))
			// 记录中断上下文进 memory,便于后续排查
			a.AddToMemory(toolMessages)
			eventCh <- events.OnError("原生工具触发致命故障(L3),已终止本次任务链路")
			close(eventCh)
			return
		}

		// 所有的工具都执行完成之后 调用LLM获取汇总消息二次提问
		logger.Info("invoke llm", zap.Int("round", round), zap.Any("tool messages", toolMessages))
		message, err = a.InvokeLLM(toolMessages)
	}

	// 超过最大迭代次数 抛出ErrorEvent
	eventCh <- events.OnError(fmt.Sprintf("智能体思考轮次超过阈值：%d", a.agentConfig.MaxIterations))
	close(eventCh)
}

// StreamingInvoke 在流式/非流式场景下该方法都是阻塞调用的
func (a *BaseAgent) StreamingInvoke(query string, eventCh chan events.AgentEvent) {
	messages := []llm.Message{{Role: llm.RoleUser, Content: query}}
	var wg sync.WaitGroup
	var shouldEnd atomic.Bool
	shouldEnd.Store(false)

	round := 0
	for round < a.agentConfig.MaxIterations {
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
			toolMsgs, abort := a.InvokeToolCalls(toolCalls, eventCh) // 这一步是阻塞调用 event直接上报给eventCh 无需监听
			messages = toolMsgs
			if abort {
				logger.Warn("streaming tool call chain aborted by error_recovery L3", zap.Int("round", round))
				a.AddToMemory(toolMsgs)
				eventCh <- events.OnError("原生工具触发致命故障(L3),已终止本次任务链路")
				shouldEnd.CompareAndSwap(false, true)
			}
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
