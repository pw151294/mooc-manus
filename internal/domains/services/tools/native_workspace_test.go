package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile 测试辅助：在 path 写入 content（自动创建父目录）
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// statFile 测试辅助：os.Stat 的别名（不暴露 os 包到测试用例侧）
func statFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func TestNativeWorkspace_WorkspaceDir(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	if got := ws.WorkspaceDir(""); got != "" {
		t.Fatalf("expect empty for empty messageId, got %q", got)
	}
	got := ws.WorkspaceDir("msg-1")
	if !strings.HasSuffix(got, filepath.Join("msg-1")) {
		t.Fatalf("workspace dir tail mismatch: %q", got)
	}
}

func TestNativeWorkspace_EnsureWorkspace(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	if _, err := ws.EnsureWorkspace(""); err == nil {
		t.Fatalf("expect error for empty messageId")
	}
	dir, err := ws.EnsureWorkspace("msg-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace failed: %v", err)
	}
	if !strings.HasPrefix(dir, ws.BaseDir()) {
		t.Fatalf("workspace dir not under baseDir: %q vs %q", dir, ws.BaseDir())
	}
	// idempotent
	if _, err := ws.EnsureWorkspace("msg-1"); err != nil {
		t.Fatalf("EnsureWorkspace second call failed: %v", err)
	}
}

func TestNativeWorkspace_ResolveInWorkspace(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)

	// 正常相对路径
	got, err := ws.ResolveInWorkspace("msg-1", "sub/file.txt")
	if err != nil {
		t.Fatalf("normal relPath failed: %v", err)
	}
	want := filepath.Join(ws.BaseDir(), "msg-1", "sub", "file.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// 拒绝 ../ 逃逸
	if _, err := ws.ResolveInWorkspace("msg-1", "../../etc/passwd"); err == nil {
		t.Fatalf("expect reject of ../ escape, got nil error")
	}
	// 绝对路径会被 filepath.Join 当相对路径处理后落回 workspace 内（与 skill safeJoin 一致）
	// 这里断言最终路径仍在 workspace 边界内
	resolved, err := ws.ResolveInWorkspace("msg-1", "/etc/passwd")
	if err != nil {
		t.Fatalf("abs path should be normalized into workspace, got err: %v", err)
	}
	if !strings.HasPrefix(resolved, ws.WorkspaceDir("msg-1")) {
		t.Fatalf("abs path escaped workspace: %q", resolved)
	}
}

func TestNativeWorkspace_Cleanup(t *testing.T) {
	ws := NewNativeWorkspace(t.TempDir(), nil, 0)
	dir, err := ws.EnsureWorkspace("msg-1")
	if err != nil {
		t.Fatalf("EnsureWorkspace failed: %v", err)
	}
	// 写入一个文件确认 Cleanup 会递归删
	target := filepath.Join(dir, "file.txt")
	if err := writeFile(target, "hi"); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}
	if err := ws.Cleanup("msg-1"); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if _, err := statFile(dir); err == nil {
		t.Fatalf("workspace dir should be removed")
	}
	// 空 messageId no-op
	if err := ws.Cleanup(""); err != nil {
		t.Fatalf("Cleanup empty should be no-op, got %v", err)
	}
}

func TestNativeWorkspace_IsSensitivePath(t *testing.T) {
	deny := []string{"/etc/shadow", "/root/.ssh", "~/.aws"}
	ws := NewNativeWorkspace(t.TempDir(), deny, 0)

	cases := []struct {
		name string
		path string
		hit  bool
	}{
		{"exact match shadow", "/etc/shadow", true},
		{"subdir under ssh", "/root/.ssh/id_rsa", true},
		{"sibling not under shadow", "/etc/shadow_backup", false},
		{"unrelated", "/tmp/foo.txt", false},
		{"relative path rejected", "etc/shadow", false}, // 非绝对路径直接拒绝命中
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ws.IsSensitivePath(c.path); got != c.hit {
				t.Fatalf("path=%q want hit=%v got=%v", c.path, c.hit, got)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	if got := expandHome("~"); got != "/Users/test" {
		t.Fatalf("~ expand failed: %q", got)
	}
	if got := expandHome("~/.aws"); got != "/Users/test/.aws" {
		t.Fatalf("~/.aws expand failed: %q", got)
	}
	if got := expandHome("/abs/path"); got != "/abs/path" {
		t.Fatalf("abs path should not change: %q", got)
	}
	if got := expandHome(""); got != "" {
		t.Fatalf("empty should stay empty: %q", got)
	}
}
