package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestProvider(t *testing.T) NativeToolsProvider {
	t.Helper()
	return NewNativeToolsProvider(NativeToolsOptions{
		WorkspaceBaseDir:      t.TempDir(),
		SensitivePathDenyList: nil,
		MaxFileReadBytes:      0, // 走默认 10 MiB
		BashCommandDenyList:   nil,
		BashTimeoutDefaultSec: 5,
		BashTimeoutMaxSec:     10,
		BashOutputCap:         1024,
		BashConcurrency:       4,
	})
}

func TestNativeToolsProvider_BuildTools(t *testing.T) {
	p := newTestProvider(t)
	tools, err := p.BuildTools("msg-1")
	if err != nil {
		t.Fatalf("BuildTools failed: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expect 3 tools, got %d", len(tools))
	}

	// 三个工具的 FunctionName 必须齐全
	want := map[string]bool{
		FileReadFunctionName: false,
		FileEditFunctionName: false,
		BashExecFunctionName: false,
	}
	for _, tool := range tools {
		for name := range want {
			if tool.HasTool(name) {
				want[name] = true
			}
		}
		if tool.ProviderName() != NativeProviderName {
			t.Fatalf("expect provider name %s, got %s", NativeProviderName, tool.ProviderName())
		}
	}
	for name, hit := range want {
		if !hit {
			t.Fatalf("missing tool: %s", name)
		}
	}
}

func TestNativeToolsProvider_BuildToolsDifferentMessageIds(t *testing.T) {
	p := newTestProvider(t)
	// 两次不同 messageId 的 Build 都成功，且彼此互不污染
	if _, err := p.BuildTools("msg-A"); err != nil {
		t.Fatalf("Build for msg-A failed: %v", err)
	}
	if _, err := p.BuildTools("msg-B"); err != nil {
		t.Fatalf("Build for msg-B failed: %v", err)
	}
}

func TestNativeToolsProvider_Cleanup(t *testing.T) {
	baseDir := t.TempDir()
	p := NewNativeToolsProvider(NativeToolsOptions{
		WorkspaceBaseDir:      baseDir,
		BashTimeoutDefaultSec: 5,
		BashTimeoutMaxSec:     10,
		BashOutputCap:         1024,
		BashConcurrency:       4,
	})
	// 预先建一个 workspace 子目录 + 文件，确认 Cleanup 后被清除
	wsDir := filepath.Join(baseDir, "msg-x")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatalf("seed workspace failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "a.txt"), []byte("hi"), 0644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}
	if err := p.Cleanup("msg-x"); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if _, err := os.Stat(wsDir); err == nil {
		t.Fatalf("workspace dir should be removed: %s", wsDir)
	}
	// 空 messageId no-op
	if err := p.Cleanup(""); err != nil {
		t.Fatalf("Cleanup empty should no-op, got %v", err)
	}
}

func TestNativeToolsProvider_FileEditUsesProviderWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	p := NewNativeToolsProvider(NativeToolsOptions{
		WorkspaceBaseDir:      baseDir,
		BashTimeoutDefaultSec: 5,
		BashTimeoutMaxSec:     10,
		BashOutputCap:         1024,
		BashConcurrency:       4,
	})
	tools, err := p.BuildTools("msg-edit")
	if err != nil {
		t.Fatalf("BuildTools failed: %v", err)
	}

	// 拿到 fileEdit 工具，验证它写入到 provider 持有的 baseDir 子目录
	var fileEdit Tool
	for _, tool := range tools {
		if tool.HasTool(FileEditFunctionName) {
			fileEdit = tool
			break
		}
	}
	if fileEdit == nil {
		t.Fatalf("fileEdit tool not found")
	}

	// 在 workspace 内手动 seed 一个文件
	wsDir := filepath.Join(baseDir, "msg-edit")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatalf("seed workspace failed: %v", err)
	}
	target := filepath.Join(wsDir, "a.txt")
	if err := os.WriteFile(target, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	r := fileEdit.Invoke(FileEditFunctionName, `{"path":"a.txt","old_string":"world","new_string":"manus"}`)
	if !r.Success {
		t.Fatalf("fileEdit failed: %s", r.Message)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "hello manus\n" {
		t.Fatalf("file content not updated: %q", string(got))
	}
}
