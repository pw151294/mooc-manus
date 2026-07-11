# 智能体工具调用死循环干预机制 - 设计规范

**文档版本**：v1.0  
**创建日期**：2026-07-11  
**设计状态**：已评审通过  
**目标项目**：mooc-manus  
**影响范围**：`/api/agent/chat` 核心流程

---

## 文档摘要

本文档定义了**智能体工具调用熔断机制**的完整设计方案，用于解决智能体在工具调用持续失败时陷入死循环的问题。核心策略是在会话级别维护失败计数器，当同一工具+参数组合连续失败 ≥3 次时，在下一轮 LLM 请求前注入干预提示，引导智能体停止无效重试或切换策略。

**关键设计决策**：
- 采用方案一（Domain 层拦截 + 会话级 Map 存储）
- 计数器与 `ChatMemory` 生命周期绑定
- 清零策略：中间调用其他工具则历史失败计数清零（方案 B）
- 哈希策略：根据工具类型定制（方案 C）
- 干预粒度：列举所有达标工具（方案 B）

---

## 一、需求边界与不做事项

### 1.1 机制生效范围

**生效场景**：
- 仅针对 `BaseAgent`、`ReActAgent`、`PlanAgent` 三种 Agent 的工具调用流程
- 仅统计通过 `BaseAgent.InvokeToolCalls` 执行且返回 `result.Success == false` 的调用
- 仅在单个 `conversationId` 生命周期内计数（会话销毁则计数器自动清空）

**排除场景**：
- ❌ **A2A Agent**：A2A 是远程调用，工具执行在远端 Agent，本地无法感知失败细节
- ❌ **LLM 调用失败**：`InvokeLLM` 本身的网络错误或 API 异常不计入（只管工具调用）
- ❌ **参数修复失败**：`jsonrepair.JSONRepair` 失败直接返回错误消息，不进入 `InvokeTool` 流程，不计数
- ❌ **跨会话累计**：计数器不持久化，conversationId 变更后历史计数不继承

### 1.2 不做事项

- **不做自动降级**：触发熔断后不会自动切换到其他工具或修改参数，只通过 Prompt 引导 LLM 自主决策
- **不做工具黑名单**：不会永久封禁某个工具，只是在当前会话内针对特定参数组合熔断
- **不做统计持久化**：计数器数据不写入数据库，重启服务后历史失败记录清零
- **不做跨 Agent 共享**：`BaseAgent` 和 `PlanAgent` 各自独立计数（虽然共用一个 Memory，但 Counter 绑定 Agent 实例）

---

## 二、核心数据结构定义

### 2.1 计数器核心结构

**位置**：`internal/domains/models/circuit_breaker/counter.go`

```go
package circuit_breaker

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
)

// ToolCallCounter 会话级工具调用失败计数器
// 生命周期：绑定单个 conversationId，与 ChatMemory 同步创建/销毁
type ToolCallCounter struct {
    // Key: 工具名+参数指纹哈希，Value: 连续失败次数
    failureCounts map[string]int
    
    // 记录上一轮执行的所有工具 Key，用于清零判定
    lastRoundKeys map[string]bool
    
    // 记录 Key 到工具元信息的映射（用于生成干预提示）
    keyMetadata map[string]ToolCallMetadata
}

// ToolCallMetadata 工具调用元信息
type ToolCallMetadata struct {
    ToolName      string
    ParamsPreview string // 参数预览（截断后的可读形式）
}

// FailureRecord 失败记录，用于生成干预提示
type FailureRecord struct {
    ToolName      string
    ParamsPreview string
    FailCount     int
}

func NewToolCallCounter() *ToolCallCounter {
    return &ToolCallCounter{
        failureCounts: make(map[string]int),
        lastRoundKeys: make(map[string]bool),
        keyMetadata:   make(map[string]ToolCallMetadata),
    }
}
```

### 2.2 哈希 Key 生成策略（方案 C：定制化）

```go
// GenerateKey 生成工具调用唯一标识 Key
// 根据工具类型采用不同的哈希策略，避免参数微调绕过熔断
func GenerateKey(toolName string, argsJSON string) (string, error) {
    var args map[string]interface{}
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("解析参数失败: %w", err)
    }
    
    var hashInput string
    switch toolName {
    case "fileRead", "fileWrite":
        // 只哈希 path 参数
        if path, ok := args["path"].(string); ok {
            hashInput = fmt.Sprintf("%s:path=%s", toolName, path)
        }
    case "fileEdit":
        // 哈希 path + old_string 前100字符 + new_string 前100字符
        path, _ := args["path"].(string)
        oldStr, _ := args["old_string"].(string)
        newStr, _ := args["new_string"].(string)
        hashInput = fmt.Sprintf("%s:path=%s:old=%s:new=%s",
            toolName, path, truncate(oldStr, 100), truncate(newStr, 100))
    case "bashExec":
        // 完整 command 哈希
        if cmd, ok := args["command"].(string); ok {
            hashInput = fmt.Sprintf("%s:command=%s", toolName, cmd)
        }
    default:
        // 其他工具：完整参数哈希
        hashInput = fmt.Sprintf("%s:%s", toolName, argsJSON)
    }
    
    hash := sha256.Sum256([]byte(hashInput))
    return hex.EncodeToString(hash[:]), nil
}

func truncate(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen]
}
```

### 2.3 计数器核心方法

