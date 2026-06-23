# mooc-manus 项目代码规范调研报告

> 本报告用于指导 Skill 配置与版本管理模块迁移到 mooc-manus 的全部 7 个实施阶段。所有目录布局、命名风格、技术选型、冲突仲裁均以本报告为准；任何与本报告冲突的实施动作必须先回到本报告同步更新后再执行。

---

## 一、目的与范围

- **目的**：在迁移 Beedance Skill 模块前，沉淀一份与 mooc-manus 现有代码风格 100% 对齐的规范说明，使得新模块所有文件「看起来像原生代码」。
- **范围**：仅覆盖与 Skill 模块迁移直接相关的项目规范（目录、分层、命名、错误、响应、依赖注入、数据库、事务、日志、典型示例），并对 Skill 业务规范文档（`docs/skill-config-and-version-spec.md`）与 mooc-manus 现状之间的差异给出最终对齐方案。

---

## 二、mooc-manus 现状描述

### 2.1 目录结构树状图（3 层关键目录）

```
mooc-manus/
├── api/                                      // HTTP 入口层
│   ├── handlers/                             //   Gin Handler（注入 ApplicationService）
│   └── routers/                              //   路由注册 + 全量依赖注入
├── config/                                   // 配置加载（TOML）
│   ├── config.go                             //   GlobalConfig / Redis / Database / Logger 结构体
│   └── config.toml                           //   配置文件
├── docs/                                     // 文档与建表 SQL
│   └── sql/
│       └── manus_schema.sql                  //   累积建表（手写 SQL，无迁移工具）
├── internal/                                 // 业务代码主目录
│   ├── applications/                         //   应用层（编排、DTO 转换）
│   │   ├── dtos/                             //     请求 / 响应 DTO + ConvertXXX 函数
│   │   └── services/                         //     ApplicationService 接口 + 实现
│   ├── domains/                              //   领域层
│   │   ├── models/                           //     领域对象（DO）+ 值对象 + 子模块（agents/events/prompts/...）
│   │   └── services/                         //     DomainService 接口 + 实现 + 子模块（agents/flows/tools/sessions）
│   └── infra/                                //   基础设施层
│       ├── external/                         //     外部协议（sse、llm、health_checker）
│       ├── models/                           //     持久对象（PO，GORM Tag）
│       ├── repositories/                     //     Repository 接口 + 实现（GORM）
│       └── storage/                          //     Postgres / Redis 客户端单例
├── pkg/                                      // 项目内通用工具
│   └── logger/                               //   zap + lumberjack 全局日志
├── tests/                                    // 公共 mock
├── main.go                                   // 入口：init config → logger → redis → postgres → router
├── go.mod                                    // module: mooc-manus；Go 1.25
└── go.sum
```

> **关键约束**：三层模型隔离严格——`internal/infra/models`（PO）⇄`internal/domains/models`（DO）⇄`internal/applications/dtos`（DTO），转换函数命名固定为 `ConvertXxxPO2DO` / `ConvertXxxDO2PO` / `ConvertXxxRequest2DO` / `ConvertXxxDO2DTO`。

### 2.2 代码分层架构（DDD 三层 + DO/PO/DTO 三态）

```
┌─────────────────────────────────────────────────────────────────────┐
│ api/handlers/                                                        │
│   - 解析请求体 ShouldBindJSON / Param / Query                        │
│   - 调用 ApplicationService                                          │
│   - 直接 c.JSON 返回 DTO 或 gin.H{"status":"success"} / {"error":..} │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│ internal/applications/services/                                      │
│   - Application Service：编排 Domain Service + DTO 转换              │
│   - 不直接接触 GORM / PO                                              │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│ internal/domains/services/                                           │
│   - Domain Service：核心业务规则                                      │
│   - 调用 Repository，传入 PO，业务方法签名使用 DO                     │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│ internal/infra/repositories/                                         │
│   - GORM 数据访问，签名只接受 / 返回 PO                               │
│   - 事务通过 client.Transaction(func(tx *gorm.DB) error { ... })     │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│ internal/infra/storage/                                              │
│   - 全局单例：GetPostgresClient() *gorm.DB                            │
│   - GetRedisClient()                                                 │
└─────────────────────────────────────────────────────────────────────┘
```

**三态模型职责：**

| 模型 | 所在包 | 命名后缀 | GORM Tag | JSON Tag | 用途 |
|------|--------|---------|---------|---------|------|
| PO | `internal/infra/models` | `XxxPO` | ✅ | ✅（透传需要时） | 与数据库表 1:1 映射 |
| DO | `internal/domains/models` | `XxxDO` | ❌ | ❌ | 领域内流转，可含嵌套 / 值对象 |
| DTO | `internal/applications/dtos` | `XxxRequest` / `XxxDTO` | ❌ | ✅（含 `binding:"required"`） | HTTP 入参 / 出参 |

### 2.3 命名规范

