package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mooc-manus/internal/domains/models/agents"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/models/events"
)

// fakeBaseAgent 用于替换 BaseAgentDomainService，只覆盖测试所需 Chat 行为。
type fakeBaseAgent struct {
	fn func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent)
}

func (f *fakeBaseAgent) Chat(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
	f.fn(ctx, req, ch)
}
func (f *fakeBaseAgent) CreatePlan(req agents.AgentPlanCreateRequest, ch chan events.AgentEvent) {}
func (f *fakeBaseAgent) UpdatePlan(req agents.AgentPlanUpdateRequest, ch chan events.AgentEvent) {}

func makeSnapshot() *ev.AgentSnapshot {
	return &ev.AgentSnapshot{
		ID:                "s1",
		SourceAppConfigID: "cfg-1",
		SystemPrompt:      "sys",
	}
}

// 正常流：两条 assistant 消息后关闭 chan，Runner 应取到最后一条并无错误、未超时。
func TestInternalChatRunner_Normal(t *testing.T) {
	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		// 顺带验证 override 已注入
		assert.NotNil(t, req.ConfigOverride)
		assert.Equal(t, "cfg-1", req.ConfigOverride.AppConfigID)

		msg1 := &events.MessageEvent{Role: "assistant", Message: "hi"}
		msg1.Type = events.EventTypeMessage
		msg2 := &events.MessageEvent{Role: "assistant", Message: "there"}
		msg2.Type = events.EventTypeMessage
		ch <- msg1
		ch <- msg2
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:       makeSnapshot(),
		ConversationID: "c1",
		MessageID:      "m1",
		Query:          "?",
		TotalTimeout:   time.Second,
	})
	require.NoError(t, err)
	assert.Equal(t, "there", res.LastAssistantMsg)
	assert.False(t, res.DidTimeout)
	assert.NoError(t, res.Error)
}

// ErrorEvent：Runner 应把错误汇总到 Result.Error，返回 err 仍为 nil（外层协议如此）。
func TestInternalChatRunner_ErrorEvent(t *testing.T) {
	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		errEvt := &events.ErrorEvent{Error: "boom"}
		errEvt.Type = events.EventTypeError
		ch <- errEvt
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:     makeSnapshot(),
		TotalTimeout: time.Second,
	})
	require.NoError(t, err)
	require.Error(t, res.Error)
	assert.Contains(t, res.Error.Error(), "boom")
	assert.False(t, res.DidTimeout)
}

// 超时：fake 阻塞直到 ctx 结束才 close，Runner 必须在 TotalTimeout 内返回并标记超时。
func TestInternalChatRunner_Timeout(t *testing.T) {
	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		<-ctx.Done()
		close(ch)
	}}
	r := NewInternalChatRunner(fake)
	start := time.Now()
	res, err := r.Run(context.Background(), InternalChatReq{
		Snapshot:     makeSnapshot(),
		TotalTimeout: 100 * time.Millisecond,
	})
	require.NoError(t, err)
	assert.True(t, res.DidTimeout)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
}

// Snapshot 为空：Run 应直接返回错误，不发起 Chat。
func TestInternalChatRunner_NilSnapshot(t *testing.T) {
	fake := &fakeBaseAgent{fn: func(ctx context.Context, req agents.ChatRequest, ch chan events.AgentEvent) {
		t.Fatal("nil snapshot 不应触发 Chat")
	}}
	r := NewInternalChatRunner(fake)
	_, err := r.Run(context.Background(), InternalChatReq{Snapshot: nil})
	require.Error(t, err)
}
