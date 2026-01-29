package events

import (
	"time"

	"github.com/google/uuid"
)

type File struct {
	ID        string
	FileName  string
	FilePath  string
	Key       string
	Extension string
	MimeType  string
	Size      int
}

type WaitEvent struct {
	BaseEvent
}

func OnWait() AgentEvent {
	event := WaitEvent{}
	event.ID = uuid.New().String()
	event.Type = EventTypeWait
	event.CreatedAt = time.Now()
	return &event
}

type ErrorEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error"` // 错误信息
}

func OnError(errorMsg string) AgentEvent {
	errorEvent := ErrorEvent{}
	errorEvent.ID = uuid.New().String()
	errorEvent.Type = EventTypeError
	errorEvent.CreatedAt = time.Now()
	errorEvent.Timestamp = time.Now()
	errorEvent.Error = errorMsg
	return &errorEvent
}

type DoneEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
}

func OnDone() AgentEvent {
	event := DoneEvent{}
	event.ID = uuid.New().String()
	event.Type = EventTypeDone
	event.CreatedAt = time.Now()
	event.Timestamp = time.Now()
	return &event
}

type TitleEvent struct {
	BaseEvent
	Timestamp time.Time `json:"timestamp"`
	Title     string    `json:"title"` // 标题
}
