package evaluation

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/repositories"
)

// InstanceExecutor 单个评测实例执行器
// 状态推进链路：QUEUED → INITIALIZING → RUNNING → VERIFYING → PASSED / FAILED / TIMEOUT
// spec §3.5 + §4.5：由 asynq worker 每次拉一个 instanceID 交给 Execute 消费；
// 心跳 goroutine 定时刷新 heartbeat_at，同时检查目标态（TIMEOUT/CANCELED）以便中断当前 stage。
type InstanceExecutor struct {
	instRepo          repositories.EvalRunInstanceRepository
	taskRepo          repositories.EvalTaskRepository
	resultRepo        repositories.EvalResultRepository
	snapshotRepo      repositories.EvalAgentSnapshotRepository
	verifyRunner      *VerifyRunner
	chatRunner        InternalChatRunner
	aggregator        *TraceAggregator
	tracer            *tracing.Tracer // 保留但目前只用于占位（Tracer 无同步 Flush，靠 sleep 等 batch flush）
	skillExecutor     tools.SkillExecutor
	nativeProvider    tools.NativeToolsProvider
	workerID          string
	heartbeatInterval time.Duration
	chatTimeout       time.Duration
	logger            *zap.Logger
}

// NewInstanceExecutor 构造 InstanceExecutor
// 所有依赖都由外层（InitRouter）装配注入；此层不读 config。
func NewInstanceExecutor(
	instRepo repositories.EvalRunInstanceRepository,
	taskRepo repositories.EvalTaskRepository,
	resultRepo repositories.EvalResultRepository,
	snapshotRepo repositories.EvalAgentSnapshotRepository,
	verifyRunner *VerifyRunner,
	chatRunner InternalChatRunner,
	aggregator *TraceAggregator,
	tracer *tracing.Tracer,
	skillExecutor tools.SkillExecutor,
	nativeProvider tools.NativeToolsProvider,
	workerID string,
	heartbeatInterval time.Duration,
	chatTimeout time.Duration,
	logger *zap.Logger,
) *InstanceExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if heartbeatInterval <= 0 {
		heartbeatInterval = 5 * time.Second
	}
	if chatTimeout <= 0 {
		chatTimeout = 5 * time.Minute
	}
	return &InstanceExecutor{
		instRepo:          instRepo,
		taskRepo:          taskRepo,
		resultRepo:        resultRepo,
		snapshotRepo:      snapshotRepo,
		verifyRunner:      verifyRunner,
		chatRunner:        chatRunner,
		aggregator:        aggregator,
		tracer:            tracer,
		skillExecutor:     skillExecutor,
		nativeProvider:    nativeProvider,
		workerID:          workerID,
		heartbeatInterval: heartbeatInterval,
		chatTimeout:       chatTimeout,
		logger:            logger,
	}
}

// Execute 消费一个 instance：从 QUEUED 起，把状态推进到终态之一。
// 幂等策略：QUEUED→INITIALIZING 用 CAS，一旦 CAS 失败（已被其他 worker 拿走 / 状态非法）直接返回 nil，
// 避免同一 instance 被并发消费。
func (e *InstanceExecutor) Execute(ctx context.Context, instanceID string) error {
	// Step 1: CAS QUEUED → INITIALIZING（Task 4.5.1）
	ok, err := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusQueued, ev.InstanceStatusInitializing)
	if err != nil {
		return err
	}
	if !ok {
		e.logger.Warn("CAS QUEUED→INITIALIZING 失败，实例可能已被消费或状态非法",
			zap.String("instance_id", instanceID))
		return nil
	}

	inst, err := e.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	// Step 2-4: init / chat / verify / finalize（Task 4.5.2-4.5.4）
	return e.executeStages(ctx, inst)
}

