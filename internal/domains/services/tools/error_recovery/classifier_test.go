package error_recovery

import "testing"

func TestClassify_NonNativeTool(t *testing.T) {
	d := Classify("loadSkill", "任何消息")
	if d.Level != LevelNone {
		t.Fatalf("非原生工具应返回 LevelNone, got %v", d.Level)
	}
}

func TestClassify_KeywordMatches(t *testing.T) {
	cases := []struct {
		name        string
		toolName    string
		message     string
		wantLevel   Level
		wantTplKey  string
	}{
		// L3 致命
		{"sensitive_path", "fileRead", "Error: 路径命中敏感路径黑名单: /etc/shadow", LevelFatal, "sensitive_path"},
		{"denylist_hit", "bashExec", "Error: 命令被拒绝:命中黑名单模式 rm-rf-root", LevelFatal, "denylist_hit"},
		{"cmd_timeout", "bashExec", "Error: 命令超时(120s)被 SIGKILL", LevelFatal, "cmd_timeout"},
		{"cmd_too_long", "bashExec", "Error: command 长度 20480 超过上限 16384", LevelFatal, "cmd_too_long"},
		{"disk_full", "fileWrite", "Error: 写入失败: write /workspace/x: no space left on device", LevelFatal, "disk_full"},
		{"readonly_fs", "fileEdit", "Error: 写入失败: rename: read-only file system", LevelFatal, "readonly_fs"},
		{"oom_killed", "bashExec", "exit=137, truncated=false\nKilled", LevelFatal, "oom_killed"},
		{"binary_file", "fileRead", "Error: 文件包含 NUL 字节,疑似二进制文件", LevelFatal, "binary_file"},
		{"not_utf8", "fileRead", "Error: 文件不是合法的 UTF-8 文本", LevelFatal, "not_utf8"},
		{"file_too_large", "fileRead", "Error: 文件过大 (20971520 字节 > 上限 10485760 字节)", LevelFatal, "file_too_large"},
		{"no_message_id", "fileWrite", "Error: messageId 未注入,无法定位 workspace", LevelFatal, "no_message_id"},

		// L2 需追问
		{"path_empty_required", "fileRead", "Error: path parameter is required", LevelAskUser, "path_empty"},
		{"path_escape_dotdot", "fileWrite", "Error: 不允许 ../ 或绝对路径", LevelAskUser, "path_escape"},
		{"args_parse_fail", "fileEdit", "Error: 参数解析失败", LevelAskUser, "args_parse_fail"},
		{"port_busy", "bashExec", "exit=1, truncated=false\nbind: Address already in use\n(执行失败:exit status 1)", LevelAskUser, "port_busy"},
		{"desc_missing", "bashExec", "Error: description parameter is required(请简述命令用途,将记入审计日志)", LevelAskUser, "desc_missing"},

		// L1 自愈
		{"edit_ambiguous", "fileEdit", "Error: old_string 在文件中出现了 3 次(前 3 个匹配行号:[10 20 30])", LevelSelfHeal, "edit_ambiguous"},
		{"edit_no_match", "fileEdit", "Error: 未在文件中找到匹配的 old_string:foo.go", LevelSelfHeal, "edit_no_match"},
		{"is_directory", "fileRead", "Error: 路径是目录而非文件: /workspace/dir", LevelSelfHeal, "is_directory"},
		{"dir_missing", "fileWrite", "Error: 打开文件失败: open /workspace/a/b.txt: no such file or directory", LevelSelfHeal, "dir_missing"},
		{"perm_denied_open", "fileWrite", "Error: 打开文件失败: open /root/x: permission denied", LevelSelfHeal, "perm_denied"},
		{"file_not_found", "fileEdit", "Error: 文件不存在: not_exist.txt", LevelSelfHeal, "file_not_found"},
		{"cmd_not_found", "bashExec", "exit=127, truncated=false\nbash: xzz: command not found\n(执行失败:exit status 127)", LevelSelfHeal, "cmd_not_found"},
		{"dep_missing_python", "bashExec", "exit=1, truncated=false\nModuleNotFoundError: No module named 'requests'", LevelSelfHeal, "dep_missing"},
		{"env_missing", "bashExec", "exit=1, truncated=false\nbash: TOKEN: unbound variable", LevelSelfHeal, "env_missing"},
		{"io_transient_sniff", "fileRead", "Error: read sniff failed: input/output error", LevelSelfHeal, "io_transient"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.toolName, tc.message)
			if got.Level != tc.wantLevel {
				t.Fatalf("level 不匹配 want=%v got=%v", tc.wantLevel, got.Level)
			}
			if got.TemplateKey != tc.wantTplKey {
				t.Fatalf("templateKey 不匹配 want=%s got=%s", tc.wantTplKey, got.TemplateKey)
			}
			if got.Template == "" {
				t.Fatalf("template 文本不应为空")
			}
		})
	}
}

func TestClassify_GenericFallback(t *testing.T) {
	// 未命中任何关键字的 4 类原生工具失败,应兜底 L1 通用模板
	d := Classify("bashExec", "some totally unknown error message")
	if d.Level != LevelSelfHeal {
		t.Fatalf("未命中应兜底 L1, got %v", d.Level)
	}
	if d.TemplateKey != "generic_l1_fallback" {
		t.Fatalf("未命中应返回 generic_l1_fallback, got %s", d.TemplateKey)
	}
}

func TestClassify_ToolScopeFilter(t *testing.T) {
	// fileRead 出现"No module named"不应命中 bashExec 专属的 dep_missing
	d := Classify("fileRead", "Error: read rest failed: No module named x")
	if d.TemplateKey == "dep_missing" {
		t.Fatalf("dep_missing 仅对 bashExec 生效,不应命中 fileRead")
	}
}

func TestLevelPrefix(t *testing.T) {
	if LevelSelfHeal.Prefix() != "[ErrorRecovery-L1] " {
		t.Fatalf("L1 prefix 不正确")
	}
	if LevelAskUser.Prefix() != "[ErrorRecovery-L2] " {
		t.Fatalf("L2 prefix 不正确")
	}
	if LevelFatal.Prefix() != "[ErrorRecovery-L3] " {
		t.Fatalf("L3 prefix 不正确")
	}
	if LevelNone.Prefix() != "" {
		t.Fatalf("None prefix 应为空")
	}
}

func TestSkillMDEmbed(t *testing.T) {
	if SkillName() != BuiltInSkillName {
		t.Fatalf("SkillName 不匹配 embed frontmatter: got %s want %s", SkillName(), BuiltInSkillName)
	}
	if SkillDescription() == "" {
		t.Fatalf("SkillDescription 不应为空")
	}
	if SkillMD() == "" {
		t.Fatalf("SkillMD 原文不应为空")
	}
}
