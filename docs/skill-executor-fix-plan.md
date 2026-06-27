# Skill 容器化执行 P0 Bug 修复计划

> 修复 `docs/skill-executor-mount-rules.md` 中识别出的 P0 + P1 顺手修 Bug。本文档作为实施前的最终契约，PR 描述直接引用。

**状态**：决策已确认 ✓，等待实施
**最后更新**：2026-06-26（D2'/D3'/D7/D8 修订）
**关联文档**：[skill-executor-mount-rules.md](./skill-executor-mount-rules.md)

---

## 1. 决策结果摘要

| 决策点 | 选项 | 含义 |
|-------|------|------|
| **D1 文件落地责任** | **A** | 由 `ExecuteSkillTool.Invoke` 负责把 skill 文件从 storage 下载到挂载源目录；`DockerSkillExecutor` 不持有 storage / versionRepo 依赖 |
| **D2 执行模式（修订）** | **A'** | `MessageID` 与 SSE 流的 messageId 绑定（从 `ChatRequest.MessageId` 透传），容器生命周期 = 单条用户消息生命周期；事实上激活容器池模式 |
| **D3 缓存策略（修订）** | **A'** | 跨 messageId 重新下载；同 messageId 内 workDir 已存在则跳过下载（用「目录非空」做天然 guard，零额外状态） |
| **D4 挂载路径约定** | **A** | 容器内统一挂载到 `/workspace/skills/${SkillID}-${Version}/`；移除 `/workspace/skill`（无 s） |
| **D5 路径安全校验** | 默认采纳 | `safeJoin(base, path)` 拒绝含 `..` 的 SkillFile.Path，防止跳出挂载目录 |
| **D6 隐藏文件支持** | **A** | 不支持，skill 包不应包含 `.` 开头的文件；文档显式约定 |
| **D7 Cleanup 触发粒度（新增）** | **A** | 容器和 SSE 流共生死。清理调用上移到 `applications/services/agent.go` 的 `defer sse.CloseChat` 路径，用 `messageId` 作为 cleanup key（替代当前用 `ConversationId` 的实现） |
| **D8 三入口注入对称性（新增）** | **B** | `Chat` / `CreatePlan` / `UpdatePlan` 三个 application 入口都同步注入 `MessageId`，避免后续给 Plan 流加 SkillRefs 时埋雷 |

---

## 2. 目标与验收标准

### 修复目标

1. **文件落地**：skill 的 `SkillFiles`（SKILL.md、Python 脚本、依赖资源）在容器创建前下载到宿主机挂载源目录
2. **路径一致**：一次性容器模式的挂载点与 `buildEnhancedScript` 引用路径匹配
3. **失败显式化**：skill 文件缺失时，错误在容器创建前 / `ln` 阶段硬抛，不被 `2>/dev/null` 静默吞掉

### 验收标准

- ✅ LLM 调用 `executeSkill` 执行 `sre-alert-detail` skill 后，python 脚本能在容器内被正确加载执行（HTTP 请求实际发出）
- ✅ 源码内 `grep "workspace/skill"` 不再出现无 s 的孤儿路径
- ✅ `${baseDir}/skills/` 下不再出现裸 `/` 拼接的空目录名
- ✅ 删除挂载目录中某个文件后再执行，错误在 `execScript` 阶段直接报出文件名，而不是表现为最终 python 报错
- ✅ 至少 1 个集成测试覆盖正常路径 + 文件缺失分支

---

## 3. 范围

### In Scope

| 文件 | 改动性质 | 说明 |
|------|---------|------|
| `internal/domains/services/tools/execute_skill.go` | 新增依赖 + 落地逻辑 | 注入 `storage`，在 Invoke 中实现 skill 文件下载 |
| `internal/domains/services/tools/skill_executor_docker.go` | 统一挂载约定 + 错误显式化 | 删除 `/workspace/skill`，buildEnhancedScript 改硬失败 |
| `internal/domains/services/tools/skill_executor.go` | 接口微调（可选） | 如果需要明确"挂载源已就绪"语义可加注释，但不破坏接口 |
| `internal/domains/services/tools/builtin.go` | 适配 ExecuteSkillTool 构造函数 | 传入新的 storage 参数 |
| `internal/applications/dtos/skill.go`（如需） | 引用 SkillBucketName | 已存在，复用即可 |
| 新增测试 | 集成测试 | `skill_executor_docker_test.go` 或 `execute_skill_test.go` |

### Out of Scope（明确不做）

