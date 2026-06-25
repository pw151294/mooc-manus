package tools

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/repositories"
)

// BuiltinTools 返回所有内置工具实例切片
// 当前包含 loadSkill 和 executeSkill
func BuiltinTools(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	skillRefs []agents.SkillRef,
) ([]Tool, error) {
	tools := make([]Tool, 0, 2)

	// loadSkill
	loadSkill := NewLoadSkillTool(skillRepo, versionRepo, storage, skillRefs)
	if err := loadSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, loadSkill)

	// executeSkill
	executor := NewStubSkillExecutor() // 占位实现，后续可替换为 DockerSkillExecutor
	executeSkill := NewExecuteSkillTool(skillRepo, versionRepo, executor, skillRefs)
	if err := executeSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, executeSkill)

	return tools, nil
}
