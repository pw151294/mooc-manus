# mooc-manus 架构统一重构与规范体系建设 - 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一全项目模块初始化架构，建立标准化AI编码约束体系（Harness护栏层）

**Architecture:** 采用渐进式四层统一架构，将Skill和Agent模块融入InitRouter的统一流程（repo → domain → application → handler），每层内按依赖拓扑顺序排列。同时建立CLAUDE.md入口规范+.harness护栏层的三层规范体系。

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL, DDD架构, Markdown文档

**Spec Document:** docs/superpowers/specs/2026-06-25-architecture-unification-design.md

---

## 任务概览

本计划分为5个主任务：
1. **架构重构** - 重构route.go，统一模块初始化流程
2. **入口规范** - 创建CLAUDE.md和.harness目录结构
3. **护栏核心** - 编写.cursorrules和AGENTS.md
4. **知识库** - 编写ai-error-log.md和conventions.md
5. **验证优化** - 验证完整性并提交

---

### 任务1：重构InitRouter统一模块初始化架构

**目标:** 将Skill和Agent模块的初始化融入统一四层流程

**Files:**
- Modify: `api/routers/route.go:18-192`

- [ ] **步骤1: 读取当前route.go了解现有结构**

运行: 读取文件，定位Skill模块独立初始化代码段（约78-108行）和Agent模块独立初始化代码段（约111-129行）

预期: 找到需要重构的两个代码段

- [ ] **步骤2: 备份原有route.go（通过git）**

```bash
git add api/routers/route.go
git commit -m "chore: 备份route.go重构前状态"
```

- [ ] **步骤3: 重构InitRouter函数 - 第一层Repository**

删除Skill模块独立的Repository初始化（78-86行），将其融入统一Repository层。

在原有基础Repository初始化后追加：

```go
// 1.2 Skill 模块 Repository（无外部依赖）
skillProviderRepo := repositories.NewSkillProviderRepository()
skillRepo := repositories.NewSkillRepository()
skillVersionRepo := repositories.NewSkillVersionRepository()
taskExecutionRepo := repositories.NewTaskExecutionRepository()

// 1.3 FileStorage（基础设施，与 Repository 同级别）
rootDir := "./data"
if config.Cfg != nil {
	rootDir = config.Cfg.Storage.RootDir
}
fs := file_storage.NewLocalFileStorage(rootDir)
```

- [ ] **步骤4: 重构InitRouter函数 - 第二层Domain Service**

删除Skill模块独立的Domain Service初始化（96-99行），将其融入统一Domain Service层。

在appConfigDomainSvc初始化后追加：

```go
// 2.2 Skill 模块 Domain Service（无跨模块依赖）
skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo, skillRepo)
skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, skillProviderRepo, fs)
skillVersionDomainSvc := domain_svc.NewSkillVersionDomainService(skillVersionRepo, skillRepo, fs)
skillImportTaskDomainSvc := domain_svc.NewSkillImportTaskDomainService(taskExecutionRepo, skillDomainSvc, skillProviderDomainSvc, fs)
```

删除Agent模块独立初始化（111-122行），紧接Skill Domain Service后追加：

```go
// 2.3 Agent 模块 Domain Service（依赖 Skill repo，放在 Skill Domain Service 之后）
baseAgentDomainSvc := agents.NewBaseAgentDomainService(
	appConfigDomainSvc,
	providerDomainSvc,
	functionDomainSvc,
	skillRepo,
	skillVersionRepo,
	fs,
)
a2aDomainSvc := agents.NewA2ADomainService(baseAgentDomainSvc, appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
baseFlowDomainSvc := flows.NewBaseFlowDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
```

- [ ] **步骤5: 重构InitRouter函数 - 第三层Application Service**

删除Skill模块独立的Application Service初始化（102-105行），将其融入统一Application Service层。

在functionAppSvc初始化后追加：

```go
skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
skillVersionAppSvc := app_svc.NewSkillVersionApplicationService(skillVersionDomainSvc, skillDomainSvc)
skillImportTaskAppSvc := app_svc.NewSkillImportTaskApplicationService(skillImportTaskDomainSvc)
skillAppSvc := app_svc.NewSkillApplicationService(skillDomainSvc, skillVersionDomainSvc, skillProviderDomainSvc)
```

