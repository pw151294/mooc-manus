package dtos

const defaultModelTemperature = 0.7
const defaultModelMaxTokens = 8192
const defaultAgentMaxIterations = 100
const defaultAgentMaxRetries = 3
const defaultAgentMaxSearchResults = 10
const maxAgentMaxIterations = 1000
const maxAgentMaxRetries = 10
const maxAgentSearchResults = 30

// ============================================================
// Skill 模块常量
// 详见 docs/mooc-manus-code-standards-supplement.md §2
// ============================================================

// OSS 存储
const (
	SkillBucketName       = "beedance-skill"
	SkillFilePathTemplate = "%s/%s/%s"               // {skillId}/{version}/{path}
	SkillZipPublishTpl    = "%s/%s/skill-%s-%s.zip"  // 发布 ZIP：{skillId}/{version}/skill-{skillId}-{version}.zip
	SkillZipRollbackTpl   = "%s/%s/%s-%s.zip"        // 回滚 ZIP：{skillId}/{version}/{skillId}-{version}.zip
)

// Skill 列表分页默认值
const (
	SkillListDefaultPageNum  = 1
	SkillListDefaultPageSize = 10
	SkillListMaxPageSize     = 1000
	SkillNameMaxLength       = 120
	SkillDescMaxLength       = 3000
	SkillNameLikeMaxLength   = 64
)

// Skill ZIP 上传文件大小上限（100MB）
const SkillImportMaxFileSize = 100 << 20

// Skill ZIP 导入允许的文件后缀
// 业务规则 §5.12.2：仅支持 .zip / .tar.gz / .tgz，否则返回 SKILL_IMPORT_FILE_INVALID
var SkillImportAllowedSuffixes = []string{".zip", ".tar.gz", ".tgz"}

// Skill ZIP 异步导入自动生成的 Provider 名前缀
// 业务规则 §5.12.2：ZIP 异步导入会自动给 providerName 加时间戳前缀以避免唯一键冲突
// 实际格式：{Prefix}{taskId 前 8 位}_{原文件名（去后缀）}
const SkillZipImportProviderPrefix = "ZIP_IMPORT_"
