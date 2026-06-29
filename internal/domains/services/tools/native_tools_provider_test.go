package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mooc-manus/config"
)

// newTestProvider 测试辅助：用 t.TempDir() 作为 WorkspaceBaseDir 构造 provider
// bash 配置使用与 bash_exec_test.go 一致的小上限便于测试
func newTestProvider(t *testing.T) NativeToolsProvider {
	t.Helper()
	return NewNativeToolsProvider(config.NativeConfig{
		WorkspaceBaseDir:      t.TempDir(),
		SensitivePathDenyList: nil,
		MaxFileReadBytes:      0, // 走默认 10 MiB
		BashCommandDenyList:   nil,
		BashTimeoutDefault:    5,
		BashTimeoutMax:        10,
		BashOutputCap:         1024,
		BashConcurrency:       4,
	}, "")
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
	p := NewNativeToolsProvider(config.NativeConfig{
		WorkspaceBaseDir:   baseDir,
		BashTimeoutDefault: 5,
		BashTimeoutMax:     10,
		BashOutputCap:      1024,
		BashConcurrency:    4,
	}, "")
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
	p := NewNativeToolsProvider(config.NativeConfig{
		WorkspaceBaseDir:   baseDir,
		BashTimeoutDefault: 5,
		BashTimeoutMax:     10,
		BashOutputCap:      1024,
		BashConcurrency:    4,
	}, "")
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

// TestNativeToolsProvider_WorkspaceBaseDirFallback 验证 WorkspaceBaseDir 为空时
// 自动回退到 ${storageRootDir}/native-workspace
func TestNativeToolsProvider_WorkspaceBaseDirFallback(t *testing.T) {
	storageRoot := t.TempDir()
	p := NewNativeToolsProvider(config.NativeConfig{
		WorkspaceBaseDir:   "", // 故意留空触发回退
		BashTimeoutDefault: 5,
		BashTimeoutMax:     10,
		BashOutputCap:      1024,
		BashConcurrency:    4,
	}, storageRoot)

	// 用 Cleanup 路径间接验证 provider 真的把 workspace 拼到了回退目录
	tools, err := p.BuildTools("msg-fb")
	if err != nil {
		t.Fatalf("BuildTools failed: %v", err)
	}

	// 触发 fileEdit 写一个文件，验证写入到回退目录下
	var fileEdit Tool
	for _, tool := range tools {
		if tool.HasTool(FileEditFunctionName) {
			fileEdit = tool
			break
		}
	}
	expectedDir := filepath.Join(storageRoot, "native-workspace", "msg-fb")
	if err := os.MkdirAll(expectedDir, 0755); err != nil {
		t.Fatalf("seed workspace failed: %v", err)
	}
	target := filepath.Join(expectedDir, "a.txt")
	if err := os.WriteFile(target, []byte("hello fallback\n"), 0644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}
	r := fileEdit.Invoke(FileEditFunctionName, `{"path":"a.txt","old_string":"fallback","new_string":"works"}`)
	if !r.Success {
		t.Fatalf("fileEdit failed (回退目录未生效): %s", r.Message)
	}
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), "works") {
		t.Fatalf("expect 'works' in file, got: %q", string(got))
	}

	// 清理
	_ = p.Cleanup("msg-fb")
}
