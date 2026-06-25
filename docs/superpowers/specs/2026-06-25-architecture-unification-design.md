# mooc-manus 架构统一重构与Harness规范体系建设 - 设计文档

> **项目**: mooc-manus  
> **设计日期**: 2026-06-25  
> **设计目标**: 统一全项目模块初始化架构 + 建立标准化AI编码约束体系  
> **设计版本**: v1.0

---

## 一、项目背景与问题诊断

### 1.1 当前问题

**核心问题：模块初始化架构割裂**

当前项目`api/routers/route.go`的`InitRouter`函数存在严重的架构不统一问题：

1. **Tool/AppConfig模块**：严格遵循统一的四层初始化流程（repo → domain → application → handler）
2. **Skill模块**：完全脱离全局初始化流程，在主流程之外独立完成四层初始化
3. **Agent模块**：因依赖Skill的repo，被迫也采用独立初始化方式

**问题影响：**
- 项目模块初始化架构割裂，可读性差
- 新增模块无统一范式可遵循
- 代码可维护性和可扩展性降低
- 违背项目DDD分层架构设计理念

**根本原因：**
项目缺乏标准化AI编码约束体系，Claude Code编码前无统一规范可遵循，导致AI忽视现有架构范式，私自采用非标实现。

### 1.2 重构目标

**主目标1：架构重构**
- 统一Skill和Agent模块的初始化架构
- 将所有模块融入InitRouter的四层统一流程
- 保持严格分层：repo → domain → application → handler
- 每层内部按依赖拓扑顺序排列

**主目标2：规范体系建设**
- 建立Harness护栏层，约束AI编码行为
- 编写CLAUDE.md入口规范文件
- 编写.cursorrules代码风格强制约束
- 编写AGENTS.md项目顶层导航
- 建立AI纠错日志和全量规范约定

---

## 二、架构重构设计方案

### 2.1 重构方案选型

**对比了三种方案：**
1. **渐进式四层统一架构**（采用）- 保持单函数集中初始化，四层分明，每层内按依赖拓扑排序
2. 依赖注入容器化架构 - 引入容器管理，过度设计
3. 模块化分组初始化 - 拆分为多个函数，与现有风格不符

**选择方案一的原因：**
- 符合项目现状，保持"单函数集中初始化"风格
- 架构清晰，严格四层分明，易于理解维护
- 改动最小，仅调整代码顺序，风险可控
- 未来新增模块直接在对应层插入即可

### 2.2 重构后的架构蓝图

**InitRouter函数整体结构：**

