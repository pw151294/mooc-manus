# Skill 容器挂载规则与重构参考

> 本文档梳理 `DockerSkillExecutor` 当前的文件挂载规则、容器内执行视角，以及已知设计缺陷与修复建议。作为后续重构 `internal/domains/services/tools/skill_executor_docker.go` 的参考依据。

---

## 一、路径计算公式表

所有路径都由 `SkillExecutionContext` 推导得出。

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│  各路径变量的计算来源（从 SkillExecutionContext 推导）                              │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                     │
│  输入：ctx.SkillID    = "c2196700-5dd1-4a21-963f-f633bade7076"                      │
│        ctx.Version    = "v0.1.2"                                                    │
│        ctx.MessageID  = "msg-abc-123"  (空字符串→一次性容器, 非空→容器池)           │
│                                                                                     │
│  baseDir            = 配置 ${skill.base_dir} → filepath.Abs() 后的绝对路径          │
│  hostBaseDir        = 配置 ${skill.host_base_dir} (DinD 场景才用，否则等同 baseDir) │
│                                                                                     │
│  skillWorkDir       = ${baseDir}/skills/${MessageID}/${SkillID}-${Version}          │
│  skillsMessageDir   = ${baseDir}/skills/${MessageID}                                │
│  outputsDir         = ${baseDir}/outputs/${MessageID}                               │
│                                                                                     │
│  toHostPath(p):                                                                     │
│    if hostBaseDir == ""    : return p                                               │
│    else                    : return hostBaseDir + Rel(baseDir, p)                   │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

| 路径变量 | 计算公式 | 使用场景 |
|---------|---------|---------|
| `baseDir` | 配置文件 `skill.base_dir` + `filepath.Abs()` | 应用进程视角的根目录 |
| `hostBaseDir` | 配置文件 `skill.host_base_dir` | DinD 场景下宿主机视角的根目录，空值表示与 baseDir 相同 |
| `skillWorkDir` | `${baseDir}/skills/${MessageID}/${SkillID}-${Version}` | 一次性容器模式挂载源 |
| `skillsMessageDir` | `${baseDir}/skills/${MessageID}` | 容器池模式挂载源 |
| `outputsDir` | `${baseDir}/outputs/${MessageID}` | 产物落地目录（两模式共用） |

---

## 二、一次性容器模式（DISPOSABLE MODE）

**触发条件**：`ctx.MessageID == ""`

**执行入口链**：`Execute(ctx, bash)` → `executeWithDisposableContainer` → `createContainer(skillWorkDir, outputsDir)`

