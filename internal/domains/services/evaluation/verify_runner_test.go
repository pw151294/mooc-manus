package evaluation

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyRunner_ExitZero 覆盖脚本 exit 0 场景：ExitCode=0，Stdout 含预期文本
func TestVerifyRunner_ExitZero(t *testing.T) {
	r := NewVerifyRunner(60*time.Second, 64<<10)
	dir := t.TempDir()
	got, err := r.Run(context.Background(), dir, "#!/bin/bash\necho hello\nexit 0\n")
	require.NoError(t, err)
	assert.Equal(t, 0, got.ExitCode)
	assert.Contains(t, got.Stdout, "hello")
}

// TestVerifyRunner_ExitNonZero 覆盖非零退出：Stderr 透传，ExitError 不冒泡为 err
func TestVerifyRunner_ExitNonZero(t *testing.T) {
	r := NewVerifyRunner(60*time.Second, 64<<10)
	dir := t.TempDir()
	got, err := r.Run(context.Background(), dir, "#!/bin/bash\necho bad >&2\nexit 3\n")
	require.NoError(t, err) // ExitError 类不冒泡为 err
	assert.Equal(t, 3, got.ExitCode)
	assert.Contains(t, got.Stderr, "bad")
}

// TestVerifyRunner_Timeout 覆盖超时：ctx.WithTimeout 强制中断，err 含 deadline exceeded
func TestVerifyRunner_Timeout(t *testing.T) {
	r := NewVerifyRunner(300*time.Millisecond, 64<<10)
	dir := t.TempDir()
	got, err := r.Run(context.Background(), dir, "#!/bin/bash\nsleep 5\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
	// 允许 ExitCode 为 -1（进程被 SIGKILL）
	_ = got
}

// TestVerifyRunner_StdoutTruncated 覆盖输出截断：超过 cap 时保留前 cap 字节并追加 [truncated]
func TestVerifyRunner_StdoutTruncated(t *testing.T) {
	cap := 1024
	r := NewVerifyRunner(30*time.Second, cap)
	dir := t.TempDir()
	// 输出 2048 字节
	script := "#!/bin/bash\nfor i in $(seq 1 2048); do printf x; done\n"
	got, err := r.Run(context.Background(), dir, script)
	require.NoError(t, err)
	assert.Contains(t, got.Stdout, "[truncated]")
	assert.LessOrEqual(t, len(got.Stdout), cap+len("\n[truncated]")+16)
}

// TestVerifyRunner_EnvWhitelist 覆盖 env 白名单：宿主机 env 变量不应泄漏到 verify 脚本
func TestVerifyRunner_EnvWhitelist(t *testing.T) {
	r := NewVerifyRunner(10*time.Second, 64<<10)
	dir := t.TempDir()
	// 故意 export 一个 sentinel，脚本内部只能读 PATH/HOME/LANG
	require.NoError(t, os.Setenv("EVAL_SECRET_LEAK", "should_not_leak"))
	defer os.Unsetenv("EVAL_SECRET_LEAK")
	got, err := r.Run(context.Background(), dir, "#!/bin/bash\necho VAL=${EVAL_SECRET_LEAK:-notleaked}\n")
	require.NoError(t, err)
	assert.True(t, strings.Contains(got.Stdout, "notleaked"), "宿主机 env 不应泄漏到 verify 脚本，got: %s", got.Stdout)
}
