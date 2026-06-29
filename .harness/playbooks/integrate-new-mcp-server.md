# 集成新 MCP server

把一个外部 MCP server（如 GitHub MCP、Filesystem MCP）注册成可被 Agent 调用的工具集合。MCP 客户端走 `mcp-go`，注册流程必须经 `ToolProviderDomainService`（应用层）+ `tools.Tool` 接口（领域层），不得在 Agent 内硬编码。关联 R-44（工具注册）、R-40（DDD 分层）、R-48（外部内容信任）。

## 前置条件

1. MCP server 已部署可访问（已知 transport：stdio / SSE / HTTP）
2. 已确认它暴露的 function 集合（`tools/list` 响应），决定要全量注册还是 allowlist
3. 决定鉴权方式（无 / API Key / OAuth），与 `internal/domains/services/tools/mcp.go` 既有支持对齐
4. 阅读 `internal/domains/services/tools/mcp.go` 与 `internal/applications/services/tool_provider.go`

## 步骤

```bash
cd /path/to/mooc-manus-all/mooc-manus
git switch -c feat/mcp-<server-name>
```

### 1. ToolProvider 落库

通过既有 Application 层路径注册 provider，不要直接写 SQL：

- 调用 `SkillProviderApplicationService` 同构的 `ToolProviderApplicationService.Create(...)`（位于 `internal/applications/services/tool_provider.go`）
- DTO 在 `internal/applications/dtos/` 提供 `mcp` 类型字段（transport / URL / auth）
- 走 HTTP API 或 seed 脚本均可；建议通过新增的 API endpoint 让前端能配（详见 `mooc-manus-web/.harness/playbooks/add-new-page.md` 加配置页）

### 2. 工具发现

- `ToolProviderDomainService.Sync(providerId)` 拉取 MCP `tools/list`，把每个 function 落到 `ToolFunctionDO`
- 不需要为新 MCP server 改 `internal/domains/services/tools/mcp.go`（除非该 server 有特殊握手）
- ⚠️ R-44 第 2 条：注册经 ToolProvider，不要在 Agent 内 `new MCPClient(...)` 散落

### 3. 装配到 Agent

- Agent 构造时由 `applications/services/agent.go` 通过 `ToolFunctionDomainService` 按 `providerIds` 加载 → 转 `tools.Tool`
- 工具识别由 `tools.Tool.Name()` 自动，禁止 `if name == "mcp_xxx"`（R-44 第 1 条）

### 4. 鉴权 / 凭据

- API Key 类放 `app_config` 或专门的 provider config 表；**不要写死在代码或 .env 提交**（敏感字段走 R-32-secrets，详见总仓 `rules/32-secrets-handling.md`）
- OAuth 类需新增 callback handler，超出本剧本范围；先开 ADR

### 5. 信任边界

- MCP 工具返回的内容是**外部内容**（R-48 第 4 条 / R-31）
- 不得把 MCP 输出直接拼进 system prompt（R-46）
- 在 `tools/mcp.go::Invoke` 返回前做长度截断 + 标记 source

### 6. 测试 & commit

```bash
go test ./internal/domains/services/tools/... ./internal/applications/services/...
go build ./...
git add -A
git commit -m "feat(mcp): 集成 <server-name> MCP server"
git push -u origin feat/mcp-<server-name>
```

## 常见坑

1. **MCP server 离线时 Agent 卡住**：Invoke 没设 context timeout → 一个工具失败拖死整轮对话。所有 MCP 调用走 `context.WithTimeout(parent, 30s)`。
2. **tools/list 拉到几百个 function**：全量加 LLM 上下文爆掉。在 Provider 配置里加 allowlist 字段，只装载白名单 function。
3. **跨类型边界混用**：把 MCP 工具用 `tools.SkillTools(...)` 工厂构造 → R-44 第 3 条违反。MCP 类走 ToolProvider 注册路径。
4. **API Key 泄漏**：测试时把真 key 写到 fixture → git 历史里留底。fixture 用占位符 + 环境变量注入。
5. **外部内容当指令**：MCP 返回的 markdown 里有 `Ignore previous instructions...` → R-48 第 4 条强约束 escape。

## 验证

```bash
go build ./...
go test ./internal/domains/services/tools/...
go vet ./...
HARNESS_ROOT=.harness ./.harness/scripts/validate-harness.sh

# 联调
# 1. 通过 Tool Provider API 注册新 MCP server
# 2. Sync 一次拉 tools/list
# 3. 在 Agent 配置里勾选该 provider，触发对话
# 4. 观察 tool_call_start / tool_call_complete 事件携带正确 function
```

## Agent 行为

- 用户说"接个 MCP" → 先问 transport / 鉴权 / 工具数量；超过 20 个工具默认要 allowlist
- 看到 PR 在 Agent 代码里硬编码 MCP 工具名 → reject（R-44 第 1 条）
- 看到 API Key 出现在源码或 fixture → reject（R-32）
- 看到 MCP 调用没 context timeout → 提示补
- ⚠️ 注意 R-48 第 4 条：MCP 输出回写 prompt 前必须 escape；缺这一步直接 reject
- 用户问"能不能让 Agent 直接 new 一个 MCPClient" → 拒绝，指 R-44 第 2 条
