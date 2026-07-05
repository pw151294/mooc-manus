package error_recovery

import "strings"

// Decision Classify 的返回结果
// Level 为 LevelNone 时表示未命中恢复规则(通常仅当调用者未过滤 Success=true 时才会返回)
type Decision struct {
	Level       Level
	TemplateKey string
	Template    string
}

// NativeToolName 4 类原生工具的注册名白名单
// 与 file_read.go / file_write.go / file_edit.go / bash_exec.go 保持严格一致
var NativeToolName = map[string]bool{
	"fileRead":  true,
	"fileWrite": true,
	"fileEdit":  true,
	"bashExec":  true,
}

// IsNativeTool 判断 toolName 是否在 4 类原生工具白名单内
func IsNativeTool(toolName string) bool {
	return NativeToolName[toolName]
}

// pattern 单条关键字规则
// keywords 需在 message 中**全部命中**(AND 语义),用于消除歧义(如 "文件不存在" 与 "no such file" 共存的场景)
// 命中即返回对应 level + templateKey,匹配优先级由 orderedPatterns 顺序决定
type pattern struct {
	Level       Level
	TemplateKey string
	Keywords    []string // AND 匹配
	// ToolScope 限定仅对特定工具生效;空表示所有 4 类工具通用
	ToolScope map[string]bool
}

