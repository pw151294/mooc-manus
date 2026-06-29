package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invokeFileRead(t *testing.T, tool Tool, args map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := tool.Invoke(FileReadFunctionName, string(raw))
	if !r.Success {
		return r.Message, false
	}
	return r.Data.(string), true
}

func TestFileRead_InitMetadata(t *testing.T) {
	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if !tool.HasTool(FileReadFunctionName) {
		t.Fatalf("HasTool fileRead should be true")
	}
	if tool.ProviderName() != NativeProviderName {
		t.Fatalf("provider mismatch: %s", tool.ProviderName())
	}
}

func TestFileRead_HappyPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(target, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}
	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	_ = tool.Init()

	out, ok := invokeFileRead(t, tool, map[string]any{"path": target})
	if !ok {
		t.Fatalf("expect success, got: %s", out)
	}
	if !strings.Contains(out, "     1\tline1") {
		t.Fatalf("missing line number prefix: %q", out)
	}
	if !strings.Contains(out, "     3\tline3") {
		t.Fatalf("missing line3: %q", out)
	}
}

func TestFileRead_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString("L")
		sb.WriteByte(byte('0' + i%10))
		sb.WriteByte('\n')
	}
	_ = os.WriteFile(target, []byte(sb.String()), 0644)

	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	_ = tool.Init()
	out, ok := invokeFileRead(t, tool, map[string]any{
		"path":   target,
		"offset": 3,
		"limit":  2,
	})
	if !ok {
		t.Fatalf("expect success, got: %s", out)
	}
	if strings.Contains(out, "     1\t") {
		t.Fatalf("offset should skip line 1: %q", out)
	}
	if !strings.Contains(out, "     3\t") || !strings.Contains(out, "     4\t") {
		t.Fatalf("expect lines 3 and 4: %q", out)
	}
	if strings.Contains(out, "     5\t") {
		t.Fatalf("limit should cap at 2 lines: %q", out)
	}
}

func TestFileRead_SensitivePathBlocked(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	_ = os.WriteFile(secret, []byte("topsecret"), 0644)
	ws := NewNativeWorkspace(t.TempDir(), []string{dir}, 0)
	tool := NewFileReadTool(ws)
	_ = tool.Init()

	msg, ok := invokeFileRead(t, tool, map[string]any{"path": secret})
	if ok {
		t.Fatalf("expect block, got success: %s", msg)
	}
	if !strings.Contains(msg, "敏感路径黑名单") {
		t.Fatalf("expect deny message, got: %s", msg)
	}
}

func TestFileRead_BinaryRejected(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "blob")
	_ = os.WriteFile(target, []byte{0x00, 0x01, 0x02, 0x03}, 0644)

	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	_ = tool.Init()
	msg, ok := invokeFileRead(t, tool, map[string]any{"path": target})
	if ok {
		t.Fatalf("expect binary file rejected, got success: %s", msg)
	}
	if !strings.Contains(msg, "NUL") {
		t.Fatalf("expect NUL-byte hint, got: %s", msg)
	}
}

func TestFileRead_NotExist(t *testing.T) {
	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	_ = tool.Init()
	msg, ok := invokeFileRead(t, tool, map[string]any{"path": "/nonexistent/foo.txt"})
	if ok {
		t.Fatalf("expect failure, got success: %s", msg)
	}
	if !strings.Contains(msg, "文件不存在") {
		t.Fatalf("expect not-exist hint, got: %s", msg)
	}
}

func TestFileRead_IsDir(t *testing.T) {
	dir := t.TempDir()
	tool := NewFileReadTool(NewNativeWorkspace(t.TempDir(), nil, 0))
	_ = tool.Init()
	msg, ok := invokeFileRead(t, tool, map[string]any{"path": dir})
	if ok {
		t.Fatalf("expect failure for dir, got success: %s", msg)
	}
	if !strings.Contains(msg, "目录") {
		t.Fatalf("expect dir hint, got: %s", msg)
	}
}

func TestFileRead_TooLarge(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "big.txt")
	_ = os.WriteFile(target, []byte("hello\nworld\n"), 0644)
	ws := NewNativeWorkspace(t.TempDir(), nil, 5) // 5 字节上限
	tool := NewFileReadTool(ws)
	_ = tool.Init()
	msg, ok := invokeFileRead(t, tool, map[string]any{"path": target})
	if ok {
		t.Fatalf("expect failure, got success: %s", msg)
	}
	if !strings.Contains(msg, "过大") {
		t.Fatalf("expect oversize hint, got: %s", msg)
	}
}
