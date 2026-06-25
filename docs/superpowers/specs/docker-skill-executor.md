# DockerSkillExecutor 技术设计方案

## 一、目标

将 `StubSkillExecutor` 重构为 `DockerSkillExecutor`，实现基于 Docker 容器沙箱的 Skill 脚本隔离执行，完整对齐 Beedance 项目的容器生命周期管理、文件挂载、脚本执行、结果读取、资源清理全链路能力。

---

## 二、类结构改造

**文件：** `internal/domains/services/tools/skill_executor.go`

- 删除 `StubSkillExecutor` 及 `NewStubSkillExecutor`
- 新增 `DockerSkillExecutor` 结构体、`ContainerContext` 辅助类型

```go
type DockerSkillExecutor struct {
    baseDir     string
    hostBaseDir string
    dockerHost  string
    dockerImage string

    dockerClient *client.Client
    clientMu     sync.Mutex

    containerPool sync.Map // key=poolKey, value=*ContainerContext
    fileCache     sync.Map // key=poolKey, value=string(workDir)
}

type ContainerContext struct {
    ContainerID string
    WorkDir     string
}
```

**构造函数：**
```go
func NewDockerSkillExecutor(baseDir, hostBaseDir, dockerHost, dockerImage string) SkillExecutor
```

---

## 三、容器生命周期管理

### 双模式执行策略

```go
func (e *DockerSkillExecutor) Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
    if ctx.MessageID == "" {
        return e.executeWithDisposableContainer(ctx, bashCommand)
    }
    return e.executeWithContainerPool(ctx, bashCommand)
}
```

### 容器池 Key
```
poolKey = messageID:skillID:version
```

### 容器创建规范
- CPU: 2核（NanoCPUs: 2_000_000_000）
- 内存: 2GB
- 网络: bridge
- 启动命令: `tail -f /dev/null`
- 容器标签: `created_by=mooc-manus-skill-executor`

### 挂载配置

**双挂载（容器池模式）：**
- `/workspace/skills/` → `${baseDir}/skills/${messageID}/`（只读）
- `/workspace/outputs/` → `${baseDir}/outputs/${messageID}/`（读写）

**单挂载（一次性模式）：**
- `/workspace/` → 工作目录（读写）

### CleanupMessage
```go
func (e *DockerSkillExecutor) CleanupMessage(messageID string) error
// 清理容器 + skills 目录，保留 outputs 目录
```

---

## 四、目录结构设计

```
${baseDir}/
├── skills/
│   └── ${messageID}/
│       └── ${skillID}-${version}/   ← Skill 脚本文件（只读挂载）
└── outputs/
    └── ${messageID}/                ← 执行输出目录（读写挂载）
```

**路径辅助方法：**

| 方法 | 返回路径 |
|------|---------|
| `getSkillWorkDir(ctx)` | `${baseDir}/skills/${messageID}/${skillID}-${version}` |
| `getSkillsMessageDir(ctx)` | `${baseDir}/skills/${messageID}` |
| `getOutputsDir(ctx)` | `${baseDir}/outputs/${messageID}` |
| `toHostPath(path)` | Docker-in-Docker 路径转换 |

---

## 五、脚本执行链路

### 容器内增强脚本
```bash
cd /workspace/outputs
ln -sf /workspace/skills/${skillID}-${version}/* /workspace/outputs/ 2>/dev/null
export SKILL_DIR=/workspace/skills/${skillID}-${version}
${userBashScript}
```

### 执行流程
1. 快照 outputs 目录已有文件（`scanFiles`）
2. 创建 exec 实例（`ContainerExecCreate`）
3. Attach 执行，捕获 stdout/stderr（`stdcopy.StdCopy`）
4. 超时控制：`context.WithTimeout(30s)`
5. 扫描 outputs 目录新增文件
6. 构造 `SkillExecutionResult` 返回

### 输出文件追踪
- 执行前后分别扫描 outputs 目录
- 计算差集得到新增文件列表
- 返回宿主机绝对路径

---

## 六、异常处理设计

| 异常场景 | 处理策略 | 日志级别 |
|---------|---------|---------|
| Docker 客户端初始化失败 | 返回错误 | ERROR |
| 容器创建失败 | 返回错误 | ERROR |
| 脚本执行超时 | 返回超时错误 | ERROR |
| 脚本执行失败（非零退出） | 捕获 stderr 返回结果 | WARN |
| 容器销毁失败 | 记录日志，继续执行 | WARN |

**错误包装：**
```go
func wrapError(operation string, err error) error {
    return fmt.Errorf("DockerSkillExecutor.%s failed: %w", operation, err)
}
```

---

## 七、配置管理

### config/config.go 新增
```go
type SkillConfig struct {
    BaseDir     string `toml:"baseDir"`
    HostBaseDir string `toml:"hostBaseDir"`
    DockerHost  string `toml:"dockerHost"`
    DockerImage string `toml:"dockerImage"`
}
```

**默认值：**
- BaseDir: `/data/.beedance`
- DockerHost: `unix:///var/run/docker.sock`
- DockerImage: `python:3.11-slim`

### config.toml 新增
```toml
[skill]
baseDir = "/data/.beedance"
hostBaseDir = ""
dockerHost = "unix:///var/run/docker.sock"
dockerImage = "python:3.11-slim"
```

---

## 八、依赖

```
github.com/docker/docker v24.0.9+incompatible
github.com/docker/go-connections v0.5.0
github.com/moby/term v0.5.0
```

---

## 九、InitRouter 集成点

在第二层 Domain Service 段初始化 skillExecutor，注入到 `NewBaseAgentDomainService`：

```go
// 2.2.5 Skill 执行器
skillExecutor := tools.NewDockerSkillExecutor(
    config.Cfg.Skill.BaseDir,
    config.Cfg.Skill.HostBaseDir,
    config.Cfg.Skill.DockerHost,
    config.Cfg.Skill.DockerImage,
)

// 2.3 Agent Domain Service
baseAgentDomainSvc := agents.NewBaseAgentDomainService(
    ...,
    skillExecutor,
    ...,
)
```

---

## 十、分步实施计划

| 步骤 | 内容 | 优先级 |
|------|------|------|
| 1 | 安装 Docker SDK 依赖，添加 config.SkillConfig | 高 |
| 2 | skill_executor.go：新增结构体、辅助方法、Docker 客户端懒加载 | 高 |
| 3 | 实现一次性容器模式（单挂载）| 高 |
| 4 | 实现容器池复用模式（双挂载）| 中 |
| 5 | 实现 CleanupMessage | 中 |
| 6 | 完善日志埋点和异常处理 | 中 |
| 7 | 修改 route.go 和 base_agent.go 完成集成 | 高 |