```go
// RecordFailure 记录单次失败，返回当前累计次数
// 同时记录工具元信息，用于后续生成干预提示
func (c *ToolCallCounter) RecordFailure(key string, metadata ToolCallMetadata) int {
    c.failureCounts[key]++
    c.keyMetadata[key] = metadata
    
    // 可选：防止单 Key 失败次数异常累积
    if c.failureCounts[key] > 1000 {
        c.failureCounts[key] = 1000
    }
    
    return c.failureCounts[key]
}

// StartNewRound 开始新一轮工具调用（清零上一轮未重复的失败记录）
// 实现方案 B：中间调用其他工具则清零历史失败计数
func (c *ToolCallCounter) StartNewRound(currentRoundKeys []string) {
    currentKeys := make(map[string]bool)
    for _, k := range currentRoundKeys {
        currentKeys[k] = true
    }
    
    // 清零策略：如果上一轮的 Key 在本轮未出现，说明中间调用了其他工具，清零该 Key
    for k := range c.failureCounts {
        if !currentKeys[k] {
            delete(c.failureCounts, k)
            delete(c.keyMetadata, k)
        }
    }
    
    c.lastRoundKeys = currentKeys
}

// GetTriggeredRecords 返回所有达到阈值的失败记录（阈值 >= 3）
func (c *ToolCallCounter) GetTriggeredRecords(threshold int) []FailureRecord {
    records := make([]FailureRecord, 0)
    for key, count := range c.failureCounts {
        if count >= threshold {
            meta := c.keyMetadata[key]
            records = append(records, FailureRecord{
                ToolName:      meta.ToolName,
                ParamsPreview: meta.ParamsPreview,
                FailCount:     count,
            })
        }
    }
    return records
}
```

---

## 三、主流程接入点位

基于 `/api/agent/chat` 调用链路，需要在以下五个位置埋点：

### 埋点 1：会话初始化 - 创建计数器

**位置**：`internal/domains/services/agents/base.go` - `NewBaseAgent` 函数

**改造点**：
```go
type BaseAgent struct {
    name             string
    systemPrompt     string
    retryInterval    int
    agentConfig      models.AgentConfig
    invoker          invoker.Invoker
    memory           *memory.ChatMemory
    tools            []tools.Tool
    circuitBreaker   *circuit_breaker.ToolCallCounter  // 【新增】熔断计数器
}

func NewBaseAgent(...) *BaseAgent {
    return &BaseAgent{
        // ... 原有字段
        circuitBreaker: circuit_breaker.NewToolCallCounter(),
    }
}
```

**理由**：`BaseAgent` 实例与单次对话会话绑定，Agent 销毁时计数器自动回收。

---

### 埋点 2：工具调用结果回执 - 记录失败

**位置**：`internal/domains/services/agents/base.go` - `InvokeToolCalls` 方法

**改造点**：在每个 `toolCall` 执行完成后，判断 `result.Success` 并更新计数器
```go
func (a *BaseAgent) InvokeToolCalls(toolCalls []llm.ToolCall, eventCh chan<- events.AgentEvent) []llm.Message {
    toolMessages := make([]llm.Message, 0, len(toolCalls))
    currentRoundKeys := make([]string, 0, len(toolCalls))  // 收集本轮所有工具 Key
    
    for _, toolCall := range toolCalls {
        toolCallID := toolCall.ID
        funcName := toolCall.Name
        funcArgs := toolCall.Arguments
        
        // ... 原有参数修复、工具查找逻辑 ...
        
        // 【埋点 2-1】生成工具调用 Key
        key, err := circuit_breaker.GenerateKey(funcName, funcArgs)
        if err != nil {
            logger.Warn("生成工具调用 Key 失败，跳过计数",
                zap.String("tool", funcName),
                zap.Error(err))
            key = "" // 设置为空 Key，后续跳过计数逻辑
        } else {
            currentRoundKeys = append(currentRoundKeys, key)
        }
        
        // 执行工具调用
        eventCh <- events.OnToolCallStart(toolCall, tool.ProviderName())
        result := a.InvokeTool(tool, funcName, funcArgs)
        eventCh <- events.OnToolCallComplete(toolCall, tool.ProviderName(), &result)
        
        // 【埋点 2-2】记录失败 + 更新计数器
        if !result.Success && key != "" {
            metadata := circuit_breaker.ToolCallMetadata{
                ToolName:      funcName,
                ParamsPreview: generateParamsPreview(funcName, funcArgs),
            }
            failCount := a.circuitBreaker.RecordFailure(key, metadata)
            logger.Info("工具调用失败，更新计数器",
                zap.String("tool", funcName),
                zap.String("key", key),
                zap.Int("failCount", failCount))
            eventCh <- events.OnToolCallFail(toolCall, tool.ProviderName(), &result)
        }
        
        // ... 原有 toolMessages 构建逻辑 ...
    }
    
    // 【埋点 3】本轮结束，触发清零检查
    a.circuitBreaker.StartNewRound(currentRoundKeys)
    
    return toolMessages
}

// generateParamsPreview 生成参数预览（用于干预提示展示）
func generateParamsPreview(toolName string, argsJSON string) string {
    var args map[string]interface{}
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return argsJSON[:min(len(argsJSON), 50)]
    }
    
    switch toolName {
    case "fileRead", "fileWrite", "fileEdit":
        if path, ok := args["path"].(string); ok {
            return fmt.Sprintf("path=%s", path)
        }
    case "bashExec":
        if cmd, ok := args["command"].(string); ok {
            return fmt.Sprintf("command=%s", truncate(cmd, 80))
        }
    }
    
    // 默认：返回 JSON 前 100 字符
    return truncate(argsJSON, 100)
}
```

---

### 埋点 3：计数更新 - 清零策略

**位置**：`internal/domains/services/agents/base.go` - `InvokeToolCalls` 方法末尾

**时机**：每轮工具调用批次结束后，调用 `StartNewRound` 触发清零检查

