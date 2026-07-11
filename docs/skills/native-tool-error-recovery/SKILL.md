---
name: native-tool-error-recovery
description: 原生工具错误恢复手册。当智能体接收到任何 role=tool 的消息，且其 content 以「工具调用失败：」开头时（来自 fileRead / fileEdit / fileWrite / bashExec 四个原生工具的执行错误），必须优先调用 loadSkill 读取本手册，按错误关键词匹配对应的诊断与恢复流程；覆盖「文件不存在 / 权限拒绝 / 敏感路径黑名单 / old_string 未匹配或非唯一 / 命令超时 SIGKILL / 命令被黑名单拦截 / 非零退出码」等常见场景。触发词：工具调用失败、tool call failed、native tool error、文件不存在、permission denied、old_string、命中黑名单、SIGKILL、退出码非零。
---

# 原生工具错误恢复 Skill

## 一、使用姿势

- 触发条件：下一轮上下文出现 `role=tool` 且 `content` 以 `工具调用失败：Error: ` 开头。
- 命中即 `loadSkill("native-tool-error-recovery")` 读本手册一次，**禁止** `executeSkill`。
- 诊断与修复全部走 `fileRead` / `fileEdit` / `fileWrite` / `bashExec` 四个原生工具自身。
- 重试无固定上限，但**每次必须变更策略**——参数一模一样的重试等同死循环。
- 级别语义：P0 参数自修正；P1 降级方案；P2 环境修复；P3 上报用户。

## 二、错误速查表

按错误消息中的关键片段命中一行，按「恢复」列执行。

