package llm

// 测试 assertion 采用 union 字段非空判断(OfSystem/OfUser/OfAssistant/OfTool)而非 GetRole() 调用,
// 因为 openai-go v1.12.0 的 GetRole() 返回的是零值常量字段指针,不能用于区分角色。
// 字段非空验证 + 关键内容字段值 = 等价覆盖 plan 模板的测试意图。

import (
	"testing"

	"github.com/openai/openai-go"

	domainllm "mooc-manus/internal/domains/models/llm"
	"mooc-manus/internal/domains/models/invoker"
)

func TestToOpenAIMessages_System(t *testing.T) {
	msgs := []domainllm.Message{{Role: domainllm.RoleSystem, Content: "hello"}}
	out := toOpenAIMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	if out[0].OfSystem == nil {
		t.Fatalf("want OfSystem set for system role")
	}
	c := out[0].GetContent()
	s, ok := c.AsAny().(*string)
	if !ok || s == nil || *s != "hello" {
		t.Fatalf("want content=hello, got %v", c.AsAny())
	}
}

func TestToOpenAIMessages_User(t *testing.T) {
	msgs := []domainllm.Message{{Role: domainllm.RoleUser, Content: "q"}}
	out := toOpenAIMessages(msgs)
	if out[0].OfUser == nil {
		t.Fatalf("want OfUser set for user role")
	}
}

func TestToOpenAIMessages_AssistantWithToolCalls(t *testing.T) {
	msgs := []domainllm.Message{{
		Role:    domainllm.RoleAssistant,
		Content: "",
		ToolCalls: []domainllm.ToolCall{
			{ID: "tc-1", Name: "search", Arguments: `{"q":"go"}`},
		},
	}}
	out := toOpenAIMessages(msgs)
	if out[0].OfAssistant == nil {
		t.Fatalf("want OfAssistant set for assistant role")
	}
	tcs := out[0].GetToolCalls()
	if len(tcs) != 1 || tcs[0].ID != "tc-1" || tcs[0].Function.Name != "search" {
		t.Fatalf("tool calls mapping failed: %+v", tcs)
	}
}

func TestToOpenAIMessages_Tool(t *testing.T) {
	msgs := []domainllm.Message{{
		Role:       domainllm.RoleTool,
		Content:    "result",
		ToolCallID: "tc-1",
	}}
	out := toOpenAIMessages(msgs)
	if out[0].OfTool == nil {
		t.Fatalf("want OfTool set for tool role")
	}
	if id := out[0].GetToolCallID(); id == nil || *id != "tc-1" {
		t.Fatalf("want tool_call_id=tc-1, got %v", id)
	}
}

func TestToOpenAITools(t *testing.T) {
	tools := []domainllm.Tool{{
		Name:        "search",
		Description: "search the web",
		Parameters:  map[string]any{"type": "object"},
	}}
	out := toOpenAITools(tools)
	if len(out) != 1 || out[0].Function.Name != "search" {
		t.Fatalf("tool mapping failed: %+v", out)
	}
}

func TestFromOpenAIMessage_TextOnly(t *testing.T) {
	in := openai.ChatCompletionMessage{Content: "hi", Role: "assistant"}
	out := fromOpenAIMessage(in)
	if out.Role != domainllm.RoleAssistant {
		t.Fatalf("want assistant, got %s", out.Role)
	}
	if out.Content != "hi" {
		t.Fatalf("want content=hi, got %s", out.Content)
	}
	if len(out.ToolCalls) != 0 {
		t.Fatalf("want no tool calls")
	}
}

func TestFromOpenAIMessage_WithToolCalls(t *testing.T) {
	in := openai.ChatCompletionMessage{
		Role: "assistant",
		ToolCalls: []openai.ChatCompletionMessageToolCall{
			{
				ID: "tc-1",
				Function: openai.ChatCompletionMessageToolCallFunction{
					Name:      "search",
					Arguments: `{"q":"go"}`,
				},
			},
		},
	}
	out := fromOpenAIMessage(in)
	if len(out.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(out.ToolCalls))
	}
	tc := out.ToolCalls[0]
	if tc.ID != "tc-1" || tc.Name != "search" || tc.Arguments != `{"q":"go"}` {
		t.Fatalf("tool call mapping failed: %+v", tc)
	}
}

// 编译期断言：两个适配器都实现了 invoker.Invoker
var (
	_ invoker.Invoker = (*OpenAIAdapter)(nil)
	_ invoker.Invoker = (*AnthropicAdapter)(nil)
)
