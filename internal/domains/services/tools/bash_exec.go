package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

const (
	BashExecFunctionID   = "builtin-bash-exec"
	BashExecFunctionName = "bashExec"
	BashExecFunctionDesc = "在 manus 后端进程的项目根目录下执行一条 bash 命令，返回 stdout+stderr 合并文本（可能被截断）和退出码。命令在 manus 后端进程权限下执行；高危命令会被黑名单拦截；超出 timeout 会被 SIGKILL。"

	bashExecCommandMaxBytes = 16 * 1024 // 16 KiB
)

// bashExecAuditLogger 给所有 bashExec 落盘记录打 audit=native-bash 标
// 走与业务日志同一条管道，便于 ELK / grep 过滤
var (
	bashExecAuditLoggerOnce sync.Once
	bashExecAuditLogger     logger.Logger
)

func getBashExecAuditLogger() logger.Logger {
	bashExecAuditLoggerOnce.Do(func() {
		bashExecAuditLogger = logger.GetGlobalLogger().With(zap.String("audit", "native-bash"))
	})
	return bashExecAuditLogger
}

// bashExecSemaphore 进程级 bashExec 并发限流
// 容量在首个 BashExecTool 构造时初始化（取首个非零 BashConcurrency 配置）；后续重建工具不重置
var (
	bashExecSemOnce sync.Once
	bashExecSem     chan struct{}
)

func acquireBashExecSlot(capacity int) {
	bashExecSemOnce.Do(func() {
		if capacity <= 0 {
			capacity = 4
		}
		bashExecSem = make(chan struct{}, capacity)
	})
	bashExecSem <- struct{}{}
}

func releaseBashExecSlot() {
	if bashExecSem == nil {
		return
	}
	<-bashExecSem
}

// BashExecTool bashExec 内置工具：本地 exec.CommandContext 直跑，叠加黑名单 + 硬限 + audit
// R-48 偏离声明：见 .harness/rules/49-native-builtin.md
type BashExecTool struct {
	BaseTool
	denyList         *BashDenyList
	cwd              string // 工具构造时一次性 os.Getwd()，每次 Invoke 复用
	timeoutDefault   time.Duration
	timeoutMax       time.Duration
	outputCap        int
	concurrencyLimit int
	messageId        string // 仅用于 audit 日志关联
}

// NewBashExecTool 构造 BashExecTool
// 各上限若 <= 0 则回退到 plan §6.2 默认值；cwd 取构造时 os.Getwd()
func NewBashExecTool(
	denyList *BashDenyList,
	timeoutDefaultSec int,
	timeoutMaxSec int,
	outputCap int,
	concurrencyLimit int,
	messageId string,
) Tool {
	if timeoutDefaultSec <= 0 {
		timeoutDefaultSec = 120
	}
	if timeoutMaxSec <= 0 {
		timeoutMaxSec = 600
	}
	if outputCap <= 0 {
		outputCap = 32 * 1024
	}
	if concurrencyLimit <= 0 {
		concurrencyLimit = 4
	}
	cwd, err := os.Getwd()
	if err != nil {
		// 启动时拿不到 cwd 是极端异常，落到 "." 让运行时报错暴露问题
		cwd = "."
	}
	return &BashExecTool{
		denyList:         denyList,
		cwd:              cwd,
		timeoutDefault:   time.Duration(timeoutDefaultSec) * time.Second,
		timeoutMax:       time.Duration(timeoutMaxSec) * time.Second,
		outputCap:        outputCap,
		concurrencyLimit: concurrencyLimit,
		messageId:        messageId,
	}
}

func (t *BashExecTool) Init() error {
	funcDO := models.ToolFunctionDO{
		FunctionID:   BashExecFunctionID,
		ProviderID:   NativeProviderID,
		FunctionName: BashExecFunctionName,
		FunctionDesc: BashExecFunctionDesc,
		Schema: models.ToolSchema{
			Name:        BashExecFunctionName,
			Description: BashExecFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": fmt.Sprintf("要执行的 bash 命令，长度不超过 %d 字节", bashExecCommandMaxBytes),
					},
					"timeout_sec": map[string]any{
						"type":        "integer",
						"description": fmt.Sprintf("超时秒数，默认 %d，上限 %d", int(t.timeoutDefault/time.Second), int(t.timeoutMax/time.Second)),
					},
					"description": map[string]any{
						"type":        "string",
						"description": "本次命令的用途简述，用于审计日志（必填）",
					},
					"risk_level": map[string]any{
						"type":        "string",
						"enum":        []string{"safe", "dangerous"},
						"description": "本次命令的风险等级；仅当命令确定不会造成任何数据丢失、权限变更、外部副作用时才可为 safe。以下类型必须标注为 dangerous：\n1. 删除类：rm -rf、find ... -delete、mkfs、dd\n2. 权限变更类：chmod 777、chown、sudo、setuid\n3. 网络下载执行类：curl ... | sh、wget ... | bash、任何管道到 shell 的模式\n4. 系统关键路径写入：/etc、/boot、/usr、/System、系统 crontab、~/.ssh/authorized_keys\n5. 进程/系统级操作：kill -9、pkill、systemctl、fork bomb\n6. 数据库破坏性操作：DROP、TRUNCATE、DELETE 全表",
					},
					"risk_reason": map[string]any{
						"type":        "string",
						"description": "本次风险等级的判断依据；若为 safe 也需一句话说明为何安全",
					},
				},
				"required": []string{"command", "description", "risk_level", "risk_reason"},
			},
		},
	}
	t.BaseTool.providerId = NativeProviderID
	t.BaseTool.providerName = NativeProviderName
	t.BaseTool.providerType = NativeProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}
	return nil
}