**逻辑**：
- 收集本轮所有 `toolCall` 的 Key（即使成功的也收集，用于判断"是否调用了其他工具"）
- 如果上一轮失败的某个 Key 在本轮未出现，说明智能体转而尝试其他工具，清零该 Key 的计数

**代码**：已在埋点 2 中展示（`StartNewRound` 调用）

---

### 埋点 4：阈值判断 + Prompt 注入

**位置**：`internal/domains/services/agents/base.go` - `InvokeLLM` 和 `StreamingInvokeLLM` 方法开头

**时机**：向 LLM 发起请求**之前**，检查计数器并注入干预提示

**改造点**：
```go
func (a *BaseAgent) InvokeLLM(messages []llm.Message) (llm.Message, error) {
    // 【埋点 4】检查是否有工具达到熔断阈值
    triggeredRecords := a.circuitBreaker.GetTriggeredRecords(3)
    if len(triggeredRecords) > 0 {
        // 注入干预提示（追加到 messages 最后）
        interventionMsg := buildInterventionPrompt(triggeredRecords)
        messages = append(messages, llm.Message{
            Role:    llm.RoleUser,
            Content: interventionMsg,
        })
        logger.Warn("检测到工具调用死循环，注入干预提示",
            zap.Int("triggeredCount", len(triggeredRecords)),
            zap.Any("records", triggeredRecords))
    }
    
    a.AddToMemory(messages)
    // ... 原有 LLM 调用逻辑 ...
}

// StreamingInvokeLLM 同样需要在开头增加相同的熔断检查逻辑
func (a *BaseAgent) StreamingInvokeLLM(messages []llm.Message, eventCh chan<- events.AgentEvent) llm.Message {
    // 【埋点 4】检查熔断（同 InvokeLLM）
    triggeredRecords := a.circuitBreaker.GetTriggeredRecords(3)
    if len(triggeredRecords) > 0 {
        interventionMsg := buildInterventionPrompt(triggeredRecords)
        messages = append(messages, llm.Message{
            Role:    llm.RoleUser,
            Content: interventionMsg,
        })
        logger.Warn("检测到工具调用死循环，注入干预提示",
            zap.Int("triggeredCount", len(triggeredRecords)))
    }
    
    a.AddToMemory(messages)
    // ... 原有逻辑 ...
}
```

---

### 埋点 5：会话结束销毁计数器

**位置**：无需额外处理

**理由**：
- `BaseAgent` 实例在 `BaseAgentDomainService.Chat` 方法内创建，方法结束后自动回收
- Go 的 GC 会自动回收 `circuitBreaker` 字段指向的内存
- `memory.Manager` 在 `DeleteMemory(conversationId)` 时只清理 Memory，不涉及 Agent 实例

**验证点**：确保 `BaseAgentDomainService.Chat` 中每次调用都创建新的 `BaseAgent` 实例（当前代码已满足）

---

## 四、干预 Prompt 模板设计

**位置**：`internal/domains/models/circuit_breaker/prompt_builder.go`

```go
package circuit_breaker

import (
    "fmt"
    "sort"
    "strings"
)

// buildInterventionPrompt 生成干预提示（方案 B：列举所有达标工具）
func buildInterventionPrompt(records []FailureRecord) string {
    // 按失败次数降序排列（优先展示最严重的）
    sort.Slice(records, func(i, j int) bool {
        return records[i].FailCount > records[j].FailCount
    })
    
    var builder strings.Builder
    builder.WriteString("⚠️ **系统干预提示**：以下工具调用已连续失败达到上限，禁止继续重试相同参数：\n\n")
    
    // 最多展示前 10 条
    maxDisplay := 10
    if len(records) > maxDisplay {
        builder.WriteString(fmt.Sprintf("【注意：共 %d 个工具失败，仅展示前 %d 个】\n\n", 
            len(records), maxDisplay))
        records = records[:maxDisplay]
    }
    
    for i, r := range records {
        builder.WriteString(fmt.Sprintf("%d. **%s** - 已失败 %d 次\n", 
            i+1, r.ToolName, r.FailCount))
        preview := r.ParamsPreview
        if len(preview) > 100 {
            preview = preview[:100] + "..."
        }
        builder.WriteString(fmt.Sprintf("   参数预览：`%s`\n\n", preview))
    }
    
    builder.WriteString("**请选择以下操作之一**：\n\n")
    builder.WriteString("1. **向用户反馈失败原因**，说明当前子任务无法完成，结束本轮对话\n")
    builder.WriteString("2. **重新规划任务**，基于已有信息更换工具或修改参数后继续\n")
    builder.WriteString("3. **明确告知用户需要人工介入**（如文件不存在、权限不足等环境问题）\n\n")
    builder.WriteString("❌ **严禁**：继续调用上述列出的工具+参数组合\n")
    
    return builder.String()
}
```

**示例输出**：
```
⚠️ **系统干预提示**：以下工具调用已连续失败达到上限，禁止继续重试相同参数：

1. **fileRead** - 已失败 3 次
   参数预览：`path=/nonexist.txt`

**请选择以下操作之一**：

1. **向用户反馈失败原因**，说明当前子任务无法完成，结束本轮对话
2. **重新规划任务**，基于已有信息更换工具或修改参数后继续
3. **明确告知用户需要人工介入**（如文件不存在、权限不足等环境问题）

❌ **严禁**：继续调用上述列出的工具+参数组合
```

---
## 五、分层开发子任务拆解

将整个功能按模块拆解为 6 个独立子任务，按依赖顺序排列：

### 子任务 1：熔断计数器核心模块

**模块位置**：`internal/domains/models/circuit_breaker/`

