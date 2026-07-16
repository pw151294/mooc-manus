package llm

import (
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	domainllm "mooc-manus/internal/domains/models/llm"
	"mooc-manus/pkg/logger"
	"sync"

	"github.com/openai/openai-go"
	"go.uber.org/zap"
)

type OpenAIAdapter struct {
	llm         *OpenAiLLM
	lastUsage   domainllm.Usage // 新增：最近一次调用的 usage
	lastUsageMu sync.Mutex
}

func NewOpenAIAdapter(cfg models.ModelConfig) *OpenAIAdapter {
	return &OpenAIAdapter{llm: NewOpenAiLLM(cfg)}
}

func (a *OpenAIAdapter) Invoke(messages []domainllm.Message, tools []domainllm.Tool) (domainllm.Message, error) {
	resp, sdkUsage, err := a.llm.Invoke(toOpenAIMessages(messages), toOpenAITools(tools))
	if err != nil {
		return domainllm.Message{}, err
	}
	a.lastUsageMu.Lock()
	a.lastUsage = domainllm.Usage{
		PromptTokens:     int64(sdkUsage.PromptTokens),
		CompletionTokens: int64(sdkUsage.CompletionTokens),
		TotalTokens:      int64(sdkUsage.TotalTokens),
	}
	a.lastUsageMu.Unlock()
	return fromOpenAIMessage(resp), nil
}

// StreamingInvoke 透传 eventCh 给底层 OpenAiLLM.StreamingInvoke,close(eventCh) 责任由后者承担,符合 spec 4.4 约定。
func (a *OpenAIAdapter) StreamingInvoke(messages []domainllm.Message, tools []domainllm.Tool, eventCh chan<- events.AgentEvent) domainllm.Message {
	resp, sdkUsage := a.llm.StreamingInvoke(toOpenAIMessages(messages), toOpenAITools(tools), eventCh)
	a.lastUsageMu.Lock()
	a.lastUsage = domainllm.Usage{
		PromptTokens:     int64(sdkUsage.PromptTokens),
		CompletionTokens: int64(sdkUsage.CompletionTokens),
		TotalTokens:      int64(sdkUsage.TotalTokens),
	}
	a.lastUsageMu.Unlock()
	return fromOpenAIMessage(resp)
}

// LastUsage 获取最近一次调用的 usage
func (a *OpenAIAdapter) LastUsage() domainllm.Usage {
	a.lastUsageMu.Lock()
	defer a.lastUsageMu.Unlock()
	return a.lastUsage
}

func toOpenAIMessages(messages []domainllm.Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case domainllm.RoleSystem:
			out = append(out, openai.SystemMessage(m.Content))
		case domainllm.RoleUser:
			out = append(out, openai.UserMessage(m.Content))
		case domainllm.RoleAssistant:
			if len(m.ToolCalls) == 0 {
				out = append(out, openai.AssistantMessage(m.Content))
				continue
			}
			asst := openai.ChatCompletionAssistantMessageParam{}
			asst.Content.OfString = openai.String(m.Content)
			asst.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
					Type: "function",
				})
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
		case domainllm.RoleTool:
			out = append(out, openai.ToolMessage(m.Content, m.ToolCallID))
		default:
			logger.Warn("unknown role when converting to openai message", zap.String("role", string(m.Role)))
		}
	}
	return out
}

func toOpenAITools(tools []domainllm.Tool) []openai.ChatCompletionToolParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		fn := openai.FunctionDefinitionParam{}
		fn.Name = t.Name
		fn.Description = openai.String(t.Description)
		fn.Parameters = t.Parameters
		out = append(out, openai.ChatCompletionToolParam{Function: fn, Type: "function"})
	}
	return out
}

func fromOpenAIMessage(m openai.ChatCompletionMessage) domainllm.Message {
	out := domainllm.Message{
		Role:    domainllm.RoleAssistant, // OpenAI Chat Completion 响应里只会是 assistant
		Content: m.Content,
	}
	if len(m.ToolCalls) > 0 {
		out.ToolCalls = make([]domainllm.ToolCall, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, domainllm.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}
	return out
}