func (t *BashExecTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	var params struct {
		Command     string `json:"command"`
		TimeoutSec  int    `json:"timeout_sec"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal bashExec args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}
	if params.Command == "" {
		return models.ToolCallResult{Success: false, Message: "Error: command parameter is required"}
	}
	if params.Description == "" {
		return models.ToolCallResult{Success: false, Message: "Error: description parameter is required（请简述命令用途，将记入审计日志）"}
	}
	if len(params.Command) > bashExecCommandMaxBytes {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: command 长度 %d 超过上限 %d", len(params.Command), bashExecCommandMaxBytes),
		}
	}

	// 黑名单
	if t.denyList != nil {
		if name := t.denyList.Match(params.Command); name != "" {
			t.audit(params.Command, params.Description, -1, 0, 0, 0, false, name)
			return models.ToolCallResult{
				Success: false,
				Message: fmt.Sprintf("Error: 命令被拒绝：命中黑名单模式 %s。如确需执行请重新设计命令避开高危模式。", name),
			}
		}
	}

	// timeout clamp
	timeout := t.timeoutDefault
	if params.TimeoutSec > 0 {
		timeout = time.Duration(params.TimeoutSec) * time.Second
	}
	if timeout > t.timeoutMax {
		timeout = t.timeoutMax
	}

	// 并发限流
	acquireBashExecSlot(t.concurrencyLimit)
	defer releaseBashExecSlot()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	cmd.Dir = t.cwd
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	// 环境变量继承父进程

	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = -1
			}
		} else {
			exitCode = -1
		}
	}

	stdoutLen := stdoutBuf.Len()
	stderrLen := stderrBuf.Len()
	merged, truncated := truncateCombinedOutput(stdoutBuf.Bytes(), stderrBuf.Bytes(), t.outputCap)
	t.audit(params.Command, params.Description, exitCode, stdoutLen, stderrLen, duration, truncated, "")

	if timedOut {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: 命令超时（%v）被 SIGKILL\nexit=-1, truncated=%v\n%s", timeout, truncated, merged),
		}
	}
	if runErr != nil {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("exit=%d, truncated=%v\n%s\n(执行失败：%v)", exitCode, truncated, merged, runErr),
		}
	}
	return models.ToolCallResult{
		Success: true,
		Data:    fmt.Sprintf("exit=%d, truncated=%v\n%s", exitCode, truncated, merged),
	}
}

func (t *BashExecTool) InvokeWithContext(ctx context.Context, funcName, funcArgs string) models.ToolCallResult {
	return t.Invoke(funcName, funcArgs)
}

// audit 把一次 bashExec 调用的元数据落到 native-bash-audit logger
// denyName 非空表示该次调用被黑名单拒绝，没有真正 exec
func (t *BashExecTool) audit(
	command, description string,
	exitCode, stdoutBytes, stderrBytes int,
	duration time.Duration,
	truncated bool,
	denyName string,
) {
	getBashExecAuditLogger().Info("bashExec",
		zap.String("message_id", t.messageId),
		zap.String("function_name", BashExecFunctionName),
		zap.String("command", command),
		zap.String("description", description),
		zap.Int("exit_code", exitCode),
		zap.Int64("duration_ms", duration.Milliseconds()),
		zap.Int("stdout_bytes", stdoutBytes),
		zap.Int("stderr_bytes", stderrBytes),
		zap.Bool("truncated", truncated),
		zap.String("denied_by", denyName),
	)
}

// truncateCombinedOutput 把 stdout+stderr 合并并按 cap 截断
// 截断策略：保留头部 cap/16、尾部 cap*15/16，中间填提示
// 返回 (合并后文本, 是否截断)
func truncateCombinedOutput(stdout, stderr []byte, cap int) (string, bool) {
	var merged bytes.Buffer
	merged.Write(stdout)
	if len(stderr) > 0 {
		if merged.Len() > 0 && merged.Bytes()[merged.Len()-1] != '\n' {
			merged.WriteByte('\n')
		}
		merged.WriteString("[stderr]\n")
		merged.Write(stderr)
	}
	if merged.Len() <= cap || cap <= 0 {
		return merged.String(), false
	}
	headLen := cap / 16
	tailLen := cap - headLen
	if headLen < 0 || tailLen < 0 {
		return merged.String()[:cap], true
	}
	data := merged.Bytes()
	dropped := merged.Len() - headLen - tailLen
	var buf bytes.Buffer
	buf.Write(data[:headLen])
	fmt.Fprintf(&buf, "\n[... truncated %d bytes ...]\n", dropped)
	buf.Write(data[merged.Len()-tailLen:])
	return buf.String(), true
}

// SupportsRiskAssessment 覆写 BaseTool 默认实现；bashExec 是 HITL 首发接入的工具
func (t *BashExecTool) SupportsRiskAssessment() bool { return true }
