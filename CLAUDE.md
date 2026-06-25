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