**产出文件**：
- `counter.go` - `ToolCallCounter` 结构体 + 核心方法
- `key_generator.go` - `GenerateKey` 函数 + 定制化哈希策略
- `prompt_builder.go` - `buildInterventionPrompt` 干预提示生成
- `counter_test.go` - 单元测试

**职责**：
- 实现计数器 CRUD 操作
- 实现工具名+参数到哈希 Key 的映射（方案 C 定制策略）
- 实现清零逻辑（`StartNewRound`）
- 实现阈值判定与失败记录提取

**依赖**：无（纯值对象，无外部依赖）

**验收标准**：
- 单元测试覆盖率 ≥ 90%
- 支持所有定制哈希场景（fileRead/fileEdit/bashExec/default）
- 清零逻辑通过测试验证

---

### 子任务 2：BaseAgent 集成计数器

**模块位置**：`internal/domains/services/agents/base.go`

**改造点**：
1. `BaseAgent` 结构体新增 `circuitBreaker` 字段
2. `NewBaseAgent` 初始化计数器
3. `InvokeToolCalls` 埋点记录失败 + 触发清零
4. `InvokeLLM` / `StreamingInvokeLLM` 埋点判断阈值 + 注入提示

**依赖**：子任务 1（需要 `circuit_breaker` 包）

**验收标准**：
- 编译通过，无破坏现有功能
- 日志中能观测到计数器更新记录
- 手动触发失败 3 次后，下一轮 LLM 请求中包含干预提示

---

### 子任务 3：ReActAgent 和 PlanAgent 集成

**模块位置**：
- `internal/domains/services/agents/react.go`
- `internal/domains/services/agents/plan.go`

**改造点**：
- `ReActAgent` 和 `PlanAgent` 内嵌 `BaseAgent`，自动继承熔断能力
- 验证 `react.go` 和 `plan.go` 中调用 `InvokeLLM` / `InvokeToolCalls` 的路径是否正确传递计数器

**依赖**：子任务 2

**验收标准**：
- ReAct 和 Plan 模式下熔断机制生效
- 日志中正确记录对应 Agent 类型的熔断事件

---

### 子任务 4：日志与可观测性增强

**模块位置**：`internal/domains/services/agents/base.go`

**改造点**：
- 在 `RecordFailure` 时新增结构化日志（`zap.String("key", key)`, `zap.Int("failCount", count)`）
- 在 `GetTriggeredRecords` 触发时记录告警级别日志
- 在干预提示注入时记录完整的 `triggeredRecords` 内容

**依赖**：子任务 2

**验收标准**：
- 通过日志可追踪单次会话的完整熔断过程
- 日志字段完整，便于后续接入监控告警

---

### 子任务 5：异常分支处理

**覆盖场景**：
1. **哈希计算异常**：JSON 解析失败 → 降级为不计数，工具调用正常执行
2. **计数器溢出**：单个 Key 失败次数超过 1000 → 设置上限防止异常
3. **并发会话隔离**：每个 `BaseAgent` 实例独立持有计数器 → 天然隔离，无需额外处理
4. **多工具批量失败**：单轮 10 个工具全失败 → `GetTriggeredRecords` 返回所有达标记录，干预提示列举全部

**改造点**：
- `GenerateKey` 增加错误处理，返回空 Key 时降级跳过计数
- `RecordFailure` 增加上限检查（单 Key 最大失败次数 1000）
- 完善干预提示模板，支持一次性展示 10+ 条失败记录（截断前 10 条）

**依赖**：子任务 2

**验收标准**：
- 异常场景不影响工具调用正常执行
- 日志中有明确的降级/异常告警记录

---

### 子任务 6：文档与代码注释

**产出物**：
- `docs/features/tool-call-circuit-breaker.md` - 功能设计文档
- 代码注释：每个埋点位置增加 `// 【熔断机制埋点X】` 标记
- CHANGELOG：记录本次功能新增

**依赖**：子任务 1-5 全部完成

**验收标准**：
- 文档包含架构图、数据流图、配置说明
- 代码注释清晰标注五个埋点位置

---

## 六、异常分支处理细化

### 6.1 哈希计算异常

**场景**：
- `GenerateKey` 中 `json.Unmarshal` 失败（参数不是合法 JSON）
- 参数中缺少必需字段（如 `fileRead` 无 `path` 参数）

**处理策略**：
```go
func (a *BaseAgent) InvokeToolCalls(...) {
    key, err := circuit_breaker.GenerateKey(funcName, funcArgs)
    if err != nil {
        logger.Warn("生成工具调用 Key 失败，跳过计数",
            zap.String("tool", funcName),
            zap.Error(err))
        key = "" // 设置为空 Key，后续跳过计数逻辑
    }
    
    // ...工具执行...
    
    if !result.Success && key != "" {  // key 为空时不计数
        a.circuitBreaker.RecordFailure(key, metadata)
    }
}
```

**不做事项**：
- ❌ 不阻断工具执行（哈希失败不影响工具调用）
- ❌ 不抛出错误事件（只记录 Warn 日志）

---

### 6.2 计数器数值边界

**场景**：
- 单个 Key 失败次数累积到极大值（理论上 int 可达 2^31-1）
- 单个会话中 `failureCounts` map 包含数千条 Key（极端场景）

**处理策略**：
```go
func (c *ToolCallCounter) RecordFailure(key string, metadata ToolCallMetadata) int {
    c.failureCounts[key]++
    
    // 设置单 Key 失败次数上限（防止异常场景）
    if c.failureCounts[key] > 1000 {
        c.failureCounts[key] = 1000
    }
    
    return c.failureCounts[key]
}
```

**内存保护**：
- Map 大小上限：如果 `len(failureCounts) > 1000`，清理最早插入的 50% 条目（FIFO）
- 实际场景中单次会话不太可能触发，但可作为保护措施

