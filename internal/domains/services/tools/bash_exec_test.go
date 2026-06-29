package tools

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func invokeBashExec(t *testing.T, tool Tool, args map[string]any) (string, any, bool) {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := tool.Invoke(BashExecFunctionName, string(raw))
	return r.Message, r.Data, r.Success
}

func newBashExecTool(t *testing.T, deny *BashDenyList) Tool {
	t.Helper()
	if deny == nil {
		deny = NewBashDenyList(nil)
	}
	tool := NewBashExecTool(deny, 5, 10, 1024, 4, "msg-test")
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return tool
}

func TestBashExec_HappyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	tool := newBashExecTool(t, nil)
	msg, data, ok := invokeBashExec(t, tool, map[string]any{
		"command":     "echo hello-manus",
		"description": "smoke test",
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}
	s := data.(string)
	if !strings.Contains(s, "exit=0") {
		t.Fatalf("expect exit=0, got: %s", s)
	}
	if !strings.Contains(s, "hello-manus") {
		t.Fatalf("expect output to include 'hello-manus', got: %s", s)
	}
}

func TestBashExec_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	tool := newBashExecTool(t, nil)
	msg, _, ok := invokeBashExec(t, tool, map[string]any{
		"command":     "exit 7",
		"description": "non-zero exit test",
	})
	if ok {
		t.Fatalf("expect failure on non-zero exit, got success")
	}
	if !strings.Contains(msg, "exit=7") {
		t.Fatalf("expect exit=7 in message, got: %s", msg)
	}
}

func TestBashExec_DenyListBlocks(t *testing.T) {
	tool := newBashExecTool(t, nil)
	msg, _, ok := invokeBashExec(t, tool, map[string]any{
		"command":     "rm -rf /",
		"description": "should be blocked",
	})
	if ok {
		t.Fatalf("expect deny, got success")
	}
	if !strings.Contains(msg, "rm-rf-root") {
		t.Fatalf("expect deny pattern name 'rm-rf-root', got: %s", msg)
	}
}

func TestBashExec_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	// 工具默认超时 5 秒（由 newBashExecTool 设定），命令 timeout_sec 显式设为 1 秒
	tool := newBashExecTool(t, nil)
	start := time.Now()
	msg, _, ok := invokeBashExec(t, tool, map[string]any{
		"command":     "sleep 10",
		"description": "timeout test",
		"timeout_sec": 1,
	})
	elapsed := time.Since(start)
	if ok {
		t.Fatalf("expect timeout failure, got success")
	}
	if !strings.Contains(msg, "超时") {
		t.Fatalf("expect timeout hint, got: %s", msg)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout not honored: %v elapsed", elapsed)
	}
}

func TestBashExec_OutputTruncation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	// outputCap = 1024（newBashExecTool 设定）；让命令产出 > 1024 字节
	tool := newBashExecTool(t, nil)
	_, data, ok := invokeBashExec(t, tool, map[string]any{
		"command":     `for i in $(seq 1 200); do printf "abcdefghij"; done`,
		"description": "output truncation test",
	})
	if !ok {
		t.Fatalf("expect success")
	}
	s := data.(string)
	if !strings.Contains(s, "truncated=true") {
		t.Fatalf("expect truncated=true marker, got: %s", s)
	}
	if !strings.Contains(s, "truncated") || !strings.Contains(s, "bytes") {
		t.Fatalf("expect truncation marker, got: %s", s)
	}
}

func TestBashExec_StderrMerged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash not available on windows")
	}
	tool := newBashExecTool(t, nil)
	_, data, ok := invokeBashExec(t, tool, map[string]any{
		"command":     `echo out; echo err 1>&2`,
		"description": "merge stderr test",
	})
	if !ok {
		t.Fatalf("expect success")
	}
	s := data.(string)
	if !strings.Contains(s, "out") || !strings.Contains(s, "err") {
		t.Fatalf("expect both stdout and stderr, got: %s", s)
	}
	if !strings.Contains(s, "[stderr]") {
		t.Fatalf("expect [stderr] separator, got: %s", s)
	}
}

func TestBashExec_RequiresDescription(t *testing.T) {
	tool := newBashExecTool(t, nil)
	msg, _, ok := invokeBashExec(t, tool, map[string]any{
		"command": "echo x",
	})
	if ok {
		t.Fatalf("expect failure without description")
	}
	if !strings.Contains(msg, "description") {
		t.Fatalf("expect description hint, got: %s", msg)
	}
}

func TestBashExec_CommandTooLong(t *testing.T) {
	tool := newBashExecTool(t, nil)
	long := strings.Repeat("a", bashExecCommandMaxBytes+1)
	msg, _, ok := invokeBashExec(t, tool, map[string]any{
		"command":     long,
		"description": "too long",
	})
	if ok {
		t.Fatalf("expect failure for over-length command")
	}
	if !strings.Contains(msg, "上限") {
		t.Fatalf("expect length-limit hint, got: %s", msg)
	}
}

func TestTruncateCombinedOutput(t *testing.T) {
	stdout := strings.Repeat("a", 100)
	stderr := strings.Repeat("b", 100)
	got, truncated := truncateCombinedOutput([]byte(stdout), []byte(stderr), 50)
	if !truncated {
		t.Fatalf("expect truncated=true")
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expect 'truncated' marker, got: %s", got)
	}

	// 未触发截断
	got, truncated = truncateCombinedOutput([]byte("short"), nil, 100)
	if truncated {
		t.Fatalf("expect not truncated")
	}
	if got != "short" {
		t.Fatalf("unexpected merge output: %s", got)
	}
}
