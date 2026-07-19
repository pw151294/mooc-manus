package scheduler

import (
	"context"

	"go.uber.org/zap"

	evalsvc "mooc-manus/internal/domains/services/evaluation"
	"mooc-manus/pkg/logger"
)

// SweeperJob 心跳超期 + deadline 超期实例回收
// 对应 spec §7 cron_sweeper：把长时间无心跳或已过 deadline 的 instance 收敛到 TIMEOUT，
// 并触发所在 task 的 RecountAndTransit。
type SweeperJob struct {
	domain evalsvc.EvaluationDomainService
}

// NewSweeperJob 构造 SweeperJob。
func NewSweeperJob(d evalsvc.EvaluationDomainService) *SweeperJob {
	return &SweeperJob{domain: d}
}

// Run cron 触发入口；单次巡检失败仅日志（下一轮继续）。
func (j *SweeperJob) Run() {
	n, err := j.domain.SweepStaleInstances(context.Background())
	if err != nil {
		logger.Warn("sweep stale instances 失败", zap.Error(err))
		return
	}
	if n > 0 {
		logger.Info("sweep stale instances", zap.Int("count", n))
	}
}

// ReconcilerJob 修正 task 状态
// 对应 spec §7 cron_reconciler：对所有非终态 task 统一 RecountAndTransit，
// 兜底 Executor / 巡检 miss 掉的 task 状态迁移。
type ReconcilerJob struct {
	domain evalsvc.EvaluationDomainService
}

// NewReconcilerJob 构造 ReconcilerJob。
func NewReconcilerJob(d evalsvc.EvaluationDomainService) *ReconcilerJob {
	return &ReconcilerJob{domain: d}
}

// Run cron 触发入口。
func (j *ReconcilerJob) Run() {
	n, err := j.domain.ReconcileTaskStatuses(context.Background())
	if err != nil {
		logger.Warn("reconcile task statuses 失败", zap.Error(err))
		return
	}
	if n > 0 {
		logger.Info("reconcile task statuses", zap.Int("count", n))
	}
}

// DLQArchiverJob 归档 asynq DLQ 中的 dead run_instance
// 对应 spec §7 cron_dlq_archive：捞出 DLQ 中的 asynq 死信，把对应 instance 标记为终态。
type DLQArchiverJob struct {
	domain evalsvc.EvaluationDomainService
}

// NewDLQArchiverJob 构造 DLQArchiverJob。
func NewDLQArchiverJob(d evalsvc.EvaluationDomainService) *DLQArchiverJob {
	return &DLQArchiverJob{domain: d}
}

// Run cron 触发入口。
func (j *DLQArchiverJob) Run() {
	n, err := j.domain.ArchiveDeadTasks(context.Background())
	if err != nil {
		logger.Warn("archive dead tasks 失败", zap.Error(err))
		return
	}
	if n > 0 {
		logger.Info("archive dead tasks", zap.Int("count", n))
	}
}