---

### 6.3 并发会话隔离验证

**场景**：
- 两个用户同时发起对话，conversationId 不同，但可能同时调用相同工具+参数

**验证点**：
- 每个 `BaseAgent` 实例在 `BaseAgentDomainService.Chat` 中创建
- 不同 conversationId 对应不同 `ChatMemory`，也对应不同 `BaseAgent` 实例
- 计数器 `circuitBreaker` 字段是 `BaseAgent` 的成员变量，自然隔离

**测试用例**：
```go
func TestConcurrentSessionIsolation(t *testing.T) {
    // 启动两个 goroutine 模拟并发会话
    // 验证两个 Agent 的 circuitBreaker 实例地址不同
    // 验证计数器状态互不影响
}
```

---

### 6.4 多工具批量失败场景

**场景**：
- 单轮 LLM 返回 10 个 toolCall，全部失败且都达到阈值 3
- 干预提示需要列举 10 条失败记录

**处理策略**：
- 参数预览字段截断（避免干预提示占用过多 token）
- 失败记录按失败次数降序排列（优先展示最严重的）
- 最多展示前 10 条，超出部分显示截断提示

**代码示例**：已在"四、干预 Prompt 模板设计"中展示

---

### 6.5 清零策略边界场景

**场景 1**：智能体连续两轮都调用了工具 A（参数相同），中间没有其他工具

**预期行为**：
- 第一轮：A 失败 3 次，计数器 `{A: 3}`
- `StartNewRound([A])` 被调用，发现 A 在本轮出现，不清零
- 第二轮：A 再次失败，计数器累加到 `{A: 4}` ✅ 符合预期

---

**场景 2**：智能体第一轮调用 A 失败 3 次，第二轮调用 B 和 C

**预期行为**：
- 第一轮：A 失败 3 次，计数器 `{A: 3}`
- `StartNewRound([B, C])` 被调用，发现 A 不在本轮，清零 A
- 计数器变为 `{B: 0, C: 0}` ✅ 符合预期（方案 B）

---

**场景 3**：智能体第一轮调用 A 失败 3 次，第二轮调用 B 成功 + A 失败

**预期行为**：
- 第一轮：A 失败 3 次，计数器 `{A: 3}`
- 第二轮开始前，`StartNewRound([A, B])` 被调用
- 发现 A 在本轮出现，不清零，保持 `{A: 3}`
- 第二轮 A 再次失败，累加到 `{A: 4}`
- 干预提示在第三轮开始前注入 ✅ 符合预期

---
## 七、全维度测试验证方案

### 7.1 单元测试（Unit Test）

**测试框架**：Go 原生 `testing` + `testify/assert`

**测试文件**：`internal/domains/models/circuit_breaker/counter_test.go`

#### 测试用例清单

| 用例编号 | 测试对象 | 场景描述 | 断言点 |
|---------|---------|---------|--------|
| UT-01 | `GenerateKey` | fileRead 工具 - 只哈希 path | 相同 path 生成相同 Key，path 变化生成不同 Key |
| UT-02 | `GenerateKey` | fileEdit 工具 - 截断 old_string/new_string | old_string 前 100 字符相同则 Key 相同，101 字符变化不影响 |
| UT-03 | `GenerateKey` | bashExec 工具 - 完整 command 哈希 | command 任何字符变化都生成不同 Key |
| UT-04 | `GenerateKey` | 未知工具 - 完整参数哈希 | 参数任何字段变化都生成不同 Key |
| UT-05 | `GenerateKey` | 参数 JSON 非法 | 返回 error，不 panic |
| UT-06 | `RecordFailure` | 正常计数累加 | 连续调用 3 次，返回值分别为 1/2/3 |
| UT-07 | `RecordFailure` | 不同 Key 独立计数 | Key A 计数 3 次，Key B 计数 1 次，互不影响 |
| UT-08 | `StartNewRound` | 清零逻辑 - 本轮未出现的 Key | 上一轮的 Key A 在本轮未出现，A 被清零 |
| UT-09 | `StartNewRound` | 保留逻辑 - 本轮再次出现的 Key | 上一轮的 Key A 在本轮再次出现，A 不清零 |
| UT-10 | `GetTriggeredRecords` | 阈值临界值 | 失败 2 次不返回，3 次返回，4 次返回 |
| UT-11 | `GetTriggeredRecords` | 多个 Key 达标 | 3 个 Key 分别失败 3/4/2 次，返回前两个 |
| UT-12 | `buildInterventionPrompt` | 单条失败记录 | 生成的提示包含工具名、失败次数、参数预览 |
| UT-13 | `buildInterventionPrompt` | 10+ 条失败记录 | 只展示前 10 条 + 截断提示 |

**示例测试代码**：
```go
func TestGenerateKey_FileRead(t *testing.T) {
    key1, err := GenerateKey("fileRead", `{"path": "/tmp/test.txt"}`)
    assert.NoError(t, err)
    
    key2, err := GenerateKey("fileRead", `{"path": "/tmp/test.txt"}`)
    assert.NoError(t, err)
    assert.Equal(t, key1, key2, "相同 path 应生成相同 Key")
    
    key3, err := GenerateKey("fileRead", `{"path": "/tmp/other.txt"}`)
    assert.NoError(t, err)
    assert.NotEqual(t, key1, key3, "不同 path 应生成不同 Key")
}

func TestStartNewRound_ClearUnusedKeys(t *testing.T) {
    counter := NewToolCallCounter()
    metadata := ToolCallMetadata{ToolName: "fileRead", ParamsPreview: "path=/test.txt"}
    counter.RecordFailure("keyA", metadata)
    counter.RecordFailure("keyA", metadata)
    counter.RecordFailure("keyA", metadata)
    
    // 本轮只调用 keyB，keyA 应被清零
    counter.StartNewRound([]string{"keyB"})
    
    records := counter.GetTriggeredRecords(3)
    assert.Empty(t, records, "keyA 应被清零，不在触发记录中")
}
```

