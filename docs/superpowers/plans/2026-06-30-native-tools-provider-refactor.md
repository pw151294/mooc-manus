# NATIVE 工具装配链路重构:引入 NativeToolsProvider

**日期**: 2026-06-30
**类型**: 重构(refactor)
**触发**: 用户指出 `NewBaseAgentDomainService` 与 `tools.NativeTools` 签名过长,把配置注入与依赖注入混在一起,违反 DI 单一原则与 DDD 分层

## 1. 背景与现状问题

2026-06-29 落地的 NATIVE 内置工具(fileRead/fileEdit/bashExec,R-49)目前以**位置参数**方式把全部 NATIVE 配置传递给 domain service:

```go
// 现状:NewBaseAgentDomainService 13 个位置参数
NewBaseAgentDomainService(
    appConfigDomainSvc, providerDomainSvc, functionDomainSvc,
    skillRepo, versionRepo, skillExecutor, fs,
    nativeWorkspace,        // 具体类型 *tools.NativeWorkspace
    bashDenyList,           // 具体类型 *tools.BashDenyList
    bashTimeoutDefaultSec,  // int 配置
    bashTimeoutMaxSec,      // int 配置
    bashOutputCap,          // int 配置
    bashConcurrency,        // int 配置
)

// 现状:tools.NativeTools 7 个位置参数,4 个连续 int
tools.NativeTools(workspace, denyList, bashTimeoutDefault, bashTimeoutMax, bashOutputCap, bashConcurrency, messageId)
```

**违反的原则**:

| 原则 | 违反点 |
|---|---|
| DI 单一职责 | 构造函数应注入"领域协作者",当前混入配置原始值 |
| 与 Skill 体系对称性 | Skill 用 1 个 `tools.SkillExecutor` 接口装配,NATIVE 散成 6 个参数 |
| 位置参数安全性 | 4 个连续 int(timeoutDefault/Max/cap/concurrency)位置互换不会编译报错 |
| DDD 分层 | application 层暴露具体类型 `*tools.NativeWorkspace`,仅为调用 `Cleanup` |

**对照 Skill 体系**(已经是正确范式):

```go
// SkillExecutor 接口(skill_executor.go:20)
type SkillExecutor interface {
    Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error)
    CleanupMessage(messageID string) error
}
// 装配:tools.NewDockerSkillExecutor(baseDir, hostBaseDir, dockerHost, dockerImage, env)
// 注入:NewBaseAgentDomainService(..., skillExecutor, ...)
```

## 2. 决策(用户已 4 项选定)

| # | 决策 | 选定 |
|---|---|---|
| A | 重构粒度 | **引入 `NativeToolsProvider` 接口**(与 SkillExecutor 平级) |
| B | Options 归属包 | **放 `tools` 包**;route.go 显式做 config → Options mapping |
| C | Cleanup 接口 | **合进 `NativeToolsProvider`**(BuildTools + Cleanup 同接口) |
| D | 底层 New*Tool | **保留为 export 低层 API**;Provider 是高层装配语法糖 |

## 3. 目标签名

### 3.1 新增接口与值对象