删除Agent模块独立的Application Service初始化（124-126行），紧接Skill Application Service后追加：

```go
baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc)
a2aAppSvc := app_svc.NewA2AApplicationService(a2aDomainSvc)
baseFlowAppSvc := app_svc.NewFlowApplicationService(baseFlowDomainSvc)
```

- [ ] **步骤6: 重构InitRouter函数 - 第四层Handler**

删除Skill模块独立的Handler初始化（108行），将其融入统一Handler层。

删除Agent模块独立的Handler初始化（128-129行），将其融入统一Handler层。

在toolHandler初始化后追加：

```go
skillHandler := handlers.NewSkillHandler(skillAppSvc, skillProviderAppSvc, skillVersionAppSvc, skillImportTaskAppSvc)
agentHandler := handlers.NewAgentHandler(baseAgentAppSvc, a2aAppSvc)
flowHandler := handlers.NewFlowHandler(baseFlowAppSvc)
```

- [ ] **步骤7: 添加四层分隔注释**

在每层开头添加清晰的注释分隔，确保视觉层次清晰：

```go
// ============================================================
// 第一层：Repository 层（按依赖拓扑顺序初始化）
// ============================================================
// 1.1 基础模块 Repository（无外部依赖）
...
// 1.2 Skill 模块 Repository（无外部依赖）
...
// 1.3 FileStorage（基础设施，与 Repository 同级别）
...

// ============================================================
// 第二层：Domain Service 层（按依赖拓扑顺序初始化）
// ============================================================
// 2.1 基础模块 Domain Service（无跨模块依赖）
...
// 2.2 Skill 模块 Domain Service（无跨模块依赖）
...
// 2.3 Agent 模块 Domain Service（依赖 Skill repo，放在 Skill Domain Service 之后）
...

// ============================================================
// 第三层：Application Service 层（全模块并列）
// ============================================================
...

// ============================================================
// 第四层：Handler 层（全模块并列）
// ============================================================
...
```

- [ ] **步骤8: 编译验证**

运行: `go build -o mooc-manus main.go`

预期: 编译成功，无语法错误

- [ ] **步骤9: 启动验证**

运行: `./mooc-manus`（或按项目启动方式）

预期: 项目启动成功，无初始化错误

- [ ] **步骤10: 提交重构代码**

```bash
git add api/routers/route.go
git commit -m "refactor: 统一Skill和Agent模块初始化架构

- 将Skill模块的四层初始化融入InitRouter统一流程
- 将Agent模块的四层初始化融入InitRouter统一流程
- 严格按repo→domain→application→handler四层顺序
- 每层内按依赖拓扑顺序排列
- 添加清晰的层级注释分隔

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### 任务2：创建入口规范和护栏目录结构

**目标:** 建立CLAUDE.md入口规范文件和.harness护栏层目录

**Files:**
- Create: `CLAUDE.md`
- Create: `.harness/.cursorrules`
- Create: `.harness/AGENTS.md`
- Create: `.harness/knowledge/ai-error-log.md`
- Create: `.harness/knowledge/conventions.md`

- [ ] **步骤1: 创建.harness目录结构**

```bash
mkdir -p .harness/knowledge
```

- [ ] **步骤2: 编写CLAUDE.md入口规范**

创建`CLAUDE.md`文件，包含以下核心内容：

```markdown
# mooc-manus 项目 - Claude Code 编码规范

> 本文件是Claude Code在mooc-manus项目中编码的唯一入口规范，所有AI编码、修改、重构行为必须优先遵循本文件及.harness护栏层的约束。

---

## 一、基础约定

### 1.1 输出语言
- **强制要求**：所有对话、注释、文档输出语言为**中文**
- 代码标识符、技术术语保持原文（如function、repository等）

### 1.2 核心原则
1. **护栏层优先**：所有AI编码必须优先遵循`.harness/`护栏层规范
2. **禁止突破约束**：禁止私自采用非标架构/编码逻辑，禁止突破护栏约束
3. **现有范式优先**：参考现有成熟模块（tool、appConfig）的实现方式
4. **架构红线不可触碰**：不得违反项目分层架构、模块初始化规则、路由注册规则

