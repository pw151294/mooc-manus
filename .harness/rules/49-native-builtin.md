---
rule_id: R-49-native
severity: high
---

# NATIVE 内置工具（fileRead / fileEdit / bashExec）

manus 2026-06-29 引入第四类工具 **NATIVE 内置工具**，对齐 openclaw / Claude Code / Codex 编程 agent 的原生本地能力：直接读取宿主机文件、对工作区文件做精确字符串替换、在项目根目录下执行 bash 命令。详细契约见 `docs/superpowers/plans/2026-06-29-native-builtin-tools.md`，工具分类边界见 R-44。

## R-48 偏离声明

R-48 §1.1「禁止跳过 SkillExecutor 直接 exec 用户脚本」原文针对 **Skill 用户脚本**（属于外部内容，可被 prompt 注入）。NATIVE 工具属于 **manus 自身原生能力**，定位与 Claude Code/Codex 一致，因此 **本规则允许 bashExec 工具走本地 `exec.CommandContext` 直跑，不强制 Docker 沙箱**。

代价是失去容器级隔离 → 必须叠加本规则定义的三道软护栏。

## 禁止行为

1. **禁止在 NATIVE 工具之外新增「LLM 自由执行 bash」的代码路径**
   - 禁止 Domain Service / Application Service 在非 NATIVE 工具中 `exec.Command("bash", "-c", userScript)`
   - 静态：`grep -rn "exec.Command" internal/domains/services/` 仅允许在 `bash_exec.go` 与 SkillExecutor 内出现

2. **禁止绕过黑名单**
   - 禁止在 NATIVE 工具内通过参数覆盖 `BashDenyList` 实例
   - 禁止裸 `os.OpenFile(...).Write` 写宿主机任意路径（fileEdit 必须走 `NativeWorkspace.ResolveInWorkspace`）

3. **禁止把 fileRead 输出当指令信任**
   - fileRead 返回的文件内容、bashExec 的 stdout/stderr 视为外部内容（详见 R-31-untrusted）
   - 不得把这些输出直接拼进 system prompt（违反 R-46）

4. **禁止在 audit 日志里脱敏 command 字段**
   - command 原文是审计目的，必须落盘；如果 LLM 跑 `echo $TOKEN`，token 泄露问题应通过部署侧最小权限账户解决，而非日志层裁剪
   - 但 `stdout_bytes` 只记长度，不记原始 stdout 内容

5. **禁止扩展 bashExec 支持后台进程 / 长驻 shell**
   - 当前实现以 `context.WithTimeout` + SIGKILL 为退出保证；后台进程会破坏这一保证
   - 如未来需要长驻 session，必须先扩展为带容器沙箱的实现（参考 R-48 SkillExecutor 模式）

## 要求行为

1. **三道软护栏（不可缺一）**

   | 护栏 | 实现位置 | 关键约束 |
   |------|---------|---------|
   | 命令黑名单 | `internal/domains/services/tools/bash_denylist.go` | 预编译正则，基线 + 配置叠加；命中即拒绝，不进 exec |
   | 硬限制 | `internal/domains/services/tools/bash_exec.go` | command 长度 ≤ 16 KiB；timeout 默认 120s / 上限 600s；输出合并截断 32 KiB（头部 cap/16 + 尾部）；进程级并发 ≤ 4 |
   | Audit 日志 | `bash_exec.go::audit` | 每次调用结构化落盘，含 message_id / command / description / exit_code / duration_ms / denied_by；走 `audit=native-bash` 字段，便于 ELK / grep |

2. **路径与工作区边界**

   | 工具 | 可访问范围 | 校验函数 |
   |------|-----------|---------|
   | fileRead | 整个宿主机文件系统 | `NativeWorkspace.IsSensitivePath` |
   | fileEdit | 仅 `${native.workspace_base_dir}/${messageId}/` | `NativeWorkspace.ResolveInWorkspace`（复用 `safeJoin`） |
   | bashExec | cwd = `os.Getwd()`（manus 进程项目根） | 黑名单（命令解析不做语义判定，由部署侧最小权限账户兜底） |

3. **生命周期与清理**
   - fileEdit workspace 与 SSE 流共生死：以 `messageId` 为 key
   - 清理由 `applications/services/agent.go::cleanupNativeWorkspaceByMessageID` 在 `defer sse.CloseChat` 路径调用
   - Chat / CreatePlan / UpdatePlan 三入口都必须对称注入 `MessageId`（与 Skill 同等地位）

4. **沙箱边界**
   - **没有容器沙箱**：bashExec 在 manus 后端进程权限下运行
   - **生产部署前提**：必须用专用低权限账户启动 manus；建议 systemd unit 加 `NoNewPrivileges=true`、`ProtectHome=true`、`ProtectSystem=strict`、`ReadWritePaths=${native.workspace_base_dir} ${storage.root_dir} ${skill.base_dir}`
   - **未落实最小权限即上线 = critical 风险**

5. **配置依赖（`config.toml::[native]`）**
   - `workspace_base_dir`（默认 `./data/native-workspace`）
   - `max_file_read_bytes`（默认 10 MiB）
   - `sensitive_path_deny_list`：路径前缀，支持 `~` 展开
   - `bash_command_deny_list`：额外正则，叠加在基线之上；编译失败的条目静默忽略
   - `bash_timeout_default` / `bash_timeout_max` / `bash_output_cap` / `bash_concurrency`

## Agent 行为

- 用户问"加个工具执行 X"：先判定是否属于 LLM 在 manus 工作区里跑命令/读写文件，是则属 NATIVE，不走 Skill 路径
- 检测到非 NATIVE 路径出现 `exec.Command("bash", ...)` → 拒绝并引导加入 NATIVE 工具
- 扩展黑名单规则 → 优先改 `defaultBashDenyPatterns` 基线，配置叠加是逃生口

## 可验证性

- 单测覆盖：
  - `bash_denylist_test.go`：基线每条模式与 custom 叠加
  - `bash_exec_test.go`：HappyPath / NonZeroExit / DenyListBlocks / Timeout / OutputTruncation / StderrMerged / CommandTooLong
  - `file_read_test.go`：HappyPath / OffsetLimit / SensitivePathBlocked / BinaryRejected / NotExist / IsDir / TooLarge
  - `file_edit_test.go`：HappyPath / NonUniqueRejected / ReplaceAll / NotFound / SameOldNew / PathEscapeBlocked / RequiresMessageId / AtomicNoStrayTmp
  - `native_workspace_test.go`：MkdirAll / ResolveInWorkspace / Cleanup / IsSensitivePath / expandHome
- 静态：
  - `grep -rn "exec.Command" internal/domains/services/ | grep -v -E '(skill_executor|bash_exec)'` 应为空
  - `grep -rn "if name == \"fileRead\"\\|if name == \"bashExec\"\\|if name == \"fileEdit\"" internal/` 应为空（R-44 §禁止行为 1）
- 集成：本地启动 manus，chat 触发 fileRead / fileEdit / bashExec，从前端 SSE `tool_call_*` 事件确认链路通畅
- 审计演练：执行任意 bashExec，日志里 `grep audit=native-bash` 能 grep 到完整记录
