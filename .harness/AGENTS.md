# mooc-manus 项目顶层导航

> 本文档是AI编码前必读的项目架构总览，提供项目背景、分层职责、初始化流程和开发理念的全局视角。

---

## 一、项目简介

### 1.1 项目定位
**mooc-manus** 是基于Go语言的智能体编排与工具管理平台，提供：
- **Skill配置与版本管理** - 技能（Skill）的导入、发布、版本控制、文件管理
- **工具能力管理** - Tool Provider/Function的注册与调用
- **智能体编排** - Base Agent / A2A Agent / Plan-Execute Agent
- **MCP/A2A协议支持** - 多智能体协作

### 1.2 技术栈
- **语言**：Go 1.25
- **Web框架**：Gin
- **ORM**：GORM
- **数据库**：PostgreSQL
- **缓存**：Redis
- **日志**：zap + lumberjack
- **配置**：TOML
- **架构**：DDD（领域驱动设计）

---

## 二、架构设计理念

### 2.1 DDD三层模型

```
┌────────────────────────────────────────────┐
│ Handler 层 (api/handlers/)                 │  HTTP入口
│   解析请求 → 调用Application → 返回响应    │
└────────────────────────────────────────────┘
                    ↓
┌────────────────────────────────────────────┐
│ Application Service 层                     │  应用服务
│ (internal/applications/services/)          │  DTO转换 + 编排Domain Service
└────────────────────────────────────────────┘
                    ↓
┌────────────────────────────────────────────┐
│ Domain Service 层                          │  领域服务
│ (internal/domains/services/)               │  核心业务规则（使用DO）
└────────────────────────────────────────────┘
                    ↓
┌────────────────────────────────────────────┐
│ Repository 层 (internal/infra/repositories/)│  数据持久化
│   GORM操作（使用PO）                        │
└────────────────────────────────────────────┘
```

### 2.2 三态模型（PO/DO/DTO）

| 模型 | 所在包 | 命名后缀 | GORM Tag | JSON Tag | 用途 |
|------|--------|---------|---------|---------|------|
| PO | `internal/infra/models` | `XxxPO` | ✅ | ✅（透传需要时） | 与数据库表1:1映射 |
| DO | `internal/domains/models` | `XxxDO` | ❌ | ❌ | 领域内流转，可含嵌套/值对象 |
| DTO | `internal/applications/dtos` | `XxxRequest` / `XxxDTO` | ❌ | ✅（含binding:"required"） | HTTP入参/出参 |

### 2.3 依赖注入

- **方式**：手动构造，**不使用Wire/Fx等框架**
- **位置**：所有依赖在`api/routers/route.go`的`InitRouter`函数中集中管理
- **构造函数命名**：`New{Type}` → 返回接口类型

---

## 三、分层职责详解

### 3.1 Repository层
- **职责**：数据持久化，GORM操作
- **签名约定**：只接受/返回PO
- **事务处理**：使用`client.Transaction(func(tx *gorm.DB) error { ... })`
- **禁止行为**：包含业务逻辑、操作DO/DTO

### 3.2 Domain Service层
- **职责**：核心业务规则
- **使用对象**：DO（领域对象）
- **调用下层**：Repository（传入PO）
- **跨模块协作**：通过Repository接口读取其他模块数据
- **禁止行为**：直接操作GORM、直接接触DTO

### 3.3 Application Service层
- **职责**：DTO转换 + 编排Domain Service
- **不直接接触**：PO、GORM
- **主要工作**：
  1. Request → DO转换
  2. DO → DTO转换
  3. 编排多个Domain Service完成业务用例

### 3.4 Handler层
- **职责**：HTTP入口
- **工作流程**：
  1. `c.ShouldBindJSON(&req)` 解析请求
  2. 调用Application Service
  3. `c.JSON(http.StatusOK, dto)` 返回响应
- **错误处理**：通过`writeError`统一映射HTTP状态码
- **禁止行为**：直接调用Domain Service或Repository

---

## 四、模块初始化流程

### 4.1 InitRouter统一四层流程

所有模块初始化必须在`api/routers/route.go`的`InitRouter`函数内完成，严格按四层顺序：

