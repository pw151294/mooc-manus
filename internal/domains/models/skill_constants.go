package models

// Skill 模块领域层常量（枚举与领域规则相关的固定取值）
// 应用层常量（OSS 路径模板、分页默认值、文件大小上限等）放在
// internal/applications/dtos/constants.go，两份文件按层职责分离，避免循环依赖。

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

// 异步任务身份标识
const (
	SkillAppID         = "SKILL_APP"
	SkillImportAppType = "SKILL_IMPORT"
)

// Skill 版本相关
const (
	SkillInitialVersion     = "v0.1.0"
	SkillDraftVersionString = "draft"
)