```go
// internal/domains/services/tools/native_tools_provider.go(新建)

// NativeToolsOptions 装配 NativeToolsProvider 所需的全部配置
// 由 route.go 从 config.NativeConfig 字段拷贝填充;tools 包不依赖 config 包
type NativeToolsOptions struct {
    WorkspaceBaseDir      string
    SensitivePathDenyList []string
    MaxFileReadBytes      int64
    BashCommandDenyList   []string
    BashTimeoutDefaultSec int
    BashTimeoutMaxSec     int
    BashOutputCap         int
    BashConcurrency       int
}

// NativeToolsProvider 原生内置工具提供方
// 单例装配,持有 NativeWorkspace + BashDenyList + bash 配置
// 与 SkillExecutor 平级,作为 BaseAgentDomainService 的单一依赖
type NativeToolsProvider interface {
    // BuildTools 按 messageId 构造一组工具实例(fileRead + fileEdit + bashExec)
    // messageId 用于 fileEdit 隔离 workspace 与 bashExec audit 关联
    BuildTools(messageId string) ([]Tool, error)

    // Cleanup 清理指定 messageId 关联的 workspace 目录;messageId 为空时 no-op
    Cleanup(messageId string) error
}

// NewNativeToolsProvider 单例装配 NativeToolsProvider
// 内部构造 NativeWorkspace 与 BashDenyList 并持有 bash 配置;后续 BuildTools 复用这些状态
func NewNativeToolsProvider(opts NativeToolsOptions) NativeToolsProvider {
    workspace := NewNativeWorkspace(opts.WorkspaceBaseDir, opts.SensitivePathDenyList, opts.MaxFileReadBytes)
    denyList := NewBashDenyList(opts.BashCommandDenyList)
    return &nativeToolsProviderImpl{
        workspace:             workspace,
        denyList:              denyList,
        bashTimeoutDefaultSec: opts.BashTimeoutDefaultSec,
        bashTimeoutMaxSec:     opts.BashTimeoutMaxSec,
        bashOutputCap:         opts.BashOutputCap,
        bashConcurrency:       opts.BashConcurrency,
    }
}

type nativeToolsProviderImpl struct {
    workspace             *NativeWorkspace
    denyList              *BashDenyList
    bashTimeoutDefaultSec int
    bashTimeoutMaxSec     int
    bashOutputCap         int
    bashConcurrency       int
}

func (p *nativeToolsProviderImpl) BuildTools(messageId string) ([]Tool, error) {
    // 内部组装 fileRead + fileEdit + bashExec,逻辑搬自现有 tools.NativeTools
    tools := make([]Tool, 0, 3)
    fileRead := NewFileReadTool(p.workspace)
    if err := fileRead.Init(); err != nil { return nil, err }
    tools = append(tools, fileRead)
    fileEdit := NewFileEditTool(p.workspace, messageId)
    if err := fileEdit.Init(); err != nil { return nil, err }
    tools = append(tools, fileEdit)
    bashExec := NewBashExecTool(p.denyList, p.bashTimeoutDefaultSec, p.bashTimeoutMaxSec, p.bashOutputCap, p.bashConcurrency, messageId)
    if err := bashExec.Init(); err != nil { return nil, err }
    tools = append(tools, bashExec)
    return tools, nil
}

func (p *nativeToolsProviderImpl) Cleanup(messageId string) error {
    return p.workspace.Cleanup(messageId)
}
```

### 3.2 删除旧 API

```go
// internal/domains/services/tools/builtin.go
// 删除:func NativeTools(workspace, denyList, ...7 个位置参数...) ([]Tool, error)
// 保留:func SkillTools(...) — 不动
```

底层 `NewFileReadTool` / `NewFileEditTool` / `NewBashExecTool` / `NewNativeWorkspace` / `NewBashDenyList` 仍 export(单测继续直接使用)。

### 3.3 重构后 NewBaseAgentDomainService

```go
// internal/domains/services/agents/agent.go
NewBaseAgentDomainService(
    appConfigDomainSvc, providerDomainSvc, functionDomainSvc,
    skillRepo, versionRepo, skillExecutor, fs,
    nativeToolsProvider tools.NativeToolsProvider,  // ← 单一接口注入,取代原 6 个参数
)
```

字段:`nativeToolsProvider tools.NativeToolsProvider`(取代原 6 个字段)。

### 3.4 重构后 createBaseAgent

```go
// 仅在 nativeToolsProvider 已装配时启用
if s.nativeToolsProvider != nil {
    nativeTools, err := s.nativeToolsProvider.BuildTools(request.MessageId)
    if err != nil { ... }
    baseTools = append(baseTools, nativeTools...)
}
```

### 3.5 重构后 application 层

```go
// internal/applications/services/agent.go
type BaseAgentApplicationServiceImpl struct {
    agentDomainSvc      agents.BaseAgentDomainService
    skillExecutor       tools.SkillExecutor
    nativeToolsProvider tools.NativeToolsProvider  // ← 接口替代 *NativeWorkspace
}

func NewBaseAgentApplicationService(
    agentDomainSvc agents.BaseAgentDomainService,
    skillExecutor tools.SkillExecutor,
    nativeToolsProvider tools.NativeToolsProvider,
) BaseAgentApplicationService

func (s *BaseAgentApplicationServiceImpl) cleanupNativeToolsByMessageID(messageId string) {
    if s.nativeToolsProvider == nil || messageId == "" { return }
    if err := s.nativeToolsProvider.Cleanup(messageId); err != nil {
        logger.Warn("cleanup native tools failed", ...)
    }
}
```

