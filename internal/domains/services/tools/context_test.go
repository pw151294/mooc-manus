package tools

import (
	"context"
	"testing"
	"time"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/llm"
)

// mockTool 用于测试 InvokeWithContext 默认行为
type mockTool struct {
	BaseTool
	invoked  bool
	sleepDur time.Duration
}

func (m *mockTool) Init() error { return nil }

func (m *mockTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	m.invoked = true
	if m.sleepDur > 0 {
		time.Sleep(m.sleepDur)
	}
	return models.ToolCallResult{
		Success: true,
		Message: "ok",
		Data:    funcName + ":" + funcArgs,
	}
}

func (m *mockTool) InvokeWithContext(ctx context.Context, funcName, funcArgs string) models.ToolCallResult {
	select {
	case <-ctx.Done():
		return models.ToolCallResult{
			Success: false,
			Message: "context cancelled: " + ctx.Err().Error(),
		}
	default:
		return m.Invoke(funcName, funcArgs)
	}
}

func (m *mockTool) GetTools() []llm.Tool { return nil }

// TestInvokeWithContext_DefaultDelegatesToInvoke 验证默认实现正确委托到 Invoke
func TestInvokeWithContext_DefaultDelegatesToInvoke(t *testing.T) {
	m := &mockTool{}
	var tool Tool = m

	result := tool.InvokeWithContext(context.Background(), "testFunc", `{"key":"val"}`)
	if !result.Success {
		t.Fatalf("expected success, got failure: %s", result.Message)
	}
	if !m.invoked {
		t.Fatal("expected Invoke to be called")
	}
	expected := "testFunc:{\"key\":\"val\"}"
	if result.Data != expected {
		t.Fatalf("expected Data=%q, got %q", expected, result.Data)
	}
}

// TestInvokeWithContext_CancelledContext 验证已取消的 context 能被正确响应
func TestInvokeWithContext_CancelledContext(t *testing.T) {
	m := &mockTool{}
	var tool Tool = m

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	result := tool.InvokeWithContext(ctx, "testFunc", "{}")
	if result.Success {
		t.Fatal("expected failure due to cancelled context")
	}
	if result.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

// TestInvokeWithContext_Timeout 验证超时 context 能被正确响应
func TestInvokeWithContext_Timeout(t *testing.T) {
	m := &mockTool{sleepDur: 200 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// 等待 context 超时
	<-ctx.Done()

	result := m.InvokeWithContext(ctx, "slowFunc", "{}")
	if result.Success {
		t.Fatal("expected failure due to timeout")
	}
	if result.Message == "" {
		t.Fatal("expected non-empty error message on timeout")
	}
}
