package agents

import (
	"testing"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/invoker"
	llmadapter "mooc-manus/internal/infra/external/llm"
)

func TestPickInvoker_DefaultsToOpenAI(t *testing.T) {
	cfg := models.ModelConfig{Provider: ""}
	got := PickInvoker(cfg)
	if _, ok := got.(*llmadapter.OpenAIAdapter); !ok {
		t.Fatalf("default should be OpenAI, got %T", got)
	}
}

func TestPickInvoker_AnthropicBranch(t *testing.T) {
	cfg := models.ModelConfig{Provider: "anthropic"}
	got := PickInvoker(cfg)
	if _, ok := got.(*llmadapter.AnthropicAdapter); !ok {
		t.Fatalf("anthropic branch should return Anthropic adapter, got %T", got)
	}
}

// 编译期断言:PickInvoker 的返回类型实现 invoker.Invoker
var _ invoker.Invoker = PickInvoker(models.ModelConfig{})
