package scheduler

import (
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler 是 robfig/cron 的薄封装。
// 目的：
//  1. 集中在 infra 层管理 cron 生命周期（Start / Stop），避免 route.go 侵入 cron 库；
//  2. 统一开启 WithSeconds（6 段 cron）+ Recover chain，防 panic 逃出 goroutine 拖垮进程。
type Scheduler struct {
	inner  *cron.Cron
	logger *zap.Logger
}

// New 创建 Scheduler。
// logger 用于打日志（Recover chain 中记录 panic 堆栈）。
func New(logger *zap.Logger) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}
	c := cron.New(
		cron.WithSeconds(),
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
	)
	return &Scheduler{inner: c, logger: logger}
}

// AddFunc 注册一个 6 段 cron（S M H D M W）触发的函数。
func (s *Scheduler) AddFunc(spec string, cmd func()) error {
	_, err := s.inner.AddFunc(spec, cmd)
	return err
}

// Start 启动调度器（非阻塞，内部维持自己的 goroutine）。
func (s *Scheduler) Start() { s.inner.Start() }

// Stop 停止调度器；返回的 ctx.Done() 会在所有正在执行的任务返回后关闭。
func (s *Scheduler) Stop() { s.inner.Stop() }