| 维度 | 规则 | 示例 |
|------|------|------|
| 文件命名 | **蛇形 + 单数** | `tool_provider.go` / `app_config.go` / `a2a_server_config.go` |
| 包命名 | **复数**（领域目录下） | `handlers` / `services` / `repositories` / `models` / `dtos` |
| 子模块包名 | 业务名 | `agents` / `events` / `flows` / `tools` / `prompts` |
| 结构体 | 大驼峰 + 后缀 | `AppConfigPO` / `AppConfigDO` / `AppConfigDTO` / `AppConfigCreateRequest` |
| 接口 | 大驼峰 + `Service` / `Repository` | `AppConfigApplicationService` / `AppConfigDomainService` / `AppConfigRepository` |
| 实现类 | 接口名 + `Impl` | `AppConfigApplicationServiceImpl` |
| 构造函数 | `New` + 接口名（返回接口） | `NewAppConfigApplicationService(...) AppConfigApplicationService` |
| 方法（公开） | 大驼峰，动词在前 | `Create` / `Update` / `GetById` / `List` / `DeleteById` |
| 方法（私有） | 小驼峰 | `getZapLevel` / `parseSkillFiles` |
| 常量 | 蛇形小写 / 小驼峰，按文件聚合 | `defaultModelTemperature` / `postgresDsn` / `sseTimeout` |
| 转换函数 | `Convert{Src}2{Dst}` | `ConvertAppConfigDO2PO` / `ConvertAppConfigCreateRequest2DO` |
| 字段（JSON tag） | **小驼峰** | `appConfigId` / `baseUrl` / `modelName` |
| 字段（DB column） | **蛇形** | `app_config_id` / `base_url` / `model_name` |
| 时间字段 | `CreatedAt` / `UpdatedAt`（DB：`created_at` / `updated_at`） | `gorm:"column:created_at;autoCreateTime"` |

### 2.4 错误处理模式

mooc-manus 采用**最朴素的 error 直传**模式，不引入业务错误码或自定义 BizError：

```go
// Repository 层：直接返回 GORM 错误
return a.client.Where("id = ?", id).First(&po).Error

// Domain Service 层：原样透传或包装为新 error
if err != nil {
    return models.AppConfigDO{}, err
}

// Application Service 层：透传 + 必要时 DTO 转换错误
do, err := a.appConfigDomainSvc.GetById(id)
if err != nil {
    return dtos.AppConfigDTO{}, err
}

// Handler 层：分类返回 HTTP 状态码 + gin.H
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}
if err := h.appConfigAppSvc.UpdateAppConfig(req); err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}
c.JSON(http.StatusOK, gin.H{"status": "success"})
```

**Handler 层惯例：**
- 参数解析失败 → `400` + `gin.H{"error": err.Error()}`
- 业务执行失败 → `500` + `gin.H{"error": err.Error()}`
- 成功无返回值 → `200` + `gin.H{"status": "success"}`
- 成功有返回值 → `200` + DTO 对象（直接序列化）

### 2.5 响应封装模式

**项目当前无任何通用响应封装类（`Response[T]` / `SingleResponse` / `PageResponse`），全部直接 `c.JSON` 返回。**

- 单对象：`c.JSON(http.StatusOK, dto)`
- 列表：`c.JSON(http.StatusOK, []dtoList)`
- 无返回值的成功：`c.JSON(http.StatusOK, gin.H{"status": "success"})`
- 错误：`c.JSON(http.StatusXxx, gin.H{"error": err.Error()})`

### 2.6 依赖注入方式

**手动构造，集中在 `api/routers/route.go` 的 `InitRouter()` 内**，自上而下手动 new，不使用 Wire / Fx 等框架：

```go
func InitRouter() *gin.Engine {
    r := gin.Default()

    // 1) Repository
    appConfigRepo := repositories.NewAppConfigRepository()
    providerRepo := repositories.NewToolProviderRepository()
    functionRepo := repositories.NewToolFunctionRepository()

    // 2) Domain Service
    providerDomainSvc := domain_svc.NewToolProviderDomainService(providerRepo, functionRepo)
    functionDomainSvc := domain_svc.NewToolFunctionDomainService(functionRepo, providerRepo)
    appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)

    // 3) Application Service
    appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)

    // 4) Handler
    appConfigHandler := handlers.NewAppConfigHandler(appConfigAppSvc)

    // 5) 路由分组
    appConfig := r.Group("/api/app/config")
    {
        appConfig.GET("/:id", appConfigHandler.Get)
        ...
    }

    return r
}
```

> **约束**：构造函数命名固定 `New{Type}` → 返回**接口类型**，实现位于同包 `{Type}Impl`。

### 2.7 数据库迁移方式

**纯手写 SQL，单文件累积建表，无迁移工具：**

