package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func invokeFileWrite(t *testing.T, tool Tool, args map[string]any) (string, any, bool) {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := tool.Invoke(FileWriteFunctionName, string(raw))
	return r.Message, r.Data, r.Success
}

func newFileWriteTool(t *testing.T, messageId string) (Tool, *NativeWorkspace) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	tool := NewFileWriteTool(ws, messageId, "")
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return tool, ws
}

func TestFileWrite_CreateNewFile(t *testing.T) {
	tool, ws := newFileWriteTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")

	msg, data, ok := invokeFileWrite(t, tool, map[string]any{
		"path":    "test.txt",
		"content": "Hello, World!",
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}
	if data == nil {
		t.Fatal("expect data not nil")
	}

	// 验证文件已创建
	target := filepath.Join(dir, "test.txt")
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if string(content) != "Hello, World!" {
		t.Fatalf("expect 'Hello, World!', got: %q", string(content))
	}
}

func TestFileWrite_OverwriteExistingFile(t *testing.T) {
	tool, ws := newFileWriteTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "test.txt")

	// 预先创建文件
	_ = os.WriteFile(target, []byte("old content"), 0644)

	// 覆盖写入
	msg, _, ok := invokeFileWrite(t, tool, map[string]any{
		"path":    "test.txt",
		"content": "new content",
		"append":  false,
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}

	// 验证内容已覆盖
	content, _ := os.ReadFile(target)
	if string(content) != "new content" {
		t.Fatalf("expect 'new content', got: %q", string(content))
	}
}

func TestFileWrite_AppendToExistingFile(t *testing.T) {
	tool, ws := newFileWriteTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "test.txt")

	// 预先创建文件
	_ = os.WriteFile(target, []byte("line1\n"), 0644)

	// 追加写入
	msg, _, ok := invokeFileWrite(t, tool, map[string]any{
		"path":    "test.txt",
		"content": "line2\n",
		"append":  true,
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}

	// 验证内容已追加
	content, _ := os.ReadFile(target)
	if string(content) != "line1\nline2\n" {
		t.Fatalf("expect 'line1\\nline2\\n', got: %q", string(content))
	}
}

func TestFileWrite_PersistentMode(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	tool := NewFileWriteTool(ws, "msg-1", "conv-123")
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// persistent=true 写入
	msg, _, ok := invokeFileWrite(t, tool, map[string]any{
		"path":       "Plan.md",
		"content":    "# Project Plan\n",
		"persistent": true,
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}

	// 验证文件写入持久化目录
	planDir := ws.ConversationPlanDir("conv-123")
	target := filepath.Join(planDir, "Plan.md")
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read persistent file failed: %v", err)
	}
	if string(content) != "# Project Plan\n" {
		t.Fatalf("expect '# Project Plan\\n', got: %q", string(content))
	}
}

func TestFileWrite_RequireConversationIdForPersistent(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	tool := NewFileWriteTool(ws, "msg-1", "") // conversationId 为空
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// persistent=true 但 conversationId 为空，应返回错误
	msg, _, ok := invokeFileWrite(t, tool, map[string]any{
		"path":       "Plan.md",
		"content":    "content",
		"persistent": true,
	})
	if ok {
		t.Fatalf("expect failure when conversationId empty with persistent=true")
	}
	if msg != "Error: persistent=true 时需要 conversationId，当前未注入" {
		t.Fatalf("unexpected error message: %s", msg)
	}
}

func TestFileWrite_EmptyPath(t *testing.T) {
	tool, _ := newFileWriteTool(t, "m1")

	msg, _, ok := invokeFileWrite(t, tool, map[string]any{
		"path":    "",
		"content": "test",
	})
	if ok {
		t.Fatal("expect failure when path empty")
	}
	if msg != "Error: path parameter is required" {
		t.Fatalf("unexpected error message: %s", msg)
	}
}
