# Skill 系统提示词注入功能改造说明

## 一、改造概述

本次改造为智能体 `createBaseAgent` 函数新增 Skill 元信息注入系统提示词能力，完全对齐 Beedance 项目 `BaseAgent.java` 的 `initSystemPromptMessage` 和 `buildSkillsPromptSection` 实现。

**改造原则**：
- ✅ 仅在 `SkillRefs` 非空时执行 Skill 相关逻辑
- ✅ 函数语义明确化（`BuiltinTools` → `SkillTools`）
- ✅ 系统提示词拼接格式完全对齐 Beedance
- ✅ 保持向后兼容（无 Skill 场景行为不变）

---

## 二、改造内容详细说明

### 1. 新增仓储方法（`skill.go`）

**文件**：`internal/infra/repositories/skill.go`

**新增方法**：
```go
// GetByNames 批量按 skill_name 查询（用于 Skill 系统提示词拼接）
// 不存在的 name 会被跳过，返回查询到的部分结果
func (r *SkillRepositoryImpl) GetByNames(names []string) ([]models.SkillPO, error)
```

**位置**：第 84-92 行

**作用**：避免 N+1 查询问题，批量获取 Skill 描述信息用于系统提示词拼接

---

### 2. 函数重命名（`builtin.go`）

**文件**：`internal/domains/services/tools/builtin.go`

**变更**：
- **旧函数名**：`BuiltinTools`
- **新函数名**：`SkillTools`
- **注释更新**：明确说明"仅用于初始化 Skill 专属内置工具（loadSkill + executeSkill）"

**位置**：第 9-35 行

**调用方变更**：`agent.go:192` 调用处同步更新

---

### 3. 新增系统提示词构建逻辑（`agent.go`）

**文件**：`internal/domains/services/agents/agent.go`

#### 3.1 新增常量（第 218-233 行）

```go
// skillUsageRules Skill 使用规则常量（对齐 Beedance BaseAgent.java:163-179）
const skillUsageRules = `
### Skill Usage Rules (MUST follow)

**Step 1 (Required):** Call `loadSkill(skillName)` to read the Skill documentation.

**Step 2:** Follow the Skill documentation's instructions to determine your next action:
- If the documentation defines an **output format/syntax** (e.g., Markdown blocks, custom tags): output that syntax directly in your response. Do NOT call `executeSkill`.
- If the documentation requires **script execution**: call `executeSkill(skillName, bash)`. Write output files ONLY to `/workspace/outputs/`.

**Constraints:**
- Always call `loadSkill` before `executeSkill`. Never skip `loadSkill`.
- `skillName` MUST match one of the names listed above exactly. Do NOT invent skill names.
- After a successful `executeSkill` that generates a file, do not repeat the file content or add download links in your response.
`
```

**对齐依据**：Beedance `BaseAgent.java` 第 163-179 行 `SKILL_USAGE_RULES` 常量

#### 3.2 新增 buildSkillsSystemPrompt 函数（第 235-280 行）

```go
// buildSkillsSystemPrompt 构建 Skill 相关系统提示词段落
// 参考 Beedance BaseAgent.buildSkillsPromptSection (BaseAgent.java:881-912)
func (s *BaseAgentDomainServiceImpl) buildSkillsSystemPrompt(skillRefs []agents.SkillRef) (string, error)
```

**核心流程**（完全对齐 Beedance）：
1. 空值校验：`len(skillRefs) == 0` 返回空字符串
2. 提取 skillName 列表
3. 批量查询 Skill 表获取描述信息（调用 `GetByNames`）
4. 拼接 Skill 列表（格式：`- **{skillName}**: {description}`）
5. 拼接最终段落（固定标题 `## Available Skills` + skillList + `skillUsageRules`）

**对齐依据**：Beedance `BaseAgent.java` 第 881-912 行 `buildSkillsPromptSection` 方法

---

### 4. 改造 createBaseAgent 函数（`agent.go:156-215`）

#### 4.1 工具注册改造（第 191-199 行）

**变更前**：
```go
// 追加内置工具
builtinTools, err := tools.BuiltinTools(s.skillRepo, s.versionRepo, s.storage, request.SkillRefs)
if err != nil {
    logger.Error("init builtin tools failed", zap.Error(err))
    return nil, err
}
baseTools = append(baseTools, builtinTools...)
logger.Info("init builtin tools success")
```

