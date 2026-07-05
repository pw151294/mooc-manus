package sse

import (
	"bytes"
	"net/http"
	"os"
	"sync"
	"testing"

	"mooc-manus/config"
	"mooc-manus/internal/domains/models/events"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger，让 StartChat / CloseChat 里的 logger 调用不再 panic
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "sse-manager-test-log-*")
	_ = logger.InitGlobalLogger(config.LoggerConfig{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		LogDir: tmpLogDir,
	})
	code := m.Run()
	_ = os.RemoveAll(tmpLogDir)
	os.Exit(code)
}

// fakeResponseWriter 同时实现 http.ResponseWriter 与 http.Flusher，供 SSE 测试使用
type fakeResponseWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	header http.Header
	status int
}

func newFakeWriter() *fakeResponseWriter {
	return &fakeResponseWriter{header: make(http.Header)}
}

func (w *fakeResponseWriter) Header() http.Header {
	return w.header
}

func (w *fakeResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *fakeResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *fakeResponseWriter) Flush() {}

func (w *fakeResponseWriter) body() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// resetManager 每个用例前重置全局 manager，避免用例间脏数据
func resetManager() {
	manager = &SseEmitterManager{
		messageId2SseEmitter:      make(map[string]*EventHandleProtocol),
		messageId2ConversationId:  make(map[string]string),
		conversationId2MessageIds: make(map[string]map[string]struct{}),
	}
}

func TestStartChatBuildsBidirectionalIndex(t *testing.T) {
	resetManager()

	mid := StartChat(newFakeWriter(), "conv-1")
	if mid == "" {
		t.Fatal("StartChat returned empty messageId")
	}
	if !HasMessage(mid) {
		t.Fatal("HasMessage should be true right after StartChat")
	}
	mids := MessageIdsOf("conv-1")
	if len(mids) != 1 || mids[0] != mid {
		t.Fatalf("MessageIdsOf should return [%s], got %v", mid, mids)
	}
}

func TestStartChatEmptyConversationIdSkipsReverseIndex(t *testing.T) {
	resetManager()

	mid := StartChat(newFakeWriter(), "")
	if !HasMessage(mid) {
		t.Fatal("HasMessage should still be true for empty conversationId")
	}
	if got := MessageIdsOf(""); len(got) != 0 {
		t.Fatalf("MessageIdsOf(\"\") should be empty, got %v", got)
	}
}

func TestCloseChatCleansBothIndexes(t *testing.T) {
	resetManager()

	mid := StartChat(newFakeWriter(), "conv-2")
	CloseChat(mid)

	if HasMessage(mid) {
		t.Fatal("HasMessage should be false after CloseChat")
	}
	if got := MessageIdsOf("conv-2"); len(got) != 0 {
		t.Fatalf("MessageIdsOf should be empty after last messageId closed, got %v", got)
	}
}

func TestCloseChatKeepsOtherMessagesInConversation(t *testing.T) {
	resetManager()

	m1 := StartChat(newFakeWriter(), "conv-3")
	m2 := StartChat(newFakeWriter(), "conv-3")
	CloseChat(m1)

	mids := MessageIdsOf("conv-3")
	if len(mids) != 1 || mids[0] != m2 {
		t.Fatalf("only m2 should remain, got %v", mids)
	}
}

func TestCloseChatIdempotent(t *testing.T) {
	resetManager()

	mid := StartChat(newFakeWriter(), "conv-4")
	CloseChat(mid)
	// 第二次 CloseChat 不能 panic 也不能报错
	CloseChat(mid)
	CloseChat("never-existed")
}

// TestSendEventAfterAbortIsNoop 验证 Abort 之后 SendEvent 不再写入 ResponseWriter
// 这是防 broken pipe 的关键路径：CloseChat → emitter.Close → aborted=true → SendEvent drop
func TestSendEventAfterAbortIsNoop(t *testing.T) {
	resetManager()

	w := newFakeWriter()
	mid := StartChat(w, "conv-5")

	// 关掉出口，之后 domain goroutine 若还塞 event 不应写入
	CloseChat(mid)

	// SendEvent 现在应该在 manager 层就短路（找不到 messageId），warn 后 no-op
	evt := &events.MessageEvent{}
	evt.BaseEvent = events.BaseEvent{Type: "message"}
	SendEvent(evt, mid)

	if got := w.body(); got != "" {
		t.Fatalf("ResponseWriter should stay empty after CloseChat, got %q", got)
	}
}

// TestEmitterSendEventDroppedWhenAborted 直接构造 emitter 验证 Abort 后 SendEvent 短路
// 覆盖场景：假设某条并发路径拿到 emitter 引用后才走 SendEvent，此时 aborted 已翻转
func TestEmitterSendEventDroppedWhenAborted(t *testing.T) {
	w := newFakeWriter()
	emitter := &EventHandleProtocol{writer: w, flusher: w}
	emitter.Close()

	emitter.SendEvent("message", map[string]string{"foo": "bar"})
	if got := w.body(); got != "" {
		t.Fatalf("SendEvent should be no-op after Close, got %q", got)
	}
	if !emitter.Aborted() {
		t.Fatal("Aborted() should be true after Close")
	}
}
