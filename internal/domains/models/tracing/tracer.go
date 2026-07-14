// Package tracing 的 Tracer 实现：包级单例 + 异步批量 flush + 缓冲区满 drop（永不阻塞业务）。
package tracing

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"mooc-manus/pkg/logger"
)

const (
	defaultBatchSize      = 100
	defaultFlushInterval  = 5 * time.Second
	defaultBufferCapacity = 10000
)

// Tracer 承担 span_id 生成、异步批量落库、shutdown drain 等职责。
// 关键约束：所有对业务侧的调用（StartSpan / commit）都不能阻塞。
type Tracer struct {
	repo          SpanRepository
	buffer        chan *Span
	batchSize     int
	flushInterval time.Duration
	dropCounter   atomic.Int64
	errCounter    atomic.Int64
	shutdown      chan struct{}
	wg            sync.WaitGroup

	// traceCounters: traceID -> *atomic.Int32，用于生成 trace 内自增 span_id
	traceCounters sync.Map
}

// Option 是 Tracer 构造选项
type Option func(*Tracer)

// WithBatchSize 设置每批 flush 的最大 span 数
func WithBatchSize(n int) Option { return func(t *Tracer) { t.batchSize = n } }

// WithFlushInterval 设置定时 flush 的间隔
func WithFlushInterval(d time.Duration) Option { return func(t *Tracer) { t.flushInterval = d } }

// WithBufferCapacity 设置内存缓冲的最大 span 数
func WithBufferCapacity(n int) Option { return func(t *Tracer) { t.buffer = make(chan *Span, n) } }

// NewTracer 构造并启动一个 Tracer；调用方需在停止时 Shutdown。
func NewTracer(repo SpanRepository, opts ...Option) *Tracer {
	t := &Tracer{
		repo:          repo,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		buffer:        make(chan *Span, defaultBufferCapacity),
		shutdown:      make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	if t.buffer == nil {
		t.buffer = make(chan *Span, defaultBufferCapacity)
	}
	t.wg.Add(1)
	go t.runFlushLoop()
	return t
}

// StartRootSpan 由 Application 层入口调用一次；生成 span_id=0，parent=-1 的根 span。
func (t *Tracer) StartRootSpan(ctx context.Context, traceID string) (context.Context, *Span) {
	counter := new(atomic.Int32)
	t.traceCounters.Store(traceID, counter)
	s := &Span{
		TraceID:      traceID,
		SpanID:       0,
		ParentSpanID: -1,
		SpanType:     SpanTypeAgentRoot,
		StartTime:    time.Now().UnixNano(),
		tags:         make(map[string]interface{}),
		logs:         make([]LogEntry, 0),
		commitFn:     t.commit,
	}
	return contextWithSpan(ctx, s), s
}

// StartSpan 从 ctx 里取父 span，生成子 span；无 root 时返回 no-op。
func (t *Tracer) StartSpan(ctx context.Context, spanType SpanType, opName string) (context.Context, *Span) {
	parent := SpanFromContext(ctx)
	if parent == nil || parent.TraceID == "" {
		// no-op：无 root，业务不阻塞
		return ctx, newNoopSpan()
	}
	counterAny, ok := t.traceCounters.Load(parent.TraceID)
	if !ok {
		return ctx, newNoopSpan()
	}
	counter := counterAny.(*atomic.Int32)
	newID := counter.Add(1)
	s := &Span{
		TraceID:       parent.TraceID,
		SpanID:        newID,
		ParentSpanID:  parent.SpanID,
		SpanType:      spanType,
		OperationName: opName,
		// 独立列默认从 parent 继承（可被 SetAgentName / SetConversationID 覆盖）
		ConversationID: parent.ConversationID,
		AgentName:      parent.AgentName,
		StartTime:      time.Now().UnixNano(),
		tags:           make(map[string]interface{}),
		logs:           make([]LogEntry, 0),
		commitFn:       t.commit,
	}
	return contextWithSpan(ctx, s), s
}

// commit 由 Span.End() 回调；缓冲区满时 drop + 计数，永不阻塞。
func (t *Tracer) commit(s *Span) {
	select {
	case t.buffer <- s:
	default:
		t.dropCounter.Add(1)
		// 简单节流：每 1000 次 drop 打一次日志
		if t.dropCounter.Load()%1000 == 1 {
			logger.Warn("tracing buffer full, dropping span",
				zap.Int64("drop_total", t.dropCounter.Load()),
				zap.String("trace_id", s.TraceID))
		}
	}
	// root 结束时清理 traceCounters，避免内存泄漏
	if s.SpanType == SpanTypeAgentRoot {
		t.traceCounters.Delete(s.TraceID)
	}
}

// DroppedCount 返回累计丢弃 span 数（测试与监控用）
func (t *Tracer) DroppedCount() int64 { return t.dropCounter.Load() }

func (t *Tracer) runFlushLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()

	batch := make([]*Span, 0, t.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// 拷贝再送 repo，避免下轮复用切片时的引用问题
		toWrite := make([]*Span, len(batch))
		copy(toWrite, batch)
		batch = batch[:0]
		// 独立 ctx；不受 root ctx 影响
		if err := t.repo.BatchInsert(context.Background(), toWrite); err != nil {
			t.errCounter.Add(1)
			logger.Error("tracing batch insert failed",
				zap.Error(err),
				zap.Int("batch_size", len(toWrite)),
				zap.Int64("err_total", t.errCounter.Load()))
		}
	}

	for {
		select {
		case <-t.shutdown:
			// 排空 buffer 后再退出
			for {
				select {
				case s := <-t.buffer:
					batch = append(batch, s)
					if len(batch) >= t.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case s := <-t.buffer:
			batch = append(batch, s)
			if len(batch) >= t.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Shutdown 关闭 tracer；等待 flush goroutine 排空退出。ctx 超时则提前返回。
func (t *Tracer) Shutdown(ctx context.Context) error {
	select {
	case <-t.shutdown:
		return nil
	default:
		close(t.shutdown)
	}
	done := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ==== 包级全局单例 ====

var globalTracer atomic.Pointer[Tracer]

// SetGlobal 设置全局 Tracer；Application 层启动时注入。
func SetGlobal(t *Tracer) { globalTracer.Store(t) }

// Global 返回全局 Tracer；未初始化返回 nil，调用方需自行判断。
func Global() *Tracer { return globalTracer.Load() }
