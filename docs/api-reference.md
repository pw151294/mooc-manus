# mooc-manus 后端接口文档

> 本文档基于 `api/routers/route.go` 的路由注册与 `api/handlers/*.go`、`internal/applications/dtos/*.go` 的实际实现整理而成，按业务模块分组。所有接口默认走 Gin 框架，未特别声明的接口请求体均为 `application/json`，成功响应均为 HTTP `200 OK`。

## 目录

- [1. 通用约定](#1-通用约定)
- [2. 健康检查 (Status)](#2-健康检查-status)
- [3. AppConfig 应用配置](#3-appconfig-应用配置)
- [4. Tools 工具模块](#4-tools-工具模块)
- [5. Agent 智能体](#5-agent-智能体)
- [6. Flow 工作流](#6-flow-工作流)
- [7. Skill 技能主体](#7-skill-技能主体)
- [8. Skill Provider 技能提供方](#8-skill-provider-技能提供方)
- [9. Skill Import Task 导入任务](#9-skill-import-task-导入任务)
- [10. Skill Version 技能版本](#10-skill-version-技能版本)

---

## 1. 通用约定

### 1.1 错误响应

- 通用错误响应统一为 `{"error": "<错误信息>"}`，HTTP 状态码视错误类型而定。
- Skill 模块通过 `pkg/skillerr` 哨兵错误进行细分映射：
  - `ErrNotFound` → `404 Not Found`
  - `ErrDuplicate` → `409 Conflict`
  - `ErrInvalidInput` → `400 Bad Request`
  - 其他 → `500 Internal Server Error`
- 其他模块对参数错误返回 `400`，业务/系统错误返回 `500`。

### 1.2 通用成功响应

部分仅做副作用、无业务返回值的接口统一返回：

```json
{ "status": "success" }
```

### 1.3 流式响应

- Agent/Flow 的对话接口直接通过 `gin.Context.Writer` 流式写入，由各 ApplicationService 决定输出格式（SSE / 自定义事件流）。
- `Skill Import Task` 的 `detail` 接口为 `Server-Sent Events (text/event-stream)`，需保持长连接。

---

## 2. 健康检查 (Status)

路由前缀：`/api`

### 2.1 检查服务整体健康状态

- **Method / Path**：`GET /api/status`
- **Handler**：`StatusHandler.Check`
- **请求参数**：无
- **响应体**（`health_checker.HealthStatus`）：

```json
{
  "Service": "all",
  "Status": "healthy",
  "Detail": ""
}
```

- **HTTP 状态码**：
  - `200 OK`：所有检查通过（当前覆盖 Redis、Postgres）。
  - `503 Service Unavailable`：任意一项检查失败，`Detail` 字段携带聚合后的失败原因。

---

## 3. AppConfig 应用配置

路由前缀：`/api/app/config`

`AppConfig` 同时管理"模型/Agent 参数"和"A2A Server 配置"。

### 3.1 创建 AppConfig

- **Method / Path**：`POST /api/app/config`
- **Handler**：`AppConfigHandler.Add`
- **请求体**（`AppConfigCreateRequest`）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| baseUrl | string | 是 | 模型服务 BaseURL |
| apiKey | string | 是 | 模型 API Key |
| modelName | string | 是 | 模型名称 |
| temperature | float64 | 否 | 默认 0.7 |
| maxTokens | int64 | 否 | 默认 8192 |
| maxIterations | int | 否 | Agent 最大迭代次数，默认 100 |
| maxRetries | int | 否 | Agent 最大重试次数，默认 3 |
| maxSearchResults | int | 否 | 检索结果上限，默认 10 |

- **响应体**：

```json
{ "id": "<新建AppConfig的UUID>" }
```

### 3.2 修改 AppConfig

- **Method / Path**：`PUT /api/app/config/:id`
- **Handler**：`AppConfigHandler.Update`
- **路径参数**：`id` —— AppConfig ID。
- **请求体**（`AppConfigUpdateRequest`，路径中的 `id` 会回填到 `appConfigId`）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| appConfigId | string | 是 | 与路径参数一致 |
| baseUrl | string | 是 | — |
| apiKey | string | 否 | 不传则保留原值 |
| modelName | string | 是 | — |
| temperature / maxTokens / maxIterations / maxRetries / maxSearchResults | — | 否 | 同上 |

- **响应**：`{"status": "success"}`

### 3.3 查询单个 AppConfig

- **Method / Path**：`GET /api/app/config/:id`
- **Handler**：`AppConfigHandler.Get`
- **路径参数**：`id` —— AppConfig ID。
- **响应体**（`AppConfigDTO`）：

```json
{
  "appConfigId": "...",
  "baseUrl": "...",
  "modelName": "...",
  "temperature": 0.7,
  "maxTokens": 8192,
  "maxIterations": 100,
  "maxRetries": 3,
  "maxSearchResults": 10
}
```

> 注意：响应体不会回传 `apiKey`。

### 3.4 列出全部 AppConfig

- **Method / Path**：`GET /api/app/config`
- **Handler**：`AppConfigHandler.List`
- **响应体**：`AppConfigDTO[]`

### 3.5 删除 AppConfig

- **Method / Path**：`DELETE /api/app/config/:id`
- **Handler**：`AppConfigHandler.Delete`
- **路径参数**：`id`
- **响应**：`{"status": "success"}`

## 4. Tools 工具模块

路由前缀：`/api/tools`

工具模块分为 **Provider（工具提供方）** 与 **Function（工具函数）** 两个子域。

### 4.1 新增工具 Provider

- **Method / Path**：`POST /api/tools/provider`
- **Handler**：`ToolHandler.AddProvider`
- **请求体**（`AddToolProviderRequest`）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| providerName | string | 是 | — |
| providerDesc | string | 否 | — |
| providerType | string | 是 | 例如 `MCP` / 自定义 |
| providerUrl | string | 否 | — |
| providerTransport | string | 否 | 例如 `streamable_http` |

- **响应**：`{"status": "success"}`

### 4.2 更新工具 Provider

- **Method / Path**：`PUT /api/tools/provider/:id`
- **Handler**：`ToolHandler.UpdateProvider`
- **路径参数**：`id` —— Provider ID。
- **请求体**（`UpdateToolProviderRequest`，结构同上 + `providerId`，路径 `id` 会回填）。
- **响应**：`{"status": "success"}`

### 4.3 删除工具 Provider

- **Method / Path**：`DELETE /api/tools/provider/:id`
- **Handler**：`ToolHandler.DeleteProvider`
- **路径参数**：`id`
- **响应**：`{"status": "success"}`

### 4.4 列出全部工具 Provider

- **Method / Path**：`GET /api/tools/provider/list`
- **Handler**：`ToolHandler.ListProviders`
- **响应体**（`ToolProviderDTO[]`）：

```json
[
  {
    "providerId": "...",
    "providerName": "...",
    "providerDesc": "...",
    "providerType": "...",
    "providerUrl": "...",
    "providerTransport": "..."
  }
]
```

### 4.5 新增 Tool Function

- **Method / Path**：`POST /api/tools/function`
- **Handler**：`ToolHandler.AddFunction`
- **请求体**（`AddToolFunctionRequest`）：

```json
{
  "providerId": "...",
  "functionName": "...",
  "functionDesc": "...",
  "parameters": {
    "type": "object",
    "properties": {
      "fieldA": { "type": "string", "description": "...", "required": true }
    }
  }
}
```

- **校验**：`providerId` 必须存在，否则返回 `400 provider not found`。
- **响应**：`{"status": "success"}`

### 4.6 通过 MCP 批量添加 Function

- **Method / Path**：`POST /api/tools/function/mcp`
- **Handler**：`ToolHandler.AddMcpFunctions`
- **请求体**（`AddMcpFunctionsRequest`）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| providerId | string | 否 | 已有 Provider，可与 `providerName` 二选一 |
| providerName | string | 是 | 自动创建/复用的 Provider 名 |
| providerDesc | string | 否 | — |
| providerUrl | string | 否 | MCP server URL |

- **响应**：`{"status": "success"}`

### 4.7 更新 Tool Function

- **Method / Path**：`PUT /api/tools/function/:id`
- **Handler**：`ToolHandler.UpdateFunction`
- **路径参数**：`id` —— Function ID。
- **请求体**（`UpdateToolFunctionRequest`，结构同 4.5 加 `functionId`，路径 `id` 会回填）。
- **响应**：`{"status": "success"}`

### 4.8 删除 Tool Function

- **Method / Path**：`DELETE /api/tools/function/:id`
- **Handler**：`ToolHandler.DeleteFunction`
- **路径参数**：`id`
- **响应**：`{"status": "success"}`

### 4.9 按 Provider 查询 Function 列表

- **Method / Path**：`GET /api/tools/function/list?providerId=<id>`
- **Handler**：`ToolHandler.ListFunctionsByProvider`
- **Query 参数**：`providerId` —— 必填。
- **响应体**（`ToolFunctionDTO[]`）：

```json
[
  {
    "functionId": "...",
    "providerId": "...",
    "functionName": "...",
    "functionDesc": "...",
    "properties": {
      "field": { "type": "string", "description": "...", "required": true }
    }
  }
]
```

---

## 5. Agent 智能体

路由前缀：`/api/agent`

所有 Agent 接口均为流式输出（事件流），响应体由 ApplicationService 通过 `c.Writer` 直接写出，调用方按行/分块解析。

### 5.1 Agent 对话

- **Method / Path**：`POST /api/agent/chat`
- **Handler**：`AgentHandler.Chat`
- **请求体**（`ChatClientRequest`）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| streaming | bool | 是否流式 |
| apiKey | string | 可覆盖 AppConfig 中的 apiKey |
| systemPrompt | string | 系统提示词 |
| query | string | 用户输入 |
| conversationId | string | 会话 ID |
| appConfigId | string | 使用的 AppConfig |
| functionIds | string[] | 启用的 Function ID 列表 |
| providerIds | string[] | 启用的 Provider ID 列表 |
| skillRefs | SkillRef[] | `{skillId, version, skillName}` 数组，引用的 Skill |
| file | any[] | 关联文件，序列化为 `file.File` |

- **响应**：流式输出（由 BaseAgentApplicationService 决定具体协议）。


## 7. Skill 技能主体

路由前缀：`/api/skill`

Skill 主体共 9 个接口，覆盖草稿暂存、发布、CRUD、列表、详情、文件下载等。所有非文件接口默认 JSON；`draft/save` 与 `publish` 使用 `multipart/form-data`。

### 7.1 草稿暂存

- **Method / Path**：`POST /api/skill/draft/save`
- **Handler**：`SkillHandler.DraftSave`
- **Content-Type**：`multipart/form-data`
- **请求字段**：

| 表单字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| skillId | string | 否 | 为空时新建 |
| icon | string | 否 | Icon JSON 字符串（`{icon, iconBackground, iconType}`） |
| imageUrl | string | 否 | — |
| skillFiles | string(JSON) | 是 | `SkillFileStructure[]` 的 JSON 序列化字符串 |
| files | file[] | 否 | 多文件上传，与 `skillFiles` 中条目对应 |

- **限制**：单次上传总大小 ≤ 100MB（`SkillImportMaxFileSize`）。
- **响应体**（`SkillInfo`）：见 7.6 中字段定义。

### 7.2 发布

- **Method / Path**：`POST /api/skill/publish`
- **Handler**：`SkillHandler.Publish`
- **Content-Type**：`multipart/form-data`
- **请求字段**：在 7.1 基础上增加：

| 表单字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| providerId | string | 否 | 归属 Provider |
| versionDescription | string | 否 | 版本说明 |

- **响应体**：`SkillInfo`。

### 7.3 更新元数据

- **Method / Path**：`POST /api/skill/update`
- **Handler**：`SkillHandler.Update`
- **请求体**（`SkillUpdateRequest`）：

```json
{
  "skillId": "...",
  "skillName": "...",
  "description": "...",
  "status": "..."
}
```

- **响应体**：`SkillInfo`。

### 7.4 删除

- **Method / Path**：`POST /api/skill/delete`
- **Handler**：`SkillHandler.Delete`
- **请求体**：`{"skillId": "..."}`
- **响应**：`{"status": "success"}`

### 7.5 分页列表

- **Method / Path**：`POST /api/skill/list`
- **Handler**：`SkillHandler.List`
- **请求体**（`SkillListRequest`）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| providerId | string | 过滤指定 Provider |
| skillName | string | 精确匹配 |
| keyword | string | 模糊匹配（最长 64） |
| status | string | 过滤状态 |
| pageNum | int | 默认 1 |
| pageSize | int | 默认 10，最大 1000 |

- **响应体**（`SkillPageDTO`）：

```json
{
  "total": 0,
  "pageSize": 10,
  "pageNum": 1,
  "records": [ /* SkillInfo[] */ ]
}
```

### 7.6 全量列表

- **Method / Path**：`POST /api/skill/listAll`
- **Handler**：`SkillHandler.ListAll`
- **请求体**（`SkillListAllRequest`）：去掉分页字段的 7.5。
- **响应体**：`SkillInfo[]`，其中 `SkillInfo` 结构为：

```json
{
  "skillId": "...",
  "skillProviderId": "...",
  "providerName": "...",
  "skillName": "...",
  "description": "...",
  "icon": { "icon": "...", "iconBackground": "...", "iconType": "..." },
  "imageUrl": "...",
  "status": "draft|published|...",
  "latestVersion": { /* SkillVersionInfo */ },
  "versionCount": 0,
  "files": [ /* models.SkillFile */ ],
  "versions": [ /* SkillVersionInfo[] */ ],
  "createdAt": "...",
  "updatedAt": "..."
}
```

### 7.7 详情

- **Method / Path**：`POST /api/skill/detail`
- **Handler**：`SkillHandler.Detail`
- **请求体**：`{"skillId": "..."}`
- **响应体**：`SkillInfo`（含 `files`、`versions`、`latestVersion`）。

### 7.8 编辑态选择器（Skill + 版本）

- **Method / Path**：`POST /api/skill/with/version`
- **Handler**：`SkillHandler.WithVersion`
- **请求体**：可不传或传空对象（`SkillWithVersionRequest`）。
- **响应体**（`SkillWithVersionInfo[]`）：

```json
[
  {
    "skillId": "...",
    "skillProviderId": "...",
    "providerName": "...",
    "skillName": "...",
    "description": "...",
    "icon": { },
    "imageUrl": "...",
    "status": "...",
    "versionCount": 0,
    "versions": [ /* SkillVersionInfo[] */ ],
    "creator": "...",
    "createdAt": "...",
    "updatedAt": "..."
  }
]
```

### 7.9 下载文件

- **Method / Path**：`GET /api/skill/file/download?fileKey=<key>`
- **Handler**：`SkillHandler.FileDownload`
- **Query**：`fileKey`（必填，OSS 对象 Key）。
- **响应**：
  - `Content-Type: application/octet-stream`
  - `Content-Disposition: attachment; filename="<urlencoded>"`
  - 响应体为二进制文件流。

---

## 8. Skill Provider 技能提供方

路由前缀：`/api/skill/provider`

### 8.1 Git 仓库导入

- **Method / Path**：`POST /api/skill/provider/import/git`
- **Handler**：`SkillHandler.ProviderImportGit`
- **请求体**（`ImportGitRequest`）：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| providerName | string | 是 | — |
| repoUrl | string | 是 | Git URL |
| authType | string | 否 | 默认 `NONE`，支持 `HTTP_TOKEN` 等 |
| authToken | string | 否 | 当 `authType=HTTP_TOKEN` 时使用 |

- **响应体**：`SkillProviderInfo`，结构见 8.7。

### 8.2 ZIP 异步导入

- **Method / Path**：`POST /api/skill/provider/import/zip`
- **Handler**：`SkillHandler.ProviderImportZip`
- **Content-Type**：`multipart/form-data`
- **表单字段**：`file`（必填，单文件，后缀仅支持 `.zip` / `.tar.gz` / `.tgz`，大小上限 100MB）。
- **响应体**：

```json
{ "taskId": "<异步任务 ID>" }
```

- **后续**：使用 9.1 订阅任务进度。

### 8.3 ZIP 旧版同步导入

- **Method / Path**：`POST /api/skill/provider/import/zip/legacy`
- **Handler**：`SkillHandler.ProviderImportZipLegacy`
- **请求体**（`ImportZipLegacyRequest`）：

```json
{ "providerName": "..." }
```

- **响应体**：`SkillProviderInfo`。

### 8.4 同步 Provider

- **Method / Path**：`POST /api/skill/provider/sync`
- **Handler**：`SkillHandler.ProviderSync`
- **请求体**：`{"providerId": "..."}`
- **响应体**：`SkillProviderInfo`。

### 8.5 删除 Provider

- **Method / Path**：`POST /api/skill/provider/delete`
- **Handler**：`SkillHandler.ProviderDelete`
- **请求体**：`{"providerId": "..."}`
- **响应**：`{"status": "success"}`

### 8.6 Provider 列表

- **Method / Path**：`POST /api/skill/provider/list`
- **Handler**：`SkillHandler.ProviderList`
- **请求体**（`ProviderListRequest`）：

```json
{ "providerType": "", "status": "" }
```

- **响应体**：`SkillProviderInfo[]`。

### 8.7 Provider 详情

- **Method / Path**：`POST /api/skill/provider/detail`
- **Handler**：`SkillHandler.ProviderDetail`
- **请求体**：`{"providerId": "..."}`
- **响应体**（`SkillProviderInfo`）：

```json
{
  "skillProviderId": "...",
  "providerName": "...",
  "providerType": "git|zip|...",
  "authType": "NONE|HTTP_TOKEN|...",
  "repoUrl": "...",
  "status": "active|...",
  "skillCount": 0,
  "creator": "...",
  "createdAt": "...",
  "updatedAt": "..."
}
```

---

## 9. Skill Import Task 导入任务

路由前缀：`/api/skill/provider/import/task`

### 9.1 任务详情（SSE 订阅）

- **Method / Path**：`POST /api/skill/provider/import/task/detail`
- **Handler**：`SkillHandler.ImportTaskDetail`
- **请求体**：`{"taskId": "..."}`
- **响应**：`Content-Type: text/event-stream`，连接保持直到客户端断开。每条事件格式：

```
data: <SkillImportEventData JSON>\n\n
```

- **`SkillImportEventData`**：

```json
{
  "taskId": "...",
  "status": "pending|running|success|failed|...",
  "stage": "...",
  "progress": 0,
  "logs": [
    { "time": "2026-06-26T00:00:00Z", "level": "INFO", "message": "..." }
  ],
  "skillCount": 0,
  "providerId": "...",
  "errorMessage": ""
}
```

### 9.2 任务列表

- **Method / Path**：`POST /api/skill/provider/import/task/list`
- **Handler**：`SkillHandler.ImportTaskList`
- **请求体**：无业务字段（可传空 JSON `{}`）。
- **响应体**（`SkillImportTaskInfo[]`）：

```json
[
  {
    "taskId": "...",
    "fileName": "...",
    "fileSize": 0,
    "status": "...",
    "stage": "...",
    "progress": 0,
    "skillCount": 0,
    "providerId": "...",
    "createdAt": "..."
  }
]
```

### 9.3 删除任务

- **Method / Path**：`POST /api/skill/provider/import/task/delete`
- **Handler**：`SkillHandler.ImportTaskDelete`
- **请求体**：

```json
{ "taskIds": ["...", "..."] }
```

- **响应**：`{"status": "success"}`

---

## 10. Skill Version 技能版本

路由前缀：`/api/skill/version`

### 10.1 程序化创建版本

- **Method / Path**：`POST /api/skill/version/create`
- **Handler**：`SkillHandler.VersionCreate`
- **请求体**（`VersionCreateRequest`）：

```json
{ "skillId": "...", "version": "1.0.0" }
```

- **响应体**：`SkillVersionInfo`，见 10.5。

### 10.2 版本校验 / 标记为最新

- **Method / Path**：`POST /api/skill/version/validate`
- **Handler**：`SkillHandler.VersionValidate`
- **请求体**（`VersionValidateRequest`）：

```json
{ "versionId": "...", "testInputs": { } }
```

> `testInputs` 字段当前未消费，仅做预留。

- **响应体**：`SkillVersionInfo`。

### 10.3 删除版本

- **Method / Path**：`POST /api/skill/version/delete`
- **Handler**：`SkillHandler.VersionDelete`
- **请求体**：

```json
{ "skillId": "...", "version": "..." }
```

- **响应**：`{"status": "success"}`

### 10.4 版本列表

- **Method / Path**：`POST /api/skill/version/list`
- **Handler**：`SkillHandler.VersionList`
- **请求体**：`{"skillId": "..."}`
- **响应体**：`SkillVersionInfo[]`（已发布版本，倒序）。

### 10.5 版本详情

- **Method / Path**：`POST /api/skill/version/detail`
- **Handler**：`SkillHandler.VersionDetail`
- **请求体**：

```json
{ "skillId": "...", "version": "..." }
```

- **响应体**（`SkillVersionInfo`）：

```json
{
  "skillVersionId": "...",
  "skillId": "...",
  "skillName": "...",
  "version": "1.0.0",
  "description": "...",
  "metadata": {
    "name": "...",
    "description": "...",
    "author": "...",
    "version": "..."
  },
  "skillFiles": [
    { "path": "...", "fileKey": "...", "suffix": "...", "size": 0, "checksum": "..." }
  ],
  "zipFilePath": "...",
  "creator": "...",
  "publishedBy": "...",
  "createdAt": "...",
  "updatedAt": "..."
}
```

### 10.6 获取最新版本

- **Method / Path**：`POST /api/skill/version/latest`
- **Handler**：`SkillHandler.VersionLatest`
- **请求体**：`{"skillId": "..."}`
- **响应体**：`SkillVersionInfo`。

### 10.7 回滚版本

- **Method / Path**：`POST /api/skill/version/rollback`
- **Handler**：`SkillHandler.VersionRollback`
- **请求体**（`VersionRollbackRequest`）：

```json
{
  "skillId": "...",
  "targetVersion": "1.2.3",
  "pubVersionType": "patch"
}
```

> 当前实现固定按 `patch` 递增，`pubVersionType` 仅作占位。

- **响应体**：`SkillVersionInfo`（新版本）。

### 10.8 导出版本 ZIP

- **Method / Path**：`POST /api/skill/version/export`
- **Handler**：`SkillHandler.VersionExport`
- **请求体**：

```json
{ "skillId": "...", "version": "..." }
```

- **响应**：
  - `Content-Type: application/zip`
  - `Content-Disposition: attachment; filename="<skillName>-v<version>.zip"`
  - 响应体为 ZIP 文件流，错误时若 ZIP 头未写出则返回 `500 {"error": "..."}`，已写出后流可能被截断，客户端需自行校验。

---

## 附录：常用 DO/DTO 结构索引

| 名称 | 文件 | 说明 |
| --- | --- | --- |
| `AppConfigDTO` / `*Request` | `internal/applications/dtos/app_config.go` | AppConfig 模型/Agent 参数 |
| `A2AServerConfigDTO` / `*Request` | `internal/applications/dtos/a2a_server_config.go` | A2A Server 配置 |
| `ToolProviderDTO` / `*Request` | `internal/applications/dtos/tool_provider.go` | 工具 Provider |
| `ToolFunctionDTO` / `*Request` | `internal/applications/dtos/tool_function.go` | 工具 Function |
| `ChatClientRequest` / `AgentPlan*Request` | `internal/applications/dtos/agent.go` | Agent / Flow 请求 |
| `SkillInfo` / `SkillPageDTO` / `*Request` | `internal/applications/dtos/skill.go` | Skill 主体 |
| `SkillProviderInfo` / `*Request` | `internal/applications/dtos/skill_provider.go` | Skill Provider |
| `SkillVersionInfo` / `*Request` | `internal/applications/dtos/skill_version.go` | Skill 版本 |
| `SkillImportTaskInfo` / `SkillImportEventData` | `internal/applications/dtos/skill_import_task.go` | 异步导入任务 |
| `HealthStatus` | `internal/infra/external/health_checker/health_checker.go` | 健康检查响应 |

---

**最后更新：** 2026-06-26  
**生成依据：** `api/routers/route.go` + `api/handlers/*.go` + `internal/applications/dtos/*.go`
