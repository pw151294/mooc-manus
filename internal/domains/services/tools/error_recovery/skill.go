package error_recovery

import (
	"os"
	"strings"

	"mooc-manus/internal/domains/models/agents"
)

const (
	// BuiltInSkillName 内置错误恢复 Skill 的注册名,与 SKILL.md frontmatter 的 name 严格一致
	BuiltInSkillName = "native-tool-error-recovery"
	// BuiltInSkillVersion 内置 Skill 的固定版本(不入 DB,仅作日志/事件标识)
	BuiltInSkillVersion = "v0.1.0"
	// BuiltInSkillID 内置 Skill 固定 ID(避免与 UUID skill 冲突,加 native- 前缀)
	BuiltInSkillID = "native-error-recovery-builtin"
	// envDisabled 关闭内置注入的环境变量;值为 "false" / "0" / "no" 时关闭
	envDisabled = "NATIVE_ERROR_RECOVERY_ENABLED"
)

// BuiltInSkillRef 内置 Skill 的引用,用于追加到 buildSkillsSystemPrompt 的 skillRefs 副本
func BuiltInSkillRef() agents.SkillRef {
	return agents.SkillRef{
		SkillID:   BuiltInSkillID,
		Version:   BuiltInSkillVersion,
		SkillName: BuiltInSkillName,
	}
}

// IsBuiltInSkill 判断 skillName 是否指向本内置 Skill,用于 loadSkill 短路
func IsBuiltInSkill(skillName string) bool {
	return skillName == BuiltInSkillName
}

// Enabled 是否启用内置注入
// 默认启用;仅当 NATIVE_ERROR_RECOVERY_ENABLED 显式设为 "false" / "0" / "no" 时禁用
func Enabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envDisabled)))
	if v == "" {
		return true
	}
	switch v {
	case "false", "0", "no", "off", "disable", "disabled":
		return false
	}
	return true
}
