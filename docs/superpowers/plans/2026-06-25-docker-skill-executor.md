# DockerSkillExecutor 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `StubSkillExecutor` 重构为 `DockerSkillExecutor`，提供基于 Docker 容器的 Skill 脚本沙箱执行能力，对齐 Beedance 参考实现。

**Architecture:** 使用 Docker SDK for Go 操作容器，通过 `sync.Map` 实现容器池与文件缓存。一次性容器模式（无 messageID）和容器池复用模式（有 messageID）共用同一 `Execute` 入口。容器内通过双挂载（skills 只读 + outputs 读写）实现脚本与产物分离。

**Tech Stack:** Go 1.25 + github.com/docker/docker SDK + sync.Map + zap logger

---

## 文件结构

| 文件 | 操作 | 责任 |
|------|------|------|
| `config/config.go` | 修改 | 新增 `SkillConfig` 结构体 |
| `config/config.toml` | 修改 | 新增 `[skill]` 配置段 |
| `internal/domains/services/tools/skill_executor.go` | 重写 | 删除 Stub，新增 DockerSkillExecutor |
| `internal/domains/services/tools/skill_executor_docker.go` | 创建 | DockerSkillExecutor 容器/挂载/执行逻辑（拆分文件保持单文件聚焦） |
| `internal/domains/services/tools/builtin.go` | 修改 | 接受外部注入的 SkillExecutor |
| `internal/domains/services/agents/agent.go` | 修改 | BaseAgentDomainService 持有并传递 SkillExecutor |
| `api/routers/route.go` | 修改 | 第二层初始化 DockerSkillExecutor 并注入 |
| `go.mod` / `go.sum` | 修改 | 添加 docker SDK 依赖 |

---

## Task 1: 添加配置结构与默认值

**Files:**
- Modify: `config/config.go`
- Modify: `config/config.toml`

- [ ] **Step 1: 在 config.go 新增 SkillConfig 结构体**

修改 `config/config.go`，在 `StorageConfig` 之后添加：

```go
type SkillConfig struct {
	BaseDir     string `toml:"base_dir"`
	HostBaseDir string `toml:"host_base_dir"`
	DockerHost  string `toml:"docker_host"`
	DockerImage string `toml:"docker_image"`
}
```

修改 `GlobalConfig`，新增字段：
```go
type GlobalConfig struct {
	Redis        RedisConfig    `toml:"redis"`
	Database     DatabaseConfig `toml:"database"`
	LoggerConfig LoggerConfig   `toml:"logger"`
	Storage      StorageConfig  `toml:"storage"`
	Skill        SkillConfig    `toml:"skill"`
}
```

- [ ] **Step 2: 在 config.toml 新增 [skill] 段**

在 `config/config.toml` 末尾追加：
```toml
[skill]
base_dir = "/data/.beedance"
host_base_dir = ""
docker_host = "unix:///var/run/docker.sock"
docker_image = "python:3.11-slim"
```

- [ ] **Step 3: 验证配置加载**

Run: `cd /Users/panwei/Downloads/python/mcp+A2A/mooc-manus && go build ./config/...`
Expected: 编译通过，无错误。

- [ ] **Step 4: Commit**

```bash
git add config/config.go config/config.toml
git commit -m "feat(config): 新增Skill执行器配置项"
```

---

## Task 2: 安装 Docker SDK 依赖

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 添加 Docker SDK 依赖（使用 v25+ 以支持 container.ExecOptions API）**

Run:
```bash
cd /Users/panwei/Downloads/python/mcp+A2A/mooc-manus
go get github.com/docker/docker@v25.0.5+incompatible
go get github.com/docker/go-connections@v0.5.0
go mod tidy
```
Expected: go.mod 中新增 `github.com/docker/docker v25.0.5+incompatible` 和 `github.com/docker/go-connections`。

注：必须使用 v25+，因为 Task 6/7 使用的是 v25 引入的 `container.ExecOptions` 与 `container.ExecStartOptions` 类型；v24 对应类型为 `types.ExecConfig` 与 `types.ExecStartCheck`。

- [ ] **Step 2: 验证依赖可用**