- 唯一脚本：`docs/sql/manus_schema.sql`
- 字段类型：PostgreSQL 原生类型（`VARCHAR(N)` / `TEXT` / `TIMESTAMPTZ` / `JSONB` / `INTEGER` / `BOOLEAN` / `DECIMAL`）
- 主键：`VARCHAR(36) PRIMARY KEY`（UUID）
- 时间字段：`TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`
- 外键：`CONSTRAINT fk_xxx FOREIGN KEY (...) REFERENCES ... ON DELETE CASCADE`
- 索引：`CREATE INDEX idx_{table}_{column} ON {table} ({column})`
- 注释：`COMMENT ON TABLE` / `COMMENT ON COLUMN`（中文注释）
- 字段命名：蛇形小写
- 无软删除字段，无多租户字段

### 2.8 事务处理模式

**使用 GORM 原生 `client.Transaction(func(tx *gorm.DB) error { ... })`，事务体内显式调用 `tx` 而非外部 client：**

```go
// 单 Repository 内的事务
func (a *AppConfigRepositoryImpl) DeleteById(id string) error {
    return a.client.Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("id = ?", id).Delete(&models.AppConfigPO{}).Error; err != nil {
            return err
        }
        // ... 关联表清理
        return nil
    })
}

// 跨 Repository 的事务（ToolFunctionRepository 提供 Transaction 方法）
type ToolFunctionRepository interface {
    ...
    Transaction(func(txFuncRepo ToolFunctionRepository, txProviderRepo ToolProviderRepository) error) error
}

func (t *ToolFunctionRepositoryImpl) Transaction(fn func(...) error) error {
    return t.dbCli.Transaction(func(tx *gorm.DB) error {
        txFuncRepo := &ToolFunctionRepositoryImpl{dbCli: tx}
        txProviderRepo := &ToolProviderRepositoryImpl{dbCli: tx}
        return fn(txFuncRepo, txProviderRepo)
    })
}
```

### 2.9 日志记录方式

封装在 `pkg/logger/loggers.go`，基于 `go.uber.org/zap` + `lumberjack`：

```go
import (
    "mooc-manus/pkg/logger"
    "go.uber.org/zap"
)

// 直接调用包级函数（已注入全局 logger）
logger.Info("SSE connection initialized", zap.String("messageId", messageId))
logger.Error("Failed to marshal event data", zap.Error(err))
logger.Warn("provider not active", zap.String("providerId", id))

// 便捷字段构造函数：
// logger.String(key, value) / logger.Int / logger.Int64 / logger.Bool / logger.Any / logger.ErrorField(err)
```

- 初始化：`main.go` 中 `logger.InitGlobalLogger(config.Cfg.LoggerConfig)`
- 配置项：level / format（json/console）/ output（stdout/file/both）/ log_dir / log_file / max_size / max_backups / max_age / compress / enable_caller
- 日志写入位置：`config.toml` 中 `logger.log_dir` + `log_file`

### 2.10 典型代码示例（完整切面）

> 以 `AppConfig` 模块为例，呈现从 HTTP → DTO → DO → PO 的完整链路，作为 Skill 模块所有实现的参照模板。

**Step 1 —— Handler（`api/handlers/app_config.go`）**

```go
package handlers

import (
    "mooc-manus/internal/applications/dtos"
    "mooc-manus/internal/applications/services"
    "net/http"

    "github.com/gin-gonic/gin"
)

type AppConfigHandler struct {
    appConfigAppSvc services.AppConfigApplicationService
}

func NewAppConfigHandler(appConfigAppSvc services.AppConfigApplicationService) *AppConfigHandler {
    return &AppConfigHandler{appConfigAppSvc: appConfigAppSvc}
}

func (h *AppConfigHandler) Add(c *gin.Context) {
    var req dtos.AppConfigCreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    id, err := h.appConfigAppSvc.CreateAppConfig(req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"id": id})
}
```

**Step 2 —— DTO + 转换函数（`internal/applications/dtos/app_config.go`）**

```go
package dtos

type AppConfigCreateRequest struct {
    BaseUrl     string  `json:"baseUrl" binding:"required"`
    ApiKey      string  `json:"apiKey" binding:"required"`
    ModelName   string  `json:"modelName" binding:"required"`
    Temperature float64 `json:"temperature"`
    MaxTokens   int64   `json:"maxTokens"`
    // ...
}

func ConvertAppConfigCreateRequest2DO(request AppConfigCreateRequest) models.AppConfigDO {
    if request.Temperature == 0 {
        request.Temperature = defaultModelTemperature
    }
    return models.AppConfigDO{
        AppConfigID: uuid.New().String(),
        ModelConfig: models.ModelConfig{ /* ... */ },
        AgentConfig: models.AgentConfig{ /* ... */ },
    }
}
```

**Step 3 —— Application Service（`internal/applications/services/app_config.go`）**

```go
package services

type AppConfigApplicationService interface {
    CreateAppConfig(dtos.AppConfigCreateRequest) (string, error)
    // ...
}

type AppConfigApplicationServiceImpl struct {
    appConfigDomainSvc services.AppConfigDomainService
}

func NewAppConfigApplicationService(appConfigDomainSvc services.AppConfigDomainService) AppConfigApplicationService {
    return &AppConfigApplicationServiceImpl{appConfigDomainSvc: appConfigDomainSvc}
}

func (a *AppConfigApplicationServiceImpl) CreateAppConfig(request dtos.AppConfigCreateRequest) (string, error) {
    do := dtos.ConvertAppConfigCreateRequest2DO(request)
    err := a.appConfigDomainSvc.Create(do)
    return do.AppConfigID, err
}
```

