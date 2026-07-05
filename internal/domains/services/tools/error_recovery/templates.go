package error_recovery

// Level 错误恢复分层策略
type Level int

const (
	// LevelNone 未命中(仅内部使用,不对外暴露前缀)
	LevelNone Level = 0
	// LevelSelfHeal L1 可自愈类:调用配套工具补齐条件后重试
	LevelSelfHeal Level = 1
	// LevelAskUser L2 需追问类:停止重试,向用户提问确认
	LevelAskUser Level = 2
	// LevelFatal L3 致命类:终止链路,不再重试
	LevelFatal Level = 3
)

// Prefix 返回追加到 ToolMessage 尾部的固定前缀
// 形如 "[ErrorRecovery-L1] "
func (l Level) Prefix() string {
	switch l {
	case LevelSelfHeal:
		return "[ErrorRecovery-L1] "
	case LevelAskUser:
		return "[ErrorRecovery-L2] "
	case LevelFatal:
		return "[ErrorRecovery-L3] "
	default:
		return ""
	}
}

// templates 三级模板文本表,templateKey → 修复文案
// 文案必须与 SKILL.md "全量故障库" 段一一对应,任何一侧改动必须同步另一侧
var templates = map[string]string{
	// ---- 文件类工具 ----
	"file_not_found":     "检测到目标文件不存在。请先用 fileRead 或 bashExec `ls -la <parent>` 确认路径是否正确;若确需新建,改用 fileWrite。自愈上限 2 轮。",
	"dir_missing":        "检测到父目录未创建。请先 bashExec `mkdir -p <parent>` 补齐,再重试原工具;上限 2 轮。",
	"perm_denied":        "检测到目标或父目录写权限不足。请先 bashExec `ls -la <parent>` 排查,必要时 `chmod`/`chown`;仍失败升级 L2。",
	"stat_perm":          "检测到上级目录缺少 x 权限。请先 bashExec `ls -ld <parent>` 排查权限;若无法修复,升级 L2 向用户澄清。",
	"edit_no_match":      "old_string 未在文件中匹配。请先用 fileRead 读取目标片段,以实际文本重构 old_string(注意保留缩进、行尾)后重试;上限 2 轮。",
	"edit_ambiguous":     "old_string 在文件内出现多次。请扩展 old_string 到唯一上下文后重试,或在明确意图后设置 replace_all=true;不需要向用户追问。",
	"sensitive_path":     "该路径命中平台敏感路径黑名单,链路终止。请向用户说明该路径受保护,建议改用 workspace 内路径。禁止任何形式的重试或绕过。",
	"path_empty":         "path 参数为空或缺失。请向用户确认目标文件的具体路径后再触发工具,不要凭空重试。",
	"binary_file":        "目标文件含 NUL 字节,判定为二进制。链路终止;若确需读取,建议向用户说明并改用 hexdump 等专用工具。",
	"not_utf8":           "目标文件编码非 UTF-8。链路终止;向用户说明并建议先转码为 UTF-8。",
	"file_too_large":     "文件大小超过 max_file_read_bytes 上限。链路终止;若确需读取,请拆分为 offset/limit 分片重试(改参数属 L1)。",
	"is_directory":       "传入的路径是目录而非文件。请先 bashExec `ls <path>` 列出目录内容,再选择具体文件重试;上限 2 轮。",
	"no_message_id":      "messageId 未注入,属于平台级异常。链路终止;向用户说明会话上下文丢失,请重新发起对话。",
	"no_conversation_id": "conversationId 未注入(persistent 模式需要)。链路终止;向用户说明会话上下文丢失。",
	"path_escape":        "路径逃逸或格式非法(../ / 绝对路径 / 空路径)。请向用户澄清期望路径,或改用相对 workspace 的合法路径。",
	"io_transient":       "磁盘 I/O 瞬时错误。允许原样重试 1 次;仍失败升级 L3。",
	"disk_full":          "磁盘空间不足(no space left on device)。链路终止;通知用户清理磁盘后再重发对话。",
	"readonly_fs":        "目标文件系统只读(read-only file system)。链路终止;通知用户挂载点异常。",

	// ---- Bash 工具 ----
	"cmd_not_found":     "命令未安装或不在 PATH。请先 bashExec `command -v <cmd>` / `which <cmd>` 定位,必要时安装或改用等价命令;上限 2 轮。",
	"bash_syntax":       "Bash 语法错误。请检查引号、括号、换行 转义,修正后重试;上限 2 轮。",
	"exec_perm":         "脚本无执行权限(exit 126)。请先 bashExec `chmod +x <script>` 或 `ls -la <script>` 排查后重试;上限 2 轮。",
	"dep_missing":       "Python/Node 依赖缺失(ModuleNotFoundError / No module named)。请先安装缺失依赖(pip install / npm install)后重试;上限 2 轮。",
	"env_missing":       "环境变量未注入或为空(unbound variable)。请先 bashExec `env` / `printenv` 检查;若确实缺失关键凭证,升级 L2 向用户追问。",
	"port_busy":         "端口占用(Address already in use)。停止重试,向用户确认是否复用端口或改端口。",
	"cmd_timeout":       "命令超时被 SIGKILL,已达 timeout 上限。链路终止;建议向用户说明,并把任务拆分或改为异步执行。",
	"denylist_hit":      "命令命中平台高危黑名单,链路终止。禁止任何形式的重试或绕过。请向用户解释拒绝原因并建议合规替代。",
	"cmd_too_long":      "command 长度超过 16 KiB。链路终止;若确需执行,请拆成多条命令(拆分后属 L1)。",
	"desc_missing":      "description 字段缺失。请补齐命令用途描述后重试(该字段用于审计日志,必填);不需要追问用户。",
	"generic_exec_fail": "命令逻辑执行失败,exit != 0。请结合 stderr 定位;若属缺依赖/权限/环境类,请按对应模板处置;通用兜底允许 bashExec 采集环境信息后重试,上限 2 轮。",
	"args_parse_fail":   "工具入参 JSON 解析失败,属 LLM 侧参数生成问题。请重新生成合法 JSON 后重试,不要向用户追问。",
	"oom_killed":        "进程被 OOM Killer 干掉(Killed / OOM)。链路终止;向用户说明内存不足,建议缩小任务规模。",

	// ---- 兜底 ----
	"generic_l1_fallback": "未命中已知故障模式,按通用自愈范式处理:先用 bashExec 采集环境信息(`ls -la`、`pwd`、`whoami`、`df -h` 等)定位根因,再决定是否重试;上限 2 轮,超过升级 L2。",
}

// Template 返回指定 templateKey 对应的模板文本;未命中返回通用兜底文案
func Template(key string) string {
	if v, ok := templates[key]; ok {
		return v
	}
	return templates["generic_l1_fallback"]
}
