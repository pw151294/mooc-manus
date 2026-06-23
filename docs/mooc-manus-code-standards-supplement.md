# mooc-manus 代码规范调研报告 - 补充材料

> 本文档包含主报告审查后需要补充的施工细节（DDL 草案、常量清单、路由骨架、Handler 单文件封装规则），对应审查意见中的 major 级别问题 #6/#7/#8 的修复。施工时请配合主报告 `mooc-manus-code-standards.md` 使用。

---

## 补充 1：阶段 1 完整 DDL 草案（约 150 行）

**用途**：直接追加到 `docs/sql/manus_schema.sql` 末尾，或作为阶段 1 施工的蓝图。

**说明**：以下 4 张表已按照主报告 §3.2 的 12 项对齐决策完成裁剪：
- 主键全部 `VARCHAR(36)` UUID
- 时间字段 `created_at` / `updated_at`
- 无 `scope` / `subject_id` / `delete_flag`
- `ext_info` 类型 `JSONB`
- 外键 `ON DELETE RESTRICT`（skill_provider）/ `ON DELETE CASCADE`（skill_version）

```sql
-- ========================================
-- 1. skill_provider 表
-- ========================================
CREATE TABLE skill_provider
(
    skill_provider_id VARCHAR(36) PRIMARY KEY,
    provider_name     VARCHAR(128) NOT NULL UNIQUE,
    provider_type     VARCHAR(32)  NOT NULL,       -- GIT / ZIP / CUSTOM
    auth_type         VARCHAR(32),                 -- HTTP_TOKEN / NONE
    repo_url          VARCHAR(512),
    status            VARCHAR(32)  NOT NULL DEFAULT 'ACTIVE',  -- ACTIVE / DISABLED
    creator           VARCHAR(64),
    updator           VARCHAR(64),
    ext_info          JSONB,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_skill_provider_status ON skill_provider (status);
CREATE INDEX idx_skill_provider_created_at ON skill_provider (created_at);

COMMENT ON TABLE skill_provider IS 'Skill 提供者表';
COMMENT ON COLUMN skill_provider.skill_provider_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill_provider.provider_name IS '提供者名称（全局唯一）';
COMMENT ON COLUMN skill_provider.provider_type IS '提供者类型：GIT / ZIP / CUSTOM';
COMMENT ON COLUMN skill_provider.auth_type IS '认证类型：HTTP_TOKEN / NONE';
COMMENT ON COLUMN skill_provider.repo_url IS 'Git 仓库地址（provider_type=GIT 时必填）';
COMMENT ON COLUMN skill_provider.status IS '状态：ACTIVE / DISABLED';
COMMENT ON COLUMN skill_provider.ext_info IS '扩展信息（JSONB）';

-- ========================================
-- 2. skill 表
-- ========================================
CREATE TABLE skill
(
    skill_id          VARCHAR(36) PRIMARY KEY,
    skill_name        VARCHAR(120) NOT NULL UNIQUE,
    skill_provider_id VARCHAR(36)  NOT NULL REFERENCES skill_provider (skill_provider_id) ON DELETE RESTRICT,
    description       VARCHAR(3000),
    latest_version_id VARCHAR(36),                 -- 最新已发布版本（NULL 表示无已发布版本）
    status            VARCHAR(32)  NOT NULL DEFAULT 'ACTIVE',  -- ACTIVE / DISABLED
    creator           VARCHAR(64),
    updator           VARCHAR(64),
    ext_info          JSONB,                      -- 存 icon / imageUrl
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_skill_provider_id ON skill (skill_provider_id);
CREATE INDEX idx_skill_status ON skill (status);
CREATE INDEX idx_skill_created_at ON skill (created_at);

COMMENT ON TABLE skill IS 'Skill 配置表';
COMMENT ON COLUMN skill.skill_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill.skill_name IS 'Skill 名称（全局唯一）';
COMMENT ON COLUMN skill.skill_provider_id IS '所属 Provider（外键，删除 Provider 时若有 Skill 会拒绝）';
COMMENT ON COLUMN skill.latest_version_id IS '最新已发布版本 ID（指向 skill_version.skill_version_id）';
COMMENT ON COLUMN skill.ext_info IS '扩展信息（icon / imageUrl 的 JSON）';

-- ========================================
-- 3. skill_version 表
-- ========================================
CREATE TABLE skill_version
(
    skill_version_id VARCHAR(36) PRIMARY KEY,
    skill_id         VARCHAR(36)  NOT NULL REFERENCES skill (skill_id) ON DELETE CASCADE,
    version          VARCHAR(32)  NOT NULL,        -- 'draft' 或 'vMAJOR.MINOR.PATCH'
    description      VARCHAR(3000),
    metadata         JSONB,                        -- SKILL.md 解析后的 JSON
    skill_files      JSONB,                        -- SkillFile[] 数组（文件名 / 大小 / 校验和 / OSS Key）
    ext_info         JSONB,                        -- 存 zipFilePath / snapshotSkillName / snapshotIcon / snapshotImageUrl
    creator          VARCHAR(64),
    updator          VARCHAR(64),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (skill_id, version)
);

CREATE INDEX idx_skill_version_skill_id ON skill_version (skill_id);
CREATE INDEX idx_skill_version_created_at ON skill_version (created_at);

COMMENT ON TABLE skill_version IS 'Skill 版本表';
COMMENT ON COLUMN skill_version.skill_version_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill_version.skill_id IS '所属 Skill（外键，删除 Skill 时级联删除版本）';
COMMENT ON COLUMN skill_version.version IS '版本号（draft 或 vX.Y.Z，联合唯一）';
COMMENT ON COLUMN skill_version.metadata IS 'SKILL.md 解析后的 JSON（可扩展 LLM 元数据）';
COMMENT ON COLUMN skill_version.skill_files IS '版本文件列表（JSONB 数组，含文件名/大小/校验和/OSS Key）';
COMMENT ON COLUMN skill_version.ext_info IS '扩展信息（zipFilePath / 快照字段）';

-- ========================================
-- 4. task_execution 表（共用异步任务表）
-- ========================================
CREATE TABLE task_execution
(
    task_id    VARCHAR(100) PRIMARY KEY,
    app_id     VARCHAR(64) NOT NULL,           -- 'SKILL_APP'（Skill 模块固定值）
    app_type   VARCHAR(64) NOT NULL,           -- 'SKILL_IMPORT'（任务类型）
    status     VARCHAR(32) NOT NULL DEFAULT 'PROCESSING',  -- PROCESSING / COMPLETED / FAILED
    stage      VARCHAR(32),                     -- UPLOAD / EXTRACT / VALIDATE / REGISTER / COMPLETED
    progress   INT         NOT NULL DEFAULT 0,  -- 0-100
    ext_info   JSONB,                          -- logs / skillCount / providerId / errorMessage
    creator    VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at TIMESTAMPTZ                      -- 任务归档时间（完成 7 天后可归档）
);

CREATE INDEX idx_task_app_id ON task_execution (app_id);
CREATE INDEX idx_task_status ON task_execution (status);
CREATE INDEX idx_task_created_at ON task_execution (created_at);

COMMENT ON TABLE task_execution IS '异步任务执行记录表（跨模块共用）';
COMMENT ON COLUMN task_execution.task_id IS '任务 ID（业务生成，最长 100 字符）';
COMMENT ON COLUMN task_execution.app_id IS '应用 ID（Skill 模块固定 SKILL_APP）';
COMMENT ON COLUMN task_execution.app_type IS '任务类型（Skill 模块固定 SKILL_IMPORT）';
COMMENT ON COLUMN task_execution.status IS '任务状态：PROCESSING / COMPLETED / FAILED';
COMMENT ON COLUMN task_execution.stage IS '当前阶段（仅 Skill 导入任务有效）';
COMMENT ON COLUMN task_execution.progress IS '进度（0-100）';
COMMENT ON COLUMN task_execution.ext_info IS '扩展信息（logs / skillCount / providerId / errorMessage 的 JSON）';
COMMENT ON COLUMN task_execution.archived_at IS '归档时间（完成后 7 天可标记归档）';
```

