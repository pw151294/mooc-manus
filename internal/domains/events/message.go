package events

import (
	"time"

	"github.com/google/uuid"
)

type MessageEvent struct {
	BaseEvent
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	Role        string    `json:"role"`        // 消息角色: "user" 或 "assistant"
	Message     string    `json:"message"`     // 消息本身
	Attachments []File    `json:"attachments"` // 附件列表信息
}

func OnMessage(content string, attachments []File) AgentEvent {
	messageEvent := MessageEvent{}
	messageEvent.ID = uuid.New().String()
	messageEvent.Type = EventTypeMessage
	messageEvent.CreatedAt = time.Now()
	messageEvent.Timestamp = time.Now()
	messageEvent.Role = "assistant"
	messageEvent.Message = content
	messageEvent.Attachments = attachments

	return &messageEvent
}

func OnMessageEnd() AgentEvent {
	messageEvent := MessageEvent{}
	messageEvent.ID = uuid.New().String()
	messageEvent.Type = EventTypeMessage
	messageEvent.CreatedAt = time.Now()
	messageEvent.Timestamp = time.Now()
	messageEvent.Role = "assistant"

	return &messageEvent
}
