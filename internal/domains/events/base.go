package events

import (
	"time"
)

type AgentEvent interface {
	EventId() string
	EventType() string
	SaveConversationId(string)
}

type BaseEvent struct {
	ID             string    `json:"id"`             // 事件ID
	ConversationId string    `json:"conversationId"` // 全局会话ID
	MessageId      string    `json:"messageId"`      // 当前轮次对话ID
	Type           string    `json:"type"`           // 事件类型
	CreatedAt      time.Time `json:"created_at"`     // 事件创建时间
}

func (b *BaseEvent) EventId() string {
	return b.ID
}

func (b *BaseEvent) EventType() string {
	return b.Type
}

func (b *BaseEvent) SaveConversationId(conversationId string) {
	b.ConversationId = conversationId
}
