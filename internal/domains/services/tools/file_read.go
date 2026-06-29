package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"mooc-manus/internal/domains/models"
	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

const (
	NativeProviderID   = "builtin-native-provider"
	NativeProviderName = "native-provider"
	NativeProviderType = "NATIVE"

	FileReadFunctionID   = "builtin-file-read"
	FileReadFunctionName = "fileRead"
	FileReadFunctionDesc = "读取宿主机文件内容，返回带行号前缀的文本片段。支持 path 为绝对路径或相对 manus 进程工作目录的相对路径；仅支持 UTF-8 文本文件；命中敏感路径黑名单时拒绝。"

	fileReadDefaultLimit = 2000
	fileReadMaxLimit     = 5000
	// 二进制嗅探采样字节数：读取文件前 N 字节，若含 NUL 字节则判定为二进制
	fileReadBinarySniffBytes = 4096
)

// FileReadTool fileRead 内置工具：全宿主机可读，敏感路径黑名单 + 大小上限 + UTF-8 校验
type FileReadTool struct {
	BaseTool
	workspace *NativeWorkspace
}

// NewFileReadTool 构造 FileReadTool
// workspace 仅用于读取 MaxFileReadBytes 与 IsSensitivePath；不强制读取路径落在 workspace 内
func NewFileReadTool(workspace *NativeWorkspace) Tool {
	return &FileReadTool{workspace: workspace}
}

func (t *FileReadTool) Init() error {
	funcDO := models.ToolFunctionDO{
		FunctionID:   FileReadFunctionID,
		ProviderID:   NativeProviderID,
		FunctionName: FileReadFunctionName,
		FunctionDesc: FileReadFunctionDesc,
		Schema: models.ToolSchema{
			Name:        FileReadFunctionName,
			Description: FileReadFunctionDesc,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "要读取的文件路径，可以是绝对路径或相对 manus 进程工作目录的相对路径",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "从第几行开始读，1-based，默认 1",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": fmt.Sprintf("最多读取多少行，默认 %d，上限 %d", fileReadDefaultLimit, fileReadMaxLimit),
					},
				},
				"required": []string{"path"},
			},
		},
	}

	t.BaseTool.providerId = NativeProviderID
	t.BaseTool.providerName = NativeProviderName
	t.BaseTool.providerType = NativeProviderType
	t.BaseTool.functions = []models.ToolFunctionDO{funcDO}
	return nil
}

func (t *FileReadTool) Invoke(funcName, funcArgs string) models.ToolCallResult {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(funcArgs), &params); err != nil {
		logger.Error("unmarshal fileRead args failed", zap.Error(err), zap.String("func_args", funcArgs))
		return models.ToolCallResult{Success: false, Message: "Error: 参数解析失败"}
	}
	if params.Path == "" {
		return models.ToolCallResult{Success: false, Message: "Error: path parameter is required"}
	}
	if params.Offset < 0 {
		return models.ToolCallResult{Success: false, Message: "Error: offset 必须 >= 1"}
	}
	if params.Offset == 0 {
		params.Offset = 1
	}
	if params.Limit <= 0 {
		params.Limit = fileReadDefaultLimit
	}
	if params.Limit > fileReadMaxLimit {
		params.Limit = fileReadMaxLimit
	}

	absPath, err := resolveAbsPath(params.Path)
	if err != nil {
		return models.ToolCallResult{Success: false, Message: "Error: " + err.Error()}
	}

	if t.workspace != nil && t.workspace.IsSensitivePath(absPath) {
		logger.Warn("fileRead blocked by sensitive path",
			zap.String("path", absPath))
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 路径命中敏感路径黑名单: %s", absPath)}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 文件不存在: %s", absPath)}
		}
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: stat 失败: %v", err)}
	}
	if info.IsDir() {
		return models.ToolCallResult{Success: false, Message: fmt.Sprintf("Error: 路径是目录而非文件: %s", absPath)}
	}
	maxBytes := int64(10 * 1024 * 1024)
	if t.workspace != nil {
		maxBytes = t.workspace.MaxFileReadBytes()
	}
	if info.Size() > maxBytes {
		return models.ToolCallResult{
			Success: false,
			Message: fmt.Sprintf("Error: 文件过大 (%d 字节 > 上限 %d 字节): %s", info.Size(), maxBytes, absPath),
		}
	}

	content, err := readFileWithBinaryCheck(absPath)
	if err != nil {
		return models.ToolCallResult{Success: false, Message: "Error: " + err.Error()}
	}

	formatted, totalLines, returnedLines := formatWithLineNumbers(content, params.Offset, params.Limit)
	logger.Info("fileRead success",
		zap.String("path", absPath),
		zap.Int("total_lines", totalLines),
		zap.Int("returned_lines", returnedLines),
		zap.Int("offset", params.Offset),
		zap.Int("limit", params.Limit),
	)
	return models.ToolCallResult{Success: true, Data: formatted}
}

// resolveAbsPath 把用户传入的 path 规范为绝对路径
// 绝对路径直接 Clean；相对路径相对当前进程 cwd
func resolveAbsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path failed: %w", err)
	}
	return abs, nil
}

// readFileWithBinaryCheck 读取文件全文，先嗅探前 N 字节判定 utf-8
// 含 NUL 字节或 utf-8 校验失败 → 拒绝
func readFileWithBinaryCheck(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file failed: %w", err)
	}
	defer f.Close()

	sniffBuf := make([]byte, fileReadBinarySniffBytes)
	n, err := io.ReadFull(f, sniffBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read sniff failed: %w", err)
	}
	sample := sniffBuf[:n]
	if bytes.IndexByte(sample, 0) >= 0 {
		return nil, fmt.Errorf("文件包含 NUL 字节，疑似二进制文件")
	}

	// 完整读取剩余内容
	rest, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read rest failed: %w", err)
	}
	all := append(sample, rest...)

	// utf-8 校验（允许末尾不完整码点：截断时 valid 会判 false，这里只对完整内容判定）
	if !utf8.Valid(all) {
		return nil, fmt.Errorf("文件不是合法的 UTF-8 文本")
	}
	return all, nil
}

// formatWithLineNumbers 按 cat -n 风格输出
// 返回 (formatted, totalLines, returnedLines)
// offset 1-based；limit 上限已经在 caller 处 clamp
func formatWithLineNumbers(content []byte, offset, limit int) (string, int, int) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	// 单行最大 1 MiB（避免巨型一行打爆默认 64KB 缓冲区）
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var b strings.Builder
	lineNo := 0
	returned := 0
	for scanner.Scan() {
		lineNo++
		if lineNo < offset {
			continue
		}
		if returned >= limit {
			// 仍然继续 scan 以便统计 totalLines
			continue
		}
		fmt.Fprintf(&b, "%6d\t%s\n", lineNo, scanner.Text())
		returned++
	}
	return b.String(), lineNo, returned
}
