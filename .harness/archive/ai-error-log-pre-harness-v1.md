# AI编码纠错日志

> **本文档采用"只增不删"原则**：记录AI编码过程中出现的典型错误案例，每次AI编码出现典型错误时新增记录，形成持续积累的纠错记忆。

---

## 文档使用说明

### 何时阅读
- 编码前查阅：避免重复已知错误
- 重构前查阅：理解原有设计的演进
- 问题排查时：查看是否有类似错误记录

### 记录格式
每条错误记录包含以下要素：
- **时间** - 错误发生的日期
- **错误类型** - 错误分类（架构/命名/分层/规范等）
- **错误表现** - 错误的具体表现
- **错误代码片段** - 错误代码示例
- **根本原因** - 错误产生的根本原因分析
- **正确做法** - 应当采用的正确方法
- **规避方案** - 未来如何规避此类错误
- **影响范围** - 错误涉及的文件/模块
- **修复方案** - 修复链接或修复说明

---

## 错误案例 #001：模块初始化架构割裂

### 基本信息
- **时间：** 2026-06-25
- **错误类型：** 架构不统一 / 模块初始化规范违反
- **严重程度：** 高
- **影响范围：** `api/routers/route.go` InitRouter函数、Skill模块、Agent模块

### 错误表现

Skill模块在`api/routers/route.go`的`InitRouter`函数中，完全脱离全局四层初始化流程，独立完成repo/domain/application/handler四层初始化，导致：

1. **项目模块初始化架构不统一**
   - Tool/AppConfig模块：严格遵循统一的四层初始化流程
   - Skill模块：脱离全局流程，独立完成四层初始化

2. **Agent模块被迫采用独立初始化**
   - 因为Agent模块依赖Skill的Repository（skillRepo、skillVersionRepo、fs）
   - 必须等Skill完成全部初始化后才能开始，被迫脱离主流程

3. **代码可读性和可维护性大幅降低**
   - 同一函数内出现两种初始化范式
   - 新增模块无统一范式可遵循

4. **违背项目DDD分层架构设计理念**
   - 破坏了Repository→Domain→Application→Handler的严格分层

### 错误代码片段

```go
// 错误示范：Skill模块独立初始化（原route.go第78-108行）

// ============================================================
// Skill 模块（阶段 7）
// ============================================================

// 1) Repository
skillProviderRepo := repositories.NewSkillProviderRepository()
skillRepo := repositories.NewSkillRepository()
skillVersionRepo := repositories.NewSkillVersionRepository()
taskExecutionRepo := repositories.NewTaskExecutionRepository()

// 2) FileStorage
rootDir := "./data"
if config.Cfg != nil {
    rootDir = config.Cfg.Storage.RootDir
}
fs := file_storage.NewLocalFileStorage(rootDir)

// 3) Domain Service
skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo, skillRepo)
skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, skillProviderRepo, fs)
// ...

// 4) Application Service
skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
// ...

// 5) Handler
skillHandler := handlers.NewSkillHandler(...)

// ============================================================
// Agent 模块（依赖 Skill 模块的 skillRepo / skillVersionRepo / fs）
// ============================================================
baseAgentDomainSvc := agents.NewBaseAgentDomainService(..., skillRepo, skillVersionRepo, fs)
// ... Agent独立完成domain/application/handler初始化
```

### 根本原因

1. **AI编码时未优先读取项目架构规范**
   - 编码前未读取AGENTS.md，不了解项目整体架构
   - 未参考现有成熟模块（tool、appConfig）的初始化范式

2. **AI私自采用非标实现方式**
   - 忽视项目统一架构，采用"按模块独立初始化"的非标范式
   - 未识别跨模块依赖（Agent依赖Skill）应当通过依赖拓扑排序解决

3. **缺乏标准化AI编码约束体系**
   - 项目无统一规范文件可遵循
   - AI编码无明确的架构红线和禁止行为

### 正确做法

1. **编码前必读架构规范**
   - 读取`.harness/AGENTS.md`理解项目整体架构和初始化流程
   - 读取`.harness/.cursorrules`掌握强制编码规则

2. **参考现有成熟模块**
   - 对照tool、appConfig等模块的实现方式
   - 保持架构一致性

3. **严格遵循四层统一流程**
   ```go
   func InitRouter() *gin.Engine {
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
       // 第三层：Application Service 层
       // ============================================================

       // ============================================================
       // 第四层：Handler 层
       // ============================================================
   }
   ```

4. **处理跨模块依赖**
   - 在对应层级按依赖拓扑顺序排列（被依赖方在前）
   - Agent的Domain Service依赖Skill的Repository，应放在Skill Domain Service之后

### 规避方案

1. **强制读取规范**
   - 编码前必读：AGENTS.md + .cursorrules
   - 编码中参考：conventions.md + ai-error-log.md

2. **架构红线不可触碰**
   - 所有模块初始化必须在`InitRouter`函数内按四层顺序完成
   - 禁止在全局流程外独立完成任何模块的多层初始化
   - 跨模块依赖通过依赖拓扑排序解决

3. **新模块开发检查清单**
   - [ ] 编码前读取AGENTS.md + .cursorrules
   - [ ] 参考现有模块实现方式（如tool、appConfig）
   - [ ] 严格遵循四层统一初始化流程
   - [ ] 跨模块依赖按依赖拓扑顺序排列
   - [ ] 提交前对照规范自查

### 修复方案

详见设计文档：`docs/superpowers/specs/2026-06-25-architecture-unification-design.md`

修复要点：
1. 删除Skill模块独立初始化代码段（原78-108行）
2. 删除Agent模块独立初始化代码段（原111-129行）
3. 按四层架构重新组织所有模块的初始化
4. 添加清晰的层级和子注释分隔

---

**文档版本：** v1.0  
**最后更新：** 2026-06-25  
**维护原则：** 只增不删，持续积累
