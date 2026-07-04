package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

const (
	FileEditFunctionID   = "builtin-file-edit"
	FileEditFunctionName = "fileEdit"
	FileEditFunctionDesc = "在 workspace 内对文件做精确字符串替换。要求 old_string 在文件内唯一（除非 replace_all=true）；非唯一时返回前 3 个匹配的行号，请把 old_string 扩展到唯一上下文后重试。写入路径必须落在当前消息的 workspace 目录内。"
)

// FileEditTool fileEdit 内置工具：string-replace 风格
// persistent=false（默认）写入 ${messageId} 临时工作区；persistent=true 写入持久化规划目录
type FileEditTool struct {
	BaseTool
	workspace      *NativeWorkspace
	messageId      string // 与 SSE 流绑定，决定临时 workspace 子目录
	conversationId string // 用于 persistent=true 时定位持久化规划目录
}

// NewFileEditTool 构造 FileEditTool
// messageId 在 application 层注入，用于隔离不同消息的工作区
// conversationId 用于 persistent=true 时写入跨 session 的持久化规划目录
func NewFileEditTool(workspace *NativeWorkspace, messageId, conversationId string) Tool {
	return &FileEditTool{workspace: workspace, messageId: messageId, conversationId: conversationId}
}

func (t *FileEditTool) Init() error {
	funcDO := models.ToolFunctionDO{
		FunctionID:   FileEditFunctionID,
		ProviderID:   NativeProviderID,
		FunctionName: FileEditFunctionName,
		FunctionDesc: FileEditFunctionDesc,
		Schema: models.ToolSchema{
			Name:        FileEditFunctionName,
			Description: FileEditFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "相对根目录的文件路径；不允许 ../ 或绝对路径。persistent=false 时相对当前消息 workspace，persistent=true 时相对持久化规划目录",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "要被替换的原文，必须在文件内精确匹配",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "替换后的新文本",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "是否替换所有出现，默认 false",
					},
					"persistent": map[string]any{
						"type":        "boolean",
						"description": "是否写入持久化规划目录（跨 session 不清理）。Plan.md / TODO.md 等规划文件必须设为 true；代码等任务产出文件设为 false（默认）",
					},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
		},
	}

	t.BaseTool.providerId = NativeProviderID
	t.BaseTool.providerName = NativeProviderName
	t.BaseTool.providerType = NativeProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}
	return nil
}

func (t *FileEditTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	var params struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
		Persistent bool   `json:"persistent"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal fileEdit args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}
	if params.Path == "" {
		return models.ToolCallResult{Success: false, Message: "Error: path parameter is required"}
	}
	if params.OldString == "" {
		return models.ToolCallResult{Success: false, Message: "Error: old_string parameter is required"}
	}
	if params.OldString == params.NewString {
		return models.ToolCallResult{Success: false, Message: "Error: old_string 与 new_string 相同，无需替换"}
	}
	if t.workspace == nil {
		return models.ToolCallResult{Success: false, Message: "Error: workspace 未初始化"}
	}

	var absPath string
	var err error
	if params.Persistent {
		// persistent=true：写入持久化规划目录（跨 session 存活）
		if t.conversationId == "" {
			return models.ToolCallResult{Success: false, Message: "Error: persistent=true 时需要 conversationId，当前未注入"}
		}
		absPath, err = t.workspace.ResolveInConversationPlanDir(t.conversationId, params.Path)
	} else {
		// persistent=false（默认）：写入 messageId 临时工作区
		if t.messageId == "" {
			return models.ToolCallResult{Success: false, Message: "Error: messageId 未注入，无法定位 workspace"}
		}
		absPath, err = t.workspace.ResolveInWorkspace(t.messageId, params.Path)
	}
	if err != nil {
		return models.ToolCallResult{Success: false, Message: "Error: " + err.Error()}
	}

	original, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 文件不存在: %s", params.Path)}
		}
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 读取文件失败: %v", err)}
	}

	originalStr := string(original)
	occurrences := strings.Count(originalStr, params.OldString)
	if occurrences == 0 {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 未在文件中找到匹配的 old_string：%s", params.Path)}
	}
	if !params.ReplaceAll && occurrences > 1 {
		lineNos := locateMatchLines(originalStr, params.OldString, 3)
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: old_string 在文件中出现了 %d 次（前 %d 个匹配行号：%v），请扩展 old_string 直到唯一，或设置 replace_all=true",
				occurrences, len(lineNos), lineNos),
		}
	}

	var updated string
	var replaced int
	if params.ReplaceAll {
		updated = strings.ReplaceAll(originalStr, params.OldString, params.NewString)
		replaced = occurrences
	} else {
		updated = strings.Replace(originalStr, params.OldString, params.NewString, 1)
		replaced = 1
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: stat 失败: %v", err)}
	}
	if err := atomicWriteFile(absPath, []byte(updated), info.Mode()); err != nil {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 写入失败: %v", err)}
	}

	logger.Info("fileEdit success",
		zap.String("message_id", t.messageId),
		zap.String("conversation_id", t.conversationId),
		zap.Bool("persistent", params.Persistent),
		zap.String("path", absPath),
		zap.Int("replacements", replaced),
		zap.Bool("replace_all", params.ReplaceAll),
	)
	return models.ToolCallResult{
		Success: true,
		Data:    fmt.Sprintf("Edited %s: %d replacement(s)", params.Path, replaced),
	}
}

// locateMatchLines 返回 needle 在 haystack 中出现位置的行号（1-based）前 limit 个
func locateMatchLines(haystack, needle string, limit int) []int {
	if needle == "" || limit <= 0 {
		return nil
	}
	result := make([]int, 0, limit)
	start := 0
	for len(result) < limit {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			break
		}
		abs := start + idx
		// 1-based 行号 = 该位置前的 '\n' 数量 + 1
		line := strings.Count(haystack[:abs], "\n") + 1
		result = append(result, line)
		start = abs + len(needle)
		if start >= len(haystack) {
			break
		}
	}
	return result
}

// atomicWriteFile 原子写：写到同目录 .tmp 文件后 rename
// 保留 mode；rename 失败时尽力清理 tmp
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-fileEdit-*")
	if err != nil {
		return fmt.Errorf("create tmp failed: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write tmp failed: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync tmp failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close tmp failed: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod tmp failed: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}