```go
func InitRouter() *gin.Engine {
    r := gin.Default()

    // ============================================================
    // 第一层：Repository 层（按依赖拓扑顺序初始化）
    // ============================================================
    // 1.1 基础模块 Repository（无外部依赖）
    appConfigRepo := repositories.NewAppConfigRepository()
    providerRepo := repositories.NewToolProviderRepository()
    functionRepo := repositories.NewToolFunctionRepository()
    
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

    // ============================================================
    // 第二层：Domain Service 层（按依赖拓扑顺序初始化）
    // ============================================================
    // 2.1 基础模块 Domain Service（无跨模块依赖）
    providerDomainSvc := domain_svc.NewToolProviderDomainService(providerRepo, functionRepo)
    functionDomainSvc := domain_svc.NewToolFunctionDomainService(functionRepo, providerRepo)
    appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)
    
    // 2.2 Skill 模块 Domain Service（无跨模块依赖）
    skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo, skillRepo)
    skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, skillProviderRepo, fs)
    skillVersionDomainSvc := domain_svc.NewSkillVersionDomainService(skillVersionRepo, skillRepo, fs)
    skillImportTaskDomainSvc := domain_svc.NewSkillImportTaskDomainService(taskExecutionRepo, skillDomainSvc, skillProviderDomainSvc, fs)
    
    // 2.3 Agent 模块 Domain Service（依赖 Skill repo，放在 Skill Domain Service 之后）
    baseAgentDomainSvc := agents.NewBaseAgentDomainService(
        appConfigDomainSvc, providerDomainSvc, functionDomainSvc,
        skillRepo, skillVersionRepo, fs,
    )
    a2aDomainSvc := agents.NewA2ADomainService(baseAgentDomainSvc, appConfigDomainSvc, providerDomainSvc, functionDomainSvc)
    baseFlowDomainSvc := flows.NewBaseFlowDomainService(appConfigDomainSvc, providerDomainSvc, functionDomainSvc)

    // ============================================================
    // 第三层：Application Service 层（全模块并列）
    // ============================================================
    appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)
    providerAppSvc := app_svc.NewToolProviderApplicationService(providerDomainSvc)
    functionAppSvc := app_svc.NewTooLFunctionApplicationService(functionDomainSvc)
    
    skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
    skillVersionAppSvc := app_svc.NewSkillVersionApplicationService(skillVersionDomainSvc, skillDomainSvc)
    skillImportTaskAppSvc := app_svc.NewSkillImportTaskApplicationService(skillImportTaskDomainSvc)
    skillAppSvc := app_svc.NewSkillApplicationService(skillDomainSvc, skillVersionDomainSvc, skillProviderDomainSvc)
    
    baseAgentAppSvc := app_svc.NewBaseAgentApplicationService(baseAgentDomainSvc)
    a2aAppSvc := app_svc.NewA2AApplicationService(a2aDomainSvc)
    baseFlowAppSvc := app_svc.NewFlowApplicationService(baseFlowDomainSvc)

    // ============================================================
    // 第四层：Handler 层（全模块并列）
    // ============================================================
    toolHandler := handlers.NewToolHandler(providerAppSvc, functionAppSvc)
    skillHandler := handlers.NewSkillHandler(skillAppSvc, skillProviderAppSvc, skillVersionAppSvc, skillImportTaskAppSvc)
    agentHandler := handlers.NewAgentHandler(baseAgentAppSvc, a2aAppSvc)
    flowHandler := handlers.NewFlowHandler(baseFlowAppSvc)

    // ============================================================
    // 路由注册
    // ============================================================
    // ... 所有路由注册逻辑
    
    return r
}
```

### 2.3 架构设计关键决策

**决策1：跨模块依赖处理**
- Agent的BaseAgentDomainService依赖Skill的Repository（skillRepo、skillVersionRepo）
- 这看似打破"Domain只依赖同层或下层"的原则
- **判断：合理** - 跨模块通过Repository接口读取数据是DDD可接受模式
- **约束：依赖方向必须单向**（Agent → Skill，不能反向）

**决策2：FileStorage的层级归属**
- FileStorage是基础设施，与Repository处于同一抽象层级
- 放在Repository层初始化，供Domain Service使用
- 符合依赖倒置原则（Domain依赖抽象接口）

**决策3：依赖拓扑排序规则**
- 同一层内，被依赖方必须在依赖方之前初始化
- Repository层：基础repo → Skill repo → FileStorage
- Domain Service层：基础domain → Skill domain → Agent domain（依赖Skill repo）
- Application/Handler层：无严格顺序要求，按业务模块分组即可

**决策4：注释规范**
- 每层开头用分隔注释标识层级
- 每层内用子注释标识模块分组
- 保持视觉清晰，便于快速定位

### 2.4 重构影响范围

**需要修改的文件：**
- `api/routers/route.go` - 唯一需要修改的代码文件

**修改内容：**
1. 删除Skill模块独立初始化的代码段（第78-108行）
2. 删除Agent模块独立初始化的代码段（第111-129行）
3. 按新架构蓝图重新组织InitRouter函数
4. 保持所有依赖关系不变，仅调整初始化顺序
5. 保持路由注册逻辑不变

**不需要修改的文件：**
- 所有Repository/Domain/Application/Handler实现文件
- 所有Model/DTO定义文件
- 所有业务逻辑代码

**风险评估：**
- **风险等级：低** - 仅调整代码顺序，不改变依赖关系
- **验证方式：** 编译通过 + 启动成功 + 接口功能正常

---

## 三、Harness规范体系设计方案

### 3.1 规范体系架构

基于**工程护栏（Harness）**思想，建立三层规范体系：

```
项目根目录/
├── CLAUDE.md                          # 第一层：入口规范（CC每轮对话必读）
└── .harness/                          # 第二层：护栏层目录
    ├── .cursorrules                   # 强制代码风格约束
    ├── AGENTS.md                      # 项目顶层导航文件
    └── knowledge/                     # 第三层：知识库
        ├── ai-error-log.md           # AI纠错日志（只增不删）
        └── conventions.md            # 项目全量规范约定
```

