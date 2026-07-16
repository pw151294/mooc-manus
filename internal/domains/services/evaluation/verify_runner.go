package evaluation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// VerifyResult 保存 verify_script 一次执行的结果快照
// - ExitCode: 子进程退出码；超时被 SIGKILL 时可能为 -1
// - Stdout/Stderr: 各自独立按 cap 截断后的输出内容
// - Duration: 从 exec.Cmd.Run 起算的总耗时（含 fork/exec 开销）
type VerifyResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

// VerifyRunner 在受限工作区内执行 verify_script 判定评测结果
// - 超时: 通过 context.WithTimeout 强制中断
// - 输出上限: stdout/stderr 各独立按 cap 截断，超出追加 "\n[truncated]"
// - env 白名单: PATH / HOME / LANG，杜绝宿主机敏感变量注入（spec §11 风险 6）
type VerifyRunner struct {
	timeout time.Duration
	cap     int
}

// NewVerifyRunner 构造 verify_script 执行器
// timeout: 单次脚本执行硬上限（>0）；cap: stdout / stderr 各自的字节上限（>0）
func NewVerifyRunner(timeout time.Duration, cap int) *VerifyRunner {
	return &VerifyRunner{timeout: timeout, cap: cap}
}

// Run 在 workdir 下执行 verify_script
// - 若 ctx 已带超时，取 runner 与 ctx 中较短者为准（context.WithTimeout 语义）
// - runErr 为 nil 且 ExitCode==0 视为通过
// - runErr 非 nil 时若为超时（ctx.Err() != nil）也一并返回，供上层区分 verify_timeout
func (r *VerifyRunner) Run(ctx context.Context, workdir, script string) (*VerifyResult, error) {
	// 1. 写脚本到 workdir/.verify.sh，权限 0700（仅 owner 可读写执行）
	path := filepath.Join(workdir, ".verify.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return nil, err
	}

	// 2. 起 context.WithTimeout：runner 硬上限与调用方 ctx 取较短者
	ctx2, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx2, "bash", path)
	cmd.Dir = workdir
	// env 白名单：不继承宿主机全量 env（spec §11 风险 6 - 敏感变量注入）
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"LANG=C.UTF-8",
	}

	outBuf := &capBuffer{limit: r.cap}
	errBuf := &capBuffer{limit: r.cap}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf

	start := time.Now()
	runErr := cmd.Run()
	res := &VerifyResult{
		Stdout:   outBuf.String(),
		Stderr:   errBuf.String(),
		Duration: time.Since(start),
	}

	// 3. 分类处理 runErr
	// - ExitError（非零退出）：ExitCode 透传，err 置空，但若同时超时优先暴露超时
	// - 非 ExitError 但 ctx 已超时：返回 ctx 错误，供上层识别 verify_timeout
	// - 其它 fork/exec 类错误：原样冒泡
	if ee, ok := runErr.(*exec.ExitError); ok {
		res.ExitCode = ee.ExitCode()
		if ctx2.Err() != nil {
			return res, ctx2.Err()
		}
		return res, nil
	}
	if ctx2.Err() != nil {
		return res, ctx2.Err()
	}
	return res, runErr
}

// capBuffer 带上限的并发安全 io.Writer
// 超过 limit 时保留前 limit 字节 + append "\n[truncated]"，后续 Write 全部丢弃
// 并发安全: exec.Cmd 内部会用不同 goroutine 分别向 Stdout/Stderr 写入，需加锁保护
type capBuffer struct {
	mu      sync.Mutex
	limit   int
	buf     []byte
	dropped bool
}

// Write 实现 io.Writer；对调用方永远返回 len(p) 以避免 exec 提前判定错误
func (b *capBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dropped {
		return len(p), nil
	}
	remain := b.limit - len(b.buf)
	if remain <= 0 {
		b.dropped = true
		b.buf = append(b.buf, "\n[truncated]"...)
		return len(p), nil
	}
	if len(p) <= remain {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	b.buf = append(b.buf, p[:remain]...)
	b.buf = append(b.buf, "\n[truncated]"...)
	b.dropped = true
	return len(p), nil
}

// String 返回当前缓冲的字符串快照
func (b *capBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