- 容器池模式激活（保留为独立任务，需先解决 ConversationID 透传问题）
- skill 文件本地缓存
- 多租户运行时 env 注入
- outputs 目录 TTL 清理
- Docker 镜像自动 pull
- 性能调优 / 并发优化

---

## 4. 任务拆解

```
T0 ──► T1 ──► T2 ──► T3 ──► T4 ──► T5
 │      │      │      │      │      │
 │      │      │      │      │      └─ T5: 集成测试
 │      │      │      │      └─ T4: buildEnhancedScript 硬失败模板 (Bug #3)
 │      │      │      └─ T3: CleanupMessage 触发点调整 (D7=A)
 │      │      └─ T2: 统一两种模式挂载约定 (Bug #2)
 │      └─ T1: ExecuteSkillTool 文件落地 + workDir 用 messageId (Bug #1)
 └─ T0: ChatRequest.MessageId 字段 + 三入口注入 + 全链路透传 (D2'/D8)
```

### T0 — 打通 MessageID 全链路透传（D2' / D8）

**前置依赖**：无（最基础的链路打通）

**改动点**：

1. **`internal/domains/models/agents/base.go`**：
   - `ChatRequest` 结构体新增字段：
     ```go
     MessageId string  // SSE 流消息 ID，由 application 层从 sse.StartChat 注入
     ```
   - 检查 3 个 Convert 函数（PlanCreate / PlanUpdate / AgentExecute）：MessageId 是 application 层运行时生成，**不需要**从 client request 透传，Convert 函数保持不动

2. **`internal/applications/services/agent.go`**（**D8 = B：三入口都注入**）：
   - `Chat` 函数：`request := Convert(...)` 后增加 `request.MessageId = messageId`
   - `CreatePlan` 函数：同样在 `request := Convert(...)` 后注入
   - `UpdatePlan` 函数：同样注入
   - 注入位置：紧跟 `messageId := sse.StartChat(writer)` 之后

3. **`internal/domains/services/tools/builtin.go`**：
   - `SkillTools` 函数签名增加 `messageId string` 参数
   - 传递给 `NewExecuteSkillTool(...)`

4. **`internal/domains/services/agents/agent.go`**：
   - `createBaseAgent` 调用 `tools.SkillTools(...)` 时透传 `request.MessageId`
   - **移除现有的 cleanup 调用**（`agent.go:90-94`，用 `ConversationId` 那段）—— 这部分由 T3 上移到 application 层

5. **`internal/domains/services/tools/execute_skill.go`**：
   - `ExecuteSkillTool` 结构体新增字段：
     ```go
     storage   file_storage.FileStorage  // D1=A 引入
     messageId string                    // D2'=A' 透传
     baseDir   string                    // 用于计算 workDir，来自 config.Cfg.Skill.BaseDir
     ```
   - `NewExecuteSkillTool` 构造函数对应增加参数

6. **`api/routers/route.go`**：
   - `SkillTools` 调用方（如果直接在这里调用）适配新签名；如果是间接通过 domain 调用则不动
   - `NewExecuteSkillTool` 装配点适配（若 builtin.go 是装配点，则只改 builtin.go）

**完成定义**：
- 启动后从 HTTP 请求触发 Chat → 在 ExecuteSkillTool.Invoke 入口打日志能看到非空的 messageId
- `grep "request.MessageId" internal/` 至少能搜到 4 处使用（3 个入口注入 + 1 处透传到 SkillTools）

**注意点**：
- 不要在 client DTO（ChatClientRequest）加 MessageId 字段——它是 server 端生成，不是 client 输入
- Convert 函数保持不动，让 MessageId 默认零值（空串）；只由 application 层在生成后写入

---

### T1 — 文件落地链路（Bug #1，依赖 T0）

**位置**：`internal/domains/services/tools/execute_skill.go`

**改动**：

1. `Invoke` 在调用 `executor.Execute` 之前新增「准备 skill 工作目录」步骤：
   - 计算目标目录：
     ```go
     workDir := filepath.Join(t.baseDir, "skills", t.messageId,
         fmt.Sprintf("%s-%s", targetRef.SkillID, targetRef.Version))
     ```
   - **D3' 缓存检查**：如果 workDir 已存在且非空，跳过下载（同 messageId 内复用）：
     ```go
     if entries, err := os.ReadDir(workDir); err == nil && len(entries) > 0 {
         logger.Info("[skill-exec] workDir already prepared, skip download",
             zap.String("work_dir", workDir))
     } else {
         // 执行下载流程
     }
     ```
   - 下载流程：
     - 遍历 `versionDO.SkillFiles`
     - `safeJoin(workDir, file.Path)` 校验（拒绝 `..`）
     - `storage.GetObject(SkillBucketName, file.FileKey)` 读流
     - `os.MkdirAll(filepath.Dir(target), 0755)` + 写文件
   - 失败立刻返回 `ToolCallResult{Success: false}`，**不进入 executor.Execute**