**设计原则：**
1. **分层递进** - 入口规范 → 护栏约束 → 知识库详解
2. **强制优先** - CLAUDE.md明确护栏层优先级，禁止突破
3. **持续积累** - ai-error-log只增不删，形成纠错记忆
4. **贴合实际** - 所有规范基于项目源码特征定制化编写

### 3.2 CLAUDE.md 入口规范文件设计

**定位：** Claude Code每轮对话的首要读取文件

**核心内容：**
1. 输出语言约定 - 固定中文
2. 核心原则 - 护栏层优先，禁止突破约束
3. .harness文件说明 - 各文件作用、读取优先级、使用场景
4. 架构红线 - 分层架构不可违反、模块初始化规则、路由注册规则
5. 工作流程 - 编码前必读流程、编码中参考流程

### 3.3 .cursorrules 代码风格约束设计

**定位：** 强制性编码规则，解决代码风格不统一问题

**核心约束：**

**1. 命名规范**
- 文件：蛇形单数（`skill_provider.go`）
- 包：复数（`handlers`、`services`、`repositories`）
- 结构体：大驼峰+后缀（`SkillProviderPO`、`SkillDO`、`SkillDTO`）
- 接口：大驼峰+Service/Repository
- 方法：公开大驼峰动词在前、私有小驼峰
- JSON字段：小驼峰（`skillId`、`createdAt`）
- DB列：蛇形（`skill_id`、`created_at`）

**2. 分层编码规则**
- Repository：只接受/返回PO，GORM操作，无业务逻辑
- Domain Service：使用DO，核心业务规则
- Application Service：DTO转换+编排Domain Service
- Handler：参数解析+调用Application Service+返回响应

**3. 三态模型转换**
- PO↔DO：`ConvertXxxPO2DO`、`ConvertXxxDO2PO`
- DO↔DTO：`ConvertXxxRequest2DO`、`ConvertXxxDO2DTO`
- 转换函数放在对应model/dto包内

**4. 初始化范式**
- 所有模块初始化必须在`InitRouter`函数内
- 严格四层顺序：repo → domain → application → handler
- 禁止脱离主流程独立初始化

**5. 错误处理**
- 不引入业务错误码字符串
- Service层使用哨兵错误（`skillerr.ErrNotFound`）
- Handler层通过`errors.Is`映射HTTP状态码

**6. 时间字段**
- 统一：`createdAt`、`updatedAt`
- 禁止：`gmtCreate`、`gmtModified`

**7. 禁用API**
- 禁止软删除字段（`delete_flag`）
- 禁止多租户字段（`scope`、`subject_id`）
- 禁止通用响应封装（`Response[T]`）

### 3.4 AGENTS.md 项目顶层导航设计

**定位：** AI编码前必读的项目架构总览

**核心内容：**

**1. 项目简介**
- mooc-manus定位：智能体编排与工具管理平台
- 技术栈：Go 1.25 + Gin + GORM + PostgreSQL + Redis

**2. 架构设计理念**
- DDD三层模型：Handler → Application → Domain → Repository
- 三态模型：PO（数据库） / DO（领域） / DTO（传输）
- 依赖注入：手动构造，单函数集中初始化

**3. 分层职责详解**
- **Repository层**：数据持久化，GORM操作，签名只接受/返回PO
- **Domain Service层**：核心业务规则，使用DO，包含领域逻辑
- **Application Service层**：DTO转换，编排Domain Service，不直接接触PO
- **Handler层**：HTTP入口，参数解析，调用Application Service，返回响应

**4. 模块初始化流程**
- 统一在`InitRouter`函数中完成
- 严格四层顺序：Repository → Domain Service → Application Service → Handler
- 每层内部按依赖拓扑顺序排列

**5. 三态模型转换流程**
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

**6. 依赖注入方式**
- 手动构造，不使用Wire/Fx等框架
- 所有依赖在`InitRouter`函数中集中管理
- 构造函数命名：`New{Type}` → 返回接口类型