---

## 补充 2：阶段 4 完整常量清单（约 30 行）

**用途**：追加到 `internal/applications/dtos/constants.go`，避免阶段 6 出现 magic string。

**说明**：与主报告 §3.2.8（OSS bucket/路径）、业务规范文档的枚举值对应。

```go
package dtos

// ========================================
// Skill 模块常量（追加到 constants.go）
// ========================================

// OSS 存储
const (
	SkillBucketName         = "beedance-skill"           // FileStorage bucket 名称
	SkillInitialVersion     = "v0.1.0"                   // 初始版本号
	SkillDraftVersionString = "draft"                    // Draft 标识
	SkillFilePathTemplate   = "%s/%s/%s"                 // {skillId}/{version}/{fileName}
	SkillZipPathTemplate    = "%s/%s/skill-%s-%s.zip"   // {skillId}/{version}/skill-{skillId}-{version}.zip
)

// 异步任务
const (
	SkillAppID          = "SKILL_APP"                    // task_execution.app_id 固定值
	SkillImportAppType  = "SKILL_IMPORT"                 // task_execution.app_type 固定值
)

// ProviderType 枚举
const (
	ProviderTypeGit    = "GIT"
	ProviderTypeZip    = "ZIP"
	ProviderTypeCustom = "CUSTOM"
)

// AuthType 枚举
const (
	AuthTypeHttpToken = "HTTP_TOKEN"
	AuthTypeNone      = "NONE"
)

// ProviderStatus / SkillStatus 枚举
const (
	StatusActive   = "ACTIVE"
	StatusDisabled = "DISABLED"
)

// TaskStatus 枚举
const (
	TaskStatusProcessing = "PROCESSING"
	TaskStatusCompleted  = "COMPLETED"
	TaskStatusFailed     = "FAILED"
)

// TaskStage 枚举（导入任务五阶段）
const (
	TaskStageUpload    = "UPLOAD"
	TaskStageExtract   = "EXTRACT"
	TaskStageValidate  = "VALIDATE"
	TaskStageRegister  = "REGISTER"
	TaskStageCompleted = "COMPLETED"
)

// LogLevel 枚举（任务日志）
const (
	LogLevelInfo    = "INFO"
	LogLevelWarning = "WARNING"
	LogLevelError   = "ERROR"
	LogLevelDebug   = "DEBUG"
)
```

