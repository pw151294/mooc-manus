package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

// NativeWorkspace 管理 manus 原生内置工具（fileRead / fileEdit / bashExec）的工作区目录
// 与 SkillExecutor 并列；fileEdit 写盘仅限 workspace 内，fileRead 通过 IsSensitivePath 拦截敏感路径
// workspace 目录布局：${baseDir}/${messageId}/...，与 messageId 同生共死
type NativeWorkspace struct {
	baseDir             string
	sensitivePrefixes   []string // 已经过 filepath.Clean 与 ~ 展开
	maxFileReadBytes    int64
}

// NewNativeWorkspace 构造 NativeWorkspace
// baseDir 支持相对路径，构造时统一规范为绝对路径（便于 safeJoin 严格前缀比较）
// sensitivePathDenyList 支持以 ~ 开头的路径，按 $HOME 展开
// maxFileReadBytes <= 0 时回退到 10 MiB 默认值
func NewNativeWorkspace(baseDir string, sensitivePathDenyList []string, maxFileReadBytes int64) *NativeWorkspace {
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	prefixes := make([]string, 0, len(sensitivePathDenyList))
	for _, p := range sensitivePathDenyList {
		expanded := expandHome(p)
		if expanded == "" {
			continue
		}
		prefixes = append(prefixes, filepath.Clean(expanded))
	}
	if maxFileReadBytes <= 0 {
		maxFileReadBytes = 10 * 1024 * 1024
	}
	logger.Info("[native] NativeWorkspace initialized",
		zap.String("base_dir", baseDir),
		zap.Strings("sensitive_prefixes", prefixes),
		zap.Int64("max_file_read_bytes", maxFileReadBytes),
	)
	return &NativeWorkspace{
		baseDir:           baseDir,
		sensitivePrefixes: prefixes,
		maxFileReadBytes:  maxFileReadBytes,
	}
}

// BaseDir 暴露根目录，便于日志与测试断言
func (w *NativeWorkspace) BaseDir() string {
	return w.baseDir
}

// MaxFileReadBytes fileRead 单文件读取上限
func (w *NativeWorkspace) MaxFileReadBytes() int64 {
	return w.maxFileReadBytes
}

// WorkspaceDir 返回 messageId 对应的工作目录路径（不创建）
// messageId 为空时返回空串，调用方需自行处理
func (w *NativeWorkspace) WorkspaceDir(messageId string) string {
	if messageId == "" {
		return ""
	}
	return filepath.Join(w.baseDir, messageId)
}

// EnsureWorkspace 按需创建 messageId 对应的工作目录
// 已存在时无副作用；messageId 为空时返回错误
func (w *NativeWorkspace) EnsureWorkspace(messageId string) (string, error) {
	if messageId == "" {
		return "", fmt.Errorf("messageId is empty")
	}
	dir := w.WorkspaceDir(messageId)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir workspace failed: %w", err)
	}
	return dir, nil
}

// ResolveInWorkspace 把 relPath 安全地拼到 messageId 工作区上
// 复用 safeJoin（拒绝 ../ 与绝对路径）；调用前若 workspace 不存在会先创建
// 返回的绝对路径保证落在 ${baseDir}/${messageId}/ 内部
func (w *NativeWorkspace) ResolveInWorkspace(messageId, relPath string) (string, error) {
	root, err := w.EnsureWorkspace(messageId)
	if err != nil {
		return "", err
	}
	return safeJoin(root, relPath)
}

// Cleanup 删除指定 messageId 关联的整个工作目录
// messageId 为空时为 no-op；目录不存在时静默返回
func (w *NativeWorkspace) Cleanup(messageId string) error {
	if messageId == "" {
		return nil
	}
	dir := w.WorkspaceDir(messageId)
	logger.Info("[native] cleanup workspace",
		zap.String("message_id", messageId),
		zap.String("dir", dir),
	)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove workspace failed: %w", err)
	}
	return nil
}

// IsSensitivePath 判断 absPath 是否落在敏感路径黑名单前缀下
// absPath 应当是 filepath.Clean 后的绝对路径；非绝对路径返回 false（调用方应在传入前做规范化）
// 命中规则：absPath == prefix 或 absPath 以 prefix + separator 开头
func (w *NativeWorkspace) IsSensitivePath(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	if !filepath.IsAbs(cleaned) {
		return false
	}
	for _, prefix := range w.sensitivePrefixes {
		if cleaned == prefix {
			return true
		}
		if strings.HasPrefix(cleaned+string(filepath.Separator), prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// expandHome 把 ~ 或 ~/xxx 展开为 $HOME/xxx
// 无 HOME 环境变量时原样返回（不阻塞启动）
func expandHome(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