Run:
```bash
go list -m github.com/docker/docker
go list -m github.com/docker/go-connections
```
Expected: 显示版本号，无错误。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: 添加Docker SDK依赖"
```

---

## Task 3: 重写 skill_executor.go 接口骨架

**Files:**
- Modify: `internal/domains/services/tools/skill_executor.go`

保留接口定义和数据结构，删除 `StubSkillExecutor` 实现。

- [ ] **Step 1: 重写 skill_executor.go**

将文件完整替换为：

```go
package tools

// SkillExecutionContext 脚本执行上下文（最小集字段）
type SkillExecutionContext struct {
	SkillID   string // Skill ID
	Version   string // Skill 版本
	MessageID string // 容器复用标识（空字符串触发一次性容器模式）
}

// SkillExecutionResult 单次执行结果
type SkillExecutionResult struct {
	Stdout      string   // 标准输出
	Stderr      string   // 错误输出
	Status      string   // 执行状态（completed / failed）
	OutputFiles []string // 产物文件路径（宿主机绝对路径）
}

// SkillExecutor 脚本执行器接口
// 当前提供 DockerSkillExecutor，基于 Docker 容器沙箱实现资源隔离与脚本执行
type SkillExecutor interface {
	// Execute 执行 Skill 脚本，返回执行结果切片
	// 当 ctx.MessageID 为空时使用一次性容器，非空时使用容器池复用
	Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error)

	// CleanupMessage 清理指定 messageID 关联的容器与 skills 目录（保留 outputs）
	// 应在对话/消息生命周期结束时调用；messageID 为空时为 no-op
	CleanupMessage(messageID string) error
}
```

- [ ] **Step 2: 验证编译（预期会有错误，因为 builtin.go 还引用 NewStubSkillExecutor）**

Run: `go build ./internal/domains/services/tools/...`
Expected: 编译失败，提示 `undefined: NewStubSkillExecutor`（这是预期的，下一步处理）。

- [ ] **Step 3: 暂不 Commit，等 Task 4 完成后一起提交**

---

## Task 4: 实现 DockerSkillExecutor 结构体与辅助方法

**Files:**
- Create: `internal/domains/services/tools/skill_executor_docker.go`

- [ ] **Step 1: 创建 skill_executor_docker.go 文件骨架**

写入以下内容（仅结构体、构造函数、Docker 客户端懒加载、路径辅助方法）：

```go
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// DockerSkillExecutor 基于 Docker 容器的 Skill 执行器
type DockerSkillExecutor struct {
	baseDir     string
	hostBaseDir string
	dockerHost  string
	dockerImage string

	dockerClient *client.Client
	clientMu     sync.Mutex

	containerPool sync.Map // key=poolKey, value=*containerContext
	fileCache     sync.Map // key=poolKey, value=string(workDir)
}

type containerContext struct {
	containerID string
	workDir     string
	createdAt   time.Time
}

const (
	skillExecutionTimeout = 30 * time.Second
	dockerStopTimeout     = 10 // 秒
	containerLabelKey     = "created_by"
	containerLabelValue   = "mooc-manus-skill-executor"
)

// NewDockerSkillExecutor 创建 Docker 执行器实例
func NewDockerSkillExecutor(baseDir, hostBaseDir, dockerHost, dockerImage string) SkillExecutor {
	return &DockerSkillExecutor{
		baseDir:     baseDir,
		hostBaseDir: hostBaseDir,
		dockerHost:  dockerHost,
		dockerImage: dockerImage,
	}
}

// getDockerClient 懒加载 Docker 客户端
func (e *DockerSkillExecutor) getDockerClient() (*client.Client, error) {
	if e.dockerClient != nil {
		return e.dockerClient, nil
	}
	e.clientMu.Lock()
	defer e.clientMu.Unlock()
	if e.dockerClient != nil {
		return e.dockerClient, nil
	}

	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if e.dockerHost != "" {
		opts = append(opts, client.WithHost(e.dockerHost))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, wrapErr("getDockerClient", err)
	}
	e.dockerClient = cli
	return cli, nil
}

// buildPoolKey 构建容器池 Key：messageID:skillID:version
func (e *DockerSkillExecutor) buildPoolKey(ctx SkillExecutionContext) string {
	return fmt.Sprintf("%s:%s:%s", ctx.MessageID, ctx.SkillID, ctx.Version)
}

// getSkillWorkDir 获取 Skill 工作目录：${baseDir}/skills/${messageID}/${skillID}-${version}
func (e *DockerSkillExecutor) getSkillWorkDir(ctx SkillExecutionContext) string {
	skillDirName := fmt.Sprintf("%s-%s", ctx.SkillID, ctx.Version)
	return filepath.Join(e.baseDir, "skills", ctx.MessageID, skillDirName)
}

