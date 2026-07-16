package tools

import (
	"path/filepath"

	"mooc-manus/config"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

// NativeToolsProvider 原生内置工具（fileRead / fileEdit / bashExec）提供方
// 单例装配，持有 NativeWorkspace + BashDenyList + bash 上限配置
// 与 SkillExecutor 平级，作为 BaseAgentDomainService 的单一依赖
// 详细契约见 .harness/rules/49-native-builtin.md
type NativeToolsProvider interface {
	// BuildTools 按 messageId + conversationId 构造一组工具实例（fileRead + fileEdit + bashExec）
	// messageId 用于 fileEdit 隔离临时 workspace 子目录、bashExec audit 关联
	// conversationId 用于 fileEdit persistent=true 时定位持久化规划目录
	BuildTools(messageId, conversationId string) ([]Tool, error)

	// Cleanup 清理 messageId 关联的临时 workspace 目录；messageId 为空时 no-op
	// 注意：不清理 conversationId 对应的持久化规划目录
	// 与 SSE 流生命周期对齐，由 application 层在 defer 路径中触发
	Cleanup(messageId string) error

	// ConversationPlanDir 返回 conversationId 对应的持久化规划目录路径（不在 Cleanup 时删除）
	ConversationPlanDir(conversationId string) string

	// MessageWorkspaceDir 返回 messageId 对应的临时 workspace 绝对路径（可能未创建，仅拼接）
	// 评测系统 InstanceExecutor 用此路径作为 verify_script / init_script 的 workdir
	MessageWorkspaceDir(messageId string) string
}

// NewNativeToolsProvider 单例装配 NativeToolsProvider
// 直接接受 config.NativeConfig，并在内部完成默认值回退与 NativeWorkspace/BashDenyList 装配
// storageRootDir 用于 WorkspaceBaseDir 为空时回退到 ${storage.root_dir}/native-workspace
// 该签名与 NewDockerSkillExecutor 直接消费 config.Cfg.Skill.* 字段保持对称
func NewNativeToolsProvider(nativeCfg config.NativeConfig, storageRootDir string) NativeToolsProvider {
	// WorkspaceBaseDir 默认值回退（由 route.go 内迁至 provider）
	workspaceDir := nativeCfg.WorkspaceBaseDir
	if workspaceDir == "" {
		workspaceDir = filepath.Join(storageRootDir, "native-workspace")
	}

	workspace := NewNativeWorkspace(workspaceDir, nativeCfg.SensitivePathDenyList, nativeCfg.MaxFileReadBytes)
	denyList := NewBashDenyList(nativeCfg.BashCommandDenyList)

	logger.Info("[native] NativeToolsProvider initialized",
		zap.String("workspace_base_dir", workspaceDir),
		zap.Int("bash_timeout_default_sec", nativeCfg.BashTimeoutDefault),
		zap.Int("bash_timeout_max_sec", nativeCfg.BashTimeoutMax),
		zap.Int("bash_output_cap", nativeCfg.BashOutputCap),
		zap.Int("bash_concurrency", nativeCfg.BashConcurrency),
	)
	return &nativeToolsProviderImpl{
		workspace:             workspace,
		denyList:              denyList,
		bashTimeoutDefaultSec: nativeCfg.BashTimeoutDefault,
		bashTimeoutMaxSec:     nativeCfg.BashTimeoutMax,
		bashOutputCap:         nativeCfg.BashOutputCap,
		bashConcurrency:       nativeCfg.BashConcurrency,
	}
}

type nativeToolsProviderImpl struct {
	workspace             *NativeWorkspace
	denyList              *BashDenyList
	bashTimeoutDefaultSec int
	bashTimeoutMaxSec     int
	bashOutputCap         int
	bashConcurrency       int
}

func (p *nativeToolsProviderImpl) BuildTools(messageId, conversationId string) ([]Tool, error) {
	tools := make([]Tool, 0, 4)

	fileRead := NewFileReadTool(p.workspace)
	if err := fileRead.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileRead)

	fileWrite := NewFileWriteTool(p.workspace, messageId, conversationId)
	if err := fileWrite.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileWrite)

	fileEdit := NewFileEditTool(p.workspace, messageId, conversationId)
	if err := fileEdit.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileEdit)

	bashExec := NewBashExecTool(
		p.denyList,
		p.bashTimeoutDefaultSec,
		p.bashTimeoutMaxSec,
		p.bashOutputCap,
		p.bashConcurrency,
		messageId,
	)
	if err := bashExec.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, bashExec)

	return tools, nil
}

func (p *nativeToolsProviderImpl) ConversationPlanDir(conversationId string) string {
	return p.workspace.ConversationPlanDir(conversationId)
}

func (p *nativeToolsProviderImpl) Cleanup(messageId string) error {
	return p.workspace.Cleanup(messageId)
}

// MessageWorkspaceDir 转发到 NativeWorkspace.WorkspaceDir（只拼路径不创建目录）
// 空 messageId 返回空串，调用方需自行处理
func (p *nativeToolsProviderImpl) MessageWorkspaceDir(messageId string) string {
	return p.workspace.WorkspaceDir(messageId)
}
