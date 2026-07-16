package services

import (
	"context"
	"sync"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
)

// mockInvoker 实现 invoker.Invoker 接口，仅供 HITL 集成测试使用。
// 本 mock 主要为 InvokeToolCalls 提供依赖装配；测试用例不直接触发 LLM 调用。
type mockInvoker struct{}

func (m *mockInvoker) Invoke(_ []llm.Message, _ []llm.Tool) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: ""}, nil
}

func (m *mockInvoker) StreamingInvoke(_ []llm.Message, _ []llm.Tool, eventCh chan<- events.AgentEvent) llm.Message {
	close(eventCh)
	return llm.Message{Role: llm.RoleAssistant, Content: ""}
}

func (m *mockInvoker) LastUsage() llm.Usage {
	return llm.Usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0}
}

// mockTool 实现 tools.Tool 接口，可控制 SupportsRiskAssessment 与 Invoke 行为。
type mockTool struct {
	providerName   string
	functionName   string
	supportsRisk   bool
	invokeResult   models.ToolCallResult
	invokeCalls    int32 // 需要读的调用方走 atomic；这里为简化用 mu 守护
	mu             sync.Mutex
	onInvokeCalled func(funcName, funcArgs string)
}

func newMockTool(functionName string, supportsRisk bool) *mockTool {
	return &mockTool{
		providerName: "native",
		functionName: functionName,
		supportsRisk: supportsRisk,
		invokeResult: models.ToolCallResult{
			Success: true,
			Message: "ok",
			Data:    "mock tool executed",
		},
	}
}

func (t *mockTool) GetTools() []llm.Tool {
	return []llm.Tool{{
		Name:        t.functionName,
		Description: "mock tool for HITL integration test",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string"},
				"risk_level":  map[string]any{"type": "string"},
				"risk_reason": map[string]any{"type": "string"},
			},
		},
	}}
}

func (t *mockTool) HasTool(funcName string) bool { return funcName == t.functionName }

func (t *mockTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	t.mu.Lock()
	t.invokeCalls++
	cb := t.onInvokeCalled
	t.mu.Unlock()
	if cb != nil {
		cb(funcName, funcArgs)
	}
	return t.invokeResult
}

func (t *mockTool) InvokeWithContext(_ context.Context, funcName, funcArgs string) models.ToolCallResult {
	return t.Invoke(funcName, funcArgs)
}

func (t *mockTool) Init() error { return nil }

func (t *mockTool) ProviderName() string { return t.providerName }

func (t *mockTool) SupportsRiskAssessment() bool { return t.supportsRisk }

func (t *mockTool) invokedTimes() int32 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.invokeCalls
}

// drainEvents 后台消费 eventCh，防止 InvokeToolCalls 因 eventCh 阻塞发送而卡死。
// 返回收集到的所有事件；等 close(eventCh) 后返回。
func drainEvents(ch <-chan events.AgentEvent, done chan<- []events.AgentEvent) {
	out := make([]events.AgentEvent, 0)
	for ev := range ch {
		out = append(out, ev)
	}
	done <- out
}