2. 构造 SkillExecutionContext：
   ```go
   execCtx := SkillExecutionContext{
       SkillID:   targetRef.SkillID,
       Version:   targetRef.Version,
       MessageId: t.messageId,  // ← 来自 T0 透传，非空
   }
   ```

**注意点**：
- `safeJoin` 实现要点：用 `filepath.Clean` 规范化后，检查结果是否仍以 `workDir` 为前缀
- workDir 创建本身（`os.MkdirAll(workDir, 0755)`）必须做，否则首次写文件会失败

**完成定义**：手动跑一次 toolcall 后，`ls ${baseDir}/skills/${messageId}/${SkillID}-${Version}/` 能看到 SKILL.md 和 Python 脚本；二次执行同 skill 日志显示 "skip download"

---

### T2 — 统一两种模式挂载约定（Bug #2）

**位置**：`internal/domains/services/tools/skill_executor_docker.go`

**改动**：

1. `createContainer`（一次性容器模式）的 `Mounts` 配置：
   - 原：`Target: "/workspace/skill"`（无 s）
   - 新：`Target: fmt.Sprintf("/workspace/skills/%s-%s", ctx.SkillID, ctx.Version)`
   - Source 保持 `skillWorkDir`（单 skill 子目录）

2. `createPooledContainer`（容器池模式）：
   - 改为单 skill 子目录挂载语义（与一次性模式对齐）
   - 注意 D2'=A' 已激活容器池，**复用场景**是「同 messageId 多次 executeSkill」，因此挂载策略要支持「多个 skill 子目录可同时存在」
   - 实现方式：每次 Execute 重新 inspect 容器现有挂载，如缺失则补挂载 ← 但 Docker 不支持动态加 bind mount
   - **替代方案**：容器池模式下挂载 `${baseDir}/skills/${messageId}/`（整个 message 目录）到 `/workspace/skills/`（与原容器池逻辑一致，只是子目录命名固定为 `${id}-${ver}`）

   **结论**：保持容器池的整目录挂载策略不变，但确保宿主机目录结构与 enhancedScript 期望的子目录命名 (`${id}-${ver}`) 一致

3. `WorkingDir` 保持 `/workspace/outputs`
4. `buildEnhancedScript` 引用路径不变（已经是 `/workspace/skills/${id}-${ver}/`）

**完成定义**：源码内 `grep -n "/workspace/skill" *.go` 输出全部带 s

---

### T3 — Cleanup 触发点调整（D7 = A）

**位置**：
- `internal/applications/services/agent.go`（注入清理调用）
- `internal/domains/services/agents/agent.go`（移除当前的 cleanup 调用）

**改动**：

1. **从 domain 层移除**：删除 `domains/services/agents/agent.go:89-94` 这段：
   ```go
   // 对话结束后清理容器池（仅在使用了 SkillRefs 且 ConversationId 非空时有实际效果）
   if len(request.SkillRefs) > 0 && request.ConversationId != "" {
       if err := s.skillExecutor.CleanupMessage(request.ConversationId); err != nil {
           logger.Warn("cleanup skill executor failed", ...)
       }
   }
   ```

2. **在 application 层添加**：`Chat` / `CreatePlan` / `UpdatePlan` 三个函数的 `defer` 块中，在 `sse.CloseChat(messageId)` 之前增加：
   ```go
   defer func() {
       if err := s.skillExecutor.CleanupMessage(messageId); err != nil {
           logger.Warn("cleanup skill executor failed", zap.Error(err),
               zap.String("messageId", messageId))
       }
       sse.CloseChat(messageId)
   }()
   ```

3. **依赖注入**：`BaseAgentApplicationService` 现在需要持有 `skillExecutor` 引用——构造函数加参数，`route.go` 装配适配

**注意点**：
- 清理顺序：先清容器（CleanupMessage） → 再关 SSE（CloseChat）。容器清理可能慢（10 秒级 stop 超时），不要阻塞 SSE 关闭可以考虑放 goroutine，但首次实现先用同步，简单
- 清理失败要 Warn 但不阻塞流程

