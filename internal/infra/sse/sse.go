package sse

import (
	"encoding/json"
	"fmt"
	"mooc-manus/pkg/logger"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// EventHandleProtocol is a struct for handling SSE communication.
type EventHandleProtocol struct {
	timeout time.Duration
	writer  http.ResponseWriter
	flusher http.Flusher
	sync.Mutex
}

// NewEventHandleProtocol creates a new instance of EventHandleProtocol with a timeout.
func NewEventHandleProtocol(w http.ResponseWriter, timeout time.Duration) *EventHandleProtocol {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("ResponseWriter does not support flushing")
		panic("ResponseWriter does not support flushing")
	}

	logger.Info("SSE connection initialized", zap.Duration("timeout", timeout))
	return &EventHandleProtocol{
		timeout: timeout,
		writer:  w,
		flusher: flusher,
	}
}

// SendEvent sends a structured event with a name and data object to the client.
// This mimics Java's sseEmitter.send(SseEmitter.event().name(eventName).data(data))
func (s *EventHandleProtocol) SendEvent(eventName string, data interface{}) {
	s.Lock()
	defer s.Unlock()

	// Marshal data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal event data", zap.Error(err))
		return
	}

	// Write event name if provided
	if eventName != "" {
		_, err = fmt.Fprintf(s.writer, "event: %s\n", eventName)
		if err != nil {
			logger.Error("Failed to send event name", zap.Error(err))
			return
		}
	}

	// Write data and end with double newline
	_, err = fmt.Fprintf(s.writer, "data: %s\n\n", jsonData)
	if err != nil {
		logger.Error("Failed to send event data", zap.Error(err))
		return
	}

	s.flusher.Flush()
}

// Close closes the SSE connection.
func (s *EventHandleProtocol) Close() {
	logger.Info("SSE connection closed")
}

// Error sends an error message and closes the connection.
func (s *EventHandleProtocol) Error(err error) {
	// Use SendEvent to correctly format the error event
	s.SendEvent("Error", err.Error())
	s.Close()
	logger.Error("SSE connection closed with error", zap.Error(err))
}

// ContentType returns the content type for SSE.
func (s *EventHandleProtocol) ContentType() string {
	return "text/event-stream"
}