// getSkillsMessageDir 获取消息级别 skills 目录（容器挂载点）
func (e *DockerSkillExecutor) getSkillsMessageDir(ctx SkillExecutionContext) string {
	return filepath.Join(e.baseDir, "skills", ctx.MessageID)
}

// getOutputsDir 获取输出目录：${baseDir}/outputs/${messageID}
func (e *DockerSkillExecutor) getOutputsDir(ctx SkillExecutionContext) string {
	return filepath.Join(e.baseDir, "outputs", ctx.MessageID)
}

// toHostPath 将容器内路径转换为宿主机路径（Docker-in-Docker 场景）
// hostBaseDir 为空表示非 DinD 部署，直接使用容器内路径
func (e *DockerSkillExecutor) toHostPath(containerPath string) string {
	if e.hostBaseDir == "" {
		return containerPath
	}
	relPath, err := filepath.Rel(e.baseDir, containerPath)
	if err != nil {
		return containerPath
	}
	return filepath.Join(e.hostBaseDir, relPath)
}

// scanFiles 递归扫描目录，返回所有文件相对路径集合
func (e *DockerSkillExecutor) scanFiles(dir string) map[string]bool {
	files := make(map[string]bool)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return files
	}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if rel, e2 := filepath.Rel(dir, path); e2 == nil {
			files[rel] = true
		}
		return nil
	})
	return files
}

// buildEnhancedScript 拼接容器内执行脚本：cd outputs && ln -sf skills/* && export SKILL_DIR && bash
func (e *DockerSkillExecutor) buildEnhancedScript(ctx SkillExecutionContext, bashCommand string) string {
	skillDirName := fmt.Sprintf("%s-%s", ctx.SkillID, ctx.Version)
	skillDir := fmt.Sprintf("/workspace/skills/%s", skillDirName)
	return fmt.Sprintf("cd /workspace/outputs && ln -sf %s/* /workspace/outputs/ 2>/dev/null; export SKILL_DIR=%s && %s",
		skillDir, skillDir, bashCommand)
}

// wrapErr 错误包装
func wrapErr(operation string, err error) error {
	return fmt.Errorf("DockerSkillExecutor.%s failed: %w", operation, err)
}

// Execute 执行入口：根据 MessageID 选择执行模式
func (e *DockerSkillExecutor) Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	if ctx.MessageID == "" {
		return e.executeWithDisposableContainer(ctx, bashCommand)
	}
	return e.executeWithContainerPool(ctx, bashCommand)
}

// executeWithDisposableContainer 一次性容器模式（占位，Task 6 实现）
func (e *DockerSkillExecutor) executeWithDisposableContainer(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	return nil, fmt.Errorf("disposable container mode not yet implemented")
}

// executeWithContainerPool 容器池复用模式（占位，Task 7 实现）
func (e *DockerSkillExecutor) executeWithContainerPool(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	return nil, fmt.Errorf("container pool mode not yet implemented")
}

// CleanupMessage 清理指定 messageID 的所有容器和 skills 文件（占位，Task 7 实现）
func (e *DockerSkillExecutor) CleanupMessage(messageID string) error {
	return nil
}

// 编译期接口契约校验
var _ SkillExecutor = (*DockerSkillExecutor)(nil)
```

- [ ] **Step 2: 修改 builtin.go，接受外部注入 SkillExecutor**

完整替换 `internal/domains/services/tools/builtin.go`：

```go
package tools

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/repositories"
)