**变更后**：
```go
// 追加 Skill 内置工具（仅在 SkillRefs 非空时）
if len(request.SkillRefs) > 0 {
    skillTools, err := tools.SkillTools(s.skillRepo, s.versionRepo, s.storage, request.SkillRefs)
    if err != nil {
        logger.Error("init skill tools failed", zap.Error(err))
        return nil, err
    }
    baseTools = append(baseTools, skillTools...)
    logger.Info("init skill tools success", zap.Int("skill_count", len(request.SkillRefs)))
}
```

**关键改进**：
- ✅ 新增 `len(request.SkillRefs) > 0` 判断（无 Skill 场景不注册工具）
- ✅ 调用重命名后的 `SkillTools` 函数
- ✅ 日志增加 skill_count 字段

#### 4.2 系统提示词注入改造（第 201-213 行）

**新增逻辑**：
```go
// 构建系统提示词（拼接 Skill 元信息）
systemPrompt := request.SystemPrompt
if len(request.SkillRefs) > 0 {
    skillsPrompt, err := s.buildSkillsSystemPrompt(request.SkillRefs)
    if err != nil {
        logger.Warn("build skills system prompt failed", zap.Error(err))
        // 失败时仍可继续，但记录警告
    } else if skillsPrompt != "" {
        // 拼接顺序：原 systemPrompt + "\n\n" + skillsPrompt（对齐 Beedance BaseAgent.java:863-864）
        systemPrompt = systemPrompt + "\n\n" + skillsPrompt
        logger.Info("skills system prompt injected", zap.Int("skill_count", len(request.SkillRefs)))
    }
}
```

**关键要点**：
- ✅ 仅在 `len(request.SkillRefs) > 0` 时执行
- ✅ 拼接顺序：`原 SystemPrompt + "\n\n" + Skills 段`（对齐 Beedance）
- ✅ 构建失败仅记录警告，不中断 Agent 创建（对齐用户确认的"方案 B"）
- ✅ 允许原 SystemPrompt 为空，仅注入 Skills 段（对齐用户确认的"方案 B"）

**对齐依据**：Beedance `BaseAgent.java` 第 850-875 行 `initSystemPromptMessage` 方法

---

## 三、Beedance 对齐点验证

| 对齐维度 | Beedance 实现 | mooc-manus 实现 | 验证位置 |
|---------|--------------|----------------|---------|
| **空值校验** | `referenceSkills` 为空返回 null | `len(skillRefs) == 0` 返回空字符串 | agent.go:237-239 |
| **Skill 描述来源** | `ClientSkillReference.description` | 查询 Skill 表 Description 字段 | agent.go:250-253 |
| **列表拼接格式** | `- **{skillName}**: {description}` | 完全一致 | agent.go:263-268 |
| **固定标题** | `## Available Skills` | 完全一致 | agent.go:271 |
| **规则常量** | `SKILL_USAGE_RULES` (BaseAgent.java:163-179) | `skillUsageRules` (agent.go:218-233) | 完全对齐 |
| **段间分隔符** | `\n\n` | 完全一致 | agent.go:209 |
| **拼接顺序** | prePrompt → Skills 段 | systemPrompt → Skills 段 | agent.go:209 |
| **工具注册时机** | `initPromptTools` 后 | `InitTools` 后、创建 Agent 前 | agent.go:191-199 |

---

## 四、边界条件与兼容性保证

| 场景 | 行为 | 验证代码位置 |
|------|------|-------------|
| **SkillRefs 为空** | 完全跳过 Skill 工具注册和提示词注入，行为与改造前完全一致 | agent.go:191 |
| **SkillRefs 非空但 Skill 不存在** | `GetByNames` 返回空列表，记录警告但不中断 Agent 创建 | agent.go:255-257 |
| **Skill 表 Description 为空** | 仅输出 `- **<skillName>**`（无冒号和描述） | agent.go:264-267 |
| **系统提示词拼接失败** | 记录警告但继续创建 Agent（使用原始 SystemPrompt） | agent.go:204-207 |
| **SkillRef.SkillName 为空** | 过滤掉该条记录 | agent.go:242-246 |
| **SystemPrompt 为空** | 允许拼接 Skills 段（对齐用户确认的"方案 B"） | agent.go:201-213 |

---

## 五、测试场景建议