**覆盖率目标**：≥ 90%

---

### 7.2 集成测试（Integration Test）

**测试框架**：Go 原生 `testing` + Mock LLM Invoker

**测试文件**：`internal/domains/services/agents/base_circuitbreaker_test.go`

#### 测试用例清单

| 用例编号 | 测试场景 | 模拟条件 | 验证点 |
|---------|---------|---------|--------|
| IT-01 | 单轮工具失败 3 次触发熔断 | Mock 工具返回失败 3 次 | 第 4 轮 LLM 入参包含干预提示 |
| IT-02 | 工具失败 2 次未触发熔断 | Mock 工具返回失败 2 次后成功 | 无干预提示注入 |
| IT-03 | 中间调用其他工具清零 | 第 1 轮 A 失败 3 次 → 第 2 轮调用 B → 第 3 轮 A 再次失败 | 第 3 轮 A 计数从 1 开始 |
| IT-04 | 多个工具同时达标 | A 失败 3 次，B 失败 4 次 | 干预提示列举 A 和 B |
| IT-05 | 干预后 LLM 不再调用同源工具 | 触发熔断 → Mock LLM 返回文本回答 | Agent 正常结束，无循环 |
| IT-06 | 参数变化不触发熔断 | A(path=a.txt) 失败 3 次 → A(path=b.txt) 失败 | 不触发熔断（Key 不同） |

**示例测试代码**：
```go
func TestCircuitBreaker_ThreeFailuresTriggerIntervention(t *testing.T) {
    // 1. 准备 Mock Invoker（LLM 固定返回工具调用）
    mockInvoker := &MockInvoker{
        responses: []llm.Message{
            {ToolCalls: []llm.ToolCall{{ID: "1", Name: "fileRead", Arguments: `{"path":"test.txt"}`}}},
            {ToolCalls: []llm.ToolCall{{ID: "2", Name: "fileRead", Arguments: `{"path":"test.txt"}`}}},
            {ToolCalls: []llm.ToolCall{{ID: "3", Name: "fileRead", Arguments: `{"path":"test.txt"}`}}},
            {Content: "我无法读取该文件，请检查路径"}, // 第 4 轮返回文本
        },
    }
    
    // 2. 准备 Mock Tool（固定返回失败）
    mockTool := &MockTool{
        name: "fileRead",
        result: models.ToolCallResult{Success: false, Message: "文件不存在"},
    }
    
    // 3. 创建 BaseAgent
    memory := memory.NewChatMemory()
    agent := NewBaseAgent(defaultConfig, mockInvoker, memory, []tools.Tool{mockTool}, "test")
    
    // 4. 执行对话
    eventCh := make(chan events.AgentEvent)
    go agent.Invoke("读取 test.txt", eventCh)
    
    // 5. 收集最后一轮 LLM 请求的 messages
    var lastMessages []llm.Message
    for event := range eventCh {
        // 假设有 LLMRequestEvent 类型（需要新增）
        // 实际测试中可以通过 mock invoker 的调用记录获取
    }
    
    // 6. 断言：最后一轮 messages 中包含干预提示
    found := false
    for _, msg := range lastMessages {
        if msg.Role == llm.RoleUser && strings.Contains(msg.Content, "系统干预提示") {
            found = true
            assert.Contains(t, msg.Content, "fileRead")
            assert.Contains(t, msg.Content, "已失败 3 次")
        }
    }
    assert.True(t, found, "应该注入干预提示")
}
```

---

### 7.3 端到端自动化测试（E2E Test）

**测试框架**：Playwright MCP + Go HTTP Client

**测试文件**：`mooc-manus/docs/e2e/tool-call-circuit-breaker.md`

#### E2E 测试架构

```
┌─────────────────────────────────────────────────────────────┐
│  Playwright Test Script (TypeScript/JavaScript)            │
│  - 通过 MCP 调用 /api/agent/chat                            │
│  - 监听 SSE 事件流                                          │
│  - 断言计数器状态、干预提示、工具调用行为                    │
└─────────────────────────────────────────────────────────────┘
                          ↓ HTTP/SSE
┌─────────────────────────────────────────────────────────────┐
│  mooc-manus Backend                                          │
│  - BaseAgentApplicationService.Chat                          │
│  - BaseAgent 执行工具调用 + 熔断逻辑                         │
└─────────────────────────────────────────────────────────────┘
                          ↓ Tool Invoke
┌─────────────────────────────────────────────────────────────┐
│  Mock Tool Provider（测试专用）                              │
│  - 可配置返回固定失败结果                                     │
│  - 支持计数器状态查询接口                                     │
└─────────────────────────────────────────────────────────────┘
```

#### 测试用例设计

**E2E-01：死循环复现与熔断验证**

**步骤**：
1. Playwright 通过 MCP 发起 `/api/agent/chat` 请求
2. 请求体：`{query: "读取文件 /nonexist.txt", conversationId: "test-001"}`
3. 后端配置 Mock Tool：`fileRead` 固定返回失败
4. 监听 SSE 事件流，收集所有 `ToolCallFail` 事件
5. 等待第 4 轮 LLM 响应（应包含干预提示）
6. 验证后续无 `fileRead(/nonexist.txt)` 工具调用