// SkillTools 返回 Skill 专属内置工具实例切片（loadSkill + executeSkill）
// 仅当 skillRefs 非空时应调用此方法
func SkillTools(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	executor SkillExecutor,
	skillRefs []agents.SkillRef,
) ([]Tool, error) {
	tools := make([]Tool, 0, 2)

	// loadSkill
	loadSkill := NewLoadSkillTool(skillRepo, versionRepo, storage, skillRefs)
	if err := loadSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, loadSkill)

	// executeSkill
	executeSkill := NewExecuteSkillTool(skillRepo, versionRepo, executor, skillRefs)
	if err := executeSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, executeSkill)

	return tools, nil
}
```

- [ ] **Step 3: 验证编译（预期 agent.go 调用方仍报错，下一步处理）**

Run: `go build ./internal/domains/services/tools/...`
Expected: tools 包内编译通过。

Run: `go build ./...`
Expected: agent.go 调用 SkillTools 处报错——参数数量不匹配。

- [ ] **Step 4: 暂不 Commit，等 Task 5 完成集成后再提交**

---

## Task 5: 修改 BaseAgentDomainService 持有并传递 SkillExecutor

**Files:**
- Modify: `internal/domains/services/agents/agent.go`
- Modify: `api/routers/route.go`

- [ ] **Step 1: 修改 agent.go 的结构体与构造函数**

在 `internal/domains/services/agents/agent.go` 中：

修改 `BaseAgentDomainServiceImpl` 结构体（约 27 行），添加 `skillExecutor` 字段：
```go
type BaseAgentDomainServiceImpl struct {
	appConfigDomainSvc services.AppConfigDomainService
	providerDomainSvc  services.ToolProviderDomainService
	functionDomainSvc  services.ToolFunctionDomainService
	skillRepo          repositories.SkillRepository
	versionRepo        repositories.SkillVersionRepository
	skillExecutor      tools.SkillExecutor
	storage            file_storage.FileStorage
}
```

修改 `NewBaseAgentDomainService` 函数（约 36-52 行），添加 `skillExecutor` 参数：
```go
func NewBaseAgentDomainService(
	appConfigDomainSvc services.AppConfigDomainService,
	providerDomainSvc services.ToolProviderDomainService,
	functionDomainSvc services.ToolFunctionDomainService,
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	skillExecutor tools.SkillExecutor,
	storage file_storage.FileStorage,
) BaseAgentDomainService {
	return &BaseAgentDomainServiceImpl{
		appConfigDomainSvc: appConfigDomainSvc,
		providerDomainSvc:  providerDomainSvc,
		functionDomainSvc:  functionDomainSvc,
		skillRepo:          skillRepo,
		versionRepo:        versionRepo,
		skillExecutor:      skillExecutor,
		storage:            storage,
	}
}
```

修改 `SkillTools` 调用点（约 192 行），传入 `s.skillExecutor`：
```go
skillTools, err := tools.SkillTools(s.skillRepo, s.versionRepo, s.storage, s.skillExecutor, request.SkillRefs)
```

- [ ] **Step 2: 修改 route.go 注入 DockerSkillExecutor**

在 `api/routers/route.go` 第 56 行（Skill 模块 Domain Service 之后、Agent Domain Service 之前），插入执行器初始化代码。

需要先在 import 段添加 tools 包导入：
```go
import (
	"mooc-manus/api/handlers"
	"mooc-manus/config"
	app_svc "mooc-manus/internal/applications/services"
	domain_svc "mooc-manus/internal/domains/services"
	"mooc-manus/internal/domains/services/agents"
	"mooc-manus/internal/domains/services/flows"
	"mooc-manus/internal/domains/services/tools"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/external/health_checker"
	"mooc-manus/internal/infra/repositories"

	"github.com/gin-gonic/gin"
)
```

在第 55 行（`skillImportTaskDomainSvc := ...` 之后）和第 57 行（`// 2.3 Agent 模块 Domain Service` 之前）插入：

```go
	// 2.2.5 Skill 执行器（Docker 容器沙箱）
	skillExecutor := tools.NewDockerSkillExecutor(
		config.Cfg.Skill.BaseDir,
		config.Cfg.Skill.HostBaseDir,
		config.Cfg.Skill.DockerHost,
		config.Cfg.Skill.DockerImage,
	)
```

修改 `agents.NewBaseAgentDomainService` 调用（约第 58-65 行），添加 `skillExecutor` 参数：
```go
	baseAgentDomainSvc := agents.NewBaseAgentDomainService(
		appConfigDomainSvc,
		providerDomainSvc,
		functionDomainSvc,
		skillRepo,
		skillVersionRepo,
		skillExecutor,
		fs,
	)
```

- [ ] **Step 3: 完整编译验证**

Run: `cd /Users/panwei/Downloads/python/mcp+A2A/mooc-manus && go build ./...`
Expected: 全项目编译通过，无错误。

- [ ] **Step 4: Commit Task 3-5 的所有改动**