三处 defer 改调 `cleanupNativeToolsByMessageID(messageId)`(原名 `cleanupNativeWorkspaceByMessageID`,语义升级)。

### 3.6 重构后 route.go

```go
// 2.2.6 NATIVE 内置工具 Provider 装配
nativeWorkspaceDir := config.Cfg.Native.WorkspaceBaseDir
if nativeWorkspaceDir == "" {
    nativeWorkspaceDir = filepath.Join(rootDir, "native-workspace")
}
nativeToolsProvider := tools.NewNativeToolsProvider(tools.NativeToolsOptions{
    WorkspaceBaseDir:      nativeWorkspaceDir,
    SensitivePathDenyList: config.Cfg.Native.SensitivePathDenyList,
    MaxFileReadBytes:      config.Cfg.Native.MaxFileReadBytes,
    BashCommandDenyList:   config.Cfg.Native.BashCommandDenyList,
    BashTimeoutDefaultSec: config.Cfg.Native.BashTimeoutDefault,
    BashTimeoutMaxSec:     config.Cfg.Native.BashTimeoutMax,
    BashOutputCap:         config.Cfg.Native.BashOutputCap,
    BashConcurrency:       config.Cfg.Native.BashConcurrency,
})

// 2.3 Domain Service:从 13 参缩到 8 参
baseAgentDomainSvc := agents.NewBaseAgentDomainService(
    appConfigDomainSvc, providerDomainSvc, functionDomainSvc,
    skillRepo, skillVersionRepo, skillExecutor, fs,
    nativeToolsProvider,
)

// 3.3 Application Service:nativeWorkspace → nativeToolsProvider
baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc, skillExecutor, nativeToolsProvider)
```

## 4. 变更文件清单

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `internal/domains/services/tools/native_tools_provider.go` | **新建** | `NativeToolsProvider` 接口、`NativeToolsOptions` 值对象、`nativeToolsProviderImpl` 实现 |
| `internal/domains/services/tools/native_tools_provider_test.go` | **新建** | 接口契约测试:BuildTools 返回 3 个工具、各工具元数据正确、Cleanup 透传到 workspace |
| `internal/domains/services/tools/builtin.go` | 修改 | 删除 `NativeTools(...)` 函数;保留 `SkillTools` |
| `internal/domains/services/agents/agent.go` | 修改 | struct 字段 6→1;构造函数参数 13→8;`createBaseAgent` 调 `provider.BuildTools` |
| `internal/applications/services/agent.go` | 修改 | `nativeWorkspace *tools.NativeWorkspace` → `nativeToolsProvider tools.NativeToolsProvider`;cleanup 方法重命名 |
| `api/routers/route.go` | 修改 | §2.2.6 段重写为 Provider 装配;Domain/Application 调用点同步 |
| `.harness/rules/49-native-builtin.md` | 修改 | §要求行为 1 表格工厂入口由 `tools.NativeTools(...)` 更新为 `tools.NewNativeToolsProvider(...)`;装配示意码块更新 |
| `.harness/rules/44-tool-registration.md` | 修改 | NATIVE 行的工厂表达式同步 |

## 5. 不变项(明确说明)

- `NewFileReadTool` / `NewFileEditTool` / `NewBashExecTool` / `NewNativeWorkspace` / `NewBashDenyList` / `BashDenyList` / `NativeWorkspace` **签名不变**
- 既有 5 份单测(`file_read_test.go` / `file_edit_test.go` / `bash_exec_test.go` / `native_workspace_test.go` / `bash_denylist_test.go`)**全部不动**
- `config.NativeConfig` 不变
- `config.toml` `[native]` 段不变
- R-49 三道软护栏、生命周期、可访问路径表 **语义零变化**

## 6. 验证策略

1. `go build ./...` 编译通过
2. `go test ./internal/domains/services/tools/ -count=1 -run 'TestNative|TestBash|TestFile|TestLocate|TestExpandHome|TestTruncate|TestSafeJoin'` 全绿(包含新增 provider 测试)
3. 新增 `TestNativeToolsProvider_BuildTools` / `TestNativeToolsProvider_Cleanup` 单测,覆盖率 ≥80%
4. 静态:`grep -rn "tools.NativeTools(" .` 应为空(旧函数已删)
5. 静态:`grep -rn "*tools.NativeWorkspace" internal/applications/` 应为空(application 层不再感知具体类型)
6. `bash .harness/scripts/sync-bridges.sh` 同步桥接层摘要;`bash .harness/scripts/validate-harness.sh` 通过

