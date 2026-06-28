package memory

import (
	"slices"

	"mooc-manus/internal/domains/models/llm"
)

type ChatMemory struct {
	messages        []llm.Message
	toolCallId2Name map[string]string
}

func NewChatMemory() *ChatMemory {
	return &ChatMemory{
		messages:        make([]llm.Message, 0),
		toolCallId2Name: make(map[string]string),
	}
}

func (c *ChatMemory) AddMessage(message llm.Message) {
	c.recordToolCalls(message)
	c.messages = append(c.messages, message)
}

func (c *ChatMemory) AddMessages(messages []llm.Message) {
	if len(messages) == 0 {
		return
	}
	for _, message := range messages {
		c.recordToolCalls(message)
	}
	c.messages = append(c.messages, messages...)
}

func (c *ChatMemory) GetMessages() []llm.Message { return c.messages }

func (c *ChatMemory) GetLastMessage() *llm.Message {
	if len(c.messages) == 0 {
		return nil
	}
	return &c.messages[len(c.messages)-1]
}

func (c *ChatMemory) Rollback() {
	if len(c.messages) > 0 {
		c.messages = c.messages[:len(c.messages)-1]
	}
}

// Compact 把已经被消费的 browser / bing_search 工具结果替换成 "removed"
// 修正：原实现误判 role == "tools"（复数），导致循环体永不执行；新实现使用 llm.RoleTool。
func (c *ChatMemory) Compact() {
	for i := range c.messages {
		if c.messages[i].Role != llm.RoleTool {
			continue
		}
		funcName := c.toolCallId2Name[c.messages[i].ToolCallID]
		if slices.Contains([]string{"browser", "bing_search"}, funcName) {
			c.messages[i].Content = "removed"
		}
	}
}

func (c *ChatMemory) IsEmpty() bool { return len(c.messages) == 0 }

func (c *ChatMemory) recordToolCalls(message llm.Message) {
	if message.Role != llm.RoleAssistant {
		return
	}
	for _, tc := range message.ToolCalls {
		c.toolCallId2Name[tc.ID] = tc.Name
	}
}
