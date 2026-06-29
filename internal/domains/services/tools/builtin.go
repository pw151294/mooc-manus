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

// NativeTools 返回 manus 原生内置工具实例切片（fileRead + fileEdit + bashExec）
// 与 SkillTools 平级，属于 NATIVE 工具分类（R-44 表格新增的第四类）
// 详细契约见 .harness/rules/49-native-builtin.md 与 docs/superpowers/plans/2026-06-29-native-builtin-tools.md
//
// 入参：
//   - workspace: NativeWorkspace，由 route.go 单例装配
//   - denyList:  bash 命令黑名单实例，由 route.go 装配
//   - bashTimeoutDefaultSec / bashTimeoutMaxSec / bashOutputCap / bashConcurrency: 来自 config.Native
//   - messageId: SSE 流消息 ID，与 SkillTools 共用，由 application 层从 sse.StartChat 注入
func NativeTools(
	workspace *NativeWorkspace,
	denyList *BashDenyList,
	bashTimeoutDefaultSec int,
	bashTimeoutMaxSec int,
	bashOutputCap int,
	bashConcurrency int,
	messageId string,
) ([]Tool, error) {
	tools := make([]Tool, 0, 3)

	fileRead := NewFileReadTool(workspace)
	if err := fileRead.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileRead)

	fileEdit := NewFileEditTool(workspace, messageId)
	if err := fileEdit.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileEdit)

	bashExec := NewBashExecTool(denyList, bashTimeoutDefaultSec, bashTimeoutMaxSec, bashOutputCap, bashConcurrency, messageId)
	if err := bashExec.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, bashExec)

	return tools, nil
}