---

## 补充 3：阶段 7 路由注册骨架（约 60 行）

**用途**：在 `api/routers/route.go` 的 `InitRouter()` 函数末尾追加，作为阶段 7 施工蓝图。

**说明**：包含完整 DI 链路（4 Repo → 4 DomainSvc → 4 AppSvc → 1 Handler）+ 4 个路由分组 + form-data 处理 + SSE 范式。**所有 27 个接口集中在单一 `SkillHandler` 内**，由 `api/handlers/skill.go` 单文件维护。

```go
// ========================================
// Skill 模块路由注册（追加到 api/routers/route.go InitRouter() 末尾）
// ========================================

// 1) Repository
skillProviderRepo := repositories.NewSkillProviderRepository()
skillRepo := repositories.NewSkillRepository()
skillVersionRepo := repositories.NewSkillVersionRepository()
taskExecutionRepo := repositories.NewTaskExecutionRepository()

// 2) FileStorage
fileStorage := file_storage.NewLocalFileStorage(config.Cfg.Storage.RootDir) // 需先在 config.toml 中新增 [storage] root_dir = "./data"

// 3) Domain Service
skillProviderDomainSvc := domain_svc.NewSkillProviderDomainService(skillProviderRepo)
skillDomainSvc := domain_svc.NewSkillDomainService(skillRepo, skillVersionRepo, fileStorage)
skillVersionDomainSvc := domain_svc.NewSkillVersionDomainService(skillVersionRepo, skillRepo, fileStorage)
skillImportTaskDomainSvc := domain_svc.NewSkillImportTaskDomainService(taskExecutionRepo, skillProviderRepo, skillRepo, skillVersionRepo, fileStorage)

// 4) Application Service
skillProviderAppSvc := app_svc.NewSkillProviderApplicationService(skillProviderDomainSvc)
skillAppSvc := app_svc.NewSkillApplicationService(skillDomainSvc)
skillVersionAppSvc := app_svc.NewSkillVersionApplicationService(skillVersionDomainSvc)
skillImportTaskAppSvc := app_svc.NewSkillImportTaskApplicationService(skillImportTaskDomainSvc)

// 5) Handler（单一 SkillHandler 持有 4 个 ApplicationService）
skillHandler := handlers.NewSkillHandler(skillAppSvc, skillProviderAppSvc, skillVersionAppSvc, skillImportTaskAppSvc)

// 6) 路由分组（全部由同一个 skillHandler 承载）
skill := r.Group("/api/v1/skill")
{
	// Skill 子域 9 个接口
	skill.POST("/draft/save", skillHandler.DraftSave)                // form-data
	skill.POST("/publish", skillHandler.Publish)                     // form-data
	skill.POST("/update", skillHandler.Update)
	skill.POST("/delete", skillHandler.Delete)
	skill.POST("/list", skillHandler.List)
	skill.POST("/listAll", skillHandler.ListAll)
	skill.POST("/detail", skillHandler.Detail)
	skill.POST("/with/version", skillHandler.WithVersion)
	skill.GET("/file/download", skillHandler.FileDownload)           // GET + query params
}

skillProvider := r.Group("/api/v1/skill/provider")
{
	// Provider 子域 7 个接口（method 加 Provider 前缀避免与 Skill 子域同名）
	skillProvider.POST("/import/git", skillHandler.ProviderImportGit)
	skillProvider.POST("/import/zip", skillHandler.ProviderImportZip)               // form-data
	skillProvider.POST("/import/zip/legacy", skillHandler.ProviderImportZipLegacy)  // form-data
	skillProvider.POST("/sync", skillHandler.ProviderSync)
	skillProvider.POST("/delete", skillHandler.ProviderDelete)
	skillProvider.POST("/list", skillHandler.ProviderList)
	skillProvider.POST("/detail", skillHandler.ProviderDetail)
}

skillImportTask := r.Group("/api/v1/skill/provider/import/task")
{
	// 导入任务子域 3 个接口（method 加 ImportTask 前缀）
	skillImportTask.POST("/detail", skillHandler.ImportTaskDetail)   // SSE 订阅
	skillImportTask.POST("/list", skillHandler.ImportTaskList)
	skillImportTask.POST("/delete", skillHandler.ImportTaskDelete)
}

skillVersion := r.Group("/api/v1/skill/version")
{
	// Version 子域 8 个接口（method 加 Version 前缀）
	skillVersion.POST("/create", skillHandler.VersionCreate)
	skillVersion.POST("/validate", skillHandler.VersionValidate)
	skillVersion.POST("/delete", skillHandler.VersionDelete)
	skillVersion.POST("/list", skillHandler.VersionList)
	skillVersion.POST("/detail", skillHandler.VersionDetail)
	skillVersion.POST("/latest", skillHandler.VersionLatest)
	skillVersion.POST("/rollback", skillHandler.VersionRollback)
	skillVersion.POST("/export", skillHandler.VersionExport)          // ZIP 流式下载
}
```

