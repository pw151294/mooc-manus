package memory

import (
	"testing"

	"mooc-manus/internal/domains/models/llm"
)

func TestChatMemory_AddAndGetMessages(t *testing.T) {
	m := NewChatMemory()
	m.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	m.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "hello"})

	msgs := m.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" || msgs[1].Content != "hello" {
		t.Fatalf("messages content mismatch: %+v", msgs)
	}
}

func TestChatMemory_Compact_RemovesBrowserToolResult(t *testing.T) {
	m := NewChatMemory()
	// assistant 发起对 browser 工具的调用
	m.AddMessage(llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID: "tc-1", Name: "browser", Arguments: "{}",
		}},
	})
	// tool 返回的网页内容
	m.AddMessage(llm.Message{
		Role:       llm.RoleTool,
		Content:    "<html>...long page...</html>",
		ToolCallID: "tc-1",
	})

	m.Compact()

	msgs := m.GetMessages()
	var toolMsg *llm.Message
	for i := range msgs {
		if msgs[i].Role == llm.RoleTool {
			toolMsg = &msgs[i]
		}
	}
	if toolMsg == nil {
		t.Fatalf("expected tool message preserved after compact")
	}
	if toolMsg.Content != "removed" {
		t.Fatalf("expected browser tool content compacted to 'removed', got %q", toolMsg.Content)
	}
}

func TestChatMemory_Compact_KeepsOtherToolResults(t *testing.T) {
	m := NewChatMemory()
	m.AddMessage(llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID: "tc-2", Name: "calculator", Arguments: "{}",
		}},
	})
	m.AddMessage(llm.Message{
		Role:       llm.RoleTool,
		Content:    "42",
		ToolCallID: "tc-2",
	})

	m.Compact()

	msgs := m.GetMessages()
	var toolMsg *llm.Message
	for i := range msgs {
		if msgs[i].Role == llm.RoleTool {
			toolMsg = &msgs[i]
		}
	}
	if toolMsg == nil || toolMsg.Content != "42" {
		t.Fatalf("non-browser/search tool result should remain intact, got %+v", toolMsg)
	}
}
