---
rule_id: R-47-memory
severity: critical
---

# ChatMemory 生命周期与隔离

`ChatMemory` 保存单一 conversation 的 LLM 对话历史（参考 `internal/domains/models/memory/memory.go` 与 `manager.go`）。所有访问统一走 `memory.FetchMemory(conversationId)` / `memory.DeleteMemory(conversationId)`。conversationId 视为 secret 级别（与总仓 R-32-secrets，详见 mooc-manus-all/.harness/rules/32-secrets-handling.md 关联），任何路径上的越权读取都会造成跨用户数据泄漏。

## 禁止行为

1. **禁止跨 conversation 读取彼此历史**
   - 禁止直接遍历 `manager.conversationId2Memory`
   - 禁止把另一 conversation 的 `ChatMemory.GetMessages()` 拷贝到当前 Agent
   - 禁止以 prefix / 通配方式 fetch（如 `for k := range manager.conversationId2Memory { ... }`）

2. **禁止 conversationId 出现在低信任面**
   - 禁止打日志原文（`logger.Info("...", zap.String("conversationId", id))`）：必须 mask（如保留前 8 位 + `...`）
   - 禁止作为 URL 查询参数（仅走 header / body）
   - 禁止回传外部内容（MCP / A2A 入参不得包含本会话 conversationId）

3. **禁止伪 ID / 空 ID 通过**
   - `conversationId == ""` 时 `FetchMemory` 返回的是共享 sink，必然跨用户串扰
   - 入口（Handler / Application）必须校验 conversationId 非空 + UUID 形式 + 与登录态绑定

4. **禁止永远不清理的 Memory**
   - 不主动 evict 会让 `manager.conversationId2Memory` 无界增长 → OOM 风险

## 要求行为

1. **conversationId 是隔离主键**
   - 所有 Memory 操作走 `FetchMemory` / `DeleteMemory`
   - Agent 工厂内拿到的 `*ChatMemory` 必须由当前 conversationId 决定（参考 `applications/services/agent.go` 注入 `memory.FetchMemory(req.ConversationId)`）

2. **TTL / 清理策略**
   - 策略：超 N 小时无活动自动 evict（推荐 N=24，可按部署量调）
   - 实现建议：
     - 在 `manager.go` 增加 `lastAccessAt map[string]time.Time` + 周期性扫描 goroutine
     - 或对话结束（done 事件）后由 application 层显式 `DeleteMemory(conversationId)`
   - 强制下线（用户登出 / 主动结束会话）必须 `DeleteMemory`

3. **conversationId 与 messageId 边界**
   - conversationId：会话级别，长期；隔离 Memory
   - messageId：单条消息级别，短暂；隔离 Skill 容器（详见 R-48）
   - 两者不可互换；日志 mask 规则相同

4. **跨域协作不传 conversationId 原文**
   - 调 MCP / A2A 工具时不得透传 conversationId；仅传业务参数
   - 工具结果回写 Memory 由 Agent 在受控边界内完成

## Agent 行为

- 用户请求"清空历史" → 走 `DeleteMemory(conversationId)`，并发 `done` 事件让前端重置
- 检测 `conversationId2Memory` 被外部访问 → 拒绝并要求改走 FetchMemory
- 看到 `zap.String("conversationId", id)` / URL `?cid=` → 替换为 mask 形式
- 新增"跨会话推荐"等需聚合多会话历史的特性 → 拒绝（违反隔离），引导改为按用户 ID 在 application 层聚合（且仍不直接读 Memory，走持久化 ChatLog）

## 可验证性

- 静态：
  - `grep -rn "conversationId2Memory" .` 仅允许出现在 `internal/domains/models/memory/`
  - `grep -rn "zap.String(.conversationId.," .` 应为空（统一改为 mask helper）
  - `grep -rn "?conversationId=" .` 应为空
- 单测：
  - 并发 100 个不同 conversationId 调 `FetchMemory`，断言互不串扰
  - `FetchMemory("") + AddMessage` → 应 reject（需在 application 层加校验）
- 集成测试：模拟 24h 无活动后断言对应 Memory 被 evict