### SkillHandler 结构体定义

```go
// api/handlers/skill.go
type SkillHandler struct {
	skillAppSvc            services.SkillApplicationService
	skillProviderAppSvc    services.SkillProviderApplicationService
	skillVersionAppSvc     services.SkillVersionApplicationService
	skillImportTaskAppSvc  services.SkillImportTaskApplicationService
}

func NewSkillHandler(
	skillAppSvc services.SkillApplicationService,
	skillProviderAppSvc services.SkillProviderApplicationService,
	skillVersionAppSvc services.SkillVersionApplicationService,
	skillImportTaskAppSvc services.SkillImportTaskApplicationService,
) *SkillHandler {
	return &SkillHandler{
		skillAppSvc:           skillAppSvc,
		skillProviderAppSvc:   skillProviderAppSvc,
		skillVersionAppSvc:    skillVersionAppSvc,
		skillImportTaskAppSvc: skillImportTaskAppSvc,
	}
}
```

### form-data 处理范式

草稿暂存 / 发布 / ZIP 导入等接口需要上传文件（`multipart/form-data`），Handler 内解析示例：

```go
func (h *SkillHandler) DraftSave(c *gin.Context) {
	// 1) 解析 form-data
	if err := c.Request.ParseMultipartForm(100 << 20); err != nil { // 100MB
		writeError(c, fmt.Errorf("parse multipart form failed: %w", services.ErrInvalidInput))
		return
	}

	// 2) 构造请求
	var req dtos.SkillDraftSaveRequest
	req.SkillName = c.PostForm("skillName")
	req.ProviderId = c.PostForm("providerId")
	req.Description = c.PostForm("description")
	// ... 其他字段

	// 3) 获取文件句柄
	form := c.Request.MultipartForm
	files := form.File["skillFiles"] // 对应前端 FormData.append("skillFiles", file)

	// 4) 调用 Service（传入 files）
	skillId, err := h.skillAppSvc.DraftSave(c.Request.Context(), &req, files)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"skillId": skillId})
}
```

