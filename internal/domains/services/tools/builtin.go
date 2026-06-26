package tools

import (
	"mooc-manus/internal/domains/models/agents"
	"mooc-manus/internal/infra/external/file_storage"
	"mooc-manus/internal/infra/repositories"
)

// SkillTools 返回 Skill 专属内置工具实例切片（loadSkill + executeSkill）
// 仅当 skillRefs 非空时应调用此方法
// messageId 用于 ExecuteSkillTool 隔离不同消息的容器与工作目录，由 application 层注入
func SkillTools(
	skillRepo repositories.SkillRepository,
	versionRepo repositories.SkillVersionRepository,
	storage file_storage.FileStorage,
	executor SkillExecutor,
	skillRefs []agents.SkillRef,
	messageId string,
) ([]Tool, error) {
	tools := make([]Tool, 0, 2)

	// loadSkill
	loadSkill := NewLoadSkillTool(skillRepo, versionRepo, storage, skillRefs)
	if err := loadSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, loadSkill)

	// executeSkill
	executeSkill := NewExecuteSkillTool(skillRepo, versionRepo, storage, executor, skillRefs, messageId)
	if err := executeSkill.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, executeSkill)

	return tools, nil
}