```go
func InitRouter() *gin.Engine {
    r := gin.Default()
    
    // ============================================================
    // 第一层：Repository 层（按依赖拓扑顺序初始化）
    // ============================================================
    // 1.1 基础模块 Repository
    // 1.2 Skill 模块 Repository
    // 1.3 FileStorage（基础设施）
    
    // ============================================================
    // 第二层：Domain Service 层（按依赖拓扑顺序初始化）
    // ============================================================
    // 2.1 基础模块 Domain Service
    // 2.2 Skill 模块 Domain Service
    // 2.3 Agent 模块 Domain Service（依赖 Skill repo）
    
    // ============================================================
    // 第三层：Application Service 层（全模块并列）
    // ============================================================
    
    // ============================================================
    // 第四层：Handler 层（全模块并列）
    // ============================================================
    
    // ============================================================
    // 路由注册
    // ============================================================
    
    return r
}
```

### 4.2 依赖拓扑排序原则

- 同一层内，**被依赖方必须在依赖方之前初始化**
- **Repository层**：基础repo → Skill repo
- **基础设施层**（与Repository同级别）：FileStorage（位于`internal/infra/external/file_storage`）
- **Domain Service层**：基础domain → Skill domain → Agent domain（依赖Skill repo）
- **Application/Handler层**：无严格顺序，按业务模块分组

### 4.3 强制规则

- ❌ **禁止**：在全局流程外独立完成任何模块的多层初始化
- ✅ **要求**：每层内按业务模块分组，使用清晰的子注释标识

---

## 五、三态模型转换流程

### 5.1 完整转换链路

```
HTTP Request (JSON)
    ↓ c.ShouldBindJSON
[DTO] XxxRequest
    ↓ ConvertXxxRequest2DO  (在 dtos/ 包内)
[DO] XxxDO (领域层流转)
    ↓ ConvertXxxDO2PO       (在 domains/models/ 包内)
[PO] XxxPO (GORM操作)
    ↓ Repository.Create
数据库持久化
```

### 5.2 转换函数命名规范

| 转换方向 | 函数命名 | 放置位置 |
|---------|---------|---------|
| Request → DO | `ConvertXxxRequest2DO` | `internal/applications/dtos/xxx.go` |
| DO → DTO | `ConvertXxxDO2DTO` | `internal/applications/dtos/xxx.go` |
| DO → PO | `ConvertXxxDO2PO` | `internal/domains/models/xxx.go` |
| PO → DO | `ConvertXxxPO2DO` | `internal/domains/models/xxx.go` |

---

## 六、业务域拆分规则

### 6.1 当前业务域

| 业务域 | 职责 | 关键文件 |
|--------|------|---------|
| **tool** | 工具能力管理（Provider/Function） | `tool_provider.go`、`tool_function.go` |
| **skill** | 技能配置与版本管理 | `skill.go`、`skill_provider.go`、`skill_version.go` |
| **agent** | 智能体编排（Base/A2A/Plan-Execute） | `agents/base.go`、`agents/a2a.go`、`agents/plan.go` |
| **flow** | 工作流编排 | `flows/flow.go`、`flows/plan_react.go` |
| **appConfig** | 应用配置（模型/A2A服务器） | `app_config.go` |

### 6.2 拆分原则

- **按业务能力划分**，每个域包含完整四层实现
- **跨域协作**通过Repository接口（如Agent依赖Skill的Repository）
- **依赖方向单向**（Agent → Skill，不能反向）
- **禁止**循环依赖

---

## 七、开发核心理念

### 7.1 设计原则
1. **简单直接** - 不引入过度设计（如DI框架、通用响应封装、错误码体系）
2. **代码即文档** - 良好命名+必要注释，避免冗余文档
3. **错误直传** - 不引入业务错误码，使用哨兵错误+文本描述

### 7.2 编码哲学
- **遵循现有范式** - 参考tool、appConfig等成熟模块的实现
- **保持架构一致性** - 严格遵循四层分层和命名规范
- **避免重复造轮子** - 复用项目已有能力（logger、storage、sse等）

### 7.3 重构原则
- **风险最小化** - 仅调整代码顺序，不改变依赖关系
- **验证三步走** - 编译通过 + 启动成功 + 接口功能正常
- **保留现有功能** - 重构不引入新功能，纯架构调整

---

## 八、关联文档

| 文档 | 作用 | 路径 |
|------|------|------|
| **CLAUDE.md** | AI编码入口规范 | `/CLAUDE.md` |
| **.cursorrules** | 强制代码风格约束 | `/.harness/.cursorrules` |
| **conventions.md** | 全量规范约定 | `/.harness/knowledge/conventions.md` |
| **ai-error-log.md** | AI纠错日志 | `/.harness/knowledge/ai-error-log.md` |
| **mooc-manus-code-standards.md** | 详细代码规范 | `/docs/mooc-manus-code-standards.md` |

---

**最后更新：** 2026-06-25  
**维护者：** 项目团队