// executeStages 组装 4.5.2 / 4.5.3 / 4.5.4 主流程。
// 关键约束：所有 stage 之间用 CAS 推进状态；单一 stage 失败通过 finalizeError 统一落地。
func (e *InstanceExecutor) executeStages(ctx context.Context, inst *ev.RunInstance) error {
	// 心跳 goroutine：定时刷 heartbeat_at + 感知 TIMEOUT/CANCELED 立即取消 stage ctx
	stageCtx, stopHB := e.startHeartbeat(ctx, inst.ID)
	defer stopHB()

	// workdir 复用 native workspace 目录（与生产 chat 一致），需提前建目录否则脚本写入失败
	workdir := e.nativeProvider.MessageWorkspaceDir(inst.MessageID)
	if err := os.MkdirAll(workdir, 0o700); err != nil {
		e.finalizeError(ctx, inst, ev.InstanceStatusInitializing, ev.InstanceStatusFailed,
			"mkdir workspace: "+err.Error())
		return nil
	}

	// init_script：可选。执行失败直接终结 INSTANCE 为 FAILED。
	if inst.CaseSnapshot.InitScript != "" {
		r, rerr := e.verifyRunner.Run(stageCtx, workdir, inst.CaseSnapshot.InitScript)
		stderr := ""
		if r != nil {
			stderr = r.Stderr
		}
		if rerr != nil {
			e.finalizeError(ctx, inst, ev.InstanceStatusInitializing, ev.InstanceStatusFailed,
				"init_script run: "+rerr.Error()+"; stderr="+stderr)
			return nil
		}
		if r.ExitCode != 0 {
			e.finalizeError(ctx, inst, ev.InstanceStatusInitializing, ev.InstanceStatusFailed,
				"init_script exit="+strconv.Itoa(r.ExitCode)+"; stderr="+stderr)
			return nil
		}
	}

	// 加载 agent snapshot（chat 需要）—— 在 INITIALIZING 阶段做，失败也算 init 失败
	snap, err := e.snapshotRepo.Get(stageCtx, inst.AgentConfigSnapshotID)
	if err != nil {
		e.finalizeError(ctx, inst, ev.InstanceStatusInitializing, ev.InstanceStatusFailed,
			"load snapshot: "+err.Error())
		return nil
	}

	// INITIALIZING → RUNNING
	ok, _ := e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusInitializing, ev.InstanceStatusRunning)
	if !ok {
		e.logger.Warn("INITIALIZING→RUNNING CAS 失败，可能已被 timeout 巡检抢占",
			zap.String("instance_id", inst.ID))
		// 安全退出：清理 workspace + 触发 task recount 让上层感知
		e.cleanupAndRecount(ctx, inst)
		return nil
	}

	// chat 阶段（Task 4.5.3）
	chatRes, cerr := e.chatRunner.Run(stageCtx, InternalChatReq{
		Snapshot:       snap,
		ConversationID: inst.ConversationID,
		MessageID:      inst.MessageID,
		Query:          inst.CaseSnapshot.TaskPrompt,
		TotalTimeout:   e.chatTimeout,
	})
	if cerr != nil {
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusFailed,
			"chat runner: "+cerr.Error())
		return nil
	}
	if chatRes.DidTimeout {
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusTimeout,
			"agent_chat_timeout")
		return nil
	}
	if chatRes.Error != nil {
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusFailed,
			"agent_error: "+chatRes.Error.Error())
		return nil
	}

	// RUNNING → VERIFYING
	ok, _ = e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusRunning, ev.InstanceStatusVerifying)
	if !ok {
		e.logger.Warn("RUNNING→VERIFYING CAS 失败", zap.String("instance_id", inst.ID))
		e.cleanupAndRecount(ctx, inst)
		return nil
	}

	// verify_script（Task 4.5.4）
	vres, verr := e.verifyRunner.Run(stageCtx, workdir, inst.CaseSnapshot.VerifyScript)
	passed := verr == nil && vres != nil && vres.ExitCode == 0
	e.finalizeVerify(ctx, inst, passed, vres, verr)
	return nil
}

// startHeartbeat 启动心跳 goroutine
// 返回派生 ctx（stage 中的每个耗时调用都应使用此 ctx，以便被心跳感知的 target 状态取消掉）
// 与 cancel func。cancel 由 defer 调度，heartbeat goroutine 感知到 ctx2.Done 会自行退出。
func (e *InstanceExecutor) startHeartbeat(ctx context.Context, id string) (context.Context, context.CancelFunc) {
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(e.heartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx2.Done():
				return
			case <-t.C:
				_ = e.instRepo.UpdateHeartbeat(ctx, id, e.workerID, time.Now())
				s, err := e.instRepo.GetStatus(ctx, id)
				if err == nil && (s == ev.InstanceStatusTimeout || s == ev.InstanceStatusCanceled) {
					cancel()
					return
				}
			}
		}
	}()
	return ctx2, cancel
}

