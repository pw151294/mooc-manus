package tools

import (
	"context"
	"encoding/json"
	"testing"

	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
)

// subagentMockTool 实现 Tool 接口用于子智能体测试
type subagentMockTool struct {
	toolNames []string
}

func (m *subagentMockTool) GetTools() []llm.Tool {
	tools := make([]llm.Tool, 0, len(m.toolNames))
	for _, name := range m.toolNames {
		tools = append(tools, llm.Tool{Name: name})
	}
	return tools
}

func (m *subagentMockTool) HasTool(funcName string) bool {
	for _, name := range m.toolNames {
		if name == funcName {
			return true
		}
	}
	return false
}

func (m *subagentMockTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	return models.ToolCallResult{Success: true}
}

func (m *subagentMockTool) InvokeWithContext(
	ctx context.Context, funcName, funcArgs string,
) models.ToolCallResult {
	return models.ToolCallResult{Success: true}
}

func (m *subagentMockTool) Init() error            { return nil }
func (m *subagentMockTool) ProviderName() string   { return "mock" }
func (m *subagentMockTool) SupportsRiskAssessment() bool { return false }

func newTestSubagentTool(baseTools []Tool) *SubagentTool {
	return NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil, // invoker 在参数校验阶段不使用
		baseTools,
		nil, // pendingSink 在参数校验阶段不使用
		"test-msg-id",
		nil, // parentEventCh 在参数校验阶段不使用
		nil, // agentRunner 在参数校验阶段不使用
	)
}

func TestSubagentTool_RejectRecursiveCall(t *testing.T) {
	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead", "bashExec"}}}
	tool := newTestSubagentTool(baseTools)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "do something",
		AllowedTools:    []string{"fileRead", SubagentToolName},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))
	if result.Success {
		t.Fatal("应拒绝包含 dispatchSubagent 的 allowed_tools")
	}
	if result.Message == "" {
		t.Fatal("错误信息不能为空")
	}
}

func TestSubagentTool_RejectInvalidTools(t *testing.T) {
	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead", "bashExec"}}}
	tool := newTestSubagentTool(baseTools)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "do something",
		AllowedTools:    []string{"fileRead", "nonExistentTool"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))
	if result.Success {
		t.Fatal("应拒绝包含未注册工具的 allowed_tools")
	}
}

func TestSubagentTool_RejectEmptyTaskDescription(t *testing.T) {
	baseTools := []Tool{&subagentMockTool{toolNames: []string{"fileRead"}}}
	tool := newTestSubagentTool(baseTools)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "",
		AllowedTools:    []string{"fileRead"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))
	if result.Success {
		t.Fatal("应拒绝空的 task_description")
	}
}

func TestSubagentTool_ValidParamsAccepted(t *testing.T) {
	baseTools := []Tool{
		&subagentMockTool{toolNames: []string{"fileRead", "fileEdit"}},
		&subagentMockTool{toolNames: []string{"bashExec"}},
	}
	parentEventCh := make(chan events.AgentEvent, 100)
	mockRunner := func(ctx context.Context, cfg AgentRunConfig, eventCh chan events.AgentEvent) {
		eventCh <- &events.MessageEvent{Message: "任务完成"}
		close(eventCh)
	}
	tool := NewSubagentTool(
		models.AgentConfig{MaxIterations: 10},
		nil,
		baseTools,
		nil,
		"test-msg-id",
		parentEventCh,
		mockRunner,
	)

	args, _ := json.Marshal(SubagentParams{
		TaskDescription: "读取并修改文件",
		Context:         "在 /tmp 目录下操作",
		AllowedTools:    []string{"fileRead", "bashExec"},
	})
	result := tool.InvokeWithContext(context.Background(), SubagentToolName, string(args))
	if !result.Success {
		t.Fatalf("合法参数应被接受, got: %s", result.Message)
	}
}

func TestSubagentTool_HasTool(t *testing.T) {
	tool := newTestSubagentTool(nil)
	if !tool.HasTool(SubagentToolName) {
		t.Fatal("HasTool 应识别 dispatchSubagent")
	}
	if tool.HasTool("otherTool") {
		t.Fatal("HasTool 不应识别其他工具名")
	}
}

func TestSubagentTool_GetTools(t *testing.T) {
	tool := newTestSubagentTool(nil)
	tools := tool.GetTools()
	if len(tools) != 1 {
		t.Fatalf("GetTools 应返回 1 个工具, got %d", len(tools))
	}
	if tools[0].Name != SubagentToolName {
		t.Fatalf("工具名应为 %s, got %s", SubagentToolName, tools[0].Name)
	}
	if tools[0].Parameters == nil {
		t.Fatal("Parameters 不应为 nil")
	}
}