**Step 4 —— Domain Service（`internal/domains/services/app_config.go`）**

```go
package services

type AppConfigDomainService interface {
    Create(models.AppConfigDO) error
    // ...
}

type AppConfigDomainServiceImpl struct {
    appConfigRepo     repositories.AppConfigRepository
    functionDomainSvc ToolFunctionDomainService
}

func NewAppConfigDomainService(
    appConfigRepo repositories.AppConfigRepository,
    functionDomainSvc ToolFunctionDomainService) AppConfigDomainService {
    return &AppConfigDomainServiceImpl{
        appConfigRepo:     appConfigRepo,
        functionDomainSvc: functionDomainSvc,
    }
}

func (a *AppConfigDomainServiceImpl) Create(do models.AppConfigDO) error {
    po := models.ConvertAppConfigDO2PO(do)
    return a.appConfigRepo.Create(po)
}
```

**Step 5 —— DO + 值对象 + 转换函数（`internal/domains/models/app_config.go`）**

```go
package models

import infra "mooc-manus/internal/infra/models"

type AppConfigDO struct {
    ModelConfig      ModelConfig
    AgentConfig      AgentConfig
    A2AServerConfigs []A2AServerConfig
    AppConfigID      string
}

func ConvertAppConfigDO2PO(appConfigDO AppConfigDO) infra.AppConfigPO {
    return infra.AppConfigPO{
        ID:        appConfigDO.AppConfigID,
        BaseUrl:   appConfigDO.ModelConfig.BaseUrl,
        // ...
    }
}

func ConvertAppConfigPO2DO(appConfigPO infra.AppConfigPO) AppConfigDO {
    return AppConfigDO{ /* ... */ }
}
```

**Step 6 —— Repository + GORM（`internal/infra/repositories/app_config.go`）**

```go
package repositories

import (
    "mooc-manus/internal/infra/models"
    "mooc-manus/internal/infra/storage"
    "gorm.io/gorm"
)

type AppConfigRepository interface {
    Create(models.AppConfigPO) error
    // ...
}

type AppConfigRepositoryImpl struct {
    client *gorm.DB
}

func NewAppConfigRepository() AppConfigRepository {
    return &AppConfigRepositoryImpl{client: storage.GetPostgresClient()}
}

func (a *AppConfigRepositoryImpl) Create(po models.AppConfigPO) error {
    return a.client.Create(&po).Error
}

func (a *AppConfigRepositoryImpl) DeleteById(id string) error {
    return a.client.Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("id = ?", id).Delete(&models.AppConfigPO{}).Error; err != nil {
            return err
        }
        // 级联清理关联表
        return nil
    })
}
```

**Step 7 —— PO + GORM Tag（`internal/infra/models/app_config.go`）**

```go
package models

import "time"

type AppConfigPO struct {
    ID          string    `gorm:"type:varchar(36);primary_key" json:"id"`
    BaseUrl     string    `gorm:"type:varchar(255);not null" json:"baseUrl"`
    ApiKey      string    `gorm:"type:varchar(255);not null" json:"apiKey"`
    ModelName   string    `gorm:"type:varchar(100);not null" json:"modelName"`
    Temperature float64   `gorm:"type:decimal(3,2);not null" json:"temperature"`
    MaxTokens   int64     `gorm:"type:integer;not null" json:"maxTokens"`
    CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"createdAt"`
    UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updatedAt"`
}

func (AppConfigPO) TableName() string {
    return "app_config"
}
```

**Step 8 —— 在 `api/routers/route.go` 中注册（DI + 路由）**

```go
appConfigRepo := repositories.NewAppConfigRepository()
appConfigDomainSvc := domain_svc.NewAppConfigDomainService(appConfigRepo, functionDomainSvc)
appConfigAppSvc := app_svc.NewAppConfigApplicationService(appConfigDomainSvc)
appConfigHandler := handlers.NewAppConfigHandler(appConfigAppSvc)

