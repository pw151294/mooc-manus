package agents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/domains/services/tools"
)

// mockInvokerWithUsage 实现带 Usage 的 mock invoker
type mockInvokerWithUsage struct {
	usage llm.Usage
}

func (m *mockInvokerWithUsage) Invoke(_ []llm.Message, _ []llm.Tool) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: "test"}, nil
}

func (m *mockInvokerWithUsage) StreamingInvoke(_ []llm.Message, _ []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
	close(eventCh)
	return llm.Message{Role: llm.RoleAssistant, Content: "test"}
}

func (m *mockInvokerWithUsage) LastUsage() llm.Usage {
	return m.usage
}

// 单元测试：验证 finalizeLLMSpanSuccess 正确写入 usage tag
func TestFinalizeLLMSpanSuccess_WritesUsageTags(t *testing.T) {
	// 准备
	mockInv := &mockInvokerWithUsage{}
	mockInv.usage = llm.Usage{PromptTokens: 123, CompletionTokens: 456, TotalTokens: 579}

	agent := NewBaseAgent(
		models.AgentConfig{MaxRetries: 1, MaxIterations: 1},
		mockInv,
		memory.NewChatMemory(),
		[]tools.Tool{},
		"test prompt",
	)

	span := &tracing.Span{
		TraceID:   "test-trace",
		SpanID:    1,
		SpanType:  tracing.SpanTypeLLMCall,
		StartTime: time.Now().UnixNano(),
	}

	// 执行
	agent.finalizeLLMSpanSuccess(span, 2)

	// 验证
	tags := span.TagsSnapshot()
	assert.Equal(t, int64(123), tags["llm.io.prompt_units"], "prompt_units 应被写入")
	assert.Equal(t, int64(456), tags["llm.io.completion_units"], "completion_units 应被写入")
	assert.Equal(t, int64(579), tags["llm.io.total_units"], "total_units 应被写入")
	assert.Equal(t, 2, tags["llm.tool_calls_count"], "tool_calls_count 应被写入")
}
