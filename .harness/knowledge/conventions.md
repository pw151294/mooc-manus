# mooc-manus 项目全量规范约定

> 本文档整合项目全维度开发规范，作为AI编码、代码评审、重构优化的核心依据和单一权威规范来源。

---

## 一、命名规范

### 1.1 文件命名
- 规则：蛇形命名 + 单数形式
- 示例：`skill_provider.go`、`app_config.go`、`a2a_server_config.go`
- 禁止：驼峰命名、复数形式

### 1.2 包命名
- 规则：复数形式（领域目录下）
- 示例：`handlers`、`services`、`repositories`、`models`、`dtos`
- 特例：子模块包名使用业务名（`agents`、`flows`、`tools`、`sessions`）

### 1.3 结构体命名
- PO（持久对象）：`AppConfigPO`、`SkillProviderPO`、`SkillVersionPO`
- DO（领域对象）：`AppConfigDO`、`SkillDO`、`SkillVersionDO`
- DTO（传输对象）：`AppConfigDTO`、`SkillInfoDTO`
- Request：`AppConfigCreateRequest`、`SkillDraftSaveRequest`

### 1.4 接口与实现
- 接口：大驼峰 + Service/Repository
  - Service示例：`SkillApplicationService`、`SkillDomainService`
  - Repository示例：`SkillRepository`、`ToolProviderRepository`
- 实现：接口名 + Impl（仅Service/Repository层）
  - 示例：`SkillRepositoryImpl`、`SkillApplicationServiceImpl`
- Handler层：无接口，直接定义结构体（如`SkillHandler`、`ToolHandler`）

### 1.5 构造函数
- Service/Repository：`New{InterfaceName}(...) {InterfaceName}`，返回接口类型（实现类对外不暴露）
  - 示例：`func NewSkillRepository() SkillRepository`
  - 示例：`func NewSkillApplicationService(...) SkillApplicationService`
- Handler：`New{Type}(...) *{Type}`，返回指针实现
  - 示例：`func NewSkillHandler(...) *SkillHandler`

### 1.6 方法命名
- 公开方法：大驼峰，动词在前（`Create`、`GetById`、`DeleteById`）
- 私有方法：小驼峰（`getZapLevel`、`parseSkillFiles`）

### 1.7 转换函数
- PO↔DO：`ConvertXxxPO2DO`、`ConvertXxxDO2PO`
- DO↔DTO：`ConvertXxxRequest2DO`、`ConvertXxxDO2DTO`

### 1.8 字段命名
- JSON：小驼峰（`skillId`、`baseUrl`、`createdAt`）
- DB列：蛇形小写（`skill_id`、`base_url`、`created_at`）
- 时间字段：统一`CreatedAt`/`UpdatedAt`（DB：`created_at`/`updated_at`）
- **禁止**：`gmtCreate`、`gmtModified`

---

## 二、目录结构规范

### 2.1 顶层结构

```
mooc-manus/
├── api/                              # HTTP入口层
│   ├── handlers/                     #   Gin Handler
│   └── routers/                      #   路由注册 + 全量DI
├── config/                           # 配置加载（TOML）
├── docs/                             # 文档与SQL
├── internal/                         # 业务代码主目录
│   ├── applications/                 #   应用层
│   │   ├── dtos/                    #     DTO + Convert
│   │   └── services/                #     ApplicationService
│   ├── domains/                      #   领域层
│   │   ├── models/                  #     DO + 值对象
│   │   │   ├── agents/             #       智能体相关DO
│   │   │   ├── events/             #       事件相关DO
│   │   │   ├── file/               #       文件相关DO
│   │   │   ├── memory/             #       内存相关DO
│   │   │   └── prompts/            #       提示词相关DO
│   │   └── services/                #     DomainService
│   │       ├── agents/             #       智能体服务
│   │       ├── flows/              #       工作流服务
│   │       ├── sessions/           #       会话服务
│   │       └── tools/              #       工具服务
│   └── infra/                        #   基础设施层
│       ├── external/                #     外部协议
│       │   ├── file_storage/       #       文件存储
│       │   ├── health_checker/     #       健康检查
│       │   ├── llm/                #       LLM客户端
│       │   └── sse/                #       SSE封装
│       ├── models/                  #     PO
│       ├── repositories/            #     Repository
│       └── storage/                 #     DB客户端单例（postgres/redis）
├── pkg/                              # 通用工具
│   ├── logger/                      #   zap日志
│   ├── skillerr/                    #   哨兵错误
│   └── skillmd/                     #   Skill元数据解析
├── tests/                            # 公共mock
├── .harness/                         # AI编码规范护栏
│   ├── .cursorrules                 #   强制代码风格约束
│   ├── AGENTS.md                    #   项目顶层导航
│   └── knowledge/                   #   知识库
│       ├── ai-error-log.md         #     AI纠错日志
│       └── conventions.md          #     全量规范约定
└── main.go                           # 入口
```

