package tools

import (
	"regexp"
	"strconv"
)

// denyPattern 单条黑名单规则
// Name 用于 audit 日志与 LLM 错误回灌；Pattern 是预编译的正则
type denyPattern struct {
	Name    string
	Pattern *regexp.Regexp
}

// defaultBashDenyPatterns 默认基线黑名单（启动时强制加载）
// 对应 plan §6.1；新增规则时在这里追加，配置文件 BashCommandDenyList 是 额外 叠加项
var defaultBashDenyPatterns = []denyPattern{
	// 文件系统破坏
	{"rm-rf-root", regexp.MustCompile(`(?i)\brm\s+-[a-zA-Z]*[rf][a-zA-Z]*\s+(/|/\s|/$|/\*)`)},
	{"rm-rf-home", regexp.MustCompile(`(?i)\brm\s+-[a-zA-Z]*[rf][a-zA-Z]*\s+~(/|\s|$)`)},
	{"overwrite-block-device", regexp.MustCompile(`(?i)>\s*/dev/[shvn]d[a-z]\d*`)},
	{"mkfs", regexp.MustCompile(`(?i)\bmkfs(\.[a-z0-9]+)?\b`)},
	{"dd-of-device", regexp.MustCompile(`(?i)\bdd\b[^|;&\n]*\bof=/dev/[shvn]d`)},

	// Fork 炸弹
	{"fork-bomb", regexp.MustCompile(`:\s*\(\s*\)\s*\{[^}]*:\s*\|\s*:[^}]*&[^}]*\}\s*;\s*:`)},

	// 远程下载即执行
	{"curl-pipe-sh", regexp.MustCompile(`(?i)\bcurl\b[^|;&\n]*\|\s*(sh|bash|zsh|ksh)\b`)},
	{"wget-pipe-sh", regexp.MustCompile(`(?i)\bwget\b[^|;&\n]*\|\s*(sh|bash|zsh|ksh)\b`)},

	// 敏感文件读取（与 fileRead SensitivePathDenyList 形成对偶）
	{"read-shadow", regexp.MustCompile(`(?i)\b(cat|less|more|head|tail|cp|mv|grep|awk|sed|tee)\b[^|;&\n]*\s/etc/shadow\b`)},
	{"read-ssh-private", regexp.MustCompile(`(?i)\b(cat|less|more|head|tail|cp|mv|grep|awk|sed|tee)\b[^|;&\n]*\s~?/?\.ssh/(id_|.*_rsa|.*_ed25519|.*_dsa)`)},
	{"read-aws-creds", regexp.MustCompile(`(?i)\b(cat|less|more|head|tail|cp|mv|grep|awk|sed|tee)\b[^|;&\n]*\s~?/?\.aws/credentials\b`)},

	// 用户/权限变更
	{"useradd", regexp.MustCompile(`(?i)\b(useradd|userdel|usermod)\b`)},
	{"passwd-root", regexp.MustCompile(`(?i)\bpasswd\s+root\b`)},
	{"chmod-777-root", regexp.MustCompile(`(?i)\bchmod\s+-R\s+0?777\s+/(\s|$)`)},

	// 进程级核打击
	{"kill-init", regexp.MustCompile(`(?i)\bkill\s+-9\s+1\b`)},
	{"shutdown", regexp.MustCompile(`(?i)\b(shutdown|reboot|halt|poweroff)\b`)},
}

// BashDenyList 工具持有的黑名单实例
// 基线 + 配置叠加；通过 NewBashDenyList 构造，避免每次 Match 重新编译正则
type BashDenyList struct {
	patterns []denyPattern
}

// NewBashDenyList 构造黑名单
// extra 来自 config.Native.BashCommandDenyList，按字符串编译失败的条目会被忽略
// 返回值携带的 patterns 包含基线 + extra 顺序
func NewBashDenyList(extra []string) *BashDenyList {
	all := make([]denyPattern, 0, len(defaultBashDenyPatterns)+len(extra))
	all = append(all, defaultBashDenyPatterns...)
	for i, raw := range extra {
		if raw == "" {
			continue
		}
		re, err := regexp.Compile(raw)
		if err != nil {
			// 不直接 panic：配置错误不应阻塞服务启动
			continue
		}
		all = append(all, denyPattern{
			Name:    "custom-" + strconv.Itoa(i),
			Pattern: re,
		})
	}
	return &BashDenyList{patterns: all}
}

// Match 返回首条命中规则的 Name；未命中返回空串
// 调用方：if name := dl.Match(cmd); name != "" { reject }
func (d *BashDenyList) Match(cmd string) string {
	for _, p := range d.patterns {
		if p.Pattern.MatchString(cmd) {
			return p.Name
		}
	}
	return ""
}