appConfig := r.Group("/api/app/config")
{
    appConfig.GET("/:id", appConfigHandler.Get)
    appConfig.PUT("/:id", appConfigHandler.Update)
    appConfig.POST("", appConfigHandler.Add)
    appConfig.DELETE("/:id", appConfigHandler.Delete)
    appConfig.GET("", appConfigHandler.List)
}
```

---

## 三、与 Beedance Skill 业务规范的冲突识别与对齐决策

### 3.1 冲突总览表（最终方案）

| # | 冲突点 | Beedance 文档要求 | mooc-manus 现状 | **最终方案** |
|---|---|---|---|---|
| 1 | 主键 | `BIGINT` + 独立发号器（起始 100000） | `VARCHAR(36)` UUID | **维持 mooc-manus 现状**：UUID + `github.com/google/uuid` |
| 2 | 时间字段 | `gmt_create` / `gmt_modified` | `created_at` / `updated_at` | **维持 mooc-manus 现状**：`created_at` / `updated_at` |
| 3 | 软删除 | `delete_flag tinyint(1)` | 物理删除 | **维持 mooc-manus 现状**：物理删除（`Delete` 直接落地） |
| 4 | JSON 存储 | `TEXT` + `ext_info` 标准 Key | `JSONB`（如 `a2a_server_config.ext_info`） | **遵照折中方案**：列类型 `JSONB`，GORM `type:jsonb`，Go 字段 `string` 序列化 |
| 5 | 响应封装 | `SingleResponse` / `ListResponse` / `PageResponse`（含 success/code/message/traceId） | 直接 `c.JSON(http.StatusOK, dto)` | **维持 mooc-manus 现状**：单对象 / 列表 / 分页直接返回 |
| 6 | 错误码 | 17 个业务错误码字符串（`SKILL_NOT_FOUND` 等） | `gin.H{"error": err.Error()}` | **维持 mooc-manus 现状**：Service 层返回带文本的 `error`，Handler 直接透传 |
| 7 | 多租户 | `scope` + `subject_id` 全表隔离 | 无租户体系 | **维持 mooc-manus 现状**：不引入 scope/subject_id；唯一性约束改为全局唯一 |
| 8 | OSS 文件存储 | `beedance-skill` Bucket | 无 | **遵照推荐**：抽象 `FileStorage` 接口 + 本地实现（根 `./data/beedance-skill`） |
| 9 | 标签 | `lhs_source=SKILL, rhs_source=SYS_TAG` 关系表 | 无 | **维持 mooc-manus 现状**：删除接口中所有 tag 相关字段，不引入 TagRelationService |
| 10 | task_execution | 共用任务表（`app_id=SKILL_APP, app_type=SKILL_IMPORT`） | 无 | **遵照推荐**：新增 `task_execution` 表 + Repository，沿用文档语义 |
| 11 | SQL 组织 | — | 单文件累积 `docs/sql/manus_schema.sql` | **维持 mooc-manus 现状**：追加到 `manus_schema.sql`，不新建文件 |
| 12 | 文件命名 | — | 蛇形单数 `tool_provider.go` | **维持 mooc-manus 现状**：`skill.go` / `skill_provider.go` / `skill_version.go` / `task_execution.go` |

### 3.2 各冲突点详细说明与对接口的影响

#### 3.2.1 主键 → UUID

- **表定义**：`skill_id / skill_provider_id / skill_version_id / task_id` 全部 `VARCHAR(36) PRIMARY KEY`。
- **生成方式**：在 `Convert{Request}2DO` 或 `Convert{DO}2PO` 函数中调用 `uuid.New().String()`，与 `AppConfigDO` / `ToolProviderDO` 一致。
- **接口影响**：响应字段 `skillId / providerId / versionId` 类型由 `int64` 改为 `string`。
- **业务影响**：版本号生成（`vMAJOR.MINOR.PATCH`）与主键无关，按文档规则照常实施。

#### 3.2.2 时间字段 → created_at / updated_at

- **表定义**：`created_at` / `updated_at` 均为 `TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP`。
- **GORM Tag**：`gorm:"column:created_at;autoCreateTime"` / `gorm:"column:updated_at;autoUpdateTime"`。
- **接口影响**：响应字段统一改为 `createdAt` / `updatedAt`（文档中的 `gmtCreate` / `gmtModified` / `gmtArchive` 一律替换）。
- **`task_execution.gmt_archive`** 改为 `archived_at TIMESTAMPTZ`，对应字段名 `archivedAt`。

#### 3.2.3 软删除 → 物理删除

- 不增加 `delete_flag` 字段；删除接口（`POST /api/v1/skill/delete` / `/skill/provider/delete` / `/skill/version/delete`）直接物理删除。
- 文档「Provider 删除为软删除」改为：物理删除 Provider 记录 + **校验其下无 Skill**，有则拒绝（返回 `error` 文本 `provider has active skills`）。

#### 3.2.4 JSON 存储 → JSONB + Go string

- **PO 定义示例**：
  ```go
  ExtInfo  string `gorm:"column:ext_info;type:jsonb" json:"extInfo"`
  Metadata string `gorm:"column:metadata;type:jsonb" json:"metadata"`
  ```
- **业务读写**：Service 层使用 `encoding/json` 序列化 / 反序列化；DO 中以结构体方式持有（如 `SkillExtInfo struct { ZipFilePath string; SnapshotSkillName string; ... }`），转换函数完成 string ↔ struct 互转。
- **`ext_info` 标准 Key 沿用文档**：`zipFilePath / snapshotSkillName / snapshotIcon / snapshotImageUrl`（移除 `snapshotTagIds`，因不引入标签）。
- **Skill.ext_info 标准 Key**：`icon / imageUrl`。

#### 3.2.5 响应封装 → 直接 c.JSON

- 不引入 `SingleResponse[T]` / `PageResponse[T]`，不引入 `traceId` 字段。
- **单对象响应**：`c.JSON(http.StatusOK, skillInfoDTO)`。
- **列表响应**：`c.JSON(http.StatusOK, []SkillInfoDTO{})`。
- **分页响应（保留 4 字段）**：`c.JSON(http.StatusOK, gin.H{"total": total, "pageSize": ps, "pageNum": pn, "records": records})`，或封装为 `SkillPageDTO struct { Total int64; PageSize int; PageNum int; Records []SkillInfo }` 后直接返回（**推荐后者**，避免 gin.H 散落各处）。
- **空成功 / 删除成功**：`c.JSON(http.StatusOK, gin.H{"status": "success"})`。
- **文件下载 / SSE / 导出 ZIP**：保留 `application/octet-stream` / `text/event-stream` / `application/zip` 流式写入语义。

#### 3.2.6 错误码 → err.Error() 透传

- 不引入 `SKILL_NOT_FOUND` 等业务错误码字符串。
- Service 层使用 `errors.New("skill not found")` / `fmt.Errorf("skill name duplicate: %s", name)` 返回带文本的 error。
- Handler 层：参数错误 `400`、其他错误 `500`，统一 `gin.H{"error": err.Error()}`。
- **业务校验失败的文本约定**（前后端约定，便于调试，无强制结构）：
  - `skill not found` / `skill provider not found` / `skill version not found`
  - `skill.md not found in files` / `skill name is required` / `skill name too long`
  - `skill name duplicate: {name}` / `import file invalid` / `import task not found`

#### 3.2.7 多租户 → 移除 scope / subject_id

- **表定义**：所有 Skill 表去掉 `subject_id` / `scope` / `env` 字段。
- **唯一性约束**：
  - `skill.skill_name` **全局唯一**（`UNIQUE (skill_name)`）。
  - `skill_provider.provider_name` **全局唯一**（`UNIQUE (provider_name)`）。
  - `skill_version (skill_id, version)` **联合唯一**（`UNIQUE (skill_id, version)`）。
- **请求体**：移除 `appId / appVersion / conversationSource / tenantId / projectId / scope` 字段；DTO 不定义 BaseRequest。
- **查询**：Repository 的 `WHERE` 条件只剩业务字段；列表查询不再有「scope+subject_id 维度隔离」。

#### 3.2.8 OSS → FileStorage 抽象 + 本地实现

**接口定义（`internal/infra/external/storage/file_storage.go`）：**

```go
package storage