---

## 二、护栏层文件说明

### 2.1 .harness/ 目录结构

```
.harness/
├── .cursorrules           # 强制代码风格约束（命名/分层/转换函数）
├── AGENTS.md              # 项目顶层导航（架构/分层职责/初始化流程）
└── knowledge/
    ├── ai-error-log.md   # AI纠错日志（典型错误案例，只增不删）
    └── conventions.md    # 项目全量规范约定（命名/目录/规范汇总）
```

### 2.2 读取优先级

**编码前必读：**
1. `.harness/AGENTS.md` - 理解项目整体架构、分层职责、初始化流程
2. `.harness/.cursorrules` - 掌握强制编码规则、命名规范、禁用API

**编码中参考：**
3. `.harness/knowledge/conventions.md` - 查阅完整规范约定
4. `.harness/knowledge/ai-error-log.md` - 避免重复已知错误

### 2.3 使用场景

| 场景 | 必读文件 | 说明 |
|------|---------|------|
| 新增模块/功能 | AGENTS.md + .cursorrules | 理解架构 + 掌握规则 |
| 修改现有代码 | .cursorrules + conventions.md | 遵循风格 + 查阅规范 |
| 重构代码 | AGENTS.md + ai-error-log.md | 理解原有设计 + 避免已知错误 |
| 问题排查 | ai-error-log.md | 查看是否有类似错误记录 |

---

## 三、项目架构红线

### 3.1 分层架构（不可违反）

**严格四层模型：**
```
Handler 层 (api/handlers/)
    ↓ 调用
Application Service 层 (internal/applications/services/)
    ↓ 调用
Domain Service 层 (internal/domains/services/)
    ↓ 调用
Repository 层 (internal/infra/repositories/)
```

**禁止行为：**
- ❌ Handler直接调用Domain Service或Repository
- ❌ Application Service直接调用Repository
- ❌ 跨层级调用或反向依赖

### 3.2 模块初始化规则（不可违反）

**统一初始化流程：**
- 所有模块初始化必须在`api/routers/route.go`的`InitRouter`函数内完成
- 严格按四层顺序：Repository → Domain Service → Application Service → Handler
- 每层内部按依赖拓扑顺序排列（被依赖方在前，依赖方在后）
- **禁止**在全局流程外独立完成任何模块的多层初始化

### 3.3 路由注册规则（不可违反）

- 所有路由注册必须在`InitRouter`函数的路由注册段完成
- 路由分组使用`r.Group()`
- 路由路径命名遵循RESTful风格

---

## 四、工作流程

### 4.1 编码前必读流程

1. 读取`.harness/AGENTS.md` - 理解项目整体架构
2. 读取`.harness/.cursorrules` - 掌握强制编码规则
3. 参考现有成熟模块实现（如tool、appConfig）
4. 确认自己的实现符合架构红线

### 4.2 编码中参考流程

1. 遇到规范疑问时查阅`.harness/knowledge/conventions.md`
2. 遇到错误时查阅`.harness/knowledge/ai-error-log.md`
3. 保持与现有代码风格一致

---

**最后更新：** 2026-06-25  
**维护者：** 项目团队
```

- [ ] **步骤3: 提交入口规范和目录结构**

```bash
git add CLAUDE.md .harness/
git commit -m "docs: 添加CLAUDE.md入口规范和.harness护栏目录

- 创建CLAUDE.md作为AI编码入口规范
- 定义输出语言、核心原则、护栏层优先级
- 说明.harness目录结构和文件作用
- 明确架构红线：分层架构、初始化规则、路由注册规则
- 定义编码前必读流程和编码中参考流程

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### 任务3：编写护栏核心文件

**目标:** 编写.cursorrules代码风格约束和AGENTS.md项目导航

**Files:**
- Modify: `.harness/.cursorrules`
- Modify: `.harness/AGENTS.md`

- [ ] **步骤1: 编写.cursorrules强制代码风格约束**


创建`.harness/.cursorrules`文件（由于内容较长，采用分段编写）：

