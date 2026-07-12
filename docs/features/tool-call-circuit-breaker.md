# 智能体工具调用熔断机制

## 功能概述

当智能体在工具调用持续失败时，会陷入死循环重复调用相同工具+参数。本机制在会话级别维护失败计数器，当同一工具+参数组合连续失败 ≥3 次时，自动注入干预提示阻断循环。

## 工作原理

1. **会话级计数器**：每个 `BaseAgent` 实例内部维护 `ToolCallCounter`（生命周期绑定 conversationId，与 `ChatMemory` 同步创建/销毁）。
2. **定制化哈希 Key**：根据工具类型采用不同哈希策略（避免 LLM 通过参数微调绕过熔断）
   - `fileRead` / `fileWrite`：只哈希 `path` 参数
   - `fileEdit`：哈希 `path + old_string 前 100 字符 + new_string 前 100 字符`
   - `bashExec`：完整 `command` 哈希
   - 其他工具：完整参数 JSON 哈希
   - 类型断言失败或字段缺失时回落到完整参数哈希，避免空 Key 碰撞
3. **失败记录**：`InvokeToolCalls` 每次工具失败后调用 `RecordFailure`，记录工具名+参数预览元信息。工具不存在（`GetTool == nil`）也算失败并计入。
4. **清零策略**：本轮工具调用结束后，若上一轮的 Key 在本轮未再出现，清零其历史计数（避免误判 —— 中间调用了其它工具的场景）。
5. **干预注入**：`InvokeLLM` / `StreamingInvokeLLM` 请求 LLM 前检查阈值，达标时向 `messages` 追加一条 `RoleUser` 干预提示，引导 LLM 停止重试。

## 五处埋点

| 埋点 | 位置 | 作用 |
|------|------|------|
| 1 | `NewBaseAgent` | 初始化 `ToolCallCounter` |
| 2 | `InvokeToolCalls` 内部 | 生成 Key + 失败时 `RecordFailure` |
| 3 | `InvokeToolCalls` 末尾 | `StartNewRound` 触发清零 |
| 4 | `InvokeLLM` / `StreamingInvokeLLM` 开头 | 检查阈值并注入干预提示 |
| 5 | `NewPlanAgent` / `NewReActAgent` | 共享 `BaseAgent` 的 counter（一个 conversation 一份） |

## 配置参数

- **熔断阈值**：3 次（硬编码于 `base.go` 的 `GetTriggeredRecords(3)`，未来可配置化）
- **单 Key 计数上限**：1000（防御性上限，避免异常场景无限累积）
- **干预提示最多展示**：10 条工具记录（按失败次数降序）
- **哈希策略**：见 `internal/domains/models/circuitbreaker/key_generator.go`

## 日志关键字

- `"工具调用失败，更新计数器"` — 每次 `RecordFailure` 触发
- `"工具调用失败（工具不存在），更新计数器"` — `GetTool == nil` 分支
- `"检测到工具调用死循环，注入干预提示"` — `InvokeLLM` 达标注入
- `"检测到工具调用死循环，注入干预提示（流式）"` — `StreamingInvokeLLM` 达标注入

日志只落 `tools: [...]` 名字数组，不落 `ParamsPreview`（避免 bash 命令原文泄露到日志）。

## 相关文件

- `internal/domains/models/circuitbreaker/` — 核心包（counter / key_generator / prompt_builder + 单元测试，覆盖率 98.6%）
- `internal/domains/services/agents/base.go` — 5 处埋点集成
- `internal/domains/services/agents/plan.go` — 共享 counter（`NewPlanAgent`）
- `internal/domains/services/agents/react.go` — 共享 counter（`NewReActAgent`）
- 设计规范：`../superpowers/specs/2026-07-11-tool-call-circuit-breaker-design.md`（若存在）
- 实现计划：`../superpowers/plans/2026-07-11-tool-call-circuit-breaker.md`

## 并发约束

`ToolCallCounter` **非线程安全**，依赖上层 `BaseAgent.Invoke` / `StreamingInvoke` 的 `wg.Wait` 保证同一时刻仅一个 goroutine 访问。若未来将 Agent 子任务并发化，需要额外加锁。
