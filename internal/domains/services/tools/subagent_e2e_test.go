package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/interrupt"
	"mooc-manus/internal/domains/models/llm"
)

// --- E2E 专用 mock ---

// mockPendingSink 实现 interrupt.PendingSink 接口
type mockPendingSink struct {
	called    bool
	messageId string
}

func (m *mockPendingSink) RegisterInterrupt(messageId string, snap interrupt.Snapshot) (<-chan interrupt.Decision, error) {
	m.called = true
	m.messageId = messageId
	ch := make(chan interrupt.Decision, 1)
	ch <- interrupt.Decision{Kind: interrupt.DecisionApprove}
	return ch, nil
}

func (m *mockPendingSink) WaitTimeout() time.Duration {
	return 30 * time.Second
}

// multiMockTool 支持多函数名的 mock Tool
type multiMockTool struct {
	toolNames    []string
	providerName string
}

func (m *multiMockTool) GetTools() []llm.Tool {
	tools := make([]llm.Tool, 0, len(m.toolNames))
	for _, name := range m.toolNames {
		tools = append(tools, llm.Tool{Name: name})
	}
	return tools
}

func (m *multiMockTool) HasTool(funcName string) bool {
	for _, name := range m.toolNames {
		if name == funcName {
			return true
		}
	}
	return false
}

func (m *multiMockTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	return models.ToolCallResult{Success: true}
}

func (m *multiMockTool) InvokeWithContext(ctx context.Context, funcName, funcArgs string) models.ToolCallResult {
	return models.ToolCallResult{Success: true}
}

func (m *multiMockTool) Init() error { return nil }
func (m *multiMockTool) ProviderName() string {
	if m.providerName != "" {
		return m.providerName
	}
	return "multi-mock"
}
func (m *multiMockTool) SupportsRiskAssessment() bool { return false }

// --- 测试场景 ---

// TestSubagentE2E_RecursionRejection 递归检测完整流程
func TestSubagentE2E_RecursionRejection(t *testing.T) {
	// 构造带多个工具的 SubagentTool
	baseTools := []Tool{
		&multiMockTool{toolNames: []string{"fileRead", "fileEdit"}, providerName: "file-provider"},
		&multiMockTool{toolNames: []string{"bashExec"}, providerName: "bash-provider"},
	}
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		eventCh <- &events.MessageEvent{Message: "不应该执行到这里"}
		close(eventCh)
	}

	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-e2e-recursion",
		parentEventCh,
		mockRunner,
	)

	// allowed_tools 包含 dispatchSubagent → 应被拒绝
	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "尝试递归调用",
		AllowedTools:    []string{"fileRead", SubagentToolName, "bashExec"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if result.Success {
		t.Fatal("期望递归调用被拒绝，但返回了 Success=true")
	}

	var subResult SubagentResult
	if err := json.Unmarshal([]byte(result.Data.(string)), &subResult); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if subResult.Success {
		t.Fatal("期望 SubagentResult.Success=false")
	}
	// 错误信息应包含"递归"或 "dispatchSubagent"
	if !strings.Contains(subResult.Error, "递归") && !strings.Contains(subResult.Error, "dispatchSubagent") {
		t.Fatalf("错误信息应包含'递归'或'dispatchSubagent', got: %s", subResult.Error)
	}
}

// TestSubagentE2E_IndependentCircuitBreaker 熔断隔离
// 验证子智能体内部的失败不影响外层调用结果
func TestSubagentE2E_IndependentCircuitBreaker(t *testing.T) {
	baseTools := []Tool{
		&multiMockTool{toolNames: []string{"fileRead"}, providerName: "file-provider"},
	}
	parentEventCh := make(chan events.AgentEvent, 100)

	// mock agentRunner：模拟子智能体执行多次工具调用失败，但最终产出结果
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		// 模拟多次工具调用失败（内部熔断场景）
		for i := 0; i < 5; i++ {
			eventCh <- &events.ToolEvent{
				FunctionName: "fileRead",
				Status:       events.ToolEventStatusFailed,
			}
		}
		// 子智能体内部有错误事件
		eventCh <- &events.ErrorEvent{Error: "子智能体内部工具调用多次失败"}
		// 但最终仍产出了消息
		eventCh <- &events.MessageEvent{Message: "尽管有错误，任务仍部分完成"}
		close(eventCh)
	}

	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-e2e-circuit",
		parentEventCh,
		mockRunner,
	)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "执行可能失败的操作",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	// 关键验证：SubagentTool 本身调用成功返回（即使子智能体内部有错误事件）
	// 因为最终有 MessageEvent 输出，所以外层结果是 Success
	if !result.Success {
		t.Fatalf("子智能体内部失败不应影响外层 SubagentTool 调用结果, got: %v", result.Message)
	}

	var subResult SubagentResult
	if err := json.Unmarshal([]byte(result.Data.(string)), &subResult); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if !subResult.Success {
		t.Fatalf("期望 SubagentResult.Success=true（有最终输出）, error: %s", subResult.Error)
	}
	if subResult.Output != "尽管有错误，任务仍部分完成" {
		t.Fatalf("输出不匹配, got: %s", subResult.Output)
	}
}

