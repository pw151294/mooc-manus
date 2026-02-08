package events

import (
	"mooc-manus/internal/domains/models"
	"time"

	"github.com/google/uuid"
)

func OnMessage(content string, attachments []models.File) AgentEvent {
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
	messageEvent.Type = EventTypeMessageEnd
	messageEvent.CreatedAt = time.Now()
	messageEvent.Timestamp = time.Now()
	messageEvent.Role = "assistant"

	return &messageEvent
}
