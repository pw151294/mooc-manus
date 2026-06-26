package tools

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mooc-manus/config"
	"mooc-manus/internal/applications/dtos"
	"mooc-manus/pkg/logger"
)

// TestMain 初始化全局 logger，让 buildEnhancedScript / prepareSkillWorkDir 中的 logger.Info 不再 panic
func TestMain(m *testing.M) {
	tmpLogDir, _ := os.MkdirTemp("", "skill-exec-test-log-*")
	_ = logger.InitGlobalLogger(config.LoggerConfig{
		Level:  "info",
		Format: "console",
		Output: "stdout",
		LogDir: tmpLogDir,
	})
	code := m.Run()
	_ = os.RemoveAll(tmpLogDir)
	os.Exit(code)
}

// ---------- safeJoin 单元测试 ----------

func TestSafeJoin_AcceptsCleanRelativePath(t *testing.T) {
	base := "/tmp/skill-base"
	got, err := safeJoin(base, "sub/dir/file.py")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/tmp/skill-base/sub/dir/file.py"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestSafeJoin_RejectsParentEscape(t *testing.T) {
	base := "/tmp/skill-base"
	cases := []string{
		"../escape.py",
		"sub/../../escape.py",
		"./../../escape.py",
	}
	for _, c := range cases {
		if _, err := safeJoin(base, c); err == nil {
			t.Errorf("expected error for relPath=%q, got nil", c)
		}
	}
}

// 注：Go 的 filepath.Join 对绝对路径形如 "/etc/passwd" 会拼接为 "<base>/etc/passwd"，
// 不会真的跳到 /etc/passwd，所以本场景下 safeJoin 是安全的（已被包含在 base 内）。
// 不再额外断言这种 case，但保留 ../ 跳出的核心防御。

// ---------- buildEnhancedScript 单元测试（T2 路径匹配 + T4 硬失败模板） ----------

func TestBuildEnhancedScript_ReferencesContainerMountPath(t *testing.T) {
	e := &DockerSkillExecutor{}
	ctx := SkillExecutionContext{
		SkillID:   "skill-abc",
		Version:   "v0.1.2",
		MessageID: "msg-xyz",
	}
	script := e.buildEnhancedScript(ctx, "python3 main.py")

	wantPath := "/workspace/skills/skill-abc-v0.1.2"
	if !strings.Contains(script, wantPath) {
		t.Errorf("script does not reference %q\nfull script:\n%s", wantPath, script)
	}
}

func TestBuildEnhancedScript_ContainsHardFailGuard(t *testing.T) {
	e := &DockerSkillExecutor{}
	ctx := SkillExecutionContext{SkillID: "s", Version: "v1"}
	script := e.buildEnhancedScript(ctx, "echo ok")

	if !strings.Contains(script, "set -e") {
		t.Error("expected `set -e` in script")
	}
	expectedExit := fmt.Sprintf("exit %d", SkillFilesMissingExitCode)
	if !strings.Contains(script, expectedExit) {
		t.Errorf("expected %q in script", expectedExit)
	}
	if !strings.Contains(script, "FATAL: no skill files found") {
		t.Error("expected fatal message in script")
	}
	// 关键：不能再有原来的吞错误模式
	if strings.Contains(script, "2>/dev/null;") {
		t.Error("legacy `2>/dev/null;` pattern should be removed")
	}
}

func TestBuildEnhancedScript_NoLegacyWorkspaceSkillTarget(t *testing.T) {
	e := &DockerSkillExecutor{}
	ctx := SkillExecutionContext{SkillID: "s", Version: "v1"}
	script := e.buildEnhancedScript(ctx, "echo ok")

	// 确保不出现无 s 的孤儿路径
	// 用 strings.Contains 检查 "/workspace/skill " (skill 后跟空格或单引号或换行)
	// 但实际路径 /workspace/skills/... 也包含 /workspace/skill，所以做精确判定
	for _, badPattern := range []string{
		"/workspace/skill ", "/workspace/skill/", "/workspace/skill\n", "/workspace/skill\"",
	} {
		if strings.Contains(script, badPattern) {
			t.Errorf("legacy path %q found in script:\n%s", badPattern, script)
		}
	}
}

// ---------- envKeys 与 buildEnvList 单元测试 ----------

func TestEnvKeys_SortedAndStable(t *testing.T) {
	m := map[string]string{
		"SRE_TOKEN":          "v3",
		"SRE_MCP_SERVER_URL": "v1",
		"SRE_WORKSPACE_ID":   "v2",
	}
	keys := envKeys(m)
	want := []string{"SRE_MCP_SERVER_URL", "SRE_TOKEN", "SRE_WORKSPACE_ID"}
	if len(keys) != len(want) {
		t.Fatalf("got %d keys want %d", len(keys), len(want))
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("at %d: got %q want %q", i, keys[i], want[i])
		}
	}
}

