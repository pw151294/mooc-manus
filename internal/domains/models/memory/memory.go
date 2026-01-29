package memory

import (
	"slices"

	"github.com/openai/openai-go"
)

type ChatMemory struct {
	messages        []openai.ChatCompletionMessageParamUnion
	toolCallId2Name map[string]string
}

func NewChatMemory() *ChatMemory {
	return &ChatMemory{
		messages:        make([]openai.ChatCompletionMessageParamUnion, 0),
		toolCallId2Name: make(map[string]string),
	}
}

func (c *ChatMemory) GetMessageRole(message openai.ChatCompletionMessageParamUnion) string {
	return *message.GetRole()
}
func (c *ChatMemory) AddMessage(message openai.ChatCompletionMessageParamUnion) {
	c.recordToolCalls(message)
	c.messages = append(c.messages, message)
}
func (c *ChatMemory) AddMessages(messages []openai.ChatCompletionMessageParamUnion) {
	if len(messages) == 0 {
		return
	}
	for _, message := range messages {
		c.recordToolCalls(message)
	}
	c.messages = append(c.messages, messages...)
}
func (c *ChatMemory) GetMessages() []openai.ChatCompletionMessageParamUnion {
	return c.messages
}
func (c *ChatMemory) GetLastMessage() *openai.ChatCompletionMessageParamUnion {
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
func (c *ChatMemory) Compact() { // 记忆压缩 将记忆中已经执行的工具（搜索/网页源码获取/浏览器访问结果等）这类已经执行过的消息进行压缩检索
	for _, message := range c.messages {
		if *message.GetRole() != "tools" {
			continue
		}
		toolCallID := *message.GetToolCallID()
		funcName := c.toolCallId2Name[toolCallID]
		if slices.Contains([]string{"browser", "bing_search"}, funcName) {
			message.OfTool.Content.OfString = openai.String("removed") // 从记忆中移除和搜索/网页源码获取等工具的message
		}
	}
}
func (c *ChatMemory) IsEmpty() bool {
	return len(c.messages) == 0
}
func (c *ChatMemory) recordToolCalls(message openai.ChatCompletionMessageParamUnion) {
	if *message.GetRole() != "assistant" {
		return // 不是assistant消息肯定没有toolCall信息
	}
	// 记录工具调用的ID和function之间的映射关系
	toolCalls := message.GetToolCalls()
	for _, toolCall := range toolCalls {
		toolCallId := toolCall.ID
		funcName := toolCall.Function.Name
		c.toolCallId2Name[toolCallId] = funcName
	}
}