import "io"

type FileStorage interface {
    PutObject(bucket, key string, reader io.Reader, size int64, contentType string) (checksum string, err error)
    GetObject(bucket, key string) (io.ReadCloser, error)
    CopyObject(srcBucket, srcKey, dstBucket, dstKey string) error
    RemoveObjects(bucket string, keys []string) error
    Exists(bucket, key string) (bool, error)
    GetSize(bucket, key string) (int64, error)
}
```

**本地实现（`internal/infra/external/storage/local.go`）：**

- 根目录：`./data/`（可在 `config.toml` 中配置 `[storage] root_dir`）。
- Bucket 映射为子目录：`{root_dir}/{bucket}/{key}`。
- `PutObject` 写文件 + 计算 MD5 作为 `checksum`。
- `CopyObject` 使用 `io.Copy`。
- `RemoveObjects` 使用 `os.Remove`。
- 全部目录在写入时使用 `os.MkdirAll(filepath.Dir(p), 0755)` 自动创建。

**Skill 模块约定的 bucket / 路径**：

- Bucket 名常量：`beedance-skill`（与文档一致，便于未来切换 MinIO）。
- 文件路径：`{skillId}/{version}/{path}`
- ZIP 包路径（发布）：`{skillId}/{version}/skill-{skillId}-{version}.zip`
- ZIP 包路径（回滚）：`{skillId}/{version}/{skillId}-{version}.zip`

#### 3.2.9 标签 → 移除相关字段

- 请求体删除：`tagIds`。
- 响应体删除：`tags`、`categoryTags`。
- 列表查询过滤字段删除：`categoryTagId`。
- 版本快照 `ext_info` 删除：`snapshotTagIds`。
- 不定义 `TagRelationService` 接口；不引入任何标签相关表。

#### 3.2.10 task_execution → 新增表 + Repository

- 表名：`task_execution`，列与文档第 2.5 节一致，但作下列调整：
  - `task_id` → `VARCHAR(100) PRIMARY KEY`（保持文档语义）
  - 去掉 `scope` / `subject_id` / `env`
  - `gmt_create` → `created_at`；`gmt_modified` → `updated_at`；`gmt_archive` → `archived_at`
  - `ext_info` → `JSONB`
- 索引：仅保留 `idx_task_app_id (app_id)` / `idx_task_status (status)` / `idx_task_created_at (created_at)`。
- Repository（`internal/infra/repositories/task_execution.go`）提供：`Create / Update / GetById / ListByAppType / DeleteByIds / UpdateProgress`。
- Skill 模块固定写入 `app_id = "SKILL_APP"`、`app_type = "SKILL_IMPORT"`。

#### 3.2.11 SQL 组织 → 追加到 manus_schema.sql

- 直接在 `docs/sql/manus_schema.sql` 末尾追加 4 张表（`skill_provider` / `skill` / `skill_version` / `task_execution`）。
- 不新建 `skill_schema.sql`。
- 保留 `COMMENT ON TABLE` / `COMMENT ON COLUMN` 中文注释风格。

#### 3.2.12 文件命名 → 蛇形单数

- 所有 Skill 相关文件均为蛇形单数，与现有 `tool_provider.go / app_config.go` 一致。
- 不为 Skill 单独建子目录（如 `handlers/skill/`），所有文件平铺在原包内。

---

## 四、Skill 模块迁移落位指引

### 4.1 目录布局（按阶段）

```
阶段 1 (DDL)：
  docs/sql/manus_schema.sql
    ├ skill_provider 建表 + 索引 + 注释
    ├ skill           建表 + 索引 + 注释
    ├ skill_version   建表 + 索引 + 注释
    └ task_execution  建表 + 索引 + 注释