```
┌──────────────────────────── DISPOSABLE MODE (一次性容器) ────────────────────────────┐
│                                                                                       │
│  宿主机文件系统                              容器内文件系统                          │
│  ─────────────────                          ──────────────                          │
│                                                                                       │
│  ${baseDir}/                                /                                         │
│   ├── skills/                                └── workspace/                           │
│   │    └── ""/    ← MessageID 空字符串            ├── skill/    ◄────┐               │
│   │         └── ${SkillID}-${Version}/  ═══bind══════════════════════┤  Mount #1     │
│   │              └── (期望放 skill 文件)                              │  (无 s)       │
│   │                                                                   │               │
│   └── outputs/                                    ├── outputs/  ◄────┐│  WorkingDir   │
│        └── ""/  ═════════════════════════════bind═════════════════════┤  (无 子目录)  │
│                                                                       ││               │
│                                                                       ││               │
│                                                    ╳ /workspace/skills/${SkillID}-${Version}/  │
│                                                       └─── ← buildEnhancedScript 引用此路径   │
│                                                              但容器内根本不存在！(挂载点是 "/workspace/skill" 无 s)│
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

**关键观察**：
- 宿主机挂载源：`${baseDir}/skills/""/${SkillID}-${Version}/`（MessageID 为空导致出现裸 `/`）
- 容器内挂载点：`/workspace/skill`（**无 s**）
- `buildEnhancedScript` 引用：`/workspace/skills/${SkillID}-${Version}/*`（**有 s**）
- **路径不匹配**：一次性容器模式下永远找不到 skill 文件

---

## 三、容器池模式（POOL MODE）

**触发条件**：`ctx.MessageID != ""`

**执行入口链**：`Execute(ctx, bash)` → `executeWithContainerPool` → `createPooledContainer(ctx)`

```
┌──────────────────────────── POOL MODE (容器池复用) ──────────────────────────────────┐
│                                                                                       │
│  宿主机文件系统                              容器内文件系统                          │
│  ─────────────────                          ──────────────                          │
│                                                                                       │
│  ${baseDir}/                                /                                         │
│   ├── skills/                                └── workspace/                           │
│   │    └── ${MessageID}/  ═════════bind═══════════►├── skills/  ◄──── Mount #1       │
│   │         │                                       │   │   (有 s, 整个目录挂载)     │
│   │         ├── ${SkillID-A}-${Version}/            │   │                              │
│   │         │    ├── SKILL.md                       │   ├── ${SkillID-A}-${Version}/  │
│   │         │    └── *_call.py                      │   │    ├── SKILL.md             │
│   │         │                                       │   │    └── *_call.py            │
│   │         └── ${SkillID-B}-${Version}/            │   │                              │
│   │              ├── SKILL.md                       │   └── ${SkillID-B}-${Version}/  │
│   │              └── *.py                           │        ├── SKILL.md             │
│   │                                                 │        └── *.py                 │
│   │     (期望文件结构，当前实际为空)                │                                  │
│   │                                                 │                                  │
│   └── outputs/                                      └── outputs/  ◄──── Mount #2     │
│        └── ${MessageID}/  ═════════bind═════════════════►(产物落地点 + WorkingDir)   │
│             └── (脚本产物 + 软链)                                                     │
│                                                                                       │
│  enhancedScript 引用： /workspace/skills/${SkillID}-${Version}/ ─── ✓ 路径匹配        │
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

**关键观察**：
- 宿主机挂载源：`${baseDir}/skills/${MessageID}/`（**整个 message 目录，不下钻到 skill 子目录**）
- 容器内挂载点：`/workspace/skills`（**有 s**）+ `${SkillID}-${Version}` 子目录结构
- `buildEnhancedScript` 引用：`/workspace/skills/${SkillID}-${Version}/*` ✓ 路径匹配
- 但宿主机 `skills/${MessageID}/` 目录是空的，没有任何模块负责把 skill 文件落地到这里

---

## 四、buildEnhancedScript 在容器内的执行视角

`buildEnhancedScript` 给原始 bash 命令拼装了前置指令，最终在容器内执行的脚本形如：

```bash
cd /workspace/outputs && ln -sf <skillDir>/* /workspace/outputs/ 2>/dev/null; export SKILL_DIR=<skillDir> && <原始 bashCommand>
```

```
┌──────────────────────── enhancedScript 在容器内的执行路径 ───────────────────────────┐
│                                                                                       │
│  容器内文件系统（容器池模式视角）                                                     │
│                                                                                       │
│  /workspace/                                                                          │
│   ├── skills/                                                                         │
│   │   └── ${SkillID}-${Version}/                ① ln -sf .../* /workspace/outputs/   │
│   │        ├── SKILL.md           ─────────────────┐                                  │
│   │        ├── sre_alert_detail_call.py  ─────┐    │                                  │
│   │        └── deps/                          │    │                                  │
│   │                                           │    │                                  │
│   │                  软链接                   │    │                                  │
│   │                  (符号链接)               ▼    ▼                                  │
│   │                                                                                   │
│   └── outputs/  ← cd 切到这里 (WorkingDir)                                            │
│        ├── SKILL.md → /workspace/skills/.../SKILL.md           (link)                 │
│        ├── sre_alert_detail_call.py → /workspace/skills/.../*.py (link)               │
│        ├── deps/ → /workspace/skills/.../deps/                  (link)                │
│        │                                                                              │
│        └── (脚本运行后新增的实体文件，被 scanFiles 识别为产物)                        │
│                                                                                       │
│  enhancedScript 完整链：                                                              │
│   ┌─────────────────────────────────────────────────────────────────────────────┐    │
│   │ cd /workspace/outputs                                                       │    │
│   │   ↓                                                                         │    │
│   │ && ln -sf /workspace/skills/${SkillID}-${Version}/* /workspace/outputs/ \   │    │
│   │      2>/dev/null   ← 错误被吞掉 (这里是失败的隐蔽点)                        │    │
│   │   ;  ← 注意：分号不是 && ，即便 ln 失败也继续                               │    │
│   │ export SKILL_DIR=/workspace/skills/${SkillID}-${Version}                    │    │
│   │   ↓                                                                         │    │
│   │ && <LLM 写的原始 bashCommand，如 python3 *_call.py --event-id xxx>          │    │
│   │     ← Python 在当前目录 (outputs) 找 *.py，找不到就 No such file            │    │
│   └─────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

### 前置指令逐段释义

| 段 | 指令 | 作用 |
|---|------|------|
| 1 | `cd /workspace/outputs` | 切换到产物目录，所有相对路径以此为根 |
| 2 | `ln -sf <skillDir>/* /workspace/outputs/ 2>/dev/null` | 把 skill 文件软链到 outputs，"扁平化" skill 文件结构供 LLM 简洁引用 |
| 3 | `;`（不是 `&&`） | 无条件继续，**ln 失败也不中断** ← 失败被静默吞掉的根源 |
| 4 | `export SKILL_DIR=<skillDir>` | 注入 skill 原始路径环境变量，供脚本通过 `$SKILL_DIR` 显式定位 |
| 5 | `&& <原始 bashCommand>` | 执行 LLM 给出的业务命令 |

---

## 五、两种模式核心差异速查

```
                          一次性模式 (disposable)            容器池模式 (pool)
                          ──────────────────────            ───────────────────
  触发条件                ctx.MessageID == ""               ctx.MessageID != ""
  容器生命周期            执行完即销毁                       与 MessageID 共生死

  宿主机挂载源 (skill)    ${baseDir}/skills/""/             ${baseDir}/skills/
                          ${SkillID}-${Version}             ${MessageID}
                          ↑ 单个 skill 子目录               ↑ 整个 message 目录

  容器内挂载点 (skill)    /workspace/skill   ⚠ 无 s         /workspace/skills  ✓ 有 s

  宿主机挂载源 (outputs)  ${baseDir}/outputs/""/            ${baseDir}/outputs/
                                                            ${MessageID}

  容器内挂载点 (outputs)  /workspace/outputs                /workspace/outputs

  enhancedScript 引用     /workspace/skills/${...}/*  ⚠     /workspace/skills/${...}/*
                          (匹配失败：实际是 /skill 无 s)    (路径匹配)

  Cleanup 触发            执行结束 defer 立即清理            CleanupMessage 显式触发
```

---

## 六、已知设计缺陷清单

按优先级排序，作为重构 backlog。

### P0 — 阻塞 Skill 执行的根因

#### Bug #1：Skill 文件从未落地到挂载目录

**现象**：宿主机 `${baseDir}/skills/.../` 目录在容器创建时仅做 `os.MkdirAll`，目录为空。

**影响**：`ln -sf <skillDir>/*` 找不到源文件，Python 脚本在 `/workspace/outputs/` 下找不到 `*_call.py`，报 `No such file or directory`。

**根因**：当前代码缺失"把 SkillVersion 的 `SkillFiles` 通过 `FileStorage.GetObject` 下载并写入挂载源目录"这一步。

**修复方向**：
- 方案 A（短期）：在 `DockerSkillExecutor.Execute` 入口注入 `versionRepo` + `storage` 依赖，每次执行前同步文件
- 方案 B（推荐）：在 `ExecuteSkillTool.Invoke` 内完成文件准备，executor 只负责挂载已就绪的目录

#### Bug #2：一次性容器模式挂载点与 enhancedScript 路径不一致

**现象**：
- `createContainer` 挂载 `Target: "/workspace/skill"`（无 s）
- `buildEnhancedScript` 引用 `/workspace/skills/${SkillID}-${Version}`（有 s + 子目录）

**影响**：一次性容器模式下 ln 必失败。

**修复方向**：统一两种模式的挂载约定，建议都用 `/workspace/skills/${SkillID}-${Version}/` 子目录结构。

---

### P1 — 隐蔽性问题，影响排查体验

#### Bug #3：ln 错误被双重吞掉

**现象**：`ln -sf ... 2>/dev/null` + `;` 让 ln 失败时既不报错也不中断。

**影响**：失败点（缺少 skill 文件）和错误暴露点（python 找不到文件）相距很远，排查困难。

**修复方向**：
```bash
ln -sf <skillDir>/* /workspace/outputs/ \
  || { echo "skill files not found in $skillDir" >&2; exit 70; }
```
让问题在最近的位置硬失败。

#### Bug #4：MessageID 为空导致路径出现裸 `/`

**现象**：一次性容器模式下 `${baseDir}/skills/""/${SkillID}-${Version}/` 形成 `skills//xxx-v1/`。

**影响**：路径在 Linux 下能工作（多余的 `/` 被压缩），但语义不清晰，且和容器池模式形成两套不一致的目录结构。

**修复方向**：一次性容器模式生成临时 ID（如 `disposable-<uuid>`）填充 MessageID，让两种模式共享同一套路径计算逻辑。

---

### P2 — 设计权衡，按需调整

#### Issue #5：`ln -f` 对实体文件与软链的覆盖语义不一致

如果同一 messageID 跨多次 skill 执行，新 skill 的 ln 可能覆盖 outputs 里上次的产物同名文件。

#### Issue #6：`*` glob 默认不展开隐藏文件

skill 包里的 `.env`、`.config` 之类文件不会被软链到 outputs。需要时改成 `cp -rs <skillDir>/. /workspace/outputs/` 或显式开启 `dotglob`。

#### Issue #7：容器池模式下 env 在 ContainerCreate 时固化

`container.Config.Env` 在创建时绑定，会话中途 token 刷新无法热更新到容器。多用户场景需要走运行时 env 透传（参考 SkillExecutionContext 增加 Env 字段的方案）。

---

## 七、推荐重构路径

按依赖关系排序：

1. **修复 Bug #1**（文件落地）→ 让 skill 能跑通
2. **修复 Bug #2 + Bug #4**（路径统一）→ 让两种模式收敛
3. **修复 Bug #3**（错误显式化）→ 提升可观测性
4. **按需处理 P2 问题**

具体代码改造建议另开 PR 文档说明，本文档仅作现状梳理。

---

## 八、相关文件清单

| 文件 | 关键函数 | 职责 |
|------|---------|------|
| `internal/domains/services/tools/skill_executor.go` | `SkillExecutor` 接口 | 定义执行契约 |
| `internal/domains/services/tools/skill_executor_docker.go` | `Execute` / `createContainer` / `createPooledContainer` / `buildEnhancedScript` | Docker 沙箱实现 |
| `internal/domains/services/tools/execute_skill.go` | `ExecuteSkillTool.Invoke` | 工具入口，构造 SkillExecutionContext |
| `internal/domains/services/tools/load_skill.go` | `LoadSkillTool.Invoke` | Skill 文件读取参考实现（按 FileKey 单文件读流） |
| `config/config.go` | `SkillConfig` | base_dir / host_base_dir / docker_host / docker_image / env |
| `api/routers/route.go` | `InitRouter` | 装配 DockerSkillExecutor 实例 |

---

**最后更新**：2026-06-26
**维护者**：项目团队
