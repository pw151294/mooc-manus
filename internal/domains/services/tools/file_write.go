package tools

import (
	"encoding/json"
	"fmt"
	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/logger"
	"os"

	"go.uber.org/zap"
)

const (
	FileWriteFunctionID   = "file_write"
	FileWriteFunctionName = "fileWrite"
	FileWriteFunctionDesc = "在 workspace 内创建新文件并写入内容，或向已有文件追加内容。支持 persistent 参数控制是否写入持久化规划目录。"
)

// FileWriteTool fileWrite 内置工具：创建文件或追加内容
// persistent=false（默认）写入 ${messageId} 临时工作区；persistent=true 写入持久化规划目录
type FileWriteTool struct {
	BaseTool
	workspace      *NativeWorkspace
	messageId      string // 与 SSE 流绑定，决定临时 workspace 子目录
	conversationId string // 用于 persistent=true 时定位持久化规划目录
}

// NewFileWriteTool 构造 FileWriteTool
// messageId 在 application 层注入，用于隔离不同消息的工作区
// conversationId 用于 persistent=true 时写入跨 session 的持久化规划目录
func NewFileWriteTool(workspace *NativeWorkspace, messageId, conversationId string) Tool {
	return &FileWriteTool{workspace: workspace, messageId: messageId, conversationId: conversationId}
}

func (t *FileWriteTool) Init() error {
	funcDO := models.ToolFunctionDO{
		FunctionID:   FileWriteFunctionID,
		ProviderID:   NativeProviderID,
		FunctionName: FileWriteFunctionName,
		FunctionDesc: FileWriteFunctionDesc,
		Schema: models.ToolSchema{
			Name:        FileWriteFunctionName,
			Description: FileWriteFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "相对根目录的文件路径；不允许 ../ 或绝对路径。persistent=false 时相对当前消息 workspace，persistent=true 时相对持久化规划目录",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "要写入的内容",
					},
					"append": map[string]any{
						"type":        "boolean",
						"description": "是否追加模式。true 时追加到文件末尾，false（默认）时覆盖原文件",
					},
					"persistent": map[string]any{
						"type":        "boolean",
						"description": "是否写入持久化规划目录（跨 session 不清理）。Plan.md / TODO.md 等规划文件必须设为 true；代码等任务产出文件设为 false（默认）",
					},
				},
				"required": []string{"path", "content"},
			},
		},
	}

	t.BaseTool.providerId = NativeProviderID
	t.BaseTool.providerName = NativeProviderName
	t.BaseTool.providerType = NativeProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}
	return nil
}

func (t *FileWriteTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	var params struct {
		Path       string `json:"path"`
		Content    string `json:"content"`
		Append     bool   `json:"append"`
		Persistent bool   `json:"persistent"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal fileWrite args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}
	if params.Path == "" {
		return models.ToolCallResult{Success: false, Message: "Error: path parameter is required"}
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

	var flag int
	var mode os.FileMode
	if params.Append {
		// 追加模式：O_APPEND | O_CREATE | O_WRONLY
		flag = os.O_APPEND | os.O_CREATE | os.O_WRONLY
		mode = 0644
	} else {
		// 覆盖模式：O_CREATE | O_WRONLY | O_TRUNC
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		mode = 0644
	}

	file, err := os.OpenFile(absPath, flag, mode)
	if err != nil {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 打开文件失败: %v", err)}
	}
	defer file.Close()

	if _, err := file.WriteString(params.Content); err != nil {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 写入失败: %v", err)}
	}

	action := "created/overwritten"
	if params.Append {
		action = "appended"
	}

	logger.Info("fileWrite success",
		zap.String("message_id", t.messageId),
		zap.String("conversation_id", t.conversationId),
		zap.Bool("persistent", params.Persistent),
		zap.String("path", absPath),
		zap.Int("content_bytes", len(params.Content)),
		zap.Bool("append", params.Append),
	)
	return models.ToolCallResult{
		Success: true,
		Data:    fmt.Sprintf("File %s: %s (%d bytes)", action, params.Path, len(params.Content)),
	}
}