**第一部分：命名规范**

```
# mooc-manus 项目代码风格强制约束

## 一、命名规范

### 1.1 文件命名
- 规则：蛇形命名 + 单数形式
- 示例：`skill_provider.go`、`app_config.go`、`a2a_server_config.go`
- 禁止：驼峰命名、复数形式

### 1.2 包命名
- 规则：复数形式（领域目录下）
- 示例：`handlers`、`services`、`repositories`、`models`、`dtos`
- 特例：子模块包名使用业务名（`agents`、`flows`、`tools`）

### 1.3 结构体命名
- 规则：大驼峰 + 后缀
- PO（持久对象）：`AppConfigPO`、`SkillProviderPO`
- DO（领域对象）：`AppConfigDO`、`SkillDO`
- DTO（传输对象）：`AppConfigDTO`、`SkillInfoDTO`
- Request（请求对象）：`AppConfigCreateRequest`、`SkillDraftSaveRequest`

### 1.4 接口命名
- 规则：大驼峰 + Service/Repository
- 示例：`AppConfigApplicationService`、`SkillDomainService`、`SkillRepository`

### 1.5 实现类命名
- 规则：接口名 + Impl
- 示例：`AppConfigApplicationServiceImpl`、`SkillRepositoryImpl`

### 1.6 构造函数命名
- 规则：New + 接口名（返回接口类型）
- 示例：`NewAppConfigApplicationService(...) AppConfigApplicationService`
- 禁止：返回实现类型

### 1.7 方法命名
- 公开方法：大驼峰，动词在前
  - 示例：`Create`、`Update`、`GetById`、`List`、`DeleteById`
- 私有方法：小驼峰
  - 示例：`getZapLevel`、`parseSkillFiles`

### 1.8 常量命名
- 规则：蛇形小写或小驼峰，按文件聚合
- 示例：`defaultModelTemperature`、`postgresDsn`、`sseTimeout`

### 1.9 转换函数命名
- 规则：Convert{Src}2{Dst}
- PO↔DO：`ConvertAppConfigPO2DO`、`ConvertAppConfigDO2PO`
- DO↔DTO：`ConvertAppConfigRequest2DO`、`ConvertAppConfigDO2DTO`

### 1.10 JSON字段命名
- 规则：小驼峰
- 示例：`appConfigId`、`baseUrl`、`modelName`、`createdAt`、`updatedAt`

### 1.11 数据库列命名
- 规则：蛇形小写
- 示例：`app_config_id`、`base_url`、`model_name`、`created_at`、`updated_at`

### 1.12 时间字段命名
- 规则：统一使用`CreatedAt`/`UpdatedAt`
- DB列：`created_at`/`updated_at`
- JSON：`createdAt`/`updatedAt`
- **禁止**：`gmtCreate`、`gmtModified`、`gmt_create`

---

## 二、分层编码规则

### 2.1 Repository层规则
- **职责**：数据持久化，GORM操作
- **签名**：只接受/返回PO
- **禁止**：业务逻辑、DO/DTO操作
- **事务**：使用`client.Transaction(func(tx *gorm.DB) error { ... })`

### 2.2 Domain Service层规则
- **职责**：核心业务规则
- **使用对象**：DO（领域对象）
- **调用下层**：Repository（传入PO）
- **禁止**：直接操作GORM、直接接触DTO

### 2.3 Application Service层规则
- **职责**：DTO转换、编排Domain Service
- **不直接接触**：PO、GORM
- **主要工作**：Request→DO转换、DO→DTO转换、编排Domain Service

### 2.4 Handler层规则
- **职责**：HTTP入口、参数解析、响应返回
- **工作流程**：
  1. `c.ShouldBindJSON(&req)` 解析请求
  2. 调用Application Service
  3. `c.JSON(http.StatusOK, dto)` 返回响应
- **禁止**：直接调用Domain Service或Repository

---

## 三、三态模型转换规则

### 3.1 PO↔DO转换
- 函数命名：`ConvertXxxPO2DO`、`ConvertXxxDO2PO`
- 放置位置：`internal/domains/models/xxx.go`
- 职责：数据库字段 ↔ 领域对象

### 3.2 DO↔DTO转换
- 函数命名：`ConvertXxxRequest2DO`、`ConvertXxxDO2DTO`
- 放置位置：`internal/applications/dtos/xxx.go`
- 职责：传输对象 ↔ 领域对象

### 3.3 转换流程
```
HTTP Request (JSON)
    ↓ c.ShouldBindJSON