| 工具 | 错误关键片段 | 根因 | 诊断 | 恢复 | 级别 |
|---|---|---|---|---|---|
| fileRead | `文件不存在:` | 路径拼错/大小写/相对路径基准错/文件未创建 | `bashExec ls -la <parent>`；`bashExec pwd` | 用真实名重发；本应产出则 `fileWrite` 补写；含用户输入路径无法自澄清则上报 | P0/P2/P3 |
| fileRead | `路径命中敏感路径黑名单:` | 落在敏感黑名单（`/etc/shadow` / SSH 私钥 / AWS 凭证） | 不诊断，直接停 | 立即停止，向用户说明并请求授权白名单路径；**禁止**用 `bashExec cat / base64` 绕过 | P3 |
| fileRead | `路径是目录而非文件:` | 把目录当文件读 | `bashExec ls -la <path>` | `bashExec find <path> -maxdepth 1 -type f` 列条目；明确文件名后重发 | P1/P0 |
| fileRead | `stat 失败:` / `permission denied` | 父目录缺 x 权限或权限链有缺口 | `bashExec ls -ld <parent>`；上溯逐级 `ls -ld` | workspace 内 `bashExec chmod +x <parent>` 补权限；系统/他用户目录直接上报 | P2/P3 |
| fileRead | `文件过大` / `字节 > 上限` | 超 10 MiB 或配置上限 | `bashExec wc -l <path>` | 分片 `fileRead` 用 `offset`+`limit`（每片 ≤2000 行）；或 `grep -n` 定位后按需读；全文需求改走 `bashExec cp/sed` | P1/P3 |
| fileRead | `包含 NUL 字节` / `不是合法的 UTF-8` | 二进制/GBK 编码/损坏 UTF-8 | `bashExec file <path>`；`bashExec head -c 32 <path> \| xxd` | 编码问题 `iconv -f gbk -t utf-8`；二进制 `strings <path> \| head`；图片/压缩包上报改工具链 | P1/P3 |
| fileEdit | `文件不存在:` | 目标文件还未创建 | — | **禁止**在不存在文件上 `fileEdit`；改调 `fileWrite` 从零创建 | P0 |
| fileEdit | `未在文件中找到匹配的 old_string:` | 含隐藏字符（tab/CRLF/尾空格）/大小写不一致/缓存原文过期 | `fileRead` 复读；`bashExec grep -n -F "<最短唯一子串>"`；`bashExec cat -A` 看行尾控制字符 | 以复读真实内容重构 `old_string`，对齐 tab/空白；仍不唯一改 `fileWrite` 全文重写（需允许覆盖） | P0/P1 |
| fileEdit | `old_string 在文件中出现了 N 次` | old_string 太短，多处能匹配 | 按错误消息给的匹配行号 `fileRead` 附近上下文 | 意图=改一处：扩 1-3 行上下文让唯一；意图=批量：加 `replace_all=true`；**禁止**原样重试 | P0 |
| fileEdit | `old_string 与 new_string 相同` | 原文当新文重传 | — | 若确无变更则跳过本次；有变更重造 `new_string` | P0 |
| fileEdit | `写入失败:` / `读取文件失败:` | 磁盘满/只读挂载/目录权限变/文件被占 | `bashExec df -h <parent>`；`bashExec mount \| grep <mp>`；`bashExec ls -ld <parent>` | 磁盘满：清 workspace 内明确临时产物（禁 `rm -rf`）；只读或权限：上报 | P2/P3 |
| fileWrite | `path parameter is required` / `参数解析失败` | 缺 path 或 JSON 结构损坏 | — | 补齐 `path` 与 `content`，确保 arguments 合法 JSON | P0 |
| fileWrite | `workspace 未初始化` / `messageId 未注入` / `conversationId 未注入` | 会话运行时上下文缺失，智能体无法自修 | — | 立即停止本轮写文件，向用户说明"会话上下文缺失"并等待人工 | P3 |
| fileWrite | `open <path>: no such file or directory` | 父目录不存在 | `bashExec ls -ld <parent>` | `bashExec mkdir -p <parent>`（仅限 workspace 或授权可写区，禁系统目录）后重试 | P2 |
| fileWrite | `open <path>: permission denied` | 父目录无写权限或文件无覆盖权限 | `bashExec ls -ld <parent>`；`bashExec ls -la <path>` | workspace 内：`chmod u+w <parent>`；或切到 workspace 可写路径；系统/他用户目录上报 | P2/P1/P3 |
| fileWrite | `open <path>: is a directory` | 同名目录占用路径 | — | 换不与目录同名的文件名重发；**禁止** `rm -rf` 删目录腾位 | P0 |
| fileWrite | `写入失败:` / `no space left on device` | 磁盘满或 quota 超限 | `bashExec df -h` | 仅清 workspace 内明确产物；非 workspace 空间不足上报 | P2/P3 |
| bashExec | `参数解析失败` / `command parameter is required` / `description parameter is required` | 缺必填参数 | — | 补齐 `command` 与 `description`（一句话意图，入审计） | P0 |
| bashExec | `command 长度 <n> 超过上限` | 单条命令 >16 KiB | — | 拆多次调用，中间文件传递；或把大命令写入脚本再 `bashExec bash /workspace/xxx.sh` | P1 |
| bashExec | `命令被拒绝：命中黑名单模式 <name>` | 命中安全黑名单 | 不诊断，直接换赛道 | 见第三节「bashExec 安全红线」；**禁止**参数微调重试 | P3 |
| bashExec | `命令超时` / `SIGKILL` / `exit=-1` | 超 `timeout_sec` 或系统上限（默认 30s，最大 300s） | 从 `merged_output` 看走到哪步被杀 | 合理场景显式提高 `timeout_sec`≤300；或拆子步骤落中间文件；能后台化则 `nohup ... > log 2>&1 &`；**禁止**原样重发 | P0/P1 |
| bashExec | 非零退出 + `command not found` | 二进制未装或 PATH 缺 | `bashExec which <cmd>`；`bashExec type <cmd>` | 安装到 workspace 或用等价内置（`awk`/`sed`/`python3`）替代 | P0/P2 |
| bashExec | 非零退出 + `No such file or directory` | 输入文件或目录不存在 | `bashExec ls -la <parent>` | 定位真实路径；`mkdir -p` 补父目录 | P2 |
| bashExec | 非零退出 + `Permission denied` | 无 x/w/r | `bashExec ls -l` | workspace 内 `chmod +x`；系统路径上报 | P2/P3 |
| bashExec | 非零退出 + `Address already in use` | 端口被占 | `bashExec lsof -i :<port>` | 换端口；**禁止** kill 非本次任务启动的进程 | P0/P1 |
| bashExec | 非零退出 + `Connection refused` / `timed out` | 目标服务未起或防火墙 | `curl -sS -o /dev/null -w '%{http_code}\n' <url>` 探活 | 服务未起先起服务；真连不上上报 | P1/P3 |
| bashExec | 非零退出 + `syntax error near unexpected token` | bash 语法错 | `bash -n <(echo "<cmd>")` 静态校验 | 重写命令，注意引号/反引号/`$()` | P0 |
| bashExec | 非零退出 + `Is a directory` / `Not a directory` | 路径类型判定错 | — | 换用适配对象类型的命令（`ls`/`cat`） | P0 |
| bashExec | 非零退出 + `Text file busy` / `Resource temporarily unavailable` | 文件被占用或句柄上限 | — | 延后一小步先跑别的命令再试；反复出现上报 | P1/P3 |
| bashExec | 非零退出 + stderr 为空 | 命令自身逻辑判定失败 | — | 追加 `2>&1` 或 `set -x` 收集信息后重试 | P0 |
| bashExec | `truncated=true` 输出截断 | 合并输出超 32 MiB cap | — | 加过滤（`\| grep`/`\| head -n 200`/`\| awk 'NR<=100'`）；或分段落盘到 workspace 后分片 `fileRead`；**禁止**不加过滤重试 | P0/P1 |