```bash
git add internal/domains/services/tools/skill_executor.go \
        internal/domains/services/tools/skill_executor_docker.go \
        internal/domains/services/tools/builtin.go \
        internal/domains/services/agents/agent.go \
        api/routers/route.go
git commit -m "refactor(skill): 重构StubSkillExecutor为DockerSkillExecutor骨架"
```

---

## Task 6: 实现一次性容器模式

**Files:**
- Modify: `internal/domains/services/tools/skill_executor_docker.go`

- [ ] **Step 1: 添加容器创建辅助方法**

在 `skill_executor_docker.go` 末尾追加（在 `var _ SkillExecutor = (*DockerSkillExecutor)(nil)` 接口契约校验语句之前）：

```go
// createContainer 创建一次性容器（单挂载 /workspace 读写）
func (e *DockerSkillExecutor) createContainer(workDir string) (string, error) {
	cli, err := e.getDockerClient()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", wrapErr("createContainer.MkdirAll", err)
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "bridge",
		Resources: container.Resources{
			NanoCPUs: 2_000_000_000,
			Memory:   2 * 1024 * 1024 * 1024,
		},
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   e.toHostPath(workDir),
				Target:   "/workspace",
				ReadOnly: false,
			},
		},
	}

	containerCfg := &container.Config{
		Image:      e.dockerImage,
		Cmd:        []string{"tail", "-f", "/dev/null"},
		WorkingDir: "/workspace",
		Labels:     map[string]string{containerLabelKey: containerLabelValue},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := cli.ContainerCreate(ctx, containerCfg, hostConfig, nil, nil, "")
	if err != nil {
		return "", wrapErr("ContainerCreate", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", wrapErr("ContainerStart", err)
	}

	logger.Info("Docker container created (disposable)",
		zap.String("container_id", resp.ID),
		zap.String("work_dir", workDir),
		zap.String("image", e.dockerImage))
	return resp.ID, nil
}
```

在文件顶部 import 段替换为：
```go
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)
```

注：`strings` 包用于 Task 7 Step 3 的 `CleanupMessage`，提前 import。

- [ ] **Step 2: 添加脚本执行辅助方法**

继续在 `skill_executor_docker.go` 中添加：

```go
// execScript 在容器内执行脚本，返回 stdout/stderr
func (e *DockerSkillExecutor) execScript(containerID, script string) (string, string, error) {
	cli, err := e.getDockerClient()
	if err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), skillExecutionTimeout)
	defer cancel()

	execResp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          []string{"/bin/sh", "-c", script},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", "", wrapErr("ContainerExecCreate", err)
	}

	attach, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", wrapErr("ContainerExecAttach", err)
	}
	defer attach.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attach.Reader)
		done <- copyErr
	}()

	select {
	case copyErr := <-done:
		if copyErr != nil {
			return stdoutBuf.String(), stderrBuf.String(), wrapErr("StdCopy", copyErr)
		}
	case <-ctx.Done():
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("script execution timeout after %v", skillExecutionTimeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), nil
}

// stopAndRemoveContainer 停止并删除容器（容错）
func (e *DockerSkillExecutor) stopAndRemoveContainer(containerID string) {
	cli, err := e.getDockerClient()
	if err != nil {
		logger.Warn("stopAndRemoveContainer get client failed", zap.Error(err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	timeout := dockerStopTimeout
	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		logger.Warn("ContainerStop failed", zap.String("container_id", containerID), zap.Error(err))
	}
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		logger.Warn("ContainerRemove failed", zap.String("container_id", containerID), zap.Error(err))
		return
	}
	logger.Info("Container removed", zap.String("container_id", containerID))
}
```

- [ ] **Step 3: 实现 executeWithDisposableContainer**

替换原占位实现：

```go
// executeWithDisposableContainer 一次性容器模式
func (e *DockerSkillExecutor) executeWithDisposableContainer(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	tempID := fmt.Sprintf("disposable-%d", time.Now().UnixNano())
	skillDirName := fmt.Sprintf("%s-%s", ctx.SkillID, ctx.Version)
	workDir := filepath.Join(e.baseDir, "skills", tempID, skillDirName)

	containerID, err := e.createContainer(workDir)
	if err != nil {
		return nil, err
	}
	defer e.stopAndRemoveContainer(containerID)

	// 一次性模式直接在 /workspace 执行用户脚本，不做软链接增强
	stdout, stderr, execErr := e.execScript(containerID, bashCommand)

	status := "completed"
	if execErr != nil {
		status = "failed"
		logger.Error("disposable container execute failed",
			zap.String("container_id", containerID), zap.Error(execErr))
		return nil, execErr
	}

	logger.Info("disposable container executed",
		zap.String("container_id", containerID),
		zap.Int("stdout_len", len(stdout)), zap.Int("stderr_len", len(stderr)))

	return []SkillExecutionResult{{
		Stdout: stdout,
		Stderr: stderr,
		Status: status,
	}}, nil
}
```

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 5: Commit**

