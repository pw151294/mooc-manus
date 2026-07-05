package sse

import (
	"mooc-manus/internal/domains/models/events"
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
	// messageId2SseEmitter：单条 SSE 连接的正向索引
	messageId2SseEmitter map[string]*EventHandleProtocol
	// messageId2ConversationId：反向索引，用于 CloseChat 时同步清理会话侧
	messageId2ConversationId map[string]string
	// conversationId2MessageIds：会话 → 活跃 messageId 集合，供 StopConversation 使用
	conversationId2MessageIds map[string]map[string]struct{}
}

// StartChat 注册一条新的 SSE 连接
// conversationId 可为空（历史调用点尚未适配时）；空 conversationId 不会建立反向索引
func StartChat(w http.ResponseWriter, conversationId string) string {
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

	if conversationId != "" {
		manager.messageId2ConversationId[messageId] = conversationId
		mids, ok := manager.conversationId2MessageIds[conversationId]
		if !ok {
			mids = make(map[string]struct{})
			manager.conversationId2MessageIds[conversationId] = mids
		}
		mids[messageId] = struct{}{}
	}

	logger.Info("SSE connection initialized",
		zap.String("messageId", messageId),
		zap.String("conversationId", conversationId))
	return messageId
}

// CloseChat 关闭指定 messageId 的 SSE 连接
// - 触发 emitter.Close 标记 aborted，后续 SendEvent 会被 drop
// - 从正/反向映射中移除；会话侧集合为空时同步删掉
// 与 StopMessage / defer 清理链复用
func CloseChat(messageId string) {
	manager.Lock()
	defer manager.Unlock()

	sseEmitter, ok := manager.messageId2SseEmitter[messageId]
	if !ok {
		return
	}
	sseEmitter.Close()
	delete(manager.messageId2SseEmitter, messageId)

	if conversationId, ok := manager.messageId2ConversationId[messageId]; ok {
		delete(manager.messageId2ConversationId, messageId)
		if mids, ok := manager.conversationId2MessageIds[conversationId]; ok {
			delete(mids, messageId)
			if len(mids) == 0 {
				delete(manager.conversationId2MessageIds, conversationId)
			}
		}
	}
}

// HasMessage 判断 messageId 是否仍有活跃 SSE 连接（未被 CloseChat）
// 供 StopMessage 上报 cleaned.sse 精确取值
func HasMessage(messageId string) bool {
	manager.Lock()
	defer manager.Unlock()

	_, ok := manager.messageId2SseEmitter[messageId]
	return ok
}

// MessageIdsOf 返回 conversationId 下所有活跃 messageId
// 供 StopConversation 遍历清理；返回快照，调用方可自由迭代
func MessageIdsOf(conversationId string) []string {
	manager.Lock()
	defer manager.Unlock()

	mids, ok := manager.conversationId2MessageIds[conversationId]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(mids))
	for mid := range mids {
		out = append(out, mid)
	}
	return out
}

func SendEvent(event events.AgentEvent, messageId string) {
	manager.Lock()
	sseEmitter, ok := manager.messageId2SseEmitter[messageId]
	manager.Unlock()
	if !ok {
		logger.Warn("SendEvent: messageId not found (possibly aborted)", zap.String("messageId", messageId))
		return
	}
	sseEmitter.SendEvent(event.EventType(), event)
}

func init() {
	manager = &SseEmitterManager{
		messageId2SseEmitter:      make(map[string]*EventHandleProtocol),
		messageId2ConversationId:  make(map[string]string),
		conversationId2MessageIds: make(map[string]map[string]struct{}),
	}
}
