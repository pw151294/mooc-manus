package circuitbreaker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// GenerateKey 生成工具调用唯一标识 Key
// 根据工具类型采用不同的哈希策略
func GenerateKey(toolName string, argsJSON string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	var hashInput string
	switch toolName {
	case "fileRead", "fileWrite":
		if path, ok := args["path"].(string); ok {
			hashInput = fmt.Sprintf("%s:path=%s", toolName, path)
		} else {
			hashInput = fmt.Sprintf("%s:%s", toolName, argsJSON)
		}
	case "fileEdit":
		if path, ok := args["path"].(string); ok {
			oldStr, _ := args["old_string"].(string)
			newStr, _ := args["new_string"].(string)
			hashInput = fmt.Sprintf("%s:path=%s:old=%s:new=%s",
				toolName, path, truncate(oldStr, 100), truncate(newStr, 100))
		} else {
			hashInput = fmt.Sprintf("%s:%s", toolName, argsJSON)
		}
	case "bashExec":
		if cmd, ok := args["command"].(string); ok {
			hashInput = fmt.Sprintf("%s:command=%s", toolName, cmd)
		} else {
			hashInput = fmt.Sprintf("%s:%s", toolName, argsJSON)
		}
	default:
		hashInput = fmt.Sprintf("%s:%s", toolName, argsJSON)
	}

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:]), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// GenerateParamsPreview 生成参数预览
func GenerateParamsPreview(toolName string, argsJSON string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return truncate(argsJSON, 50)
	}

	switch toolName {
	case "fileRead", "fileWrite", "fileEdit":
		if path, ok := args["path"].(string); ok {
			return fmt.Sprintf("path=%s", path)
		}
	case "bashExec":
		if cmd, ok := args["command"].(string); ok {
			return fmt.Sprintf("command=%s", truncate(cmd, 80))
		}
	}
	return truncate(argsJSON, 100)
}