```bash
git add internal/domains/services/tools/skill_executor_docker.go
git commit -m "feat(skill): 实现一次性容器模式"
```

---

## Task 7: 实现容器池复用模式与 CleanupMessage

**Files:**
- Modify: `internal/domains/services/tools/skill_executor_docker.go`

- [ ] **Step 1: 添加双挂载容器创建方法**

在 `skill_executor_docker.go` 中追加：

```go
// createPooledContainer 创建容器池复用容器（双挂载：skills 只读 + outputs 读写）
func (e *DockerSkillExecutor) createPooledContainer(execCtx SkillExecutionContext) (string, error) {
	cli, err := e.getDockerClient()
	if err != nil {
		return "", err
	}

	skillsDir := e.getSkillsMessageDir(execCtx)
	outputsDir := e.getOutputsDir(execCtx)
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return "", wrapErr("createPooledContainer.MkdirAll(skills)", err)
	}
	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		return "", wrapErr("createPooledContainer.MkdirAll(outputs)", err)
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "bridge",
		Resources: container.Resources{
			NanoCPUs: 2_000_000_000,
			Memory:   2 * 1024 * 1024 * 1024,
		},
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   e.toHostPath(skillsDir),
				Target:   "/workspace/skills",
				ReadOnly: true,
			},
			{
				Type:     mount.TypeBind,
				Source:   e.toHostPath(outputsDir),
				Target:   "/workspace/outputs",
				ReadOnly: false,
			},
		},
	}

	containerCfg := &container.Config{
		Image:      e.dockerImage,
		Cmd:        []string{"tail", "-f", "/dev/null"},
		WorkingDir: "/workspace",
		Labels: map[string]string{
			containerLabelKey: containerLabelValue,
			"message_id":      execCtx.MessageID,
			"skill_id":        execCtx.SkillID,
		},
	}

	dockerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := cli.ContainerCreate(dockerCtx, containerCfg, hostConfig, nil, nil, "")
	if err != nil {
		return "", wrapErr("ContainerCreate(pooled)", err)
	}
	if err := cli.ContainerStart(dockerCtx, resp.ID, container.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(dockerCtx, resp.ID, container.RemoveOptions{Force: true})
		return "", wrapErr("ContainerStart(pooled)", err)
	}

	logger.Info("Docker container created (pooled)",
		zap.String("container_id", resp.ID),
		zap.String("message_id", execCtx.MessageID),
		zap.String("skill_id", execCtx.SkillID))
	return resp.ID, nil
}

// downloadSkillFiles 准备 Skill 文件目录（占位实现，未来可接入 OSS 下载）
func (e *DockerSkillExecutor) downloadSkillFiles(ctx SkillExecutionContext) (string, error) {
	workDir := e.getSkillWorkDir(ctx)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", wrapErr("downloadSkillFiles.MkdirAll", err)
	}
	return workDir, nil
}
```

- [ ] **Step 2: 实现 executeWithContainerPool**

替换原占位实现：

