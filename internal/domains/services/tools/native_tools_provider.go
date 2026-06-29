package tools

import (
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

// NativeToolsOptions 装配 NativeToolsProvider 所需的全部配置
// 由 route.go 从 config.NativeConfig 字段拷贝填充；本结构刻意放在 tools 包内，
// 以保持 domain 层不依赖 config 包（DDD 分层约束）
type NativeToolsOptions struct {
	WorkspaceBaseDir      string
	SensitivePathDenyList []string
	MaxFileReadBytes      int64
	BashCommandDenyList   []string
	BashTimeoutDefaultSec int
	BashTimeoutMaxSec     int
	BashOutputCap         int
	BashConcurrency       int
}

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
// 内部一次性构造 NativeWorkspace 与 BashDenyList；BuildTools 复用这些状态
func NewNativeToolsProvider(opts NativeToolsOptions) NativeToolsProvider {
	workspace := NewNativeWorkspace(opts.WorkspaceBaseDir, opts.SensitivePathDenyList, opts.MaxFileReadBytes)
	denyList := NewBashDenyList(opts.BashCommandDenyList)
	logger.Info("[native] NativeToolsProvider initialized",
		zap.Int("bash_timeout_default_sec", opts.BashTimeoutDefaultSec),
		zap.Int("bash_timeout_max_sec", opts.BashTimeoutMaxSec),
		zap.Int("bash_output_cap", opts.BashOutputCap),
		zap.Int("bash_concurrency", opts.BashConcurrency),
	)
	return &nativeToolsProviderImpl{
		workspace:             workspace,
		denyList:              denyList,
		bashTimeoutDefaultSec: opts.BashTimeoutDefaultSec,
		bashTimeoutMaxSec:     opts.BashTimeoutMaxSec,
		bashOutputCap:         opts.BashOutputCap,
		bashConcurrency:       opts.BashConcurrency,
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
