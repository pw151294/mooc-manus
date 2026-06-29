# ChatMemory 生命周期与隔离

> 后端 LLM 历史消息存储在进程内的 `ChatMemory`，以 `conversationId` 为隔离主键。强约束见 `R-47-memory`；本文聚焦"代码层面的状态机 + 当前实现的边界与已知缺口"。

源码：`internal/domains/models/memory/{memory.go,manager.go}`。

## 数据结构

```go
type ChatMemory struct {
    messages        []llm.Message
    toolCallId2Name map[string]string  // 反向索引：用于 Compact 时定位"哪个 toolCallId 来自哪个工具"
}
```

- `messages` 是单一会话的全量对话历史（含 system / user / assistant / tool 四种 Role），通过 `llm.Message` 值对象保持厂商无关（R-42 / ADR-0001）。
- `toolCallId2Name` 仅在 assistant 消息有 `ToolCalls` 时通过 `recordToolCalls` 写入。

## 全局 Manager（`manager.go`）

```go
var manager *memoryManager  // package-level singleton

type memoryManager struct {
    sync.Mutex
    conversationId2Memory map[string]*ChatMemory
}

func FetchMemory(conversationId string) *ChatMemory
func DeleteMemory(conversationId string) // memory.go 中实际把 messages 与 toolCallId2Name 都置空再 delete
```

- `init()` 在包加载时构造 `manager`，外部访问全部走 `FetchMemory` / `DeleteMemory` —— R-47 §"conversationId 是隔离主键" 强制这一点。
- `FetchMemory("")` 会返回 **共享 sink**（全空字符串都映射到同一 `*ChatMemory`），这是 R-47 §"禁止伪 ID / 空 ID 通过" 警告的"跨用户串扰"入口。当前代码在 Handler / Application 入口校验 `conversationId` 必须非空（`internal/applications/services/agent.go::Chat` 兜底 `uuid.New()`，但仅作为防御，业务层应在更早层校验 UUID 形式 + 鉴权绑定）。

## ChatMemory 的对外 API（`memory.go`）

| 方法 | 用途 | 调用方 |
|------|------|--------|
| `NewChatMemory` | 仅 manager 内部用 | `FetchMemory` 首次缓存 |
| `AddMessage(msg)` | 单条追加，自动 record toolCalls | `BaseAgent.AddToMemory` 首条 system |
| `AddMessages(msgs)` | 批量追加 | `BaseAgent.AddToMemory` 主要路径 |
| `GetMessages()` | 全量返回（**引用，非拷贝**） | `BaseAgent.GetMessages` |
| `GetLastMessage()` | 取末尾一条 | 当前未广泛使用，预留 |
| `Rollback()` | 移除末尾一条 | 预留 |
| `Compact()` | 把 `browser` / `bing_search` 工具结果替换为 `"removed"` | 长会话压缩，按需调用 |
| `IsEmpty()` | 判空，用于决定是否注入 systemPrompt | `BaseAgent.AddToMemory` 首次 |

### 注意：`Compact` 的历史 bug

注释里明确记录："原实现误判 `role == "tools"`（复数），导致循环体永不执行；新实现使用 `llm.RoleTool`。" —— 这条记录值得保留，避免后人误改回去。它也是 R-47 校验"role 常量统一"的真实事故。

## Agent 端的写入约定

`BaseAgent.AddToMemory`（`base.go`）：

```go
func (a *BaseAgent) AddToMemory(messages []llm.Message) {
    if a.memory.IsEmpty() {
        a.memory.AddMessage(llm.Message{Role: llm.RoleSystem, Content: a.systemPrompt})
    }
    a.memory.AddMessages(messages)
}
```

- 首次写入会预置 systemPrompt（来自 PromptManager，详见 `prompt-management.md`）。
- 之后每轮 LLM 调用前 `InvokeLLM` / `StreamingInvokeLLM` 都会先 `AddToMemory(messages)`，再 `GetMessages()` 拿全量传给 Invoker。
- "assistant 空响应"会写一条 `{Assistant, ""}` + `{User, "AI无响应内容，请继续"}`（见 `StreamingInvokeLLM`），靠下一轮 LLM 自我修复。

## 当前 TTL / 清理状态（缺口）

R-47 §"TTL / 清理策略" 给出方向：

> - 超 N 小时无活动自动 evict（推荐 N=24）
> - `manager.go` 增加 `lastAccessAt map[string]time.Time` + 周期性扫描 goroutine
> - 或对话结束（done 事件）后由 application 层显式 `DeleteMemory(conversationId)`

**当前实现尚未落地 TTL**。仅当 Application 层调 `DeleteMemory` 时才清理；正常请求路径里没有调用点。这是一个已知技术债：

- 风险：`manager.conversationId2Memory` 无界增长 → 长时间运行进程 OOM。
- 暂行措施：依赖容器重启 / 定期重新部署清理；不建议在生产长跑。
- 跟进锚点：见 `R-47-memory` Phase 4 修订段（"现状基线"），及对应 plan 落地任务。

## 与 A2A 子 Agent 的子会话语义

`A2ADomainServiceImpl.createA2AExecutor`（`a2a.go:120`）把 conversationId 派生为：

```
agentConversationId := fmt.Sprintf("%s::%s", srvCfg.ID, conversationId)
chatMemory := memory.FetchMemory(agentConversationId)
```

含义：每个远端 a2a server 对应的本地代理 Agent 都有独立 ChatMemory，与"父 Agent"完全隔离。这避免了子 Agent 的工具结果污染父会话的 LLM 上下文。

## 安全与日志规范

R-47 §"conversationId 出现在低信任面" 明确：

- 禁止 `zap.String("conversationId", id)` 直接打日志 —— 必须 mask（保留前 8 位 + `...`）。
- conversationId 不得作为 URL 查询参数。
- 不得透传给 MCP / A2A 工具入参。

当前部分日志点仍在打全量 conversationId，是 Phase 11 桥接层烘焙后的 follow-up；可通过 `grep -rn "zap.String(.conversationId."` 检索剩余点。

## 静态检查清单（R-47 §"可验证性"）

```
grep -rn "conversationId2Memory" .                  # 仅允许 internal/domains/models/memory/
grep -rn "zap.String(.conversationId.," .           # 应为空（统一 mask）
grep -rn "?conversationId=" .                       # 应为空
```

并发用例：100 个不同 conversationId 并发 `FetchMemory` 后断言互不串扰；空 ID 的 `FetchMemory` + `AddMessage` 路径需在 application 层 reject。

## 与父仓 R-32-secrets 的协同

R-47 把 conversationId 视作 secret 级别字段，与父仓 `mooc-manus-all/.harness/rules/32-secrets-handling.md` 的 secret 处理原则一致：日志 mask、不出现在 URL、不出现在外部协议入参。messageId（R-48 限定 Skill 容器边界）同等处理。
