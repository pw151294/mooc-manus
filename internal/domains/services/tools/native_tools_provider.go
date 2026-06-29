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
	// BuildTools 按 messageId 构造一组工具实例（fileRead + fileEdit + bashExec）
	// messageId 用于 fileEdit 隔离 workspace 子目录、bashExec audit 关联
	BuildTools(messageId string) ([]Tool, error)

	// Cleanup 清理 messageId 关联的 workspace 目录；messageId 为空时 no-op
	// 与 SSE 流生命周期对齐，由 application 层在 defer 路径中触发
	Cleanup(messageId string) error
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

func (p *nativeToolsProviderImpl) BuildTools(messageId string) ([]Tool, error) {
	tools := make([]Tool, 0, 3)

	fileRead := NewFileReadTool(p.workspace)
	if err := fileRead.Init(); err != nil {
		return nil, err
	}
	tools = append(tools, fileRead)

	fileEdit := NewFileEditTool(p.workspace, messageId)
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

func (p *nativeToolsProviderImpl) Cleanup(messageId string) error {
	return p.workspace.Cleanup(messageId)
}