// only 构造工具作用域集合
func only(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// orderedPatterns 按优先级从高到低排列:先 L3(致命),再 L2(需追问),兜底 L1
// 顺序至关重要:如 "打开文件失败" + "permission denied" 必须先于 "打开文件失败" + "no such file"
// 命中即返回,不再向下匹配
var orderedPatterns = []pattern{
	// ---- L3 致命类(优先命中) ----
	{Level: LevelFatal, TemplateKey: "sensitive_path", Keywords: []string{"命中敏感路径黑名单"}},
	{Level: LevelFatal, TemplateKey: "denylist_hit", Keywords: []string{"命令被拒绝", "黑名单"}, ToolScope: only("bashExec")},
	{Level: LevelFatal, TemplateKey: "cmd_timeout", Keywords: []string{"命令超时", "SIGKILL"}, ToolScope: only("bashExec")},
	{Level: LevelFatal, TemplateKey: "cmd_too_long", Keywords: []string{"command 长度", "超过上限"}, ToolScope: only("bashExec")},
	{Level: LevelFatal, TemplateKey: "disk_full", Keywords: []string{"no space left on device"}},
	{Level: LevelFatal, TemplateKey: "readonly_fs", Keywords: []string{"read-only file system"}},
	{Level: LevelFatal, TemplateKey: "oom_killed", Keywords: []string{"Killed"}, ToolScope: only("bashExec")},
	{Level: LevelFatal, TemplateKey: "binary_file", Keywords: []string{"NUL 字节"}, ToolScope: only("fileRead")},
	{Level: LevelFatal, TemplateKey: "not_utf8", Keywords: []string{"不是合法的 UTF-8"}, ToolScope: only("fileRead")},
	{Level: LevelFatal, TemplateKey: "file_too_large", Keywords: []string{"文件过大"}, ToolScope: only("fileRead")},
	{Level: LevelFatal, TemplateKey: "no_message_id", Keywords: []string{"messageId 未注入"}},
	{Level: LevelFatal, TemplateKey: "no_conversation_id", Keywords: []string{"conversationId"}},

	// ---- L2 需追问类 ----
	{Level: LevelAskUser, TemplateKey: "path_empty", Keywords: []string{"path parameter is required"}},
	{Level: LevelAskUser, TemplateKey: "path_empty", Keywords: []string{"path is empty"}},
	{Level: LevelAskUser, TemplateKey: "path_escape", Keywords: []string{"不允许 ../"}},
	{Level: LevelAskUser, TemplateKey: "path_escape", Keywords: []string{"不允许绝对路径"}},
	{Level: LevelAskUser, TemplateKey: "path_escape", Keywords: []string{"resolve path failed"}},
	{Level: LevelAskUser, TemplateKey: "port_busy", Keywords: []string{"Address already in use"}, ToolScope: only("bashExec")},
	{Level: LevelAskUser, TemplateKey: "desc_missing", Keywords: []string{"description parameter is required"}, ToolScope: only("bashExec")},
	{Level: LevelAskUser, TemplateKey: "args_parse_fail", Keywords: []string{"参数解析失败"}},

	// ---- L1 可自愈类 ----
	// fileEdit 多匹配 —— 特殊:文案已引导 LLM 扩展 old_string,归 L1 而非 L2
	{Level: LevelSelfHeal, TemplateKey: "edit_ambiguous", Keywords: []string{"old_string 在文件中出现了"}, ToolScope: only("fileEdit")},
	{Level: LevelSelfHeal, TemplateKey: "edit_no_match", Keywords: []string{"未在文件中找到匹配的 old_string"}, ToolScope: only("fileEdit")},
	{Level: LevelSelfHeal, TemplateKey: "is_directory", Keywords: []string{"路径是目录而非文件"}, ToolScope: only("fileRead")},

	// 权限/目录/存在类 —— 优先匹配 permission 再匹配 not-exist,避免 "打开文件失败: ...permission denied" 走到 dir_missing
	{Level: LevelSelfHeal, TemplateKey: "perm_denied", Keywords: []string{"打开文件失败", "permission denied"}},
	{Level: LevelSelfHeal, TemplateKey: "perm_denied", Keywords: []string{"写入失败", "permission denied"}},
	{Level: LevelSelfHeal, TemplateKey: "stat_perm", Keywords: []string{"stat 失败", "permission denied"}},
	{Level: LevelSelfHeal, TemplateKey: "dir_missing", Keywords: []string{"打开文件失败", "no such file or directory"}},
	{Level: LevelSelfHeal, TemplateKey: "file_not_found", Keywords: []string{"文件不存在"}},

	// bashExec 自愈类
	{Level: LevelSelfHeal, TemplateKey: "cmd_not_found", Keywords: []string{"command not found"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "bash_syntax", Keywords: []string{"syntax error"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "bash_syntax", Keywords: []string{"unexpected"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "exec_perm", Keywords: []string{"Permission denied"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "dep_missing", Keywords: []string{"ModuleNotFoundError"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "dep_missing", Keywords: []string{"No module named"}, ToolScope: only("bashExec")},
	{Level: LevelSelfHeal, TemplateKey: "env_missing", Keywords: []string{"unbound variable"}, ToolScope: only("bashExec")},

	// I/O 瞬时
	{Level: LevelSelfHeal, TemplateKey: "io_transient", Keywords: []string{"read sniff failed"}, ToolScope: only("fileRead")},
	{Level: LevelSelfHeal, TemplateKey: "io_transient", Keywords: []string{"read rest failed"}, ToolScope: only("fileRead")},

	// bashExec 通用兜底(exit != 0 且未命中前面所有规则)
	{Level: LevelSelfHeal, TemplateKey: "generic_exec_fail", Keywords: []string{"执行失败"}, ToolScope: only("bashExec")},
}

// Classify 给定 4 类原生工具的失败 Message,返回三级分层策略 + 修复模板
// 调用方保证 toolName 在 IsNativeTool 白名单内且 Success=false;否则返回 LevelNone
// 未命中任何规则时兜底 L1 通用模板
func Classify(toolName, message string) Decision {
	if !IsNativeTool(toolName) {
		return Decision{Level: LevelNone}
	}

	for _, p := range orderedPatterns {
		if p.ToolScope != nil && !p.ToolScope[toolName] {
			continue
		}
		if allContains(message, p.Keywords) {
			return Decision{
				Level:       p.Level,
				TemplateKey: p.TemplateKey,
				Template:    Template(p.TemplateKey),
			}
		}
	}

	return Decision{
		Level:       LevelSelfHeal,
		TemplateKey: "generic_l1_fallback",
		Template:    Template("generic_l1_fallback"),
	}
}

// allContains 判断 msg 是否包含 keywords 中的所有关键字(AND 语义)
// keywords 为空视为不命中,避免误击穿
func allContains(msg string, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	for _, kw := range keywords {
		if !strings.Contains(msg, kw) {
			return false
		}
	}
	return true
}