## 三、bashExec 安全红线

黑名单命中（`Error: 命令被拒绝：命中黑名单模式 <name>`）适用总原则，与规则细节无关：

- **禁绕过**：不用换行拼接、变量拼接、base64/hex 解码、eval、反引号、subshell 等等价手段规避。
- **禁参数微调**：`rm -rf /` → `rm -r /` → `rm -f /` 之类反复试等同恶意，命中即换赛道。
- **换安全等价命令**：
  - 远程脚本：`curl url \| sh` → 拆 `curl -o /workspace/<msgId>/x.sh url` +（**用户授权后**）`bash /workspace/<msgId>/x.sh`。
  - 精确删除：`rm -rf` → 明确文件名 `rm <file>` 或 `find <dir> -name '<pat>' -maxdepth 1 -delete`。
  - 权限修复：`chmod -R 0777` → 具体文件 `chmod 644 <file>` / 具体目录 `chmod 755 <dir>`。
- **敏感文件禁读**：`/etc/shadow` / SSH 私钥 / AWS 凭证任何形态的拷贝、grep、tee、cat 都会被拦。
- **策略枯竭必上报**：同一黑名单模式下切过 3 种等价命令仍失败，停止尝试，上报请求人工授权。

## 四、上报模板

黑名单拦截：

```
[黑名单拦截报告]
- 触发工具：bashExec
- 命中模式：<name>（如 curl-pipe-sh）
- 原始意图：<一句话>
- 已尝试策略：<列出实际尝试过的等价命令>
- 请求授权：<希望用户如何决策>
```

其它上报（系统级破坏 / 凭证读取 / 跨用户目录 / 策略枯竭 / 运行时上下文缺失）：

```
[原生工具错误上报]
- 工具：<fileRead / fileEdit / fileWrite / bashExec>
- 错误关键词：<引用错误消息中最能定位问题的短语>
- 已尝试的策略：<按时间顺序列出>
- 建议人工介入的具体动作：<一句话，避免开放式提问>
```

## 五、更新日志

- v0.2.0：主体重构为 6 列速查表；保留使用姿势、安全红线、上报模板三段辅助文本；覆盖原 v0.1.0 全部 23 个错误场景。
- v0.1.0：初始版本，长文档形态。
