package evaluation

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	ev "mooc-manus/internal/domains/models/evaluation"
	"mooc-manus/internal/domains/models/tracing"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/repositories"
	"mooc-manus/pkg/logger"
)

// evalWorkspacePlaceholder task_prompt 内可用的占位符，chat 前会被替换为当前实例的
// message workspace 绝对路径。用途：让评测用例的 prompt 能直接告诉 agent workspace 位置，
// 从而覆盖需要绝对路径的 fileRead 能力（fileEdit / fileWrite 相对 workspace，不受影响）。
const evalWorkspacePlaceholder = "${EVAL_WORKSPACE}"

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
) *InstanceExecutor {
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
	}
}

// Execute 消费一个 instance：从 QUEUED 起，把状态推进到终态之一。
// 幂等策略：QUEUED→INITIALIZING 用 CAS，一旦 CAS 失败（已被其他 worker 拿走 / 状态非法）直接返回 nil，
// 避免同一 instance 被并发消费。
func (e *InstanceExecutor) Execute(ctx context.Context, instanceID string) error {
	startedAt := time.Now()
	// Step 1: CAS QUEUED → INITIALIZING（Task 4.5.1）
	ok, err := e.instRepo.CASStatus(ctx, instanceID, ev.InstanceStatusQueued, ev.InstanceStatusInitializing)
	if err != nil {
		logger.Error("EVAL_STAGE_INIT_CAS_ERR",
			zap.String("instance_id", instanceID),
			zap.String("worker_id", e.workerID),
			zap.Error(err))
		return err
	}
	if !ok {
		logger.Warn("EVAL_STAGE_INIT_CAS_MISS",
			zap.String("instance_id", instanceID),
			zap.String("worker_id", e.workerID),
			zap.String("hint", "已被其他 worker 拿走 / 状态非法 / 巡检抢占"))
		return nil
	}

	inst, err := e.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		logger.Error("EVAL_STAGE_INIT_LOAD_ERR",
			zap.String("instance_id", instanceID),
			zap.Error(err))
		return err
	}
	logger.Info("EVAL_STAGE_INIT",
		zap.String("instance_id", inst.ID),
		zap.String("task_id", inst.TaskID),
		zap.String("case_id", inst.CaseID),
		zap.String("conversation_id", inst.ConversationID),
		zap.String("message_id", inst.MessageID),
		zap.Int("attempt", inst.Attempt),
		zap.String("worker_id", e.workerID),
		zap.Bool("has_init_script", inst.CaseSnapshot.InitScript != ""),
		zap.Int64("started_epoch_ms", startedAt.UnixMilli()))

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
		initStart := time.Now()
		r, rerr := e.verifyRunner.Run(stageCtx, workdir, inst.CaseSnapshot.InitScript)
		stderr := ""
		exitCode := -1
		if r != nil {
			stderr = r.Stderr
			exitCode = r.ExitCode
		}
		logger.Info("EVAL_STAGE_INIT_SCRIPT_DONE",
			zap.String("instance_id", inst.ID),
			zap.String("case_id", inst.CaseID),
			zap.String("workdir", workdir),
			zap.Int("exit_code", exitCode),
			zap.Int64("duration_ms", time.Since(initStart).Milliseconds()),
			zap.NamedError("run_err", rerr))
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
		logger.Error("EVAL_STAGE_SNAPSHOT_LOAD_ERR",
			zap.String("instance_id", inst.ID),
			zap.String("snapshot_id", inst.AgentConfigSnapshotID),
			zap.Error(err))
		e.finalizeError(ctx, inst, ev.InstanceStatusInitializing, ev.InstanceStatusFailed,
			"load snapshot: "+err.Error())
		return nil
	}

	// INITIALIZING → RUNNING
	ok, _ := e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusInitializing, ev.InstanceStatusRunning)
	if !ok {
		logger.Warn("EVAL_STAGE_RUN_CAS_MISS",
			zap.String("instance_id", inst.ID),
			zap.String("hint", "已被 timeout 巡检抢占或状态被外部改动"))
		// 安全退出：清理 workspace + 触发 task recount 让上层感知
		e.cleanupAndRecount(ctx, inst)
		return nil
	}
	// task_prompt 占位符替换：把 ${EVAL_WORKSPACE} 展开为实际 workspace 绝对路径，
	// 让 agent 能拿到绝对路径喂给 fileRead（相对路径工具 fileEdit / fileWrite 不受影响）。
	query := strings.ReplaceAll(inst.CaseSnapshot.TaskPrompt, evalWorkspacePlaceholder, workdir)
	chatStart := time.Now()
	logger.Info("EVAL_STAGE_RUN_ENTER",
		zap.String("instance_id", inst.ID),
		zap.String("task_id", inst.TaskID),
		zap.String("case_id", inst.CaseID),
		zap.String("conversation_id", inst.ConversationID),
		zap.String("message_id", inst.MessageID),
		zap.String("snapshot_id", snap.ID),
		zap.String("model", snap.Model.ModelName),
		zap.String("workdir", workdir),
		zap.Int("prompt_len", len(query)),
		zap.Bool("workspace_placeholder_used", strings.Contains(inst.CaseSnapshot.TaskPrompt, evalWorkspacePlaceholder)))

	// chat 阶段（Task 4.5.3）
	chatRes, cerr := e.chatRunner.Run(stageCtx, InternalChatReq{
		Snapshot:       snap,
		ConversationID: inst.ConversationID,
		MessageID:      inst.MessageID,
		Query:          query,
		TotalTimeout:   e.chatTimeout,
	})
	chatDur := time.Since(chatStart).Milliseconds()
	if cerr != nil {
		logger.Error("EVAL_STAGE_RUN_ERR",
			zap.String("instance_id", inst.ID),
			zap.Int64("duration_ms", chatDur),
			zap.Error(cerr))
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusFailed,
			"chat runner: "+cerr.Error())
		return nil
	}
	if chatRes.DidTimeout {
		logger.Warn("EVAL_STAGE_RUN_TIMEOUT",
			zap.String("instance_id", inst.ID),
			zap.Int64("duration_ms", chatDur),
			zap.Duration("chat_timeout", e.chatTimeout))
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusTimeout,
			"agent_chat_timeout")
		return nil
	}
	if chatRes.Error != nil {
		logger.Warn("EVAL_STAGE_RUN_AGENT_ERR",
			zap.String("instance_id", inst.ID),
			zap.Int64("duration_ms", chatDur),
			zap.Error(chatRes.Error))
		e.finalizeError(ctx, inst, ev.InstanceStatusRunning, ev.InstanceStatusFailed,
			"agent_error: "+chatRes.Error.Error())
		return nil
	}
	logger.Info("EVAL_STAGE_RUN_DONE",
		zap.String("instance_id", inst.ID),
		zap.Int64("duration_ms", chatDur),
		zap.Int("last_msg_len", len(chatRes.LastAssistantMsg)))

	// RUNNING → VERIFYING
	ok, _ = e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusRunning, ev.InstanceStatusVerifying)
	if !ok {
		logger.Warn("EVAL_STAGE_VERIFY_CAS_MISS",
			zap.String("instance_id", inst.ID),
			zap.String("hint", "RUNNING→VERIFYING 抢占失败"))
		e.cleanupAndRecount(ctx, inst)
		return nil
	}

	// verify_script（Task 4.5.4）
	verifyStart := time.Now()
	vres, verr := e.verifyRunner.Run(stageCtx, workdir, inst.CaseSnapshot.VerifyScript)
	passed := verr == nil && vres != nil && vres.ExitCode == 0
	exitCode := -1
	stdoutBytes, stderrBytes := 0, 0
	if vres != nil {
		exitCode = vres.ExitCode
		stdoutBytes = len(vres.Stdout)
		stderrBytes = len(vres.Stderr)
	}
	logger.Info("EVAL_STAGE_VERIFY_DONE",
		zap.String("instance_id", inst.ID),
		zap.String("case_id", inst.CaseID),
		zap.Bool("passed", passed),
		zap.Int("exit_code", exitCode),
		zap.Int("stdout_bytes", stdoutBytes),
		zap.Int("stderr_bytes", stderrBytes),
		zap.Int64("duration_ms", time.Since(verifyStart).Milliseconds()),
		zap.NamedError("run_err", verr))
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
		logger.Warn("EVAL_STAGE_FINALIZE_ERROR_CAS_MISS",
			zap.String("instance_id", inst.ID),
			zap.String("from", string(from)),
			zap.String("to", string(to)))
	}
	now := time.Now()
	if err := e.resultRepo.Upsert(ctx, &ev.Result{
		InstanceID: inst.ID,
		Passed:     false,
		ErrorLog:   truncate(errMsg, 64<<10),
		FinishedAt: now,
	}); err != nil {
		logger.Error("EVAL_STAGE_FINALIZE_ERROR_RESULT_UPSERT_ERR",
			zap.String("instance_id", inst.ID),
			zap.Error(err))
	}
	if err := e.taskRepo.RecountAndTransit(ctx, inst.TaskID); err != nil {
		logger.Warn("EVAL_STAGE_FINALIZE_ERROR_RECOUNT_ERR",
			zap.String("task_id", inst.TaskID),
			zap.Error(err))
	}
	logger.Warn("EVAL_STAGE_FINALIZE_ERROR",
		zap.String("instance_id", inst.ID),
		zap.String("task_id", inst.TaskID),
		zap.String("case_id", inst.CaseID),
		zap.String("from", string(from)),
		zap.String("to", string(to)),
		zap.String("error_msg", errMsg))
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
		if metrics != nil {
			logger.Info("EVAL_STAGE_AGGREGATE",
				zap.String("instance_id", inst.ID),
				zap.String("conversation_id", inst.ConversationID),
				zap.String("trace_id", metrics.TraceID),
				zap.Bool("degraded", metrics.Degraded),
				zap.Int64("prompt_tokens", metrics.PromptTokens),
				zap.Int64("completion_tokens", metrics.CompletionTokens),
				zap.Int64("total_tokens", metrics.TotalTokens),
				zap.Int64("agent_latency_ms", metrics.AgentLatencyMs))
		}
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
	casOK, _ := e.instRepo.CASStatus(ctx, inst.ID, ev.InstanceStatusVerifying, target)
	if !casOK {
		logger.Warn("EVAL_STAGE_FINALIZE_CAS_MISS",
			zap.String("instance_id", inst.ID),
			zap.String("target", string(target)),
			zap.String("hint", "VERIFYING→terminal CAS 抢占失败，Result 仍会落库"))
	}

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
	if err := e.resultRepo.Upsert(ctx, result); err != nil {
		logger.Error("EVAL_STAGE_FINALIZE_RESULT_UPSERT_ERR",
			zap.String("instance_id", inst.ID),
			zap.Error(err))
	}
	if err := e.taskRepo.RecountAndTransit(ctx, inst.TaskID); err != nil {
		logger.Warn("EVAL_STAGE_FINALIZE_RECOUNT_ERR",
			zap.String("task_id", inst.TaskID),
			zap.Error(err))
	}
	logger.Info("EVAL_STAGE_FINALIZE",
		zap.String("instance_id", inst.ID),
		zap.String("task_id", inst.TaskID),
		zap.String("case_id", inst.CaseID),
		zap.String("final_status", string(target)),
		zap.Bool("passed", result.Passed),
		zap.Int("verify_exit_code", result.VerifyExitCode),
		zap.Int64("total_tokens", result.TotalTokens),
		zap.Int64("agent_latency_ms", result.AgentLatencyMs),
		zap.Int("error_log_bytes", len(result.ErrorLog)))
	e.cleanup(inst)
}

// cleanup 清理 skill 容器与 native workspace 目录（本次评测独占资源）
func (e *InstanceExecutor) cleanup(inst *ev.RunInstance) {
	if e.skillExecutor != nil {
		if err := e.skillExecutor.CleanupMessage(inst.MessageID); err != nil {
			logger.Warn("skillExecutor cleanup 失败",
				zap.String("message_id", inst.MessageID), zap.Error(err))
		}
	}
	if e.nativeProvider != nil {
		if err := e.nativeProvider.Cleanup(inst.MessageID); err != nil {
			logger.Warn("nativeProvider cleanup 失败",
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