**自动化断言**：
```typescript
test('工具调用死循环熔断', async ({ page }) => {
  const events: AgentEvent[] = [];
  
  // 1. 发起请求并订阅 SSE 事件
  const eventSource = await page.evaluate(() => {
    return new EventSource('/api/agent/chat', {
      method: 'POST',
      body: JSON.stringify({
        query: '读取文件 /nonexist.txt',
        conversationId: 'e2e-test-001',
      }),
    });
  });
  
  // 2. 收集事件
  await page.on('sse-message', (event) => {
    events.push(JSON.parse(event.data));
  });
  
  // 3. 等待对话结束
  await page.waitForTimeout(10000); // 或监听 MessageEnd 事件
  
  // 4. 断言失败次数
  const failEvents = events.filter(e => 
    e.type === 'ToolCallFail' && 
    e.payload.toolName === 'fileRead' &&
    JSON.parse(e.payload.arguments).path === '/nonexist.txt'
  );
  expect(failEvents.length).toBe(3); // 正好失败 3 次
  
  // 5. 断言干预提示（通过 MessageEvent 观察）
  const messages = events.filter(e => e.type === 'Message');
  const interventionMsg = messages.find(m => 
    m.payload.content.includes('系统干预提示') &&
    m.payload.content.includes('fileRead') &&
    m.payload.content.includes('已失败 3 次')
  );
  expect(interventionMsg).toBeDefined();
  
  // 6. 断言后续无重复调用
  const interventionIndex = events.indexOf(interventionMsg!);
  const toolCallsAfter = events
    .slice(interventionIndex + 1)
    .filter(e => e.type === 'ToolCallStart');
  const duplicateCalls = toolCallsAfter.filter(e =>
    e.payload.toolName === 'fileRead' &&
    JSON.parse(e.payload.arguments).path === '/nonexist.txt'
  );
  expect(duplicateCalls.length).toBe(0); // 无重复调用
});
```

---

**E2E-02：参数修改后允许新调用（反向用例）**

**步骤**：
1. 第 1 轮：`fileRead(/a.txt)` 失败 3 次
2. 配置 Mock LLM：第 4 轮返回 `fileRead(/b.txt)` 工具调用
3. 验证 `fileRead(/b.txt)` 正常执行，不被熔断

**自动化断言**：
```typescript
test('参数修改后不触发熔断', async ({ page }) => {
  const events: AgentEvent[] = [];
  
  // ... 类似上述流程，收集事件 ...
  
  // 断言：/a.txt 失败 3 次后，/b.txt 仍能调用
  const aFailEvents = events.filter(e =>
    e.type === 'ToolCallFail' &&
    e.payload.toolName === 'fileRead' &&
    JSON.parse(e.payload.arguments).path === '/a.txt'
  );
  expect(aFailEvents.length).toBe(3);
  
  const bStartEvents = events.filter(e =>
    e.type === 'ToolCallStart' &&
    e.payload.toolName === 'fileRead' &&
    JSON.parse(e.payload.arguments).path === '/b.txt'
  );
  expect(bStartEvents.length).toBeGreaterThan(0);
});
```

---
## 八、干预机制有效性验收基线

定义可量化的 Pass/Fail 判定标准，用于功能上线准入。

### 8.1 功能正确性基线（必须 100% 通过）

| 验收项 | 判定标准 | 测试方法 | Pass 条件 |
|-------|---------|---------|----------|
| FC-01 | 失败计数准确性 | 单元测试 UT-06/UT-07 | 所有用例通过 |
| FC-02 | 哈希 Key 唯一性 | 单元测试 UT-01~UT-04 | 所有定制策略验证通过 |
| FC-03 | 清零策略正确性 | 单元测试 UT-08/UT-09 + 集成测试 IT-03 | 所有用例通过 |
| FC-04 | 阈值触发准确性 | 集成测试 IT-01/IT-02 | 3 次触发，2 次不触发 |
| FC-05 | 干预提示注入时机 | 集成测试 IT-01 | 第 4 轮 LLM 请求前注入 |
| FC-06 | 多工具同时熔断 | 集成测试 IT-04 | 干预提示列举所有达标工具 |
| FC-07 | 参数变化不误拦截 | 集成测试 IT-06 + E2E-02 | 新参数正常执行 |

---

### 8.2 性能基线（必须满足）

| 验收项 | 判定标准 | 测试方法 | Pass 条件 |
|-------|---------|---------|----------|
| PF-01 | 哈希计算耗时 | 基准测试 | 单次 `GenerateKey` < 1ms |
| PF-02 | 计数器更新耗时 | 基准测试 | 单次 `RecordFailure` < 0.1ms |
| PF-03 | 阈值判定耗时 | 基准测试 | `GetTriggeredRecords` < 1ms（1000 条记录） |
| PF-04 | 内存占用 | 压力测试 | 单会话计数器内存 < 1MB（1000 条 Key） |
| PF-05 | 对话延迟增加 | 集成测试 | 熔断机制引入的额外延迟 < 5ms/轮 |

**基准测试示例**：
```go
func BenchmarkGenerateKey(b *testing.B) {
    args := `{"path": "/very/long/path/to/test/file.txt"}`
    for i := 0; i < b.N; i++ {
        _, _ = GenerateKey("fileRead", args)
    }
}

func BenchmarkRecordFailure(b *testing.B) {
    counter := NewToolCallCounter()
    metadata := ToolCallMetadata{
        ToolName: "fileRead",
        ParamsPreview: "path=/test.txt",
    }
    for i := 0; i < b.N; i++ {
        counter.RecordFailure(fmt.Sprintf("key%d", i%100), metadata)
    }
}
```

---

### 8.3 鲁棒性基线（必须满足）