## 7. 提交计划

单次 commit:`refactor(tools): 引入 NativeToolsProvider 抽象,清理 DI 散参`

不与上一笔 `feat(tools): 新增 NATIVE 内置工具` commit 合并 — 保持 feat / refactor 历史可追溯。

## 8. 风险

| 风险 | 评估 |
|---|---|
| 接口边界增加测试 mock 成本 | 极低 — provider 仅 2 个方法,且当前测试都走具体类型不需要 mock |
| 与 D7=A(Skill 容器复用)生命周期耦合误差 | 无 — Cleanup 行为透传,语义零变化 |
| 增加一层间接造成性能损耗 | 可忽略 — Provider 持有装配好的 workspace/denyList,BuildTools 只是 New 三个 struct |
| 影响已 commit 但未 push 的 8d92a2e | 不影响 — 重构作为新 commit 叠加,推送时按时间顺序两次都会上去 |

## 9. 二次重构(同日):config 直传与 NativeTools 包装

**触发**:用户指出 b4afa4e 的两处仍嫌冗长 ——
1. `route.go` §2.2.6 用 14 行做 `config.NativeConfig` → `NativeToolsOptions` 的逐字段拷贝
2. `createBaseAgent` 装配三路径不对称(Skill 走 `tools.SkillTools(...)` 函数,NATIVE 走 `provider.BuildTools(...)` 方法)

**用户决策**:
- A:`NewNativeToolsProvider` 第二参数 = `storageRootDir string`(只传需要的)
- B:`tools.NativeTools` 函数内部判 `provider == nil`,调用方简化
- C:domain 包 import config 包(可接受,与 Skill 体系 `NewDockerSkillExecutor(config.Cfg.Skill.*)` 对称)
- D:不一并包 `NewDockerSkillExecutor`(executor 是底层具体类型,无需再包)

### 9.1 签名最终演进

| 函数 | 8d92a2e(首版) | b4afa4e(一次重构) | 本次(二次重构) |
|---|---|---|---|
| `NewBaseAgentDomainService` 参数数 | 13 | 8 | 8(不变) |
| `tools.NativeTools` 签名 | `(workspace, denyList, 4×int, messageId)` | **删除** | `(provider, messageId)` 包装 |
| `NewNativeToolsProvider` 入口 | N/A | `(NativeToolsOptions)` | `(config.NativeConfig, storageRootDir)` |
| `route.go` §2.2.6 行数 | ~14 | ~14 | **1** |

### 9.2 删除项

- `tools.NativeToolsOptions` 结构体(被 `config.NativeConfig` 直传取代)
- `route.go` 中 `filepath` import 与 `nativeWorkspaceDir` 中间变量

### 9.3 新增项

- `tools.NativeTools(provider, messageId)` 函数(builtin.go)
- `tools` 包 `import "mooc-manus/config"`(domain 显式依赖 config 包,有 Skill 先例)
- 单测 `TestNativeToolsProvider_WorkspaceBaseDirFallback`(覆盖默认值回退分支)

### 9.4 三条工具装配路径最终对称

```go
// MCP + A2A
baseTools, err := tools.InitTools(providers, proId2Funcs, srvCfgs)

// Skill 内置(loadSkill + executeSkill)
if len(request.SkillRefs) > 0 {
    skillTools, err := tools.SkillTools(s.skillRepo, ..., request.MessageId)
    baseTools = append(baseTools, skillTools...)
}

// NATIVE 内置(fileRead + fileEdit + bashExec)
nativeTools, err := tools.NativeTools(s.nativeToolsProvider, request.MessageId)
if len(nativeTools) > 0 {
    baseTools = append(baseTools, nativeTools...)
}
```

三类工具调用语法完全对称,可读性显著提升。

### 9.5 验证结果

- 编译 ✓
- 单测 5 个(含 fallback)全绿
- 静态:`NativeToolsOptions` 已删除 ✓;`tools.NativeTools(` 在 agent.go 出现 1 处 ✓;`exec.Command*` 限定 ✓
- harness validate ✓
