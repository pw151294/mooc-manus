package llm

import (
	"errors"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	domainllm "mooc-manus/internal/domains/models/llm"
)

type AnthropicAdapter struct {
	cfg models.ModelConfig
}

func NewAnthropicAdapter(cfg models.ModelConfig) *AnthropicAdapter {
	return &AnthropicAdapter{cfg: cfg}
}

func (a *AnthropicAdapter) Invoke(messages []domainllm.Message, tools []domainllm.Tool) (domainllm.Message, error) {
	return domainllm.Message{}, errors.New("anthropic adapter not implemented")
}

func (a *AnthropicAdapter) StreamingInvoke(messages []domainllm.Message, tools []domainllm.Tool, eventCh chan<- events.AgentEvent) domainllm.Message {
	close(eventCh)
	return domainllm.Message{}
}