**7. 业务域拆分规则**
- 按业务能力划分：tool、skill、agent、flow、appConfig
- 每个域包含完整四层实现
- 跨域协作通过Repository接口

**8. 开发核心理念**
- 简单直接，无过度设计
- 代码即文档，注释补充非显而易见逻辑
- 错误直传，不引入复杂错误码体系

### 3.5 knowledge/ai-error-log.md 纠错日志设计

**定位：** 记录AI编码典型错误，避免重复犯错。**只增不删**原则。

**首条记录：Skill模块架构割裂问题**

```markdown
## 错误案例 #001：模块初始化架构割裂

**时间：** 2026-06-25
**错误类型：** 架构不统一

**错误表现：**
Skill模块在`api/routers/route.go`的`InitRouter`函数中，完全脱离全局四层初始化流程，独立完成repo/domain/application/handler四层初始化，导致：
1. 项目模块初始化架构不统一（Tool模块统一、Skill模块独立）
2. Agent模块因依赖Skill的repo，被迫也采用独立初始化
3. 代码可读性和可维护性大幅降低
4. 新增模块无统一范式可遵循

**错误代码片段：**
```go
// 错误示范：Skill模块独立初始化（第78-108行）
// ============================================================
// Skill 模块（阶段 7）
// ============================================================
skillProviderRepo := repositories.NewSkillProviderRepository()
skillRepo := repositories.NewSkillRepository()
// ... 完整四层初始化
skillHandler := handlers.NewSkillHandler(...)

// ============================================================
// Agent 模块（依赖 Skill 模块的 skillRepo / skillVersionRepo / fs）
// ============================================================
baseAgentDomainSvc := agents.NewBaseAgentDomainService(..., skillRepo, skillVersionRepo, fs)
// ... Agent独立初始化
```

**根本原因：**
1. AI编码时未优先读取项目架构规范（AGENTS.md）
2. 未参考现有成熟模块（tool、appConfig）的初始化范式
3. 私自采用非标实现方式，忽视项目统一架构

**正确做法：**
1. **编码前必读** AGENTS.md，理解项目整体架构和初始化流程
2. **参考现有模块** 的实现方式，保持架构一致性
3. **严格遵循四层统一流程**：
   - Repository层：所有模块repo初始化
   - Domain Service层：所有模块domain初始化
   - Application Service层：所有模块application初始化
   - Handler层：所有模块handler初始化
4. **处理跨模块依赖**：在对应层级按依赖拓扑顺序排列

**规避方案：**
- 所有模块初始化必须在`InitRouter`函数内按四层顺序完成
- 禁止在全局流程外独立完成任何模块的多层初始化
- 有跨模块依赖时，在对应层级按依赖拓扑顺序排列（被依赖方在前）
- 新增模块前，先读AGENTS.md + .cursorrules，再参考现有模块实现

**影响范围：**
- `api/routers/route.go` InitRouter函数
- Skill模块和Agent模块的初始化逻辑

**修复方案：**
按渐进式四层统一架构重构，详见设计文档2026-06-25-architecture-unification-design.md
```

### 3.6 knowledge/conventions.md 全量规范设计

**定位：** 项目全维度开发规范汇总，作为编码、评审、重构的核心依据

**核心内容：**
1. **命名规范** - 文件/包/结构体/接口/方法/字段的命名规则
2. **目录结构规范** - 项目目录组织、文件归属规则
3. **分层架构规范** - 四层职责、依赖方向、禁止行为
4. **模块初始化规范** - InitRouter统一流程、依赖拓扑排序
5. **路由注册规范** - 路由分组规则、路径命名、HTTP方法选择
6. **接口开发规范** - 请求/响应格式、错误处理、状态码规则
7. **数据库规范** - 表设计、字段类型、索引命名、事务处理
8. **代码重构规范** - 重构原则、风险控制、验证标准
9. **注释文档规范** - 何时写注释、注释内容要求

---

## 四、实施计划

### 4.1 实施阶段划分

**阶段一：架构重构（优先级最高）**
- 任务：重构`api/routers/route.go`的`InitRouter`函数
- 输出：统一四层初始化架构的route.go文件
- 验证：编译通过 + 启动成功 + 接口功能正常

