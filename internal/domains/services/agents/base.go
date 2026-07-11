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
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/invoker"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/tools"
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
func (a *BaseAgent) InvokeToolCalls(ctx context.Context, toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message {
	toolMessages := make([]llm.Message, 0, len(toolCalls))
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

	return toolMessages
}

func (a *BaseAgent) InvokeTool(tool tools.Tool, funcName, funcArgs string) models.ToolCallResult {
	// 去掉重试循环：工具失败直接返回失败结果交给 LLM 重新规划
	// 避免盲目重试导致长时间阻塞（如 bash 全盘搜索超时重试 3 次 → 17+ 分钟）
	return tool.Invoke(funcName, funcArgs)
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
