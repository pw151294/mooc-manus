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

// NativeTools 返回 NATIVE 内置工具实例切片（fileRead + fileWrite + fileEdit + bashExec）
// 与 SkillTools 平级，封装 provider.BuildTools 调用语法
// provider 为 nil 时返回 (nil, nil)，调用方无需自行判空
// messageId 用于 fileWrite/fileEdit 隔离临时 workspace 子目录、bashExec audit 关联
// conversationId 用于 fileWrite/fileEdit persistent=true 时定位持久化规划目录
func NativeTools(provider NativeToolsProvider, messageId, conversationId string) ([]Tool, error) {
	if provider == nil {
		return nil, nil
	}
	return provider.BuildTools(messageId, conversationId)
}