阶段 2 (Model)：
  internal/infra/models/
    ├ skill_provider.go      // SkillProviderPO + TableName()
    ├ skill.go               // SkillPO
    ├ skill_version.go       // SkillVersionPO
    └ task_execution.go      // TaskExecutionPO

  internal/domains/models/
    ├ skill_provider.go      // SkillProviderDO + Convert + 枚举（ProviderType/AuthType/Status）+ 领域行为
    ├ skill.go               // SkillDO + Convert + 枚举（SkillStatus）+ 领域行为（UpdateLatestVersion 等）
    ├ skill_version.go       // SkillVersionDO + Convert + SkillFile/SkillFileStructure/Icon/ImportLog 值对象 + 版本号工具
    └ task_execution.go      // TaskExecutionDO + Convert + 枚举（TaskStatus/Stage/LogLevel）+ 领域行为

阶段 3 (DTO)：
  internal/applications/dtos/
    ├ skill_provider.go      // 请求/响应 DTO + Convert
    ├ skill.go               // 请求/响应 DTO + Convert（SkillInfo / SkillPageDTO / SkillWithVersionInfo）
    ├ skill_version.go       // 请求/响应 DTO + Convert（SkillVersionInfo）
    └ skill_import_task.go   // 导入任务 DTO（SkillImportTaskInfo / SkillImportEventData）

阶段 4 (常量)：
  internal/applications/dtos/constants.go  // 追加 Skill 常量：bucket名、初始版本号(v0.1.0)、draft标识、路径模板等
  （不新增 pkg/errs，不新增错误码常量）

阶段 5 (Repository)：
  internal/infra/repositories/
    ├ skill_provider.go      // SkillProviderRepository 接口 + Impl
    ├ skill.go               // SkillRepository 接口 + Impl
    ├ skill_version.go       // SkillVersionRepository 接口 + Impl
    └ task_execution.go      // TaskExecutionRepository 接口 + Impl

阶段 6 (Service)：
  internal/domains/services/
    ├ skill_provider.go      // SkillProviderDomainService
    ├ skill.go               // SkillDomainService（含 DraftSave/Publish/Delete/List/Detail）
    ├ skill_version.go       // SkillVersionDomainService（含 Rollback/Export/Validate）
    └ skill_import_task.go   // SkillImportTaskDomainService（含 ImportFromZipAsync）

  internal/applications/services/
    ├ skill_provider.go      // SkillProviderApplicationService
    ├ skill.go               // SkillApplicationService
    ├ skill_version.go       // SkillVersionApplicationService
    └ skill_import_task.go   // SkillImportTaskApplicationService

阶段 7 (Handler + 路由 + FileStorage)：
  api/handlers/
    ├ skill.go               // SkillHandler（9 个接口）
    ├ skill_provider.go      // SkillProviderHandler（10 个接口）
    └ skill_version.go       // SkillVersionHandler（8 个接口，含 1 个 GET 下载）

  api/routers/route.go       // 追加 /api/v1/skill / /api/v1/skill/provider / /api/v1/skill/version 路由组

  internal/infra/external/storage/
    ├ file_storage.go        // FileStorage 接口
    └ local.go               // LocalFileStorage 实现