### SSE 订阅范式

导入任务详情接口（`/skill/provider/import/task/detail`）采用 SSE 长连接，Handler 内复用 `sse.EventHandleProtocol`：

```go
func (h *SkillHandler) ImportTaskDetail(c *gin.Context) {
	var req dtos.SkillImportTaskDetailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 初始化 SSE 连接（复用现有封装）
	emitter := sse.NewEventHandleProtocol(c.Writer, 60*time.Minute)

	// 调用 Service 订阅任务（Service 内持有 emitter map，异步推送事件）
	err := h.skillImportTaskAppSvc.SubscribeTask(c.Request.Context(), req.TaskId, emitter)
	if err != nil {
		emitter.Error(err)
		return
	}

	// 阻塞直到任务完成或超时（Service 内通过 emitter.SendEvent 推送进度）
	<-c.Request.Context().Done()
}
```

---

## 补充 4：Handler 单文件统一封装

主报告 §4.1 阶段 7 已更新为**单一 Handler 文件**承载全部 27 个接口：

```
api/handlers/
  └ skill.go               // SkillHandler（统一封装 27 个接口）
```

**Method 命名规则**（按业务子域加前缀，避免同名冲突）：

| 子域 | 接口数 | Method 前缀 | 示例 |
|------|--------|------------|------|
| Skill | 9 | 无前缀 | `DraftSave / Publish / Update / Delete / List / ListAll / Detail / WithVersion / FileDownload` |
| Provider | 7 | `Provider` | `ProviderImportGit / ProviderImportZip / ProviderImportZipLegacy / ProviderSync / ProviderDelete / ProviderList / ProviderDetail` |
| ImportTask | 3 | `ImportTask` | `ImportTaskDetail / ImportTaskList / ImportTaskDelete` |
| Version | 8 | `Version` | `VersionCreate / VersionValidate / VersionDelete / VersionList / VersionDetail / VersionLatest / VersionRollback / VersionExport` |

**SkillHandler 依赖**：持有 4 个 ApplicationService：

```go
type SkillHandler struct {
    skillAppSvc            services.SkillApplicationService
    skillProviderAppSvc    services.SkillProviderApplicationService
    skillVersionAppSvc     services.SkillVersionApplicationService
    skillImportTaskAppSvc  services.SkillImportTaskApplicationService
}
```

**单文件维护的好处**：
1. Skill 模块对外是一个完整能力单元，单一 Handler 体现这种聚合性。
2. 27 个接口共用同一套错误处理辅助函数（如 `writeError`），避免重复代码。
3. 与 mooc-manus 现有 5 个 Handler 文件结构对应（每个 Handler 一个文件）一致。

---

## 使用说明

1. **阶段 1 施工时**：直接复制补充 1 的 SQL 追加到 `docs/sql/manus_schema.sql`。
2. **阶段 4 施工时**：直接复制补充 2 的常量追加到 `internal/applications/dtos/constants.go`。
3. **阶段 7 施工时**：
   - 在 `api/handlers/skill.go` 中定义单一 `SkillHandler`（持有 4 个 ApplicationService），承载全部 27 个接口（method 命名规则见补充 4）。
   - 复制补充 3 的路由注册代码追加到 `api/routers/route.go` 末尾。
   - 参考 form-data / SSE 范式实现 3 类特殊接口（草稿/发布/导入 用 form-data；任务详情用 SSE；文件下载用 GET）。

---

**补充文档版本**：v1.0  
**对应主报告**：mooc-manus-code-standards.md v1.0  
**最终评审**：待用户确认（2026-06-23）