[DTO] XxxRequest
    ↓ ConvertXxxRequest2DO
[DO] XxxDO (领域层流转)
    ↓ ConvertXxxDO2PO
[PO] XxxPO (GORM操作)
    ↓ 数据库
```

---

## 四、模块初始化范式

### 4.1 强制规则
- 所有模块初始化**必须**在`api/routers/route.go`的`InitRouter`函数内完成
- **严格四层顺序**：Repository → Domain Service → Application Service → Handler
- **禁止**在全局流程外独立完成任何模块的多层初始化

### 4.2 依赖拓扑排序
- 同一层内，被依赖方必须在依赖方之前初始化
- Repository层：基础repo → Skill repo → FileStorage
- Domain Service层：基础domain → Skill domain → Agent domain（依赖Skill repo）
- Application/Handler层：无严格顺序，按业务模块分组

### 4.3 注释规范
- 每层开头用分隔注释：`// ============================================================`
- 每层内用子注释标识模块分组：`// 1.1 基础模块 Repository`

---

## 五、错误处理规范

### 5.1 不引入业务错误码
- **禁止**：定义`SKILL_NOT_FOUND`等业务错误码字符串
- **使用**：标准error + 文本描述

### 5.2 Service层使用哨兵错误
```go
var (
    ErrNotFound     = errors.New("not found")
    ErrDuplicate    = errors.New("duplicate")
    ErrInvalidInput = errors.New("invalid input")
)

// 返回时包装
return fmt.Errorf("skill not found: %w", ErrNotFound)
```

### 5.3 Handler层映射HTTP状态码
```go
func writeError(c *gin.Context, err error) {
    switch {
    case errors.Is(err, services.ErrNotFound):
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
    case errors.Is(err, services.ErrDuplicate):
        c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
    case errors.Is(err, services.ErrInvalidInput):
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    default:
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    }
}
```

---

## 六、响应格式规范

### 6.1 不使用通用响应封装
- **禁止**：定义`Response[T]`、`SingleResponse`、`PageResponse`等通用类型
- **直接返回**：`c.JSON(http.StatusOK, dto)`

### 6.2 响应格式
- 单对象：`c.JSON(http.StatusOK, dto)`
- 列表：`c.JSON(http.StatusOK, []dtoList)`
- 无返回值成功：`c.JSON(http.StatusOK, gin.H{"status": "success"})`
- 错误：`c.JSON(http.StatusXxx, gin.H{"error": err.Error()})`

### 6.3 分页响应（特例）
- Skill模块新增范式，定义在`dtos/skill.go`：
```go
type SkillPageDTO struct {
    Total    int64        `json:"total"`
    PageSize int          `json:"pageSize"`
    PageNum  int          `json:"pageNum"`
    Records  []SkillInfo  `json:"records"`
}
```

---

## 七、禁用API清单

### 7.1 禁止软删除
- **禁止字段**：`delete_flag`、`deleted_at`
- **删除方式**：物理删除

### 7.2 禁止多租户字段
- **禁止字段**：`scope`、`subject_id`、`tenant_id`、`project_id`

### 7.3 禁止通用响应封装
- **禁止类型**：`Response[T]`、`SingleResponse[T]`、`PageResponse[T]`

### 7.4 时间字段统一
- **使用**：`createdAt`、`updatedAt`
- **禁止**：`gmtCreate`、`gmtModified`
```

- [ ] **步骤2: 提交.cursorrules文件**

```bash
git add .harness/.cursorrules
git commit -m "docs: 添加.cursorrules代码风格强制约束