**完成定义**：
- 单条消息执行结束后，宿主机 `${baseDir}/skills/${messageId}/` 目录被删除
- `docker ps -a --filter label=mooc_message_id=${messageId}` 返回空

---

### T4 — 错误显式化（Bug #3 顺手修）

**位置**：`internal/domains/services/tools/skill_executor_docker.go` 的 `buildEnhancedScript`

**改动**：

1. 旧脚本模板：
   ```
   cd /workspace/outputs && ln -sf <skillDir>/* /workspace/outputs/ 2>/dev/null; export SKILL_DIR=<skillDir> && <bash>
   ```

2. 新脚本模板：
   ```bash
   set -e
   cd /workspace/outputs
   if ! ls <skillDir>/*.py >/dev/null 2>&1 && ! ls <skillDir>/SKILL.md >/dev/null 2>&1; then
     echo "[skill-exec] FATAL: no skill files found in <skillDir>" >&2
     exit 70
   fi
   ln -sf <skillDir>/* /workspace/outputs/
   export SKILL_DIR=<skillDir>
   <原始 bashCommand>
   ```

3. Go 侧 `execScript` 完成后判断 `exitCode == 70`，包装成"skill 文件缺失"特定错误信息回灌给 LLM

**完成定义**：人为删除 workDir 里所有文件后再执行，错误信息直接定位"no skill files found in /workspace/skills/xxx-vX"

---

### T5 — 测试

**新增文件**：`internal/domains/services/tools/execute_skill_test.go` + `skill_executor_docker_test.go`（视脚手架现状决定）

**最小测试集**：
1. **正常路径**：mock storage 返回完整 skill 文件 → 验证文件下载 → 容器创建 → exec 成功 → 产物收集
2. **storage 失败**：mock storage 返回 ErrNotFound → 验证 ExecuteSkillTool 在 Execute 前返回错误
3. **路径校验**：mock versionRepo 返回含 `..` 的 SkillFile.Path → 验证 safeJoin 拒绝
4. **路径匹配性**：单测 `buildEnhancedScript` 输出，断言含 `/workspace/skills/${id}-${ver}/`
5. **同 messageId 缓存**：同 ExecuteSkillTool 实例两次 Invoke 同 skill，验证第二次没触发 storage.GetObject

**完成定义**：`go test ./internal/domains/services/tools/...` 全部通过

---

## 5. 实施顺序与每步的产出物

| Step | 任务 | 产出 | 验证手段 |
|------|------|------|---------|
| 1 | T0 | ChatRequest.MessageId 字段 + 三入口注入 + SkillTools 透传 + ExecuteSkillTool 持有 | `go build` 通过 + 日志显示非空 messageId |
| 2 | T1 | ExecuteSkillTool 文件落地实现 + workDir 用 messageId | `go build` 通过 + 手动跑一次能看到目录下文件 |
| 3 | T2 | skill_executor_docker.go 挂载统一 | `go build` 通过 + 埋点日志显示新挂载路径 |
| 4 | T3 | Cleanup 调用上移到 application 层 + 用 messageId 作为 key | 手动跑后 `${baseDir}/skills/${messageId}/` 被清理 |
| 5 | T4 | buildEnhancedScript 硬失败模板 | 日志显示新脚本结构 + 删除文件后报 exit 70 |
| 6 | T5 | 测试文件 | `go test ./internal/domains/services/tools/...` 通过 |
| 7 | 联调 | 真实跑通 sre-alert-detail | python 脚本能发出 HTTP 请求，能拿到告警详情 |

---

## 6. 风险与回滚

### 已知风险

1. **构造函数破坏面（扩大）**：`NewExecuteSkillTool` 新增 `storage` + `messageId` + `baseDir` 参数；`SkillTools` 函数签名增加 `messageId`；`BaseAgentApplicationService` 持有 `skillExecutor` —— 全部都是必要改动，无法避免
2. **Cleanup 时序**：`defer` 块中 `CleanupMessage` 同步执行（含 docker stop 10 秒超时），可能延长 HTTP 响应关闭时间。第一版用同步实现，若联调发现明显延迟再改 goroutine
3. **safeJoin 实现细节**：要用 `filepath.Clean` + 前缀检查，不能只看 `..` 字符串（被绕过：`./../foo`）
4. **MessageId 注入对称性（D8=B）**：3 个入口都要注入，遗漏任何一个会导致该入口下 SkillRefs 调用时 messageId 为空 → workDir 路径裸 `/`
5. **Cleanup 调用点上移（D7=A）**：原 domain 层的清理逻辑必须删除，否则两处同时清理会导致重复 docker stop 调用（虽然幂等，但日志噪音）