```

### 4.2 接口路径保留清单（按 Beedance 文档 §3）

| 路径 | 方法 | Handler |
|------|------|---------|
| `/api/v1/skill/draft/save` | POST | SkillHandler.DraftSave |
| `/api/v1/skill/publish` | POST | SkillHandler.Publish |
| `/api/v1/skill/update` | POST | SkillHandler.Update |
| `/api/v1/skill/delete` | POST | SkillHandler.Delete |
| `/api/v1/skill/list` | POST | SkillHandler.List |
| `/api/v1/skill/listAll` | POST | SkillHandler.ListAll |
| `/api/v1/skill/detail` | POST | SkillHandler.Detail |
| `/api/v1/skill/with/version` | POST | SkillHandler.WithVersion |
| `/api/v1/skill/file/download` | GET | SkillHandler.FileDownload |
| `/api/v1/skill/provider/import/git` | POST | SkillProviderHandler.ImportGit |
| `/api/v1/skill/provider/import/zip` | POST | SkillProviderHandler.ImportZip |
| `/api/v1/skill/provider/import/zip/legacy` | POST | SkillProviderHandler.ImportZipLegacy |
| `/api/v1/skill/provider/import/task/detail` | POST | SkillProviderHandler.ImportTaskDetail (SSE) |
| `/api/v1/skill/provider/import/task/list` | POST | SkillProviderHandler.ImportTaskList |
| `/api/v1/skill/provider/import/task/delete` | POST | SkillProviderHandler.ImportTaskDelete |
| `/api/v1/skill/provider/sync` | POST | SkillProviderHandler.Sync |
| `/api/v1/skill/provider/delete` | POST | SkillProviderHandler.Delete |
| `/api/v1/skill/provider/list` | POST | SkillProviderHandler.List |
| `/api/v1/skill/provider/detail` | POST | SkillProviderHandler.Detail |
| `/api/v1/skill/version/create` | POST | SkillVersionHandler.Create |
| `/api/v1/skill/version/validate` | POST | SkillVersionHandler.Validate |
| `/api/v1/skill/version/delete` | POST | SkillVersionHandler.Delete |
| `/api/v1/skill/version/list` | POST | SkillVersionHandler.List |
| `/api/v1/skill/version/detail` | POST | SkillVersionHandler.Detail |
| `/api/v1/skill/version/latest` | POST | SkillVersionHandler.Latest |
| `/api/v1/skill/version/rollback` | POST | SkillVersionHandler.Rollback |
| `/api/v1/skill/version/export` | POST | SkillVersionHandler.Export |

> 共 22 个 Handler 方法，与文档 §3 完全一致。

### 4.3 SSE 复用现有封装

- 直接使用 `internal/infra/external/sse/sse.go` 提供的 `EventHandleProtocol`。
- 任务订阅接口（`/api/v1/skill/provider/import/task/detail`）的 emitter 管理可以在 `SkillImportTaskDomainService` 内自行维护一个 `map[taskId]*sse.EventHandleProtocol`（参考 `sse/manager.go` 现有写法）。

### 4.4 用户决策对原 Beedance 接口的影响汇总

> 如果未来需要与 Beedance 平台对接同一前端，应注意以下接口契约差异：

| 维度 | 差异 |
|------|------|
| 响应外层 | 无 `success/code/message/traceId` 包裹，直接返回 data |
| 错误返回 | 无业务错误码字符串，仅 `{"error": "<message>"}` |
| 主键 | string（UUID）替代 int64 |
| 时间字段 | `createdAt/updatedAt` 替代 `gmtCreate/gmtModified` |
| 多租户 | 请求/响应不携带 scope/tenantId/projectId/subjectId |
| 标签 | 完全移除 tagIds/tags/categoryTags/categoryTagId 字段 |
| 软删除 | 删除接口为物理删除 |

---

## 五、验收标准

- 调研报告完成并由用户确认 ✅（本文件）
- 阶段 1-7 实施过程中，所有目录布局、命名风格、依赖注入方式、事务模式、日志调用方式必须与本报告的「现状描述」一致。
- 阶段 1-7 中遇到任何本报告未覆盖的风格冲突时，须先修订本报告后再施工。

---

## 附录 A：Skill 模块涉及的现有项目能力清单

| 能力 | 路径 | 使用方式 |
|------|------|---------|
| Postgres 客户端 | `internal/infra/storage/postgres.go` | `storage.GetPostgresClient()` |
| Redis 客户端 | `internal/infra/storage/redis.go` | （Skill 模块暂不使用） |
| SSE 封装 | `internal/infra/external/sse/sse.go` | `sse.NewEventHandleProtocol(w, timeout)` |
| zap 日志 | `pkg/logger/loggers.go` | `logger.Info(msg, zap.Field...)` |
| 配置加载 | `config/config.go` | `config.Cfg.Database` / `config.Cfg.LoggerConfig` |
| UUID 生成 | `github.com/google/uuid` | `uuid.New().String()` |

## 附录 B：本次新增的项目级能力

| 新增项 | 路径 | 用途 |
|--------|------|------|
| FileStorage 抽象 | `internal/infra/external/storage/file_storage.go` | OSS 抽象接口 |
| LocalFileStorage 实现 | `internal/infra/external/storage/local.go` | 文件系统实现，根目录 `./data/` |
| `task_execution` 表 | `docs/sql/manus_schema.sql` | 异步任务执行记录（共用） |
| TaskExecutionRepository | `internal/infra/repositories/task_execution.go` | 异步任务持久化 |
| config.toml `[storage]` 段 | `config/config.toml` + `config/config.go` | FileStorage 根目录配置 |

---

**报告版本**：v1.0  
**最终评审**：用户已确认（2026-06-23）
