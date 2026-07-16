package services

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/textproto"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"mooc-manus/config"
	"mooc-manus/internal/applications/dtos"
	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/infra/repositories"
)

// ---------- fakes ----------

// fakeMQ 记录 Enqueue 调用；用于验证 CreateTask 的编排行为。
type fakeMQ struct {
	mu       sync.Mutex
	calls    []struct {
		ID      string
		Attempt int
		High    bool
	}
	errOnID  string
	errValue error
}

func (m *fakeMQ) EnqueueRunInstance(_ context.Context, id string, attempt int, useHigh bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct {
		ID      string
		Attempt int
		High    bool
	}{id, attempt, useHigh})
	if id == m.errOnID {
		return m.errValue
	}
	return nil
}

func (m *fakeMQ) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// fakeInstRepo 只实现 List 用于收集测试实例；其余方法返回 nil。
type fakeInstRepo struct {
	listResult []*ev.RunInstance
}

func (r *fakeInstRepo) Create(_ context.Context, _ *ev.RunInstance) error { return nil }
func (r *fakeInstRepo) GetByID(_ context.Context, _ string) (*ev.RunInstance, error) {
	return nil, nil
}
func (r *fakeInstRepo) GetStatus(_ context.Context, _ string) (ev.InstanceStatus, error) {
	return "", nil
}
func (r *fakeInstRepo) List(_ context.Context, _ repositories.InstanceListFilter, _, _ int) ([]*ev.RunInstance, int64, error) {
	return r.listResult, int64(len(r.listResult)), nil
}
func (r *fakeInstRepo) Update(_ context.Context, _ *ev.RunInstance) error { return nil }
func (r *fakeInstRepo) Delete(_ context.Context, _ string) error          { return nil }
func (r *fakeInstRepo) UpdateTraceID(_ context.Context, _, _ string) error {
	return nil
}
func (r *fakeInstRepo) ListStaleInstances(_ context.Context, _ time.Time) ([]*ev.RunInstance, error) {
	return nil, nil
}
func (r *fakeInstRepo) UpdateHeartbeat(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (r *fakeInstRepo) CASStatus(_ context.Context, _ string, _, _ ev.InstanceStatus) (bool, error) {
	return true, nil
}

// fakeDomain 只实现 CreateTask，其他方法在本测试不使用则 panic 提示 & 返回零值。
type fakeDomain struct {
	createTaskFn func(ctx context.Context, name string, cids, acs []string) (*ev.Task, error)
	createCalls  int32
}

func (d *fakeDomain) CreateCase(_ context.Context, _ *ev.Case) (*ev.Case, error) { return nil, nil }
func (d *fakeDomain) UpdateCase(_ context.Context, _ *ev.Case) (*ev.Case, error) { return nil, nil }
func (d *fakeDomain) DeleteCase(_ context.Context, _ string) error               { return nil }
func (d *fakeDomain) ListCases(_ context.Context, _ repositories.CaseListFilter, _, _ int) ([]*ev.Case, int64, error) {
	return nil, 0, nil
}
func (d *fakeDomain) GetCase(_ context.Context, _ string) (*ev.Case, error) { return nil, nil }
func (d *fakeDomain) CreateTask(ctx context.Context, name string, cids, acs []string) (*ev.Task, error) {
	atomic.AddInt32(&d.createCalls, 1)
	return d.createTaskFn(ctx, name, cids, acs)
}
func (d *fakeDomain) ListTasks(_ context.Context, _ repositories.TaskListFilter, _, _ int) ([]*ev.Task, int64, error) {
	return nil, 0, nil
}
func (d *fakeDomain) GetTask(_ context.Context, _ string) (*ev.Task, error)          { return nil, nil }
func (d *fakeDomain) RetryTaskFailedInstances(_ context.Context, _ string) (int, error) { return 0, nil }
func (d *fakeDomain) DeleteTask(_ context.Context, _ string) error                   { return nil }
func (d *fakeDomain) ListInstances(_ context.Context, _ string, _ repositories.InstanceListFilter, _, _ int) ([]*ev.RunInstance, int64, error) {
	return nil, 0, nil
}
func (d *fakeDomain) GetInstance(_ context.Context, _ string) (*ev.RunInstance, error) {
	return nil, nil
}
func (d *fakeDomain) RetryInstance(_ context.Context, _ string) error { return nil }
func (d *fakeDomain) DeleteInstance(_ context.Context, _ string) error {
	return nil
}
func (d *fakeDomain) ExecuteInstance(_ context.Context, _, _ string) error { return nil }
func (d *fakeDomain) SweepStaleInstances(_ context.Context) (int, error)   { return 0, nil }
func (d *fakeDomain) ReconcileTaskStatuses(_ context.Context) (int, error) { return 0, nil }
func (d *fakeDomain) ArchiveDeadTasks(_ context.Context) (int, error)      { return 0, nil }

// ---------- 构造 multipart file 用于 UploadContent 测试 ----------

func newMultipartFile(t *testing.T, body []byte, size int64) *multipart.FileHeader {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="a.txt"`)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	r := multipart.NewReader(buf, w.Boundary())
	form, err := r.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	fh := form.File["file"][0]
	if size > 0 {
		fh.Size = size
	}
	return fh
}

// ---------- Tests ----------

func TestUploadContent_TooLarge(t *testing.T) {
	svc := &evalAppImpl{
		evalCfg: config.EvaluationConfig{UploadContentMaxBytes: 5},
		logger:  zap.NewNop(),
	}
	fh := newMultipartFile(t, []byte("hello world"), 11)
	_, err := svc.UploadContent(context.Background(), fh)
	if !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("expected ErrUploadTooLarge, got %v", err)
	}
}

func TestUploadContent_NotUTF8(t *testing.T) {
	svc := &evalAppImpl{
		evalCfg: config.EvaluationConfig{UploadContentMaxBytes: 1024},
		logger:  zap.NewNop(),
	}
	// 非 UTF-8 字节序列（0xff 单独出现为非法起始）
	fh := newMultipartFile(t, []byte{0xff, 0xfe, 0xfd}, 3)
	_, err := svc.UploadContent(context.Background(), fh)
	if !errors.Is(err, ErrUploadNotUTF8) {
		t.Fatalf("expected ErrUploadNotUTF8, got %v", err)
	}
}

func TestUploadContent_OK(t *testing.T) {
	svc := &evalAppImpl{
		evalCfg: config.EvaluationConfig{UploadContentMaxBytes: 1024},
		logger:  zap.NewNop(),
	}
	body := "print('hi')\n"
	fh := newMultipartFile(t, []byte(body), int64(len(body)))
	resp, err := svc.UploadContent(context.Background(), fh)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Content != body {
		t.Fatalf("content mismatch: got %q want %q", resp.Content, body)
	}
	if resp.Size != len(body) {
		t.Fatalf("size mismatch: got %d want %d", resp.Size, len(body))
	}
}

func TestCreateTask_EnqueuesAllInstances(t *testing.T) {
	task := &ev.Task{ID: "task-1", Name: "t1", TotalCount: 4}
	instances := []*ev.RunInstance{
		{ID: "i1", TaskID: "task-1", Attempt: 0},
		{ID: "i2", TaskID: "task-1", Attempt: 0},
		{ID: "i3", TaskID: "task-1", Attempt: 0},
		{ID: "i4", TaskID: "task-1", Attempt: 0},
	}
	fd := &fakeDomain{createTaskFn: func(_ context.Context, _ string, _, _ []string) (*ev.Task, error) {
		return task, nil
	}}
	fmq := &fakeMQ{}
	fir := &fakeInstRepo{listResult: instances}
	svc := &evalAppImpl{
		domain:   fd,
		instRepo: fir,
		mq:       fmq,
		evalCfg:  config.EvaluationConfig{},
		logger:   zap.NewNop(),
	}
	view, err := svc.CreateTask(context.Background(), &dtos.TaskCreateRequest{
		Name: "t1", CaseIDs: []string{"c1", "c2"}, AgentConfigIDs: []string{"a1", "a2"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if view.ID != "task-1" {
		t.Fatalf("view id mismatch: %q", view.ID)
	}
	if fmq.CallCount() != 4 {
		t.Fatalf("expected 4 enqueue calls, got %d", fmq.CallCount())
	}
	if atomic.LoadInt32(&fd.createCalls) != 1 {
		t.Fatalf("expected domain.CreateTask called once")
	}
	// 校验都走默认队列（useHigh=false）
	for _, c := range fmq.calls {
		if c.High {
			t.Fatalf("expected default queue, got high for %s", c.ID)
		}
	}
}

func TestCreateTask_MQNilTolerated(t *testing.T) {
	task := &ev.Task{ID: "task-2", Name: "t2", TotalCount: 1}
	fd := &fakeDomain{createTaskFn: func(_ context.Context, _ string, _, _ []string) (*ev.Task, error) {
		return task, nil
	}}
	fir := &fakeInstRepo{listResult: []*ev.RunInstance{{ID: "x1"}}}
	svc := &evalAppImpl{
		domain:   fd,
		instRepo: fir,
		mq:       nil, // 允许无 mq，跳过 enqueue
		evalCfg:  config.EvaluationConfig{},
		logger:   zap.NewNop(),
	}
	view, err := svc.CreateTask(context.Background(), &dtos.TaskCreateRequest{
		Name: "t2", CaseIDs: []string{"c"}, AgentConfigIDs: []string{"a"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if view.ID != "task-2" {
		t.Fatalf("view id mismatch: %q", view.ID)
	}
}