| 验收项 | 判定标准 | 测试方法 | Pass 条件 |
|-------|---------|---------|----------|
| RB-01 | 哈希计算异常不阻断工具 | 集成测试（非法 JSON 输入） | 工具正常执行，日志记录异常 |
| RB-02 | 会话隔离 | 并发集成测试 | 10 个并发会话互不影响 |
| RB-03 | 内存泄漏 | 长时间压力测试 | 1000 轮对话后内存无异常增长 |
| RB-04 | 大批量失败 | 集成测试（单轮 50 个工具全失败） | 干预提示正常生成，前 10 条截断 |

---

### 8.4 干预有效性基线（核心验收）

这是最关键的业务指标，需要验证干预机制真正阻断了死循环。

| 验收项 | 判定标准 | 测试方法 | Pass 条件 |
|-------|---------|---------|----------|
| EF-01 | 死循环阻断率 | E2E-01 测试 | 100% 场景下第 4 轮无重复调用 |
| EF-02 | LLM 响应干预提示 | 人工回归测试 | 5/5 场景 LLM 回应"无法完成"或"更换方案" |
| EF-03 | 假阳性率（误拦截） | E2E-02 + 回归测试 | 参数变化场景 0% 误拦截 |
| EF-04 | 多轮对话上下文保持 | 集成测试 | 干预提示注入后 Memory 完整 |

**EF-02 人工回归测试说明**：
- 构造 5 个真实场景（fileRead 不存在、bashExec 权限不足、fileEdit 匹配失败等）
- 让真实 LLM（非 Mock）执行到触发熔断
- 人工检查 LLM 的最终回答是否符合干预引导（而非继续循环）

**Pass 示例回答**：
- ✅ "我已尝试 3 次读取该文件均失败，建议您检查文件路径是否正确"
- ✅ "由于权限不足无法执行该命令，我将改用查询方式获取信息"
- ❌ "让我再试一次读取 /nonexist.txt"（继续循环，Fail）

---

### 8.5 最终验收门槛

**必须全部满足以下条件方可上线**：

1. **功能正确性**：FC-01~FC-07 全部通过
2. **性能基线**：PF-01~PF-05 全部满足
3. **鲁棒性**：RB-01~RB-04 全部通过
4. **干预有效性**：
   - EF-01 达到 100%（E2E 自动化测试）
   - EF-02 达到 100%（5/5 人工测试通过）
   - EF-03 假阳性率 = 0%
   - EF-04 通过
5. **代码质量**：
   - 单元测试覆盖率 ≥ 90%
   - 所有新增代码通过 golangci-lint 检查
   - 五个埋点位置有清晰注释标记

---

## 九、需人工确认澄清的关键问题清单

以下问题已在 brainstorming 阶段确认，记录在此作为设计依据：

| 问题编号 | 问题描述 | 确认结果 | 影响范围 |
|---------|---------|---------|---------|
| Q1 | 失败计数的重置时机（连续失败 3 次后，中间调用其他工具是否清零） | **方案 B**：清零重新计数 | 清零策略实现（`StartNewRound`） |
| Q2 | 并发工具调用的计数处理（同一轮内重复调用是否独立计数） | **方案 B**：每次独立计数 | 计数器更新时机（`InvokeToolCalls` 内） |
| Q3 | 干预提示注入的粒度（告知一个还是所有达标工具） | **方案 B**：列举所有达标工具 | 干预 Prompt 模板设计 |
| Q4 | 参数哈希计算的粒度（完整哈希 vs 定制化） | **方案 C**：根据工具类型定制 | `GenerateKey` 实现复杂度 |

**补充说明**：
- Q1-Q4 已在需求澄清阶段与用户确认，设计方案基于确认结果制定
- 如后续发现现有策略不符合实际使用场景，可调整配置项（如阈值改为可配置参数）

---

## 十、附录

### 10.1 数据流图

```
┌─────────────────────────────────────────────────────────────┐
│  1. Agent 初始化                                             │
│     BaseAgent 创建 → circuitBreaker 初始化                   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  2. 工具调用执行                                             │
│     InvokeToolCalls → 逐个执行工具                           │
│     - 生成 Key（GenerateKey）                                │
│     - 工具执行（InvokeTool）                                 │
│     - 记录失败（RecordFailure if !result.Success）           │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  3. 清零策略触发                                             │
│     StartNewRound(本轮Keys) → 清理未出现的历史 Key          │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  4. 下一轮 LLM 请求前                                        │
│     InvokeLLM 开始 → GetTriggeredRecords(3)                 │
│     - 有达标记录 → buildInterventionPrompt → 注入 messages  │
│     - 无达标记录 → 正常请求                                  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│  5. LLM 响应处理                                             │
│     - 文本回答 → 结束对话                                     │
│     - 工具调用 → 返回步骤 2                                   │
└─────────────────────────────────────────────────────────────┘
```

### 10.2 配置参数说明

虽然当前设计采用硬编码阈值（3 次），但预留可配置能力：

```go
// 未来可扩展为配置项
type CircuitBreakerConfig struct {
    Threshold           int    // 熔断阈值，默认 3
    MaxKeyCount         int    // 单会话最大 Key 数量，默认 1000
    MaxFailurePerKey    int    // 单 Key 最大失败次数上限，默认 1000
    EnableLogging       bool   // 是否启用详细日志，默认 true
}
```

当前实现中这些值为常量，后续可根据实际使用情况改为配置文件读取。

---

## 文档变更记录

| 版本 | 日期 | 变更说明 | 作者 |
|-----|------|---------|------|
| v1.0 | 2026-07-11 | 初始版本，完成需求定义、技术方案、测试规范 | Claude Opus 4.7 |

---

**文档结束**