// TestSubagentE2E_HITLContext HITL 审批上下文传递
func TestSubagentE2E_HITLContext(t *testing.T) {
	baseTools := []Tool{
		&multiMockTool{toolNames: []string{"bashExec"}, providerName: "bash-provider"},
	}
	parentEventCh := make(chan events.AgentEvent, 100)
	sink := &mockPendingSink{}

	// 用于捕获 agentRunner 收到的配置
	var capturedCfg AgentRunConfig
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		capturedCfg = cfg
		eventCh <- &events.MessageEvent{Message: "HITL 测试完成"}
		close(eventCh)
	}

	mainMessageId := "msg-hitl-001"
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		sink,
		mainMessageId,
		parentEventCh,
		mockRunner,
	)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "需要审批的操作",
		AllowedTools:    []string{"bashExec"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if !result.Success {
		t.Fatalf("期望成功, got: %s", result.Message)
	}

	// 验证 cfg.PendingSink 非 nil
	if capturedCfg.PendingSink == nil {
		t.Fatal("AgentRunner 收到的 cfg.PendingSink 不应为 nil")
	}

	// 验证 cfg.MessageId 格式为 {主messageId}-sub-{uuid}
	if !strings.HasPrefix(capturedCfg.MessageId, mainMessageId+"-sub-") {
		t.Fatalf("MessageId 格式错误, 期望前缀 '%s-sub-', got: %s",
			mainMessageId, capturedCfg.MessageId)
	}
	// 验证 -sub- 后面有内容（uuid 部分）
	suffix := strings.TrimPrefix(capturedCfg.MessageId, mainMessageId+"-sub-")
	if len(suffix) == 0 {
		t.Fatal("MessageId 的 uuid 部分不应为空")
	}
}

// TestSubagentE2E_ToolFilteringCorrectness 工具过滤正确性
func TestSubagentE2E_ToolFilteringCorrectness(t *testing.T) {
	// 构造多个 mock Tool（各含不同的 funcName）
	toolA := &multiMockTool{toolNames: []string{"fileRead"}, providerName: "provider-A"}
	toolB := &multiMockTool{toolNames: []string{"bashExec"}, providerName: "provider-B"}
	toolC := &multiMockTool{toolNames: []string{"webSearch"}, providerName: "provider-C"}
	baseTools := []Tool{toolA, toolB, toolC}
	parentEventCh := make(chan events.AgentEvent, 100)

	var capturedCfg AgentRunConfig
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		capturedCfg = cfg
		eventCh <- &events.MessageEvent{Message: "过滤测试完成"}
		close(eventCh)
	}

	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-e2e-filter",
		parentEventCh,
		mockRunner,
	)

	// allowed_tools 只包含部分工具名
	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "只用文件和搜索",
		AllowedTools:    []string{"fileRead", "webSearch"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if !result.Success {
		t.Fatalf("期望成功, got: %s", result.Message)
	}

	// 验证收到的 cfg.Tools 只包含被请求的工具 provider
	if len(capturedCfg.Tools) != 2 {
		t.Fatalf("期望过滤后有 2 个 Tool provider, got %d", len(capturedCfg.Tools))
	}

	// 验证包含 provider-A 和 provider-C，不包含 provider-B
	providerNames := make(map[string]bool)
	for _, tl := range capturedCfg.Tools {
		providerNames[tl.ProviderName()] = true
	}
	if !providerNames["provider-A"] {
		t.Fatal("期望包含 provider-A (fileRead)")
	}
	if !providerNames["provider-C"] {
		t.Fatal("期望包含 provider-C (webSearch)")
	}
	if providerNames["provider-B"] {
		t.Fatal("不应包含 provider-B (bashExec)")
	}
}

// TestSubagentE2E_MultipleToolsProvider 多函数 Provider 场景
// 一个 provider 包含多个函数，allowed_tools 只包含其中一个，验证整个 provider 被包含
func TestSubagentE2E_MultipleToolsProvider(t *testing.T) {
	// 一个 mock Tool provider 包含多个函数
	multiProvider := &multiMockTool{
		toolNames:    []string{"fileRead", "fileEdit"},
		providerName: "file-provider",
	}
	singleProvider := &multiMockTool{
		toolNames:    []string{"bashExec"},
		providerName: "bash-provider",
	}
	baseTools := []Tool{multiProvider, singleProvider}
	parentEventCh := make(chan events.AgentEvent, 100)

	var capturedCfg AgentRunConfig
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		capturedCfg = cfg
		eventCh <- &events.MessageEvent{Message: "多函数 provider 测试完成"}
		close(eventCh)
	}

	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"msg-e2e-multi-provider",
		parentEventCh,
		mockRunner,
	)

	// allowed_tools 只包含 fileRead（file-provider 的一个函数）
	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "只请求 fileRead",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))

	if !result.Success {
		t.Fatalf("期望成功, got: %s", result.Message)
	}

	// 验证整个 file-provider 被包含在子智能体工具集中
	if len(capturedCfg.Tools) != 1 {
		t.Fatalf("期望 1 个 Tool provider, got %d", len(capturedCfg.Tools))
	}
	if capturedCfg.Tools[0].ProviderName() != "file-provider" {
		t.Fatalf("期望 provider 为 file-provider, got: %s",
			capturedCfg.Tools[0].ProviderName())
	}
	// 验证该 provider 仍包含所有函数（fileRead + fileEdit）
	providerTools := capturedCfg.Tools[0].GetTools()
	if len(providerTools) != 2 {
		t.Fatalf("期望 provider 包含 2 个函数, got %d", len(providerTools))
	}
}