```go
// executeWithContainerPool 容器池复用模式
func (e *DockerSkillExecutor) executeWithContainerPool(execCtx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	poolKey := e.buildPoolKey(execCtx)

	// 获取或下载 Skill 文件
	if _, ok := e.fileCache.Load(poolKey); !ok {
		workDir, err := e.downloadSkillFiles(execCtx)
		if err != nil {
			return nil, err
		}
		e.fileCache.Store(poolKey, workDir)
	}

	// 获取或创建容器（LoadOrStore 保证并发安全）
	var ctxObj *containerContext
	if existing, ok := e.containerPool.Load(poolKey); ok {
		ctxObj = existing.(*containerContext)
	} else {
		containerID, err := e.createPooledContainer(execCtx)
		if err != nil {
			return nil, err
		}
		newCtx := &containerContext{
			containerID: containerID,
			workDir:     e.getSkillWorkDir(execCtx),
			createdAt:   time.Now(),
		}
		actual, loaded := e.containerPool.LoadOrStore(poolKey, newCtx)
		if loaded {
			// 并发竞态：另一个 goroutine 已经创建，删除我们刚创建的
			e.stopAndRemoveContainer(containerID)
			ctxObj = actual.(*containerContext)
		} else {
			ctxObj = newCtx
		}
	}

	// 执行前快照 outputs 目录
	outputsDir := e.getOutputsDir(execCtx)
	beforeFiles := e.scanFiles(outputsDir)

	// 构造增强脚本并执行
	enhancedScript := e.buildEnhancedScript(execCtx, bashCommand)
	stdout, stderr, execErr := e.execScript(ctxObj.containerID, enhancedScript)

	status := "completed"
	if execErr != nil {
		status = "failed"
		logger.Error("pooled container execute failed",
			zap.String("container_id", ctxObj.containerID),
			zap.String("pool_key", poolKey),
			zap.Error(execErr))
		return []SkillExecutionResult{{
			Stdout: stdout,
			Stderr: stderr,
			Status: status,
		}}, execErr
	}

	// 扫描新增文件
	afterFiles := e.scanFiles(outputsDir)
	var outputFiles []string
	for f := range afterFiles {
		if !beforeFiles[f] {
			outputFiles = append(outputFiles, filepath.Join(outputsDir, f))
		}
	}

	logger.Info("pooled container executed",
		zap.String("container_id", ctxObj.containerID),
		zap.String("pool_key", poolKey),
		zap.Int("stdout_len", len(stdout)),
		zap.Int("stderr_len", len(stderr)),
		zap.Int("output_files", len(outputFiles)))

	return []SkillExecutionResult{{
		Stdout:      stdout,
		Stderr:      stderr,
		Status:      status,
		OutputFiles: outputFiles,
	}}, nil
}
```

- [ ] **Step 3: 实现 CleanupMessage**

替换原占位实现：

```go
// CleanupMessage 清理指定 messageID 的所有容器和 skills 目录，保留 outputs 目录
func (e *DockerSkillExecutor) CleanupMessage(messageID string) error {
	if messageID == "" {
		return nil
	}
	prefix := messageID + ":"
	cleanedContainers := 0
	cleanedDirs := 0

	// 清理容器
	e.containerPool.Range(func(key, value any) bool {
		k := key.(string)
		if !strings.HasPrefix(k, prefix) {
			return true
		}
		ctxObj := value.(*containerContext)
		e.stopAndRemoveContainer(ctxObj.containerID)
		e.containerPool.Delete(k)
		cleanedContainers++
		return true
	})

	// 清理 skills 目录（保留 outputs）
	e.fileCache.Range(func(key, value any) bool {
		k := key.(string)
		if !strings.HasPrefix(k, prefix) {
			return true
		}
		workDir := value.(string)
		messageSkillsDir := filepath.Dir(workDir)
		if _, err := os.Stat(messageSkillsDir); err == nil {
			if rmErr := os.RemoveAll(messageSkillsDir); rmErr != nil {
				logger.Warn("RemoveAll skills dir failed",
					zap.String("dir", messageSkillsDir), zap.Error(rmErr))
			} else {
				cleanedDirs++
			}
		}
		e.fileCache.Delete(k)
		return true
	})

	logger.Info("CleanupMessage done",
		zap.String("message_id", messageID),
		zap.Int("containers", cleanedContainers),
		zap.Int("skills_dirs", cleanedDirs))
	return nil
}
```

并确保 import 段已包含 `"strings"`（在 Task 6 已提前 import）。

- [ ] **Step 4: 编译验证**

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 5: Commit**

```bash
git add internal/domains/services/tools/skill_executor_docker.go
git commit -m "feat(skill): 实现容器池复用模式与CleanupMessage"
```

---

## Task 7.5: 在 Agent 对话结束时调用 CleanupMessage

**Files:**
- Modify: `internal/domains/services/agents/agent.go`

CleanupMessage 必须在每次对话结束后调用，否则容器与 skills 目录会持续堆积。当前 `Chat` 入口的请求结构体里如果带有 `MessageID` 字段（或类似会话标识），需在 goroutine 收尾后调用 `s.skillExecutor.CleanupMessage(messageID)`。

- [ ] **Step 1: 探查 ChatRequest 是否携带 MessageID 字段**

