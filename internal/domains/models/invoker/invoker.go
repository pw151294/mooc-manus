package invoker

import (
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
)

// Invoker 是 LLM 调用的领域接口,由适配器层(OpenAI / Anthropic 等)实现。
//
// StreamingInvoke 的 channel 生命周期约定:eventCh 由调用方创建并传入;
// 适配器负责在流式接收过程中向 eventCh 写入事件,并在流结束(或出错上报后)调用 close(eventCh),
// 调用方无需自行 close。
type Invoker interface {
	Invoke(messages []llm.Message, tools []llm.Tool) (llm.Message, error)
	StreamingInvoke(messages []llm.Message, tools []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message
	LastUsage() llm.Usage // 新增：获取最近一次调用的 token 消耗
}
