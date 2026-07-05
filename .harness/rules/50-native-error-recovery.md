---
rule_id: R-50-error-recovery
severity: high
---

# NATIVE 工具错误恢复(三级分层策略)

manus 2026-07-05 引入 4 类原生工具的**错误恢复 Skill 与平台拦截 Hook**,解决过去"工具失败全丢给 LLM 自行推理"导致的无脑重试、任务卡死问题。规则正文见 `internal/domains/services/tools/error_recovery/embed/SKILL.md`,分类逻辑见同目录 `classifier.go`。

## 前置依赖

- R-49 NATIVE 内置工具的三道软护栏(黑名单/硬限/audit)是本规则的错误信号源
- R-46 提示词管理:本规则通过 System Prompt 默认注入内置 Skill 元信息,不违反 R-46 的模板管理边界
- R-31 外部内容信任边界:工具 result.Message 仍视为外部内容,不得原样拼进 system prompt(本规则只把 Message 用于关键字匹配,不做二次注入)

## 禁止行为

1. **禁止在 4 类原生工具之外扩展 error_recovery Hook 生效范围**
   - `error_recovery.NativeToolName` 白名单严格限定 `fileRead / fileWrite / fileEdit / bashExec`
   - Skill / MCP / A2A / CUSTOM 工具的失败不得触发本 Hook,避免误伤

2. **禁止用平台层"代模型重试"绕过 L1 自愈范式**
   - L1 修复必须由 LLM 在下一轮 tool_call 里自主完成
   - `InvokeToolCalls` 内不得引入 retry 状态机代 LLM 调用配套工具

3. **禁止在同一 messageId 内对同一 template_id 循环触发超过 2 次**
   - 命中 L1 且 template_key 重复 → LLM 侧应强制升级到 L2 或 L3
   - 命中 L3 → 平台立即 abort,不再进入下一轮 InvokeLLM

4. **禁止修改 result.Success 语义**
   - Hook 只能追加 `[ErrorRecovery-Ln]` 前缀到 ToolMessage.Content,不得把 Success=false 翻转为 true
   - L3 场景通过 `abort` 返回值 + `eventCh <- events.OnError` 中断,不改变 result 本身

5. **禁止把内置错误恢复 Skill 写进 DB / OSS**
   - 内置 Skill 只能走 `//go:embed`,与用户 Skill 的 DB/OSS 通路完全隔离
   - `loadSkill` 命中 `native-tool-error-recovery` 时短路直接返回 embed,不查 skillRepo

## 要求行为

1. **三级分层策略必须完整落地**

   | 层级 | 触发特征 | 处置 | 中断链路 |
   |-----|---------|------|:-------:|
   | L1 可自愈 | 权限缺失 / 目录未创建 / 依赖未装 / 环境变量缺失 / 文件不存在 | Hook 追加模板到 ToolMessage,LLM 下一轮自主调用配套工具补齐后重试 | 否 |
   | L2 需追问 | 路径歧义 / 参数模糊 / 端口占用 | Hook 追加模板,LLM 停止工具重试,向用户提问 | 否 |
   | L3 致命 | 黑名单命中 / 敏感路径 / 超时 SIGKILL / 磁盘满 / OOM | Hook 追加模板 + `abort=true`,主循环 `OnError` + `close(eventCh)` + `return` | **是** |

2. **Hook 落地位置固定**
   - 文件:`internal/domains/services/agents/base.go`
   - 函数:`(*BaseAgent).InvokeToolCalls`,在 `result := a.InvokeTool(...)` 之后、组装 ToolMessage 之前
   - `Invoke` 与 `StreamingInvoke` 主循环都必须消费 `InvokeToolCalls` 的 `abort` 返回值

3. **SKILL.md 与 classifier.go 双向同步**
   - "全量故障库"表格每一行 → `classifier.go orderedPatterns` 一条 `pattern`
   - 关键字变更必须同步更新两侧,单侧修改视为破坏契约
   - 新增故障类型:先加 template + pattern,再补 SKILL.md 表格,最后补 classifier_test.go 用例

4. **门控开关**
   - 环境变量 `NATIVE_ERROR_RECOVERY_ENABLED=false/0/no/off` 关闭默认注入,用于灰度回滚
   - 默认启用,不需要显式设 true

5. **配套工具边界**
   - L1 自愈使用的配套工具**只能是 4 类原生工具本身**(bashExec / fileRead / fileEdit / fileWrite)
   - 不得引入 MCP / A2A / 用户 Skill 作为"绕道"补齐前置条件

## Agent 行为

- 用户问"某个原生工具报错怎么处理" → 先看 `SKILL.md` 的"全量故障库"表格,匹配后按分级模板执行
- 新增原生工具类型(fileMove / fileCopy 等) → 必须同步把新工具加入 `NativeToolName` 白名单 + 增补故障库
- 观察到 LLM 明显偏离分级模板(如 L3 场景仍在重试) → 检查 Hook 是否真的注入,`grep "error_recovery hook applied"` 日志确认
- 修改 4 类工具的 error 文案 → **必须同步**改 `classifier.go` 的 keywords,否则分类器击穿到通用兜底 L1

## 可验证性

- 单测:`go test ./internal/domains/services/tools/error_recovery/...`
- 集成:`go test ./internal/domains/services/agents/... -run TestInvokeToolCalls_`
- 手工 e2e:见 `docs/superpowers/plans/2026-07-05-native-error-recovery.md`(如存在)或 SKILL.md 中的三类场景示例
