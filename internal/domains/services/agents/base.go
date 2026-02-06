package agents

import (
	"errors"
	"fmt"
	"mooc-manus/internal/domains/events"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/llm"
	"mooc-manus/pkg/logger"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaptinlin/jsonrepair"
	"github.com/openai/openai-go"
	"go.uber.org/zap"
)

type BaseAgent struct {
	name          string
	systemPrompt  string
	retryInterval int
	agentConfig   models.AgentConfig
	llm           *llm.OpenAiLLM
	memory        *memory.ChatMemory
	tools         []tools.Tool
}

func NewBaseAgent(agentConfig models.AgentConfig, llm *llm.OpenAiLLM, memory *memory.ChatMemory, tools []tools.Tool, systemPrompt string) *BaseAgent {
	agent := &BaseAgent{}
	agent.agentConfig = agentConfig
	agent.llm = llm
	agent.memory = memory
	agent.tools = tools
	agent.systemPrompt = systemPrompt
	agent.retryInterval = 5
	return agent
}

func (a *BaseAgent) GetAvailableTools() []openai.ChatCompletionToolParam {
	availableTools := make([]openai.ChatCompletionToolParam, 0)
	if len(a.tools) > 0 {
		for _, tool := range a.tools {
			availableTools = append(availableTools, tool.GetTools()...)
		}
	}

	return availableTools
}

func (a *BaseAgent) GetMessages() []openai.ChatCompletionMessageParamUnion {
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

func (a *BaseAgent) AddToMemory(messages []openai.ChatCompletionMessageParamUnion) {
	if a.memory.IsEmpty() {
		a.memory.AddMessage(openai.SystemMessage(a.systemPrompt))
	}
	a.memory.AddMessages(messages)
}

// InvokeToolCalls 执行工具调用并采集ToolMessage 注：该方法不管在流式还是非流式的场景下都是阻塞调用的
func (a *BaseAgent) InvokeToolCalls(toolCalls []openai.ChatCompletionMessageToolCall, eventCh chan<- events.AgentEvent) []openai.ChatCompletionMessageParamUnion {
	toolMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		toolCallId := toolCall.ID
		funcName := toolCall.Function.Name
		funcArgs := toolCall.Function.Arguments
		// 使用jsonrepair修复funcArgs
		repairedArgs, err := jsonrepair.JSONRepair(funcArgs)
		if err != nil {
			logger.Error("repair tool call args failed", zap.Error(err), zap.String("function args", funcArgs))
			errMsg := fmt.Sprintf("工具调用参数不符合规范，修复失败：%v", err)
			toolMessages = append(toolMessages, openai.ToolMessage(errMsg, toolCallId))
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
			toolMessages = append(toolMessages, openai.ToolMessage(errMsg, toolCallId))
			result := models.ToolCallResult{}
			result.Success = false
			result.Message = errMsg
			eventCh <- events.OnToolCallFail(toolCall, "", &result)
			continue
		}
		// 开始工具调用
		eventCh <- events.OnToolCallStart(toolCall, tool.ProviderName())
		result := a.InvokeTool(tool, funcName, funcArgs)
		eventCh <- events.OnToolCallComplete(toolCall, tool.ProviderName(), &result)
		if !result.Success {
			eventCh <- events.OnToolCallFail(toolCall, tool.ProviderName(), &result)
			toolMessages = append(toolMessages, openai.ToolMessage("工具调用失败："+result.Message, toolCallId))
		} else {
			toolMessages = append(toolMessages, openai.ToolMessage(models.ConvertToolCallResult2Text(result), toolCallId))
		}
	}

	return toolMessages
}

func (a *BaseAgent) InvokeTool(tool tools.Tool, funcName, funcArgs string) models.ToolCallResult {
	attempt := 0
	for attempt < a.agentConfig.MaxRetries {
		result := tool.Invoke(funcName, funcArgs)
		if result.Success {
			return result
		}
		attempt++
	}
	return models.ToolCallResult{
		Success: false,
		Message: "工具调用失败",
	}
}

// StreamingInvokeLLM 在一个round内调用大模型不管在流式/非流式场景下都是阻塞调用的
func (a *BaseAgent) StreamingInvokeLLM(messages []openai.ChatCompletionMessageParamUnion, eventCh chan<- events.AgentEvent) openai.ChatCompletionMessage {
	a.AddToMemory(messages)
	availableTools := a.GetAvailableTools()
	messagesToAdd := make([]openai.ChatCompletionMessageParamUnion, 0, 0)
	llmEventCh := make(chan events.AgentEvent)
	var message openai.ChatCompletionMessage
	logger.Info("begin llm streaming chat", zap.Any("messages", a.GetMessages()), zap.Any("available tools", availableTools))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		message = a.llm.StreamingInvoke(a.GetMessages(), availableTools, llmEventCh)
		content := message.Content
		toolCalls := message.ToolCalls
		if content == "" && len(toolCalls) == 0 {
			logger.Info("content and toolCalls are both empty")
			messagesToAdd = append(messagesToAdd, openai.AssistantMessage(content))
			messagesToAdd = append(messagesToAdd, openai.UserMessage("AI无响应内容，请继续"))
		} else if len(toolCalls) > 0 {
			logger.Info("tool calls returned", zap.Any("toolCalls", toolCalls))
			messagesToAdd = append(messagesToAdd, message.ToParam())
		} else {
			logger.Info("content returned", zap.Any("content", content))
			messagesToAdd = append(messagesToAdd, openai.AssistantMessage(content))
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

func (a *BaseAgent) InvokeLLM(messages []openai.ChatCompletionMessageParamUnion) (openai.ChatCompletionMessage, error) {
	a.AddToMemory(messages)
	availableTools := a.GetAvailableTools()
	logger.Info("begin llm chat", zap.Any("messages", a.GetMessages()), zap.Any("available tools", availableTools))

	attempt := 0
	errs := make([]error, 0)
	for attempt < a.agentConfig.MaxRetries {
		messagesToAdd := make([]openai.ChatCompletionMessageParamUnion, 0, 0)
		message, err := a.llm.Invoke(a.GetMessages(), availableTools)
		if err == nil {
			content := message.Content
			toolCalls := message.ToolCalls
			if content == "" && len(toolCalls) == 0 {
				logger.Info("content and tool calls are both empty")
				messagesToAdd = append(messagesToAdd, openai.AssistantMessage(content))
				messagesToAdd = append(messagesToAdd, openai.UserMessage("AI无响应内容，请继续"))
			} else if len(toolCalls) > 0 {
				logger.Info("tool calls returned", zap.Any("toolCalls", toolCalls))
				messagesToAdd = append(messagesToAdd, message.ToParam())
			} else {
				logger.Info("content returned", zap.Any("content", content))
				messagesToAdd = append(messagesToAdd, openai.AssistantMessage(content))
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

	return openai.ChatCompletionMessage{}, fmt.Errorf("对话重试次数达到最大值：%v", errors.Join(errs...))
}

func (a *BaseAgent) Invoke(query string, eventCh chan events.AgentEvent) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0)
	messages = append(messages, openai.UserMessage(query))
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
		toolMessages := a.InvokeToolCalls(toolCalls, eventCh)

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
	messages := make([]openai.ChatCompletionMessageParamUnion, 0)
	messages = append(messages, openai.UserMessage(query))
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
			messages = a.InvokeToolCalls(toolCalls, eventCh) // 这一步是阻塞调用 event直接上报给eventCh 无需监听
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