- 定义命名规范：文件/包/结构体/接口/方法/字段
- 定义分层编码规则：Repository/Domain/Application/Handler
- 定义三态模型转换规则：PO/DO/DTO
- 定义模块初始化范式：四层顺序/依赖拓扑
- 定义错误处理规范：哨兵错误/HTTP状态码映射
- 定义响应格式规范：直接返回/无通用封装
- 定义禁用API清单：软删除/多租户/通用响应

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```


- [ ] **步骤3: 编写AGENTS.md项目顶层导航（简化版）**

创建`.harness/AGENTS.md`文件，包含核心架构说明（完整内容见设计文档）：

关键章节：
1. 项目简介 - mooc-manus定位与技术栈
2. 架构设计理念 - DDD三层模型、三态模型
3. 分层职责详解 - Repository/Domain/Application/Handler
4. 模块初始化流程 - InitRouter统一四层流程
5. 三态模型转换流程 - PO/DO/DTO转换
6. 依赖注入方式 - 手动构造
7. 业务域拆分规则 - tool/skill/agent/flow
8. 开发核心理念 - 简单直接、代码即文档

- [ ] **步骤4: 提交护栏核心文件**

```bash
git add .harness/AGENTS.md
git commit -m "docs: 添加AGENTS.md项目顶层导航文档

- 定义项目架构设计理念：DDD三层+三态模型
- 说明分层职责：Repository/Domain/Application/Handler
- 详解模块初始化流程：InitRouter四层统一
- 描述三态模型转换流程：PO/DO/DTO
- 明确依赖注入方式：手动构造
- 定义业务域拆分规则和开发理念

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### 任务4：编写知识库文件

**目标:** 编写ai-error-log.md纠错日志和conventions.md全量规范

**Files:**
- Modify: `.harness/knowledge/ai-error-log.md`
- Modify: `.harness/knowledge/conventions.md`

- [ ] **步骤1: 编写ai-error-log.md首条错误记录**

创建`.harness/knowledge/ai-error-log.md`，记录Skill模块架构割裂问题（参考设计文档3.5节完整内容）：

核心内容：
- 错误案例#001：模块初始化架构割裂
- 错误表现、错误代码片段
- 根本原因分析
- 正确做法、规避方案
- 影响范围、修复方案链接

- [ ] **步骤2: 编写conventions.md全量规范约定**

创建`.harness/knowledge/conventions.md`，整合项目全维度规范：

9大规范维度：
1. 命名规范 - 文件/包/结构体/接口/方法/字段
2. 目录结构规范 - 项目目录组织
3. 分层架构规范 - 四层职责/依赖方向
4. 模块初始化规范 - InitRouter统一流程
5. 路由注册规范 - 分组规则/路径命名
6. 接口开发规范 - 请求/响应/错误处理
7. 数据库规范 - 表设计/字段类型/事务
8. 代码重构规范 - 原则/风险控制
9. 注释文档规范 - 何时写/内容要求

- [ ] **步骤3: 提交知识库文件**

```bash
git add .harness/knowledge/
git commit -m "docs: 添加knowledge知识库文件

- ai-error-log.md：记录Skill模块架构割裂错误案例
- conventions.md：整合9大维度全量规范约定
- 采用只增不删原则，形成持续积累的纠错记忆
- 作为AI编码参考和规范查阅的权威来源

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### 任务5：验证完整性并最终提交

**目标:** 验证规范体系完整性，确认所有文件就绪

**Files:**
- Verify: `CLAUDE.md`
- Verify: `.harness/.cursorrules`
- Verify: `.harness/AGENTS.md`
- Verify: `.harness/knowledge/ai-error-log.md`
- Verify: `.harness/knowledge/conventions.md`
- Verify: `api/routers/route.go`

- [ ] **步骤1: 验证文件完整性**

检查清单：
```bash
# 验证文件存在
ls -la CLAUDE.md
ls -la .harness/.cursorrules
ls -la .harness/AGENTS.md
ls -la .harness/knowledge/ai-error-log.md
ls -la .harness/knowledge/conventions.md