### 2.2 文件归属规则
- PO定义：`internal/infra/models/`
- DO定义：`internal/domains/models/`
- DTO定义：`internal/applications/dtos/`
- Repository：`internal/infra/repositories/`
- Domain Service：`internal/domains/services/`
- Application Service：`internal/applications/services/`
- Handler：`api/handlers/`
- 路由注册：`api/routers/route.go`

---

## 三、分层架构规范

### 3.1 严格四层模型

```
Handler 层 (api/handlers/)
    ↓ 调用
Application Service 层 (internal/applications/services/)
    ↓ 调用
Domain Service 层 (internal/domains/services/)
    ↓ 调用
Repository 层 (internal/infra/repositories/)
```

### 3.2 各层职责

| 层级 | 职责 | 使用对象 | 调用下层 | 禁止 |
|------|------|---------|---------|------|
| Handler | HTTP入口，参数解析+响应 | Request/DTO | Application Service | 直接调用Domain/Repository |
| Application Service | DTO转换+编排 | DO | Domain Service | 直接接触PO/GORM |
| Domain Service | 核心业务规则 | DO | Repository（传入PO） | 直接操作GORM/接触DTO |
| Repository | 数据持久化 | PO | GORM | 业务逻辑/DO/DTO操作 |

### 3.3 依赖方向
- **单向依赖**：Handler → Application → Domain → Repository
- **跨模块协作**：通过Repository接口（如Agent依赖Skill Repository）
- **禁止**：循环依赖、反向依赖

---

## 四、模块初始化规范

### 4.1 强制规则
- 所有模块初始化**必须**在`api/routers/route.go`的`InitRouter`函数内
- **严格四层顺序**：Repository → Domain Service → Application Service → Handler
- **禁止**：脱离主流程独立初始化

### 4.2 依赖拓扑排序
- 同一层内，被依赖方在前
- Repository层：基础repo → Skill repo
- 基础设施层（与Repository同级）：FileStorage
- Domain Service层：基础domain → Skill domain → Agent domain（依赖Skill repo）

### 4.3 注释规范
```go
// ============================================================
// 第一层：Repository 层（按依赖拓扑顺序初始化）
// ============================================================
// 1.1 基础模块 Repository（无外部依赖）
// 1.2 Skill 模块 Repository（无外部依赖）
// 1.3 FileStorage（基础设施，与 Repository 同级别）
```

---

## 五、路由注册规范

### 5.1 注册位置
- 所有路由注册必须在`InitRouter`函数的路由注册段完成
- 路由注册段位于Handler层初始化之后

### 5.2 分组规则
- 使用`r.Group()`按业务域分组
- **路径前缀：统一使用 `/api/...`**（项目当前未启用版本号前缀）
- 示例：
  ```go
  skill := r.Group("/api/skill")
  {
      skill.POST("/draft/save", skillHandler.DraftSave)
      // ...
  }
  ```

### 5.3 路径命名
- 遵循RESTful风格
- 资源名称：复数（`skills`）或单数+动作（`skill/draft/save`）
- 路径使用小写+连字符或斜杠分隔

---

## 六、接口开发规范

### 6.1 请求格式
- JSON请求：使用`c.ShouldBindJSON(&req)`解析
- form-data：使用`c.Request.ParseMultipartForm(maxSize)`解析
- 字段校验：使用`binding:"required"`tag

### 6.2 响应格式
- 单对象：`c.JSON(http.StatusOK, dto)`
- 列表：`c.JSON(http.StatusOK, dtos)`其中`dtos []XxxDTO`
- 无返回值成功：`c.JSON(http.StatusOK, gin.H{"status": "success"})`
- 错误：`c.JSON(http.StatusXxx, gin.H{"error": err.Error()})`

### 6.3 错误处理
- Service层：返回标准error + 哨兵错误包装，使用`fmt.Errorf("...: %w", sentinelErr)`保留上下文
- Handler层：通过`writeError`统一映射HTTP状态码
- 哨兵错误：定义在`pkg/{xxx}err`独立包（如`pkg/skillerr`）

参考范式（来自 `api/handlers/skill.go`）：
```go
import "mooc-manus/pkg/skillerr"

// Service层包装哨兵错误，保留具体上下文
return nil, fmt.Errorf("skillFiles is required: %w", skillerr.ErrInvalidInput)

// Handler层通过errors.Is识别哨兵
case errors.Is(err, skillerr.ErrInvalidInput):
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
```

### 6.4 HTTP状态码映射
- `400 Bad Request`：参数解析失败、`ErrInvalidInput`
- `404 Not Found`：`ErrNotFound`
- `409 Conflict`：`ErrDuplicate`
- `500 Internal Server Error`：其他未分类错误

### 6.5 分页响应（Skill模块特例）
- 模块内定义XxxPageDTO（不抽离为通用类型）
- 标准字段：`total`、`pageSize`、`pageNum`、`records`

---

## 七、数据库规范

