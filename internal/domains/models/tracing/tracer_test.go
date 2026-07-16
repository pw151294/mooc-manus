package tracing

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mooc-manus/config"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger，让 Tracer.commit / runFlushLoop 中的 logger.Warn/Error 不再 panic。
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "tracing-test-log-*")
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

// fakeRepo 记录 BatchInsert 调用，用于 Tracer 单测
type fakeRepo struct {
	mu     sync.Mutex
	calls  int
	spans  []*Span
	injErr error
}

func (r *fakeRepo) BatchInsert(_ context.Context, spans []*Span) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.spans = append(r.spans, spans...)
	return r.injErr
}

func (r *fakeRepo) FindByTraceID(context.Context, string) ([]*SpanNode, error) {
	return nil, nil
}

func (r *fakeRepo) ListTraces(context.Context, TraceFilter, int, int) ([]*TraceSummary, int64, error) {
	return nil, 0, nil
}

func (r *fakeRepo) ListByConversationID(context.Context, string) ([]*Span, error) {
	return nil, nil
}

func newTestTracer(repo SpanRepository, batchSize int, flush time.Duration, bufCap int) *Tracer {
	return NewTracer(repo,
		WithBatchSize(batchSize),
		WithFlushInterval(flush),
		WithBufferCapacity(bufCap),
	)
}

func TestTracer_StartRootSpan(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 100, time.Second, 100)
	defer tr.Shutdown(context.Background())

	ctx, root := tr.StartRootSpan(context.Background(), "trace-x")
	assert.Equal(t, "trace-x", root.TraceID)
	assert.Equal(t, int32(0), root.SpanID)
	assert.Equal(t, int32(-1), root.ParentSpanID)
	got := SpanFromContext(ctx)
	assert.Same(t, root, got)
}

func TestTracer_StartSpan_FromContext(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 100, time.Second, 100)
	defer tr.Shutdown(context.Background())

	ctx, root := tr.StartRootSpan(context.Background(), "trace-x")
	_ = root
	ctx, child1 := tr.StartSpan(ctx, SpanTypeAgentRound, "")
	_, child2 := tr.StartSpan(ctx, SpanTypeAgentRound, "")

	assert.Equal(t, int32(1), child1.SpanID)
	assert.Equal(t, int32(0), child1.ParentSpanID)
	assert.Equal(t, int32(2), child2.SpanID)
	assert.Equal(t, int32(1), child2.ParentSpanID)
	assert.Equal(t, "trace-x", child2.TraceID)
}

func TestTracer_StartSpan_NoParent(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 100, time.Second, 100)
	defer tr.Shutdown(context.Background())

	_, s := tr.StartSpan(context.Background(), SpanTypeLLMCall, "")
	assert.NotNil(t, s)
	// no-op 语义：End 不 commit（ended 已为 true）
	s.End()
	assert.Equal(t, 0, repo.calls)
}

func TestTracer_BufferFullDrop(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 1000, time.Hour, 3) // 极小缓冲
	defer func() {
		_ = tr.Shutdown(context.Background())
	}()

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	var started int32
	for i := 0; i < 20; i++ {
		_, span := tr.StartSpan(ctx, SpanTypeToolCall, "t")
		atomic.AddInt32(&started, 1)
		span.End() // 触发 commit 到 buffer
	}
	assert.Greater(t, tr.DroppedCount(), int64(0))
	// 业务不阻塞：20 个 span 全部创建成功
	assert.Equal(t, int32(20), started)
}

func TestTracer_BatchFlushBySize(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 3, time.Hour, 100)
	defer tr.Shutdown(context.Background())

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	for i := 0; i < 3; i++ {
		_, s := tr.StartSpan(ctx, SpanTypeToolCall, "t")
		s.End()
	}
	// 等待 goroutine 消费
	assert.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls >= 1
	}, time.Second, 5*time.Millisecond)
	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, len(repo.spans), 3)
}

func TestTracer_BatchFlushByTimer(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 100, 100*time.Millisecond, 100)
	defer tr.Shutdown(context.Background())

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	_, s := tr.StartSpan(ctx, SpanTypeToolCall, "t")
	s.End()

	assert.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestTracer_Shutdown(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 100, time.Hour, 100)

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	for i := 0; i < 5; i++ {
		_, s := tr.StartSpan(ctx, SpanTypeToolCall, "t")
		s.End()
	}
	err := tr.Shutdown(context.Background())
	assert.NoError(t, err)
	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.GreaterOrEqual(t, len(repo.spans), 5)
}

func TestTracer_ConcurrentSpanIDGen(t *testing.T) {
	repo := &fakeRepo{}
	tr := newTestTracer(repo, 1000, time.Hour, 10000)
	defer tr.Shutdown(context.Background())

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	var wg sync.WaitGroup
	ids := sync.Map{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, s := tr.StartSpan(ctx, SpanTypeToolCall, "t")
			ids.Store(s.SpanID, struct{}{})
		}()
	}
	wg.Wait()
	// 100 个唯一 span_id
	count := 0
	ids.Range(func(_, _ any) bool { count++; return true })
	assert.Equal(t, 100, count)
}

func TestTracer_BatchInsertError_Discard(t *testing.T) {
	repo := &fakeRepo{injErr: errors.New("db down")}
	tr := newTestTracer(repo, 3, time.Hour, 100)
	defer tr.Shutdown(context.Background())

	ctx, _ := tr.StartRootSpan(context.Background(), "trace-x")
	for i := 0; i < 3; i++ {
		_, s := tr.StartSpan(ctx, SpanTypeToolCall, "t")
		s.End()
	}
	assert.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		return repo.calls >= 1
	}, time.Second, 10*time.Millisecond)
	// 错误批被丢弃，行为不 panic 即可
}