# 验证route.go编译
go build -o mooc-manus main.go
```

预期：所有文件存在，编译成功

- [ ] **步骤2: 验证架构重构效果**

检查InitRouter函数：
- ✅ 四层分明：Repository → Domain → Application → Handler
- ✅ 每层有清晰注释分隔
- ✅ Skill模块已融入统一流程
- ✅ Agent模块已融入统一流程
- ✅ 依赖拓扑顺序正确

- [ ] **步骤3: 验证规范体系完整性**

检查规范文件：
- ✅ CLAUDE.md包含：语言约定/核心原则/文件说明/架构红线/工作流程
- ✅ .cursorrules包含：命名/分层/转换/初始化/错误/响应/禁用API
- ✅ AGENTS.md包含：架构/分层/初始化/转换/依赖/域拆分/理念
- ✅ ai-error-log.md包含：Skill架构割裂错误案例
- ✅ conventions.md包含：9大规范维度

- [ ] **步骤4: 功能验证**

启动项目并测试关键接口：
```bash
# 启动项目
./mooc-manus

# 测试Tool接口
curl http://localhost:8080/api/tools/provider/list

# 测试Skill接口
curl http://localhost:8080/api/v1/skill/listAll -X POST

# 测试Agent接口
curl http://localhost:8080/api/agent/... （根据实际接口调整）
```

预期：所有接口正常响应

- [ ] **步骤5: 创建最终提交标签**

```bash
git tag -a v1.0-architecture-unified -m "架构统一重构与规范体系建设完成

主要变更：
1. 统一Skill和Agent模块初始化架构，融入四层统一流程
2. 建立CLAUDE.md入口规范+.harness护栏层规范体系
3. 完成.cursorrules/AGENTS.md/ai-error-log.md/conventions.md编写
4. 确保所有模块初始化架构一致，代码规范统一

验收标准：
✅ 架构重构完成，四层分明
✅ 规范体系完整，文档齐全
✅ 编译通过，功能正常
✅ 代码简洁规范，无冗余

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"

git push origin v1.0-architecture-unified
```

---

## 验收清单

### 架构重构验收
- [ ] `api/routers/route.go`重构完成
- [ ] 严格四层分明：Repository → Domain → Application → Handler
- [ ] 所有模块融入统一初始化流程（无独立初始化）
- [ ] 每层内部按依赖拓扑顺序排列
- [ ] 编译通过，无语法错误
- [ ] 项目启动成功
- [ ] 所有接口功能正常（Skill、Agent、Tool、Flow、AppConfig）

### 规范体系验收
- [ ] 根目录`CLAUDE.md`文件创建，内容完整
- [ ] `.harness/`目录创建，结构符合设计
- [ ] `.harness/.cursorrules`文件创建，强制规则完整
- [ ] `.harness/AGENTS.md`文件创建，项目导航完整
- [ ] `.harness/knowledge/ai-error-log.md`创建，首条记录完整
- [ ] `.harness/knowledge/conventions.md`创建，全量规范完整
- [ ] 所有规范基于项目实际源码特征编写，无通用套话
- [ ] 规范逻辑清晰、层级分明、可长期约束AI编码行为

### 整体质量验收
- [ ] 重构代码简洁规范、无冗余、无语法错误
- [ ] 规范文档针对性强、真实可用、易于理解
- [ ] 架构统一性大幅提升
- [ ] 项目稳定性、规范性、可维护性显著改善
- [ ] 未来AI编码有明确约束边界和参考范式

---

## 风险控制

### 架构重构风险
**风险：** 调整初始化顺序可能破坏现有功能
**缓解：** 仅调整代码顺序，不改变依赖关系；编译+启动+接口测试全覆盖
**回滚：** 保留git备份提交，可快速回滚

### 规范文档风险
**风险：** 规范与实际代码不匹配
**缓解：** 基于实际源码特征定制化编写；逐条对照代码检查
**验证：** 模拟AI编码场景测试规范可用性

---

## 预计耗时

- 任务1（架构重构）：30-45分钟
- 任务2（入口规范）：20-30分钟
- 任务3（护栏核心）：30-40分钟
- 任务4（知识库）：40-50分钟
- 任务5（验证优化）：15-20分钟

**总计：** 约2-3小时

---

**计划版本：** v1.0  
**计划创建日期：** 2026-06-25  
**对应设计文档：** docs/superpowers/specs/2026-06-25-architecture-unification-design.md

