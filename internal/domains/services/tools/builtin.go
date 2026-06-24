package tools

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/repositories"
)

// BuiltinTools 返回所有内置工具实例切片
// 当前仅包含 loadSkill，未来可扩展 executeSkill / think 等
func BuiltinTools(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	skillRefs []agents.SkillRef,
) ([]Tool, error) {
	tools := make([]Tool, 0, 1)

	// loadSkill
	loadSkill := NewLoadSkillTool(skillRepo, versionRepo, storage, skillRefs)
	if err := loadSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, loadSkill)

	// 未来扩展点：
	// executeSkill := NewExecuteSkillTool(...)
	// tools = append(tools, executeSkill)

	return tools, nil
}