### 7.1 表设计
- 主键：`VARCHAR(36) PRIMARY KEY`（UUID）
- 时间字段：`created_at`/`updated_at`（`TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`）
- JSON字段：`JSONB`类型
- 字符串：`VARCHAR(N)`或`TEXT`

### 7.2 字段命名
- 列名：蛇形小写（`skill_id`、`created_at`）
- 外键：`xxx_id`格式
- 索引：`idx_{table}_{column}`

### 7.3 约束
- 外键：`ON DELETE CASCADE`（强关联）或`ON DELETE RESTRICT`（弱关联）
- 唯一性：`UNIQUE`约束或`UNIQUE INDEX`

### 7.4 事务处理
- 使用GORM原生：`client.Transaction(func(tx *gorm.DB) error { ... })`
- 事务体内显式使用`tx`而非外部`client`

### 7.5 禁用字段
- ❌ 软删除字段：`delete_flag`、`deleted_at`
- ❌ 多租户字段：`scope`、`subject_id`、`tenant_id`

### 7.6 SQL组织
- 单文件累积：`docs/sql/manus_schema.sql`
- 中文注释：`COMMENT ON TABLE` / `COMMENT ON COLUMN`

---

## 八、代码重构规范

### 8.1 重构原则
- **简单直接** - 避免过度设计
- **保留功能** - 重构不引入新功能
- **风险最小化** - 仅调整必要部分

### 8.2 重构风险控制
- 仅调整代码顺序时：风险低
- 改变依赖关系时：风险中
- 修改接口签名时：风险高，需充分测试

### 8.3 重构验证
- 编译验证：`go build`通过
- 启动验证：项目正常启动
- 功能验证：接口正常响应
- 测试验证：相关测试通过

### 8.4 重构前检查清单
- [ ] 理解原有设计意图
- [ ] 评估影响范围
- [ ] 确认测试覆盖
- [ ] 准备回滚方案

---

## 九、注释文档规范

### 9.1 何时写注释
- ✅ **应写**：非显而易见的业务规则、复杂算法、特殊约束、为何这样设计
- ✅ **应写**：公开接口的godoc（包名、函数签名说明）
- ❌ **不写**：显而易见的代码（如`// 创建用户`+`createUser()`）
- ❌ **不写**：实现细节复述

### 9.2 注释格式
- 单行：`// 注释内容`
- 多行：使用多个单行注释或`/* ... */`
- godoc：以函数名开头，描述功能

### 9.3 中文注释
- 项目统一使用中文注释
- 技术术语保持原文（如`Repository`、`Handler`）

### 9.4 TODO标记
- 格式：`// TODO(姓名): 待办事项`
- 使用场景：临时占位、待优化项
- 长期未处理的TODO需要在迭代中清理

---

## 十、日志使用规范

### 10.1 日志库
- 使用`pkg/logger`包（基于zap）
- **禁止**：`fmt.Println`、`log.Println`输出业务日志

### 10.2 日志级别
- `logger.Debug` - 调试信息
- `logger.Info` - 正常业务信息
- `logger.Warn` - 警告（业务异常但可继续）
- `logger.Error` - 错误（需要关注）

### 10.3 结构化字段
- 使用`zap.Field`构造结构化字段
- 便捷函数：`logger.String/Int/Int64/Bool/Any/ErrorField`
- 示例：`logger.Info("xxx", zap.String("key", value))`

---

## 十一、参考模块清单

### 11.1 最简范本
- **tool模块**：全链路四层均较薄，适合学习基础范式
  - `internal/infra/models/tool_provider.go`
  - `internal/domains/models/tool_provider.go`
  - `internal/applications/dtos/tool_provider.go`
  - `internal/infra/repositories/tool_provider.go`

### 11.2 完整业务参照
- **skill模块**：含分页、事务、外部存储、SSE等完整能力
  - 文件路径：见AGENTS.md业务域拆分表
  - 特例：`SkillPageDTO`分页范式

### 11.3 哨兵错误使用
- 包定义：`pkg/skillerr/errors.go`
- Handler应用：`api/handlers/skill.go`（`writeError`函数）

---

## 十二、关联文档

| 文档 | 作用 | 路径 |
|------|------|------|
| **CLAUDE.md** | AI编码入口规范 | `/CLAUDE.md` |
| **AGENTS.md** | 项目顶层导航 | `/.harness/AGENTS.md` |
| **.cursorrules** | 强制代码风格 | `/.harness/.cursorrules` |
| **ai-error-log.md** | AI纠错日志 | `/.harness/knowledge/ai-error-log.md` |
| **mooc-manus-code-standards.md** | 详细代码规范 | `/docs/mooc-manus-code-standards.md` |
| **mooc-manus-code-standards-supplement.md** | 规范补充材料 | `/docs/mooc-manus-code-standards-supplement.md` |

---

**文档版本：** v1.0  
**最后更新：** 2026-06-25  
**维护原则：** 与项目代码同步演进，作为单一权威规范来源
