package agents

import (
	"os"
	"strings"
	"testing"

	"mooc-manus/config"
	"mooc-manus/internal/domains/models"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/memory"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger,避免 InvokeToolCalls 里的 logger.Info 触发 nil 解引用
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "agents-test-log-*")
	_ = logger.InitGlobalLogger(config.LoggerConfig{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		LogDir: tmpLogDir,
	})
	code := m.Run()
	_ = os.RemoveAll(tmpLogDir)
	os.Exit(code)
}

// fakeNativeTool 用于测试 InvokeToolCalls 里的 error_recovery Hook
// 通过 provider name 假装成 NATIVE 类工具,funcName 由构造时给定
type fakeNativeTool struct {
	funcName string
	result   models.ToolCallResult
	calls    int
}

func (f *fakeNativeTool) GetTools() []llm.Tool {
	return []llm.Tool{{Name: f.funcName, Description: "fake"}}
}

func (f *fakeNativeTool) HasTool(name string) bool { return name == f.funcName }

func (f *fakeNativeTool) Invoke(name, args string) models.ToolCallResult {
	f.calls++
	return f.result
}

func (f *fakeNativeTool) Init() error { return nil }

func (f *fakeNativeTool) ProviderName() string { return "native-provider" }

// drainEvents 起 goroutine 消费 eventCh,防止 send 阻塞;返回收集的事件列表 getter
func drainEvents(ch chan events.AgentEvent) func() []events.AgentEvent {
	collected := make([]events.AgentEvent, 0)
	done := make(chan struct{})
	go func() {
		for ev := range ch {
			collected = append(collected, ev)
		}
		close(done)
	}()
	return func() []events.AgentEvent {
		<-done
		return collected
	}
}

func newAgentWithTool(t *tools.Tool, tool tools.Tool) *BaseAgent {
	return &BaseAgent{
		agentConfig: models.AgentConfig{MaxIterations: 3, MaxRetries: 1},
		memory:      memory.NewChatMemory(),
		tools:       []tools.Tool{tool},
	}
}

func TestInvokeToolCalls_L1SelfHealAppended(t *testing.T) {
	tool := &fakeNativeTool{
		funcName: "fileWrite",
		result: models.ToolCallResult{
			Success: false,
			Message: "Error: 打开文件失败: open /workspace/a/b.txt: no such file or directory",
		},
	}
	agent := newAgentWithTool(nil, tool)
	ch := make(chan events.AgentEvent, 16)
	getEvents := drainEvents(ch)

	toolCalls := []llm.ToolCall{{ID: "c1", Name: "fileWrite", Arguments: `{"path":"a/b.txt","content":"x"}`}}
	msgs, abort := agent.InvokeToolCalls(toolCalls, ch)
	close(ch)
	getEvents()

	if abort {
		t.Fatalf("L1 场景不应触发 abort")
	}
	if len(msgs) != 1 {
		t.Fatalf("应返回 1 条 ToolMessage, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "[ErrorRecovery-L1]") {
		t.Fatalf("L1 模板未注入,content=%s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "工具调用失败：") {
		t.Fatalf("原失败消息应保留,content=%s", msgs[0].Content)
	}
}

func TestInvokeToolCalls_L2AskUserAppended(t *testing.T) {
	tool := &fakeNativeTool{
		funcName: "fileRead",
		result: models.ToolCallResult{
			Success: false,
			Message: "Error: path parameter is required",
		},
	}
	agent := newAgentWithTool(nil, tool)
	ch := make(chan events.AgentEvent, 16)
	getEvents := drainEvents(ch)

	toolCalls := []llm.ToolCall{{ID: "c1", Name: "fileRead", Arguments: `{}`}}
	msgs, abort := agent.InvokeToolCalls(toolCalls, ch)
	close(ch)
	getEvents()

	if abort {
		t.Fatalf("L2 场景不应触发 abort")
	}
	if !strings.Contains(msgs[0].Content, "[ErrorRecovery-L2]") {
		t.Fatalf("L2 模板未注入,content=%s", msgs[0].Content)
	}
}

func TestInvokeToolCalls_L3FatalAbort(t *testing.T) {
	tool := &fakeNativeTool{
		funcName: "bashExec",
		result: models.ToolCallResult{
			Success: false,
			Message: "Error: 命令被拒绝:命中黑名单模式 rm-rf-root",
		},
	}
	agent := newAgentWithTool(nil, tool)
	ch := make(chan events.AgentEvent, 16)
	getEvents := drainEvents(ch)

	toolCalls := []llm.ToolCall{{ID: "c1", Name: "bashExec", Arguments: `{"command":"rm -rf /","description":"test"}`}}
	msgs, abort := agent.InvokeToolCalls(toolCalls, ch)
	close(ch)
	getEvents()

	if !abort {
		t.Fatalf("L3 场景必须触发 abort")
	}
	if !strings.Contains(msgs[0].Content, "[ErrorRecovery-L3]") {
		t.Fatalf("L3 模板未注入,content=%s", msgs[0].Content)
	}
}

func TestInvokeToolCalls_SuccessNoHook(t *testing.T) {
	tool := &fakeNativeTool{
		funcName: "fileRead",
		result:   models.ToolCallResult{Success: true, Data: "hello"},
	}
	agent := newAgentWithTool(nil, tool)
	ch := make(chan events.AgentEvent, 16)
	getEvents := drainEvents(ch)

	toolCalls := []llm.ToolCall{{ID: "c1", Name: "fileRead", Arguments: `{"path":"x"}`}}
	msgs, abort := agent.InvokeToolCalls(toolCalls, ch)
	close(ch)
	getEvents()

	if abort {
		t.Fatalf("成功场景不应 abort")
	}
	if strings.Contains(msgs[0].Content, "[ErrorRecovery-") {
		t.Fatalf("成功场景不应注入 ErrorRecovery 模板")
	}
}

func TestInvokeToolCalls_NonNativeToolNoHook(t *testing.T) {
	// 非 4 类原生工具的失败:不注入 error_recovery 模板
	tool := &fakeNativeTool{
		funcName: "loadSkill",
		result: models.ToolCallResult{
			Success: false,
			Message: "Error: 文件不存在: xxx",
		},
	}
	agent := newAgentWithTool(nil, tool)
	ch := make(chan events.AgentEvent, 16)
	getEvents := drainEvents(ch)

	toolCalls := []llm.ToolCall{{ID: "c1", Name: "loadSkill", Arguments: `{"skillName":"x"}`}}
	msgs, abort := agent.InvokeToolCalls(toolCalls, ch)
	close(ch)
	getEvents()

	if abort {
		t.Fatalf("非原生工具失败不应 abort")
	}
	if strings.Contains(msgs[0].Content, "[ErrorRecovery-") {
		t.Fatalf("非原生工具失败不应注入 ErrorRecovery 模板,content=%s", msgs[0].Content)
	}
}