// finalizeError 统一错误落地
// 1) CAS from → to（失败仅告警，不阻塞落 result）
// 2) upsert Result（passed=false + 错误摘要）
// 3) task recount
// 4) cleanup skill / native workspace
func (e *InstanceExecutor) finalizeError(ctx context.Context, inst *ev.RunInstance,
	from, to ev.InstanceStatus, errMsg string) {
	ok, _ := e.instRepo.CASStatus(ctx, inst.ID, from, to)
	if !ok {
		e.logger.Warn("finalizeError CAS 失败",
			zap.String("id", inst.ID),
			zap.String("from", string(from)),
			zap.String("to", string(to)))
	}
	now := time.Now()
	_ = e.resultRepo.Upsert(ctx, &ev.Result{
		InstanceID: inst.ID,
		Passed:     false,
		ErrorLog:   truncate(errMsg, 64<<10),
		FinishedAt: now,
	})
	_ = e.taskRepo.RecountAndTransit(ctx, inst.TaskID)
	e.cleanup(inst)
}

// finalizeVerify verify 完成后收敛：等 tracer batch flush → aggregate → 落 result → CAS 状态 → cleanup。
// tracer 无同步 Flush API，只能靠 sleep 让异步 flush loop 落库；若 aggregate 结果 degraded 则重试一次。
func (e *InstanceExecutor) finalizeVerify(ctx context.Context, inst *ev.RunInstance,
	passed bool, vres *VerifyResult, verr error) {
	// 等异步 batch flush 一次（Tracer 无同步 Flush API）
	time.Sleep(300 * time.Millisecond)
	var metrics *Metrics
	if e.aggregator != nil {
		m, aerr := e.aggregator.Aggregate(ctx, inst.ConversationID)
		if aerr != nil || m == nil || m.Degraded {
			// 一次重试（等 500ms 让 tracer 再 flush 一轮）
			time.Sleep(500 * time.Millisecond)
			m, _ = e.aggregator.Aggregate(ctx, inst.ConversationID)
		}
		metrics = m
	}

	target := ev.InstanceStatusPassed
	errMsg := ""
	if !passed {
		target = ev.InstanceStatusFailed
		if verr != nil {
			errMsg = "verify: " + verr.Error()
		} else if vres != nil {
			errMsg = firstNonEmpty(vres.Stderr, "verify_exit_"+strconv.Itoa(vres.ExitCode))
		} else {
			errMsg = "verify_unknown_error"
		}
	}

	// VERIFYING → target；CAS 失败仅告警，Result 仍要落库以便排查
	_, _ = e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusVerifying, target)

	result := &ev.Result{
		InstanceID: inst.ID,
		Passed:     target == ev.InstanceStatusPassed,
		ErrorLog:   truncate(errMsg, 64<<10),
		FinishedAt: time.Now(),
	}
	if vres != nil {
		result.VerifyExitCode = vres.ExitCode
		result.VerifyStdout = truncate(vres.Stdout, 64<<10)
		result.VerifyStderr = truncate(vres.Stderr, 64<<10)
	}
	if metrics != nil {
		result.PromptTokens = metrics.PromptTokens
		result.CompletionTokens = metrics.CompletionTokens
		result.TotalTokens = metrics.TotalTokens
		result.AgentLatencyMs = metrics.AgentLatencyMs
		if metrics.TraceID != "" {
			_ = e.instRepo.UpdateTraceID(ctx, inst.ID, metrics.TraceID)
		}
	}
	_ = e.resultRepo.Upsert(ctx, result)
	_ = e.taskRepo.RecountAndTransit(ctx, inst.TaskID)
	e.cleanup(inst)
}

// cleanup 清理 skill 容器与 native workspace 目录（本次评测独占资源）
func (e *InstanceExecutor) cleanup(inst *ev.RunInstance) {
	if e.skillExecutor != nil {
		if err := e.skillExecutor.CleanupMessage(inst.MessageID); err != nil {
			e.logger.Warn("skillExecutor cleanup 失败",
				zap.String("message_id", inst.MessageID), zap.Error(err))
		}
	}
	if e.nativeProvider != nil {
		if err := e.nativeProvider.Cleanup(inst.MessageID); err != nil {
			e.logger.Warn("nativeProvider cleanup 失败",
				zap.String("message_id", inst.MessageID), zap.Error(err))
		}
	}
}

// cleanupAndRecount 中途 CAS 失败退出时的兜底：只做 cleanup + task recount，不覆盖已有 status
func (e *InstanceExecutor) cleanupAndRecount(ctx context.Context, inst *ev.RunInstance) {
	_ = e.taskRepo.RecountAndTransit(ctx, inst.TaskID)
	e.cleanup(inst)
}

// truncate 把长字符串截到 n 字节以内并追加提示；避免错误日志爆 DB TEXT 字段
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "\n[truncated]"
}

// firstNonEmpty 返回第一个非空字符串；便于错误摘要挑选
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}
