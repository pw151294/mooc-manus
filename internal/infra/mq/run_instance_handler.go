package mq

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"
)

// InstanceExecutorAPI RunInstanceHandler 依赖的最小执行接口
// 抽出该接口而非直接依赖 *evaluation.InstanceExecutor：
//  1. 便于测试注入 fake（Executor 结构依赖非常多）；
//  2. 保持 infra/mq 与 domains/services/evaluation 之间的松耦合。
//
// 生产装配时把 *evaluation.InstanceExecutor 传入即可（Execute 签名一致）。
type InstanceExecutorAPI interface {
	Execute(ctx context.Context, instanceID string) error
}

// RunInstanceHandler asynq TaskTypeRunInstance 消费入口
// 职责：解 payload → 加 case 令牌 → 调 executor → 释放令牌。
type RunInstanceHandler struct {
	executor InstanceExecutorAPI
	instRepo repositories.EvalRunInstanceRepository
	caseGate CaseTokenGate
}

// NewRunInstanceHandler 构造 handler。
// 依赖来自 route.go 层装配（Task 6.3），本包不涉及。
func NewRunInstanceHandler(
	executor InstanceExecutorAPI,
	instRepo repositories.EvalRunInstanceRepository,
	caseGate CaseTokenGate,
) *RunInstanceHandler {
	return &RunInstanceHandler{
		executor: executor,
		instRepo: instRepo,
		caseGate: caseGate,
	}
}

// ProcessTask asynq 消费入口。
// 失败分类：
//   - payload 反序列化失败 → SkipRetry（脏数据没救）
//   - 实例不存在（GetByID 失败）→ SkipRetry（数据已经被清或 ID 错）
//   - Redis 抖动（Acquire err）→ 返回 err 交给 asynq 重试
//   - Case token busy（Acquire=false）→ 返回普通 error，触发 RetryDelayFunc（30s）
//   - executor.Execute 失败 → 返回 err（executor 内部已经把不可重试情况写成终态）
func (h *RunInstanceHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	consumeStart := time.Now()
	var p RunInstancePayload
	if err := p.Unmarshal(task.Payload()); err != nil {
		logger.Error("EVAL_MQ_PAYLOAD_UNMARSHAL_ERR",
			zap.Int("payload_bytes", len(task.Payload())),
			zap.Error(err))
		return errors.Join(asynq.SkipRetry, err)
	}
	queueLagMs := int64(0)
	if p.EnqueuedAt > 0 {
		queueLagMs = time.Since(time.Unix(p.EnqueuedAt, 0)).Milliseconds()
	}
	logger.Info("EVAL_MQ_CONSUME_START",
		zap.String("instance_id", p.InstanceID),
		zap.Int("attempt", p.Attempt),
		zap.Int64("queue_lag_ms", queueLagMs))

	inst, err := h.instRepo.GetByID(ctx, p.InstanceID)
	if err != nil {
		logger.Error("EVAL_MQ_INSTANCE_MISS",
			zap.String("instance_id", p.InstanceID),
			zap.Error(err))
		return errors.Join(asynq.SkipRetry, err)
	}

	// 抢 case 令牌
	ok, err := h.caseGate.Acquire(ctx, inst.CaseID)
	if err != nil {
		// Redis 抖动 —— 交给 Asynq 重试
		logger.Warn("EVAL_MQ_TOKEN_ACQUIRE_ERR",
			zap.String("instance_id", p.InstanceID),
			zap.String("case_id", inst.CaseID),
			zap.Error(err))
		return err
	}
	if !ok {
		// case 并发已满，短 backoff 重投
		logger.Info("EVAL_MQ_TOKEN_BUSY",
			zap.String("instance_id", p.InstanceID),
			zap.String("case_id", inst.CaseID),
			zap.String("hint", "case 并发已满，asynq 会 30s 后重投"))
		return errors.New("case slot busy")
	}
	defer func() {
		// Release 失败不影响主流程；TTL 兜底 20min 自愈
		if rerr := h.caseGate.Release(ctx, inst.CaseID); rerr != nil {
			logger.Warn("EVAL_MQ_TOKEN_RELEASE_ERR",
				zap.String("instance_id", p.InstanceID),
				zap.String("case_id", inst.CaseID),
				zap.Error(rerr))
		}
	}()
	logger.Info("EVAL_MQ_TOKEN_ACQUIRED",
		zap.String("instance_id", p.InstanceID),
		zap.String("case_id", inst.CaseID))

	execErr := h.executor.Execute(ctx, p.InstanceID)
	logger.Info("EVAL_MQ_CONSUME_DONE",
		zap.String("instance_id", p.InstanceID),
		zap.String("case_id", inst.CaseID),
		zap.Int64("consume_duration_ms", time.Since(consumeStart).Milliseconds()),
		zap.NamedError("execute_err", execErr))
	return execErr
}