### 场景 1：无 Skill 场景
- **输入**：`request.SkillRefs = []`
- **预期**：
  - 不调用 `SkillTools`（工具列表不含 loadSkill/executeSkill）
  - SystemPrompt 不含 `## Available Skills` 段
  - 日志不含 "init skill tools success"

### 场景 2：单个 Skill 场景
- **输入**：`request.SkillRefs = [{SkillID: "1", SkillName: "apm-rca", Version: "v1.0.0"}]`
- **预期**：
  - 注册 loadSkill/executeSkill 工具
  - SystemPrompt 含 1 行 Skill 列表：`- **apm-rca**: 调用根因分析助手定位故障`
  - SystemPrompt 含 `### Skill Usage Rules` 段

### 场景 3：多个 Skill 场景
- **输入**：`request.SkillRefs` 含 3 个 Skill
- **预期**：SystemPrompt 含 3 行 Skill 列表 + 规则段

### 场景 4：Skill 不存在场景
- **输入**：`SkillRef.SkillName = "non-existent"`
- **预期**：
  - `GetByNames` 返回空列表
  - 日志含 "no skills found for provided skillNames"
  - SystemPrompt 不含 Skills 段（返回原始 SystemPrompt）

### 场景 5：Description 为空场景
- **输入**：Skill 表 Description 字段为空
- **预期**：SystemPrompt 仅输出 `- **skillName**`（无冒号和描述）

---

## 六、关键决策记录

基于用户澄清的 5 个问题，本次改造采用以下决策：

| 问题 | 决策 | 理由 |
|------|------|------|
| **SkillRef 是否需要 Enabled 字段？** | 不新增（方案 B） | 前端传入的 SkillRefs 已做过滤，均为已启用状态 |
| **Skill Description 来源** | 仅查询 Skill 表 Description | 简化实现，Description 为空输出 `- **skillName**` |
| **SystemPrompt 为空时的行为** | 允许空 SystemPrompt + Skills 段（方案 B） | 更灵活，Skills 段本身就是系统提示词的一部分 |
| **GetByNames 方法** | 新增 `SkillRepository.GetByNames` | 避免 N+1 查询，批量获取更高效 |
| **工具注册失败的处理策略** | 保持当前行为（方案 B） | Go 惯用显式错误处理，不建议吞掉错误 |

---

## 七、变更文件汇总

| 文件 | 变更类型 | 核心变更 |
|------|---------|---------|
| `internal/infra/repositories/skill.go` | 新增方法 | `GetByNames(names []string)` 批量查询 |
| `internal/domains/services/tools/builtin.go` | 重命名函数 | `BuiltinTools` → `SkillTools` |
| `internal/domains/services/agents/agent.go` | 新增函数 + 改造逻辑 | `buildSkillsSystemPrompt` + `skillUsageRules` 常量 + `createBaseAgent` 改造 |

**影响范围**：
- ✅ 仅影响智能体初始化逻辑
- ✅ 对外接口（ChatRequest）无变更
- ✅ 无数据库 Schema 变更
- ✅ 完全向后兼容（无 Skill 场景行为不变）

---

## 八、编译验证结果

```bash
# 全量编译通过
$ go build -o /tmp/mooc-manus-build main.go && rm /tmp/mooc-manus-build
全量编译通过

# 各模块独立编译通过
$ go build ./internal/domains/services/agents/...
$ go build ./internal/domains/services/tools/...
$ go build ./internal/infra/repositories/...
```

**验证命令输出**：
- ✅ 无编译错误
- ✅ 无导入循环
- ✅ 类型检查通过

---

## 九、后续优化建议

1. **性能优化**：考虑在 ChatRequest DTO 中增加 `SkillRef.Description` 字段，避免查库（权衡前端传参冗余 vs 查询开销）
2. **缓存优化**：高频查询的 Skill 描述信息可考虑缓存（Redis / 内存缓存）
3. **监控增强**：增加 Skill 系统提示词注入成功/失败的指标埋点
4. **文档同步**：更新 API 文档，说明 SystemPrompt 的 Skills 段注入逻辑

---

## 十、参考文档

1. Beedance 源码：`BaseAgent.java` 第 850-912 行、第 163-179 行
2. Beedance 文档：《Skill系统提示词注入逻辑说明.md》
3. mooc-manus 规范：`.harness/.cursorrules` 7.2 节（禁止多租户字段）
