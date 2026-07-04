package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invokeFileEdit(t *testing.T, tool Tool, args map[string]any) (string, any, bool) {
	t.Helper()
	raw, _ := json.Marshal(args)
	r := tool.Invoke(FileEditFunctionName, string(raw))
	return r.Message, r.Data, r.Success
}

func newFileEditTool(t *testing.T, messageId string) (Tool, *NativeWorkspace) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	tool := NewFileEditTool(ws, messageId, "")
	if err := tool.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return tool, ws
}

func TestFileEdit_HappyPath(t *testing.T) {
	tool, ws := newFileEditTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(target, []byte("hello world\n"), 0644)

	msg, data, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "world",
		"new_string": "manus",
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}
	if !strings.Contains(data.(string), "1 replacement") {
		t.Fatalf("expect 1 replacement message, got: %v", data)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "hello manus\n" {
		t.Fatalf("file content not updated: %q", string(got))
	}
}

func TestFileEdit_NonUniqueRejected(t *testing.T) {
	tool, ws := newFileEditTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(target, []byte("foo\nfoo\nfoo\n"), 0644)

	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "foo",
		"new_string": "bar",
	})
	if ok {
		t.Fatalf("expect failure due to non-unique, got success")
	}
	if !strings.Contains(msg, "出现了 3 次") {
		t.Fatalf("expect occurrence count hint, got: %s", msg)
	}
	// 行号回灌：前 3 个匹配 = lines [1 2 3]
	if !strings.Contains(msg, "[1 2 3]") {
		t.Fatalf("expect line numbers [1 2 3], got: %s", msg)
	}
	// 文件内容未被修改
	got, _ := os.ReadFile(target)
	if string(got) != "foo\nfoo\nfoo\n" {
		t.Fatalf("file should not be modified, got: %q", string(got))
	}
}

func TestFileEdit_ReplaceAll(t *testing.T) {
	tool, ws := newFileEditTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(target, []byte("x\nx\nx\n"), 0644)

	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":        "a.txt",
		"old_string":  "x",
		"new_string":  "y",
		"replace_all": true,
	})
	if !ok {
		t.Fatalf("expect success, got: %s", msg)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "y\ny\ny\n" {
		t.Fatalf("replace_all failed: %q", string(got))
	}
}

func TestFileEdit_NotFound(t *testing.T) {
	tool, ws := newFileEditTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(target, []byte("hi\n"), 0644)

	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "nope",
		"new_string": "yep",
	})
	if ok {
		t.Fatalf("expect failure, got success")
	}
	if !strings.Contains(msg, "未在文件中找到") {
		t.Fatalf("expect not-found hint, got: %s", msg)
	}
}

func TestFileEdit_SameOldNew(t *testing.T) {
	tool, _ := newFileEditTool(t, "m1")
	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "x",
		"new_string": "x",
	})
	if ok {
		t.Fatalf("expect failure for identical old/new, got success")
	}
	if !strings.Contains(msg, "相同") {
		t.Fatalf("expect 'same content' hint, got: %s", msg)
	}
}

func TestFileEdit_PathEscapeBlocked(t *testing.T) {
	tool, _ := newFileEditTool(t, "m1")
	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "../../etc/passwd",
		"old_string": "root",
		"new_string": "nope",
	})
	if ok {
		t.Fatalf("expect failure for path escape, got success")
	}
	if !strings.Contains(msg, "escape") {
		t.Fatalf("expect escape hint, got: %s", msg)
	}
}

func TestFileEdit_RequiresMessageId(t *testing.T) {
	tool, _ := newFileEditTool(t, "")
	msg, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "x",
		"new_string": "y",
	})
	if ok {
		t.Fatalf("expect failure with empty messageId, got success")
	}
	if !strings.Contains(msg, "messageId") {
		t.Fatalf("expect messageId hint, got: %s", msg)
	}
}

func TestFileEdit_AtomicNoStrayTmp(t *testing.T) {
	tool, ws := newFileEditTool(t, "m1")
	dir, _ := ws.EnsureWorkspace("m1")
	target := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(target, []byte("alpha\n"), 0644)

	_, _, ok := invokeFileEdit(t, tool, map[string]any{
		"path":       "a.txt",
		"old_string": "alpha",
		"new_string": "beta",
	})
	if !ok {
		t.Fatalf("expect success")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-fileEdit-") {
			t.Fatalf("found stray tmp file: %s", e.Name())
		}
	}
}

func TestLocateMatchLines(t *testing.T) {
	got := locateMatchLines("a\nb\nfoo\nc\nfoo\nfoo\n", "foo", 3)
	if len(got) != 3 || got[0] != 3 || got[1] != 5 || got[2] != 6 {
		t.Fatalf("unexpected lines: %v", got)
	}
	if got := locateMatchLines("x", "", 3); got != nil {
		t.Fatalf("empty needle should return nil, got %v", got)
	}
}