### 回滚策略

- 单一 PR 合并，commit message 格式建议：
  ```
  fix: 修复 Skill 容器化执行链路 (Bug #1/#2/#3/#4)

  - 扩展 ChatRequest.MessageId 字段，三入口注入并透传到 ExecuteSkillTool
  - ExecuteSkillTool 中补齐 skill 文件落地逻辑（按 messageId 隔离 workDir）
  - 统一两种容器模式挂载路径到 /workspace/skills/${id}-${ver}/
  - CleanupMessage 调用上移到 application 层，与 SSE 流共生死
  - buildEnhancedScript 改为硬失败模式（exit 70）

  Refs: docs/skill-executor-mount-rules.md, docs/skill-executor-fix-plan.md
  ```
- 出问题 `git revert` 即可，本次改动**不涉及**数据库 schema、配置文件结构、对外 API 协议

---

## 7. 下一步

确认本契约后，按 T0 → T1 → T2 → T3 → T4 → T5 顺序实施。每个 T 完成后：
1. 单元层面 `go build` + `go test` 通过
2. 在 PR 描述里勾选完成状态
3. 全部完成后做一次端到端联调（`sre-alert-detail` 真实跑通）

完成的标志：把这个文档的状态从「等待实施」改为「已完成」+ 链接到对应 commit。

---

## 8. 决策放弃说明（透明记录）

为了让链路尽快跑通，本次**主动放弃**以下能力，未来按需补：

| 放弃的能力 | 替代方案 | 未来恢复路径 |
|-----------|---------|------------|
| 跨 messageId 容器复用 | 每条消息独立容器 | 引入 ConversationID 级别的 executor 入口；当前 messageId 级别已满足 |
| skill 文件全局缓存（跨 messageId） | 同 messageId 内通过"目录非空"做最小缓存 | 引入 fileCache 字段的实际使用 + 失效策略 |
| 隐藏文件支持（D6） | 文档约定不允许 | 改用 `cp -rs` 或 find + ln |
| 多租户运行时 env 注入 | 全部容器共享 staticEnv | 扩展 SkillExecutionContext.Env 字段 |
| Docker 镜像自动 pull | 部署时手动 pull | 在 createContainer 前补 ensureImage（之前讨论过） |

---

## 9. MessageID 透传链路（实施对照图）

```
HTTP 请求
  │
  ▼
api/handlers ──► dtos.ChatClientRequest（client 不传 messageId）
  │
  ▼
applications/services/agent.go  Chat / CreatePlan / UpdatePlan
  │
  ├─ messageId := sse.StartChat(writer)              ◄── ① 在 sse 层生成
  ├─ request := Convert(clientReq) → agents.ChatRequest
  ├─ request.MessageId = messageId                   ◄── ② T0 新增注入点
  │
  ├─ defer func() {
  │     skillExecutor.CleanupMessage(messageId)      ◄── ③ T3 上移的清理点
  │     sse.CloseChat(messageId)
  │  }()
  │
  └─ s.agentDomainSvc.Chat(request, eventCh)
       │
       └─ domains/services/agents/agent.go  createBaseAgent
            └─ tools.SkillTools(repo, repo, storage, executor,
                                 skillRefs, request.MessageId)  ◄── ④ T0 透传
                 └─ NewExecuteSkillTool(..., storage, messageId, baseDir)
                      └─ Invoke()
                           ├─ workDir = ${baseDir}/skills/${messageId}/${id}-${ver}/  ◄── ⑤ T1 使用
                           ├─ 同 messageId 复用：检查 workDir 非空则跳过下载
                           ├─ storage.GetObject → 写入 workDir
                           └─ execCtx.MessageId = t.messageId                          ◄── ⑥ 注入 executor
                                └─ DockerSkillExecutor.Execute（容器池模式生效）
```

**关键节点编号**：
- ① messageId 在 SSE 层生成（无改动）
- ② T0：application 层向 ChatRequest 注入
- ③ T3：cleanup 调用上移到 application 层 defer
- ④ T0：透传到 SkillTools 工厂
- ⑤ T1：用作 workDir 路径段
- ⑥ T1：注入 SkillExecutionContext