func TestEnvKeys_NilMap(t *testing.T) {
	if got := envKeys(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ---------- ExecuteSkillTool.prepareSkillWorkDir 单元测试（T1 文件落地） ----------

// fakeFileStorage 用于测试，PutObject 等仅返回 nil；GetObject 从内存 map 读
type fakeFileStorage struct {
	objects map[string][]byte // key = bucket + "/" + key
	getErr  error
}

func (f *fakeFileStorage) PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	return "", nil
}
func (f *fakeFileStorage) GetObject(bucket, key string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s/%s", bucket, key)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (f *fakeFileStorage) CopyObject(srcBucket, srcKey, dstBucket, dstKey string) error {
	return nil
}
func (f *fakeFileStorage) RemoveObjects(bucket string, keys []string) error { return nil }
func (f *fakeFileStorage) Exists(bucket, key string) (bool, error)          { return true, nil }
func (f *fakeFileStorage) GetSize(bucket, key string) (int64, error)        { return 0, nil }

// withTempBaseDir 把 config.Cfg.Skill.BaseDir 临时指向 t.TempDir()，并返回恢复函数
func withTempBaseDir(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	prev := config.Cfg
	config.Cfg = &config.GlobalConfig{
		Skill: config.SkillConfig{BaseDir: dir},
	}
	return dir, func() { config.Cfg = prev }
}

func TestPrepareSkillWorkDir_DownloadsFiles(t *testing.T) {
	baseDir, restore := withTempBaseDir(t)
	defer restore()

	storage := &fakeFileStorage{
		objects: map[string][]byte{
			dtos.SkillBucketName + "/key-skill-md": []byte("# SKILL\n"),
			dtos.SkillBucketName + "/key-main-py":  []byte("print('hello')\n"),
		},
	}
	tool := &ExecuteSkillTool{
		storage:   storage,
		messageId: "msg-test",
	}
	r := skillRefForTest("skill-id-1", "v0.1.0")
	ref := &r
	versionDO := skillVersionDOForTest([]fileEntry{
		{path: "SKILL.md", key: "key-skill-md"},
		{path: "main.py", key: "key-main-py"},
	})

	if err := tool.prepareSkillWorkDir(ref, versionDO); err != nil {
		t.Fatalf("prepareSkillWorkDir failed: %v", err)
	}

	workDir := filepath.Join(baseDir, "skills", "msg-test", "skill-id-1-v0.1.0")
	if data, err := os.ReadFile(filepath.Join(workDir, "SKILL.md")); err != nil || string(data) != "# SKILL\n" {
		t.Errorf("SKILL.md not written correctly, err=%v data=%q", err, data)
	}
	if data, err := os.ReadFile(filepath.Join(workDir, "main.py")); err != nil || string(data) != "print('hello')\n" {
		t.Errorf("main.py not written correctly, err=%v data=%q", err, data)
	}
}

func TestPrepareSkillWorkDir_SkipsWhenAlreadyPrepared(t *testing.T) {
	baseDir, restore := withTempBaseDir(t)
	defer restore()

	// 预先在目标目录写一个文件，模拟"已准备好"
	workDir := filepath.Join(baseDir, "skills", "msg-cached", "skill-id-2-v0.2.0")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "preexisting.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	// storage 的 GetObject 一旦被调用就报错——验证不会被调用
	storage := &fakeFileStorage{getErr: fmt.Errorf("storage should not be called when cached")}
	tool := &ExecuteSkillTool{
		storage:   storage,
		messageId: "msg-cached",
	}
	r := skillRefForTest("skill-id-2", "v0.2.0")
	ref := &r
	versionDO := skillVersionDOForTest([]fileEntry{
		{path: "SKILL.md", key: "key-should-not-fetch"},
	})

	if err := tool.prepareSkillWorkDir(ref, versionDO); err != nil {
		t.Fatalf("expected cache hit, got error: %v", err)
	}
}

func TestPrepareSkillWorkDir_StorageError(t *testing.T) {
	_, restore := withTempBaseDir(t)
	defer restore()

	storage := &fakeFileStorage{getErr: fmt.Errorf("object not found")}
	tool := &ExecuteSkillTool{
		storage:   storage,
		messageId: "msg-err",
	}
	r := skillRefForTest("skill-id-3", "v0.3.0")
	ref := &r
	versionDO := skillVersionDOForTest([]fileEntry{
		{path: "SKILL.md", key: "missing-key"},
	})

	if err := tool.prepareSkillWorkDir(ref, versionDO); err == nil {
		t.Fatal("expected error from storage, got nil")
	}
}

func TestPrepareSkillWorkDir_RejectsPathEscape(t *testing.T) {
	_, restore := withTempBaseDir(t)
	defer restore()

	storage := &fakeFileStorage{
		objects: map[string][]byte{
			dtos.SkillBucketName + "/k": []byte("data"),
		},
	}
	tool := &ExecuteSkillTool{
		storage:   storage,
		messageId: "msg-evil",
	}
	r := skillRefForTest("skill-id-4", "v0.4.0")
	ref := &r
	versionDO := skillVersionDOForTest([]fileEntry{
		{path: "../../../escape.txt", key: "k"},
	})

	if err := tool.prepareSkillWorkDir(ref, versionDO); err == nil {
		t.Fatal("expected error for ../ path escape, got nil")
	}
}