Run:
```bash
grep -n "MessageID\|ConversationId\|MessageId" /Users/panwei/Downloads/python/mcp+A2A/mooc-manus/internal/domains/models/agents/*.go
```

如果存在 MessageID 字段，则使用该字段作为 cleanup 标识。

- [ ] **Step 2: 在 Chat / CreatePlan / UpdatePlan 流程结束处调用 CleanupMessage**

修改 `internal/domains/services/agents/agent.go` 中的 `Chat` 方法，在 `wg.Wait()` 之后、`close(eventCh)` 之前插入：

```go
go func() {
    for event := range agentEventCh {
        logger.Debug("get event from agent", zap.String("type", event.EventType()), zap.Any("data", event))
        eventCh <- event
    }
    wg.Wait()
    logger.Info("agent invoke end")

    // 清理 Skill 执行容器与 skills 目录（保留 outputs）
    if request.MessageID != "" {
        if err := s.skillExecutor.CleanupMessage(request.MessageID); err != nil {
            logger.Warn("CleanupMessage failed",
                zap.String("message_id", request.MessageID), zap.Error(err))
        }
    }

    close(eventCh)
}()
```

注：若 `ChatRequest` 没有 `MessageID` 字段，则跳过本步骤，留待后续 ChatRequest 扩展时一并接入。验证清单中明确标注「待 ChatRequest 扩展 MessageID 后接入」。

- [ ] **Step 3: 编译验证**

Run: `go build ./...`
Expected: 编译通过。

- [ ] **Step 4: Commit**

```bash
git add internal/domains/services/agents/agent.go
git commit -m "feat(agent): 对话结束时清理Skill容器资源"
```

---

## Task 8: 应用启动验证

**Files:**
- 无文件修改，仅运行验证

- [ ] **Step 1: 全量编译**

Run: `cd /Users/panwei/Downloads/python/mcp+A2A/mooc-manus && go build ./...`
Expected: 全项目编译通过。

- [ ] **Step 2: go vet 检查**

Run: `go vet ./...`
Expected: 无 vet 警告。

- [ ] **Step 3: 验证应用可启动（不要求 Docker daemon 在线）**

Docker 客户端是懒加载的，应用启动时不会真正连接 daemon。

Run:
```bash
go build -o /tmp/mooc-manus .
ls -la /tmp/mooc-manus
```
Expected: 二进制构建成功，文件存在。

- [ ] **Step 4: Commit（如果前面有遗漏的 go.sum 改动）**

```bash
git status
# 如果有未提交的改动
git add -A && git commit -m "chore: 完成DockerSkillExecutor集成"
```

---

## 验证清单

完成所有 Task 后，对照以下清单确认：

- [ ] `StubSkillExecutor` 已删除，`DockerSkillExecutor` 替换之
- [ ] `SkillExecutor` 接口包含 `Execute` 与 `CleanupMessage` 两个方法
- [ ] `config/config.toml` 包含 `[skill]` 段，所有字段有默认值
- [ ] `route.go` 在第二层初始化 `skillExecutor` 并注入到 `BaseAgentDomainService`
- [ ] `BaseAgentDomainService.SkillTools` 调用传入 `s.skillExecutor`
- [ ] `BaseAgentDomainService.Chat` 在对话结束时调用 `CleanupMessage`（或在验证清单中明确标注「待 ChatRequest 扩展 MessageID 后接入」）
- [ ] 一次性模式：单挂载 `/workspace`，执行后立即清理容器
- [ ] 容器池模式：双挂载（skills 只读 + outputs 读写），按 `messageID:skillID:version` 复用
- [ ] `CleanupMessage` 清理容器 + skills 目录，保留 outputs
- [ ] 输出文件追踪：执行前后扫描 outputs 目录的差集
- [ ] 超时控制：`skillExecutionTimeout = 30s`
- [ ] 错误统一通过 `wrapErr` 包装
- [ ] 关键路径有 `logger.Info` / `logger.Warn` / `logger.Error` 日志

---

## 兼容性保障

- `SkillExecutor` 接口签名保持不变 → `ExecuteSkillTool.Invoke` 无需修改
- `SkillExecutionContext` / `SkillExecutionResult` 结构保持不变
- 配置项提供合理默认值，未配置时不影响应用启动
- Docker daemon 不可用时返回清晰错误信息，不导致应用崩溃