**阶段二：根目录规范文件**
- 任务1：编写`CLAUDE.md`入口规范文件
- 任务2：创建`.harness`目录
- 输出：CLAUDE.md + .harness/目录结构

**阶段三：护栏层核心文件**
- 任务1：编写`.harness/.cursorrules`代码风格约束
- 任务2：编写`.harness/AGENTS.md`项目导航
- 输出：.cursorrules + AGENTS.md

**阶段四：知识库文件**
- 任务1：创建`.harness/knowledge`目录
- 任务2：编写`ai-error-log.md`纠错日志（含Skill架构问题）
- 任务3：编写`conventions.md`全量规范
- 输出：完整知识库体系

**阶段五：验证与优化**
- 任务：验证规范体系完整性、可用性
- 输出：验证报告、优化建议

### 4.2 实施顺序说明

**为什么先重构代码？**
1. 架构重构解决当前最紧迫的问题（代码架构混乱）
2. 重构后的代码作为规范文档的最佳范例
3. 边重构边总结，规范文档更贴合实际

**为什么分阶段编写规范？**
1. 入口规范（CLAUDE.md）先行，建立约束边界
2. 护栏层核心文件次之，提供强制约束和导航
3. 知识库最后，积累详细规范和错误案例

### 4.3 实施风险控制

**风险点1：架构重构破坏现有功能**
- 缓解措施：仅调整代码顺序，不改变依赖关系
- 验证手段：编译+启动+接口测试

**风险点2：规范文档与实际代码不匹配**
- 缓解措施：基于实际源码特征定制化编写
- 验证手段：对照代码逐条检查规范正确性

**风险点3：规范过于冗长，AI难以遵循**
- 缓解措施：分层递进，核心规范优先，详细规范按需查阅
- 验证手段：模拟AI编码场景，测试规范可用性

---

## 五、验收标准

### 5.1 架构重构验收

✅ `api/routers/route.go`重构完成
✅ 严格四层分明：Repository → Domain Service → Application Service → Handler
✅ 所有模块融入统一初始化流程（无独立初始化）
✅ 每层内部按依赖拓扑顺序排列
✅ 编译通过，无语法错误
✅ 项目启动成功
✅ 所有接口功能正常（Skill、Agent、Tool、Flow、AppConfig）

### 5.2 规范体系验收

✅ 根目录`CLAUDE.md`文件创建，内容完整
✅ `.harness/`目录创建，结构符合设计
✅ `.harness/.cursorrules`文件创建，强制规则完整
✅ `.harness/AGENTS.md`文件创建，项目导航完整
✅ `.harness/knowledge/ai-error-log.md`创建，首条记录完整
✅ `.harness/knowledge/conventions.md`创建，全量规范完整
✅ 所有规范基于项目实际源码特征编写，无通用套话
✅ 规范逻辑清晰、层级分明、可长期约束AI编码行为

### 5.3 整体质量验收

✅ 重构代码简洁规范、无冗余、无语法错误
✅ 规范文档针对性强、真实可用、易于理解
✅ 架构统一性大幅提升
✅ 项目稳定性、规范性、可维护性显著改善
✅ 未来AI编码有明确约束边界和参考范式

---

## 六、后续维护建议

### 6.1 规范文档维护

**CLAUDE.md 和 AGENTS.md**
- 架构调整时同步更新
- 保持与实际代码一致

**.cursorrules**
- 发现新的编码规范违规时补充规则
- 定期审查，删除已过时规则

**ai-error-log.md**
- 只增不删原则
- 每次AI编码出现典型错误时新增记录
- 定期回顾，总结高频错误模式

**conventions.md**
- 项目规范演进时同步更新
- 作为单一权威规范来源

### 6.2 架构持续优化

**定期审查InitRouter函数**
- 检查是否有新模块未遵循四层统一流程
- 确认依赖拓扑顺序正确性

**新模块开发检查清单**
- [ ] 编码前读取AGENTS.md + .cursorrules
- [ ] 参考现有模块实现方式
- [ ] 严格遵循四层统一初始化流程
- [ ] 提交前对照规范自查

---

**设计文档版本：** v1.0  
**设计完成日期：** 2026-06-25  
**下一步：** 规范文档审查 → 用户审查 → 编写实施计划
