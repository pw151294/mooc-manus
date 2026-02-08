package events

import (
	"time"

	"github.com/google/uuid"
)

func OnWait() AgentEvent {
	event := WaitEvent{}
	event.ID = uuid.New().String()
	event.Type = EventTypeWait
	event.CreatedAt = time.Now()
	return &event
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

func OnDone() AgentEvent {
	event := DoneEvent{}
	event.ID = uuid.New().String()
	event.Type = EventTypeDone
	event.CreatedAt = time.Now()
	event.Timestamp = time.Now()
	return &event
}
