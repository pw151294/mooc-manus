package sse

import (
	"mooc-manus/internal/domains/events"
	"mooc-manus/pkg/logger"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const sseTimeout = 60

var manager *SseEmitterManager

type SseEmitterManager struct {
	sync.Mutex
	messageId2SseEmitter map[string]*EventHandleProtocol
}

func StartChat(w http.ResponseWriter) string {
	manager.Lock()
	defer manager.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter does not support flushing")
		panic("ResponseWriter does not support flushing")
	}

	sseEmitter := &EventHandleProtocol{}
	sseEmitter.timeout = sseTimeout * time.Minute
	sseEmitter.writer = w
	sseEmitter.flusher = flusher
	messageId := uuid.New().String()
	manager.messageId2SseEmitter[messageId] = sseEmitter
	logger.Info("SSE connection initialized", zap.String("messageId", messageId))
	return messageId
}

func CloseChat(messageId string) {
	manager.Lock()
	defer manager.Unlock()

	if sseEmitter, ok := manager.messageId2SseEmitter[messageId]; ok {
		sseEmitter.Close()
		delete(manager.messageId2SseEmitter, messageId)
	}
}

func SendEvent(event events.AgentEvent, messageId string) {
	if sseEmitter, ok := manager.messageId2SseEmitter[messageId]; !ok {
		logger.Error("SendEvent: not found messageId", zap.String("messageId", messageId))
	} else {
		sseEmitter.SendEvent(event.EventType(), event)
	}
}

func init() {
	manager = &SseEmitterManager{
		messageId2SseEmitter: make(map[string]*EventHandleProtocol),
	}
}
