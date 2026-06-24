# Skill 配置 & 版本管理业务结构化规范文档

---

## 1. 业务概述

### 1.1 模块定位

Skill 模块用于在 Beedance 平台上沉淀「可复用的执行类技能资产」。每个 Skill 由一份元数据描述文件（SKILL.md）和若干脚本/资源文件组成，可被智能体（Agent）、工作流（Workflow）等上层应用引用调用。

模块由三类业务对象组成：

| 业务对象 | 含义 |
|---------|------|
| Skill 提供者（Skill Provider） | Skill 的来源/归集方，对应一次外部导入或一个内置容器，用于把 Skill 分组管理 |
| Skill | 一个具名的、可被引用的能力单元，对应一项可执行技能 |
| Skill 版本（Skill Version） | 同一个 Skill 的某次「快照」，包含该次提交的全部文件、元数据、版本号、ZIP 包等内容 |

### 1.2 核心业务场景

1. **Skill 提供者管理**：通过 Git 仓库导入、ZIP 压缩包导入（同步/异步两种）、自定义内置创建三种方式，登记一个 Skill 来源。
2. **Skill 草稿编辑**：在线编辑 Skill 的文件、图标、描述、标签等信息，保存为「草稿版本（draft）」。
3. **Skill 发布**：把当前草稿固化为一个新的正式版本，自动按语义化版本递增版本号，并同步将文件复制到正式版本目录、生成 ZIP 包。
4. **Skill 版本管理**：分页查询版本列表、查看版本详情、获取最新已验证版本、删除指定版本、单独验证版本、版本回滚（以历史版本内容生成新版本）、版本导出为 ZIP。
5. **Skill 详情/列表**：分页查询、不分页查询、含版本列表查询、详情查询，支持按提供者、名称模糊、关键字、状态、标签筛选。
6. **Skill 删除**：物理删除 Skill 及其全部版本、标签关联、对象存储文件。
7. **导入任务追踪**：ZIP 异步导入会生成导入任务，前端通过 SSE 订阅、列表查询、批量删除任务，并查看每个阶段的进度日志。
8. **文件下载**：根据对象存储 fileKey 直接下载 Skill 内文件。

### 1.3 上下游依赖

| 依赖方向 | 依赖对象 | 用途 |
|---------|---------|------|
| 下游依赖（Skill 模块依赖） | 对象存储（Bucket：`beedance-skill`） | Skill 文件、版本目录文件、版本 ZIP 包的存储介质 |
| 下游依赖 | 标签模块（系统标签 SysTag） | 给 Skill 关联分类标签，按标签筛选 |
| 下游依赖 | 任务执行模块（TaskExecution） | 承载 ZIP 异步导入任务的进度、阶段、日志、产物 |
| 下游依赖 | SKILL.md 元数据解析能力 | 解析 SKILL.md 的 `name`、`description` 等字段 |
| 上游引用方 | Agent / Workflow / 工作流节点 | 通过 Skill ID + 版本号引用 Skill，由 Skill 模块负责加载文件并执行 |
| 上游引用方 | 编辑态 Skill 选择器 | 通过 `Skill 列表（含版本）`接口为前端提供可选 Skill 与版本 |

### 1.4 多租户与作用域

所有 Skill 业务对象均带有作用域归属，依赖以下两个字段做隔离：

- **scope（作用域）**：`PLATFORM` 平台、`TENANT` 租户、`PROJECT` 项目、`PERSONAL` 个人、`APP` 应用
- **subject_id（归属主体ID）**：根据 scope 对应不同维度的主体标识（如租户ID、项目ID、应用ID）

所有查询、唯一性约束、保存、删除均需在 scope + subject_id 维度内进行隔离。

---
## 2. MySQL 数据库表详细设计

Skill 模块共涉及 3 张主表 + 1 张共用任务表（用于 Skill 异步导入任务）。

### 2.1 关联数据表清单

| 表名 | 表注释 | 引擎/字符集 |
|-----|-------|------------|
| `skill_provider` | Skill 提供者表 | InnoDB / utf8mb4 |
| `skill` | Skill 信息表 | InnoDB / utf8mb4 |
| `skill_version` | Skill 版本表 | InnoDB / utf8mb4 |
| `task_execution` | 任务执行表（与 Skill 异步 ZIP 导入复用） | InnoDB / utf8mb4 |

> Skill 各表 ID 由独立的发号器序列 `skill_provider`、`skill`、`skill_version` 提供，序列起始值为 100000。

### 2.2 `skill_provider`（Skill 提供者表）

| 字段名 | 数据类型 | 字段注释 | 主键 | 非空 | 默认值 |
|--------|---------|---------|:---:|:----:|--------|
| skill_provider_id | bigint(20) | 主键ID | 是 | 是 | 无 |
| subject_id | varchar(64) | 归属主体ID | 否 | 是 | 无 |
| scope | varchar(32) | 作用域范围 | 否 | 是 | 无 |
| provider_name | varchar(128) | 提供者名称 | 否 | 是 | 无 |
| provider_type | varchar(32) | 提供者类型：GIT / ZIP / CUSTOM | 否 | 是 | 无 |
| repo_url | varchar(2048) | 仓库地址（GIT 类型必填） | 否 | 否 | NULL |
| auth_type | varchar(32) | 认证类型：NONE / TOKEN / SSH（GIT 类型使用） | 否 | 否 | NULL |
| auth_config | text | 认证配置（加密存储，JSON 格式） | 否 | 否 | NULL |
| file_path | varchar(512) | ZIP 文件存储路径（ZIP 类型使用） | 否 | 否 | NULL |
| status | varchar(32) | 状态：ACTIVE / DISABLED | 否 | 是 | `ACTIVE` |
| creator | varchar(64) | 创建人 | 否 | 否 | NULL |
| updator | varchar(64) | 更新人 | 否 | 否 | NULL |
| ext_info | text | 扩展信息，JSON 格式 | 否 | 否 | NULL |
| gmt_create | datetime | 创建时间 | 否 | 是 | CURRENT_TIMESTAMP |
| gmt_modified | datetime | 修改时间（更新自动刷新） | 否 | 是 | CURRENT_TIMESTAMP |
| env | varchar(32) | 环境标识 | 否 | 否 | NULL |
| delete_flag | tinyint(1) | 是否删除：0-否，1-是 | 否 | 是 | 0 |

**索引设计：**

| 索引类型 | 索引名 | 字段 |
|---------|-------|------|
| 主键 | PRIMARY | skill_provider_id |
| 唯一索引 | uk_provider_name_subject_scope | provider_name, subject_id, scope |
| 普通索引 | idx_subject_scope | subject_id, scope |
| 普通索引 | idx_provider_type | provider_type |

### 2.3 `skill`（Skill 信息表）

| 字段名 | 数据类型 | 字段注释 | 主键 | 非空 | 默认值 |
|--------|---------|---------|:---:|:----:|--------|
| skill_id | bigint(20) | 主键ID | 是 | 是 | 无 |
| subject_id | varchar(64) | 归属主体ID | 否 | 是 | 无 |
| scope | varchar(32) | 作用域范围 | 否 | 是 | 无 |
| provider_id | bigint(20) | Skill 提供者ID | 否 | 是 | 无 |
| skill_name | varchar(128) | Skill 名称（唯一标识） | 否 | 是 | 无 |
| description | varchar(512) | Skill 描述 | 否 | 否 | NULL |
| latest_version_id | bigint(20) | 最新已验证版本ID（用于 latest 别名解析） | 否 | 否 | NULL |
| status | varchar(32) | 状态：ACTIVE / DISABLED | 否 | 是 | `ACTIVE` |
| creator | varchar(64) | 创建人 | 否 | 否 | NULL |
| updator | varchar(64) | 更新人 | 否 | 否 | NULL |
| ext_info | text | 扩展信息，JSON 格式（图标、imageUrl 等） | 否 | 否 | NULL |
| gmt_create | datetime | 创建时间 | 否 | 是 | CURRENT_TIMESTAMP |
| gmt_modified | datetime | 修改时间（更新自动刷新） | 否 | 是 | CURRENT_TIMESTAMP |
| env | varchar(32) | 环境标识 | 否 | 否 | NULL |
| delete_flag | tinyint(1) | 是否删除：0-否，1-是 | 否 | 是 | 0 |

**索引设计：**

| 索引类型 | 索引名 | 字段 |
|---------|-------|------|
| 主键 | PRIMARY | skill_id |
| 唯一索引 | uk_skill_name_subject_scope | skill_name, subject_id, scope |
| 普通索引 | idx_subject_scope | subject_id, scope |
| 普通索引 | idx_provider_id | provider_id |

**`ext_info` 中的标准 Key：**

| Key | 含义 |
|-----|------|
| icon | 图标 JSON（含 `icon` / `iconBackground` / `iconType` 等字段） |
| imageUrl | 自定义图片 URL |

### 2.4 `skill_version`（Skill 版本表）

| 字段名 | 数据类型 | 字段注释 | 主键 | 非空 | 默认值 |
|--------|---------|---------|:---:|:----:|--------|
| skill_version_id | bigint(20) | 主键ID | 是 | 是 | 无 |
| subject_id | varchar(64) | 归属主体ID | 否 | 是 | 无 |
| scope | varchar(32) | 作用域范围 | 否 | 是 | 无 |
| skill_id | bigint(20) | Skill ID | 否 | 是 | 无 |
| version | varchar(32) | 版本号（语义化版本 `vMAJOR.MINOR.PATCH`，草稿固定为 `draft`） | 否 | 是 | 无 |
| runtime | varchar(32) | 运行时：PYTHON / NODE / SHELL（在 V20260326 之后保留字段，发布写入由元数据决定） | 否 | 是 | 无 |
| metadata | text | SKILL.md 解析后的元数据（JSON），应用层校验非空 | 否 | 否 | NULL |
| skill_files | text | Skill 文件清单（JSON 数组，含 path / fileKey / suffix / size / checksum） | 否 | 否 | NULL |
| cache_path | varchar(512) | 缓存文件路径 | 否 | 否 | NULL |
| cache_size | bigint(20) | 缓存大小（字节，LRU 依据） | 否 | 否 | 0 |
| access_count | int(11) | 访问次数（LRU 依据） | 否 | 是 | 0 |
| last_access_time | datetime | 最后访问时间（LRU 依据） | 否 | 否 | NULL |
| creator | varchar(64) | 创建人 | 否 | 否 | NULL |
| updator | varchar(64) | 更新人 | 否 | 否 | NULL |
| ext_info | text | 扩展信息，JSON 格式 | 否 | 否 | NULL |
| gmt_create | datetime | 创建时间 | 否 | 是 | CURRENT_TIMESTAMP |
| gmt_modified | datetime | 修改时间（更新自动刷新） | 否 | 是 | CURRENT_TIMESTAMP |
| env | varchar(32) | 环境标识 | 否 | 是 | `pub` |
| delete_flag | tinyint(1) | 是否删除：0-否，1-是 | 否 | 是 | 0 |

**索引设计：**

| 索引类型 | 索引名 | 字段 |
|---------|-------|------|
| 主键 | PRIMARY | skill_version_id |
| 唯一索引 | uk_skill_version_subject | skill_id, version, subject_id, scope |
| 普通索引 | idx_subject_scope | subject_id, scope |
| 普通索引 | idx_skill_id | skill_id |
| 普通索引 | idx_last_access_time | last_access_time |

**`ext_info` 中的标准 Key：**

| Key | 含义 |
|-----|------|
| zipFilePath | 当前版本对应的对象存储 ZIP 包路径 |
| snapshotSkillName | 发布时刻的 Skill 名称快照（用于回滚恢复） |
| snapshotIcon | 发布时刻的图标快照 |
| snapshotImageUrl | 发布时刻的 imageUrl 快照 |
| snapshotTagIds | 发布时刻关联的标签 ID 列表（JSON 字符串） |

> 历史曾存在 `validation_status` / `validated_by` / `validated_at` / `pub_version_type` / `published_at` / `published_by` 字段，已在 `V20260326` 迁移脚本中统一移除，业务规则不再依赖这些字段。

### 2.5 `task_execution`（任务执行表，复用承载 Skill 异步导入任务）

| 字段名 | 数据类型 | 字段注释 | 主键 | 非空 | 默认值 |
|--------|---------|---------|:---:|:----:|--------|
| task_id | varchar(100) | 任务 ID（主键） | 是 | 是 | 无 |
| scope | varchar(32) | 作用域 | 否 | 否 | NULL |
| subject_id | varchar(64) | 主体 ID | 否 | 否 | NULL |
| app_id | varchar(64) | 应用 ID（Skill 导入固定为 `SKILL_APP`） | 否 | 否 | NULL |
| app_type | varchar(32) | 应用类型（Skill 导入固定为 `SKILL_IMPORT`） | 否 | 否 | NULL |
| run_id | varchar(100) | 运行 ID | 否 | 否 | NULL |
| conversation_id | bigint(20) unsigned | 对话 ID | 否 | 否 | NULL |
| version | varchar(100) | 版本号 | 否 | 否 | NULL |
| status | varchar(100) | 任务状态：PROCESSING / COMPLETED / FAILED | 否 | 否 | NULL |
| inputs | mediumtext | 输入参数（JSON：fileName、fileSize 等） | 否 | 否 | NULL |
| outputs | mediumtext | 输出结果（JSON：logs、skillCount、providerId、errorMessage 等） | 否 | 否 | NULL |
| creator | varchar(64) | 创建者 | 否 | 否 | NULL |
| gmt_archive | datetime | 归档时间（任务完成或失败时写入） | 否 | 否 | NULL |
| ext_info | varchar(2000) | 扩展信息（JSON：stage / progress） | 否 | 否 | NULL |
| gmt_create | datetime | 创建时间 | 否 | 否 | NULL |
| gmt_modified | datetime | 修改时间 | 否 | 否 | NULL |
| env | varchar(20) | 环境标识 | 否 | 否 | NULL |

**索引设计：**

| 索引类型 | 索引名 | 字段 |
|---------|-------|------|
| 主键 | PRIMARY | task_id |
| 普通索引 | idx_scope_subject | scope, subject_id |
| 普通索引 | idx_app_id | app_id |
| 普通索引 | idx_status | status |
| 普通索引 | idx_gmt_create | gmt_create |

---
## 3. 对外 HTTP RESTful 接口契约

### 3.1 接口通用约定

#### 3.1.1 通用请求字段（位于请求体内）

所有接口请求均以 `application/json` 或 `multipart/form-data` 提交，请求体除接口自身字段外，**统一携带以下「业务上下文」字段**（用于多租户、作用域识别）：

| 字段 | 含义 |
|------|------|
| appId | 应用 ID |
| appVersion | 应用版本 |
| conversationSource | 用户来源 |
| tenantId | 租户 ID |
| projectId | 项目 ID |
| scope | 作用域：`PLATFORM` / `TENANT` / `PROJECT` / `PERSONAL` / `APP` |

> `subjectId` 不直接由前端传，由后端依据 `scope` 自动取 `appId` / `projectId` / `tenantId` 中的对应值。

#### 3.1.2 通用响应结构

所有响应统一封装为以下三种格式之一：

**单对象返回（SingleResponse）：**

```json
{
  "success": true,
  "code": "SUCCESS",
  "message": "处理成功",
  "traceId": "<链路ID>",
  "data": { ... }
}
```

**列表返回（ListResponse）：**

```json
{
  "success": true,
  "code": "SUCCESS",
  "message": "处理成功",
  "traceId": "<链路ID>",
  "data": [ ... ]
}
```

**分页返回（PageResponse）：**

```json
{
  "success": true,
  "code": "SUCCESS",
  "message": "处理成功",
  "traceId": "<链路ID>",
  "total": 123,
  "pageSize": 10,
  "pageNum": 1,
  "records": [ ... ]
}
```

#### 3.1.3 通用分页规则

需要分页的接口（仅 Skill 列表查询使用）：

| 字段 | 含义 | 默认值 |
|------|------|-------|
| pageNum | 当前页码（从 1 开始） | 1 |
| pageSize | 每页大小 | 10 |
| maxPageSize | 单次最大允许的 pageSize | 1000，超出则被截断 |
| orderByColumn | 排序字段（驼峰传入，按下划线转换） | 无（默认按业务规则排序） |
| isAsc | 排序方向：`asc` / `desc` | `asc` |

> 即使前端传入 `orderByColumn`，Skill 列表查询的实际排序仍按业务规则（`gmt_modified DESC, skill_id DESC`）执行。

#### 3.1.4 业务异常返回规则

业务异常统一返回：

```json
{
  "success": false,
  "code": "<错误码>",
  "message": "<错误描述>",
  "traceId": "<链路ID>",
  "data": null
}
```

参数缺失会返回 `code = ILLEGAL_PARAM`、`message = 参数不合法`。其余 Skill 模块特有错误码见「业务规则清单 - 错误码」章节。

---

### 3.2 Skill 配置管理接口（前缀 `/api/v1/skill`）

#### 3.2.1 草稿暂存：`POST /api/v1/skill/draft/save`

| 项 | 说明 |
|----|------|
| 功能描述 | 暂存或更新 Skill 草稿；`skillId` 为空时同时新建 Skill 与草稿，否则更新已有 Skill 的元数据并覆盖其草稿 |
| 请求方式 | POST |
| Content-Type | `multipart/form-data` |
| 是否分页 | 否 |
| 是否排序 | 否 |
| 路径参数 | 无 |
| Query 参数 | 无 |

**请求体（form 字段）：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| files | 文件数组 | 否 | 二进制文件流，每个文件的 `originalFilename` 用于和 `skillFiles[].name` 进行匹配 |
| skillId | 数字 | 否 | 为空时新建，有值时更新 |
| skillName | 字符串 | 否 | 草稿可不传，发布时会从 SKILL.md 解析 |
| description | 字符串 | 否 | 同上 |
| icon | 字符串 | 否 | 图标 JSON（如 `{"icon":"</>","iconBackground":"#1890ff","iconType":"emoji"}`） |
| imageUrl | 字符串 | 否 | 自定义图片 URL |
| tagIds | 字符串 | 否 | 关联的标签 ID 列表 JSON 字符串，如 `"[1,2,3]"` |
| skillFiles | 字符串 | 是 | 文件结构描述列表的 JSON 字符串，每条形如 `{ "type":"file", "path":"folder/a.py", "name":"a.py" }` |

**响应体（data 部分，SkillInfo）：**

```json
{
  "skillId": 100023,
  "providerId": 100001,
  "providerName": "custom",
  "skillName": "code-reviewer",
  "description": "对代码做评审",
  "icon": { "icon": "</>", "iconBackground": "#1890ff", "iconType": "emoji" },
  "imageUrl": null,
  "categoryTags": null,
  "status": "ACTIVE",
  "latestVersion": null,
  "versionCount": 0,
  "gmtCreate": "...",
  "gmtModified": "...",
  "tags": [ { "tagId": ..., ... } ],
  "files": [ { "path": "...", "fileKey": "...", "suffix": "py", "size": 1234, "checksum": "..." } ],
  "versions": []
}
```

#### 3.2.2 发布：`POST /api/v1/skill/publish`

| 项 | 说明 |
|----|------|
| 功能描述 | 将草稿发布为新的正式版本：先保存草稿、再生成正式版本（自动按 patch 递增版本号），将 OSS 文件从 draft 复制到版本目录，最后打包 ZIP 并保存快照 |
| 请求方式 | POST |
| Content-Type | `multipart/form-data` |
| 是否分页 | 否 |
| 是否排序 | 否 |

**请求体（form 字段）：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| files | 文件数组 | 否 | 同 draft/save |
| skillId | 数字 | 否 | 为空时同时创建 Skill+草稿+正式版本 |
| providerId | 数字 | 否 | ZIP 导入场景指定，缺省时使用当前主体的 CUSTOM Provider |
| skillName | 字符串 | 否 | SKILL.md 解析后会被覆盖 |
| description | 字符串 | 否 | SKILL.md 解析后会被覆盖 |
| versionDescription | 字符串 | 否 | 版本描述，缺省时使用 Skill 描述 |
| icon | 字符串 | 否 | 图标 JSON |
| imageUrl | 字符串 | 否 | 自定义图片 URL |
| tagIds | 字符串 | 否 | 关联的标签 ID 列表 JSON 字符串 |
| skillFiles | 字符串 | 是 | 文件结构描述列表 JSON 字符串 |

**响应体：** 同 `SkillInfo`，`latestVersion` 为新发布版本，`versions` 含全部正式版本（不含 draft）。

#### 3.2.3 更新：`POST /api/v1/skill/update`

| 项 | 说明 |
|----|------|
| 功能描述 | 修改 Skill 的名称、描述、状态（仅修改基础元数据） |
| 请求方式 | POST |
| Content-Type | `application/json` |

**请求体：**

```json
{
  "skillId": 100023,
  "skillName": "...",
  "description": "...",
  "status": "ACTIVE"
}
```

**响应体：** SkillInfo（包含最新版本信息）。

#### 3.2.4 删除：`POST /api/v1/skill/delete`

| 项 | 说明 |
|----|------|
| 功能描述 | 物理删除 Skill 及其全部版本与对象存储中的全部文件，并删除标签关联 |
| 请求方式 | POST |
| Content-Type | `application/json` |

**请求体：**

```json
{ "skillId": 100023 }
```

**响应体：** `data` 为布尔值 `true`。

#### 3.2.5 分页查询：`POST /api/v1/skill/list`

| 项 | 说明 |
|----|------|
| 功能描述 | 当前 scope+subject 维度内的 Skill 分页查询 |
| 请求方式 | POST |
| 分页 | 是（pageNum/pageSize） |
| 排序 | 固定 `gmt_modified DESC, skill_id DESC`，前端传入的排序字段被忽略 |

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| providerId | 数字 | 否 | 按提供者过滤 |
| skillName | 字符串 | 否 | 按名称模糊（最长截断到 64） |
| keyword | 字符串 | 否 | 关键字模糊（同样匹配 skill_name，最长截断到 64） |
| status | 字符串 | 否 | 状态过滤（ACTIVE / DISABLED） |
| categoryTagId | 数字数组 | 否 | 按一组分类标签过滤；命中后无 skill 时直接返回空集 |
| pageNum、pageSize | 数字 | 否 | 见通用分页规则 |

**响应体：** PageResponse&lt;SkillInfo&gt;

> 当 `categoryTagId` 不为空但未命中任何 Skill 时，分页响应直接返回空 `records`，`total = 0`。

#### 3.2.6 全量查询：`POST /api/v1/skill/listAll`

| 项 | 说明 |
|----|------|
| 功能描述 | 返回当前主体下符合过滤条件的全部 Skill（不分页） |
| 请求方式 | POST |
| 分页 | 否 |
| 排序 | 同 list（`gmt_modified DESC, skill_id DESC`） |

**请求体字段：** 同 `/api/v1/skill/list` 的过滤字段（pageNum/pageSize 被忽略）。

**响应体：** ListResponse&lt;SkillInfo&gt;

#### 3.2.7 详情查询：`POST /api/v1/skill/detail`

| 项 | 说明 |
|----|------|
| 功能描述 | 查询 Skill 详情，含图标、imageUrl、关联标签、版本列表（不含 draft）、文件列表（优先正式版本，否则 draft） |
| 请求方式 | POST |
| Content-Type | `application/json` |

**请求体：**

```json
{ "skillId": 100023 }
```

**响应体：** SkillInfo（完整字段，含 `versions`、`files`、`tags`、`latestVersion`）。

#### 3.2.8 含版本列表查询：`POST /api/v1/skill/with/version`

| 项 | 说明 |
|----|------|
| 功能描述 | 编辑态 Skill 选择器使用：返回当前主体下「至少有一个正式版本」的 Skill 列表，每条带版本数组 |
| 请求方式 | POST |
| 分页 | 否 |
| 排序 | 按 `gmt_modified DESC, skill_id DESC` |

**请求体：** 仅包含通用业务上下文字段（无业务过滤参数）。

**响应体：** ListResponse&lt;SkillWithVersionInfo&gt;，每条包含 `skillId`、`providerId`、`providerName`、`skillName`、`description`、`status`、`versionCount`、`versions`、`icon`、`imageUrl`、`gmtCreate`、`creator`、`gmtModified`。

#### 3.2.9 文件下载：`GET /api/v1/skill/file/download`

| 项 | 说明 |
|----|------|
| 功能描述 | 按对象存储 fileKey 下载文件，支持 Skill 内任意文件 |
| 请求方式 | GET |
| Query 参数 | `fileKey`（必填，对象存储 Key） |
| 响应 | `application/octet-stream`，`Content-Disposition` 携带 URL 编码的文件名；fileKey 不存在时 HTTP 500 |

---

### 3.3 Skill 提供者管理接口（前缀 `/api/v1/skill/provider`）

#### 3.3.1 Git 仓库导入：`POST /api/v1/skill/provider/import/git`

| 项 | 说明 |
|----|------|
| 功能描述 | 登记一个 Git 类型 Provider；后续仓库扫描与 Skill 注册逻辑由 GIT importer 实现 |
| 请求方式 | POST |
| Content-Type | `application/json` |

**请求体：**

```json
{
  "providerName": "...",
  "repoUrl": "...",          // 必填
  "authType": "NONE | TOKEN | SSH",
  "authConfig": "..."
}
```

**响应体：** SingleResponse&lt;SkillProviderInfo&gt;

#### 3.3.2 ZIP 异步导入：`POST /api/v1/skill/provider/import/zip`

| 项 | 说明 |
|----|------|
| 功能描述 | 上传压缩包后立即返回任务 ID，导入解压 / 校验 / 注册过程异步执行 |
| 请求方式 | POST |
| Content-Type | `multipart/form-data` |

**请求体（form 字段）：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| file | 文件 | 是 | 仅支持 `.zip` / `.tar.gz` / `.tgz` |
| 其余字段 | - | - | 通用业务上下文字段 |

**响应体：**

```json
{ "data": { "taskId": "<uuid>" } }
```

#### 3.3.3 ZIP 同步导入（旧接口）：`POST /api/v1/skill/provider/import/zip/legacy`

| 项 | 说明 |
|----|------|
| 功能描述 | 兼容旧版本同步登记一个 ZIP Provider，仅创建 Provider 记录 |
| 请求方式 | POST |
| Content-Type | `application/json` |

**请求体：**

```json
{ "providerName": "..." }
```

**响应体：** SingleResponse&lt;SkillProviderInfo&gt;

#### 3.3.4 导入任务详情订阅（SSE）：`POST /api/v1/skill/provider/import/task/detail`

| 项 | 说明 |
|----|------|
| 功能描述 | 通过 Server-Sent Events 持续推送指定任务的进度快照、阶段日志、完成/失败事件 |
| 请求方式 | POST |
| Content-Type | `application/json`（响应：`text/event-stream`） |

**请求体：**

```json
{ "taskId": "<uuid>" }
```

**SSE 事件字段（`SkillImportEventData`）：**

| 字段 | 含义 |
|------|------|
| taskId | 任务 ID |
| status | PROCESSING / COMPLETED / FAILED |
| stage | UPLOAD / EXTRACT / VALIDATE / REGISTER / COMPLETED |
| progress | 0-100 |
| logs | `ImportLog` 列表，含 `time`（ISO 8601）/`level`（INFO/SUCCESS/WARNING/ERROR）/`message` |
| skillCount | 已注册的 Skill 数量（仅完成时） |
| providerId | 关联 Provider ID（仅完成时） |
| errorMessage | 失败原因（仅失败时） |

#### 3.3.5 导入任务列表：`POST /api/v1/skill/provider/import/task/list`

| 项 | 说明 |
|----|------|
| 功能描述 | 查询当前主体下所有 Skill ZIP 导入任务 |
| 分页 | 否 |
| 排序 | 由 task_execution 数据访问层默认排序 |

**请求体：** 仅包含通用业务上下文字段。

**响应体：** ListResponse&lt;SkillImportTaskInfo&gt;，每条字段：`taskId / fileName / fileSize / status / stage / progress / skillCount / gmtCreate (ISO 8601)`。

#### 3.3.6 导入任务批量删除：`POST /api/v1/skill/provider/import/task/delete`

| 项 | 说明 |
|----|------|
| 功能描述 | 批量删除指定任务，并清理其 SSE 订阅资源 |

**请求体：**

```json
{ "taskIds": ["<uuid>", "<uuid>"] }
```

**响应体：** `data = true`。

#### 3.3.7 同步 Provider：`POST /api/v1/skill/provider/sync`

| 项 | 说明 |
|----|------|
| 功能描述 | 触发 Provider 重新扫描；当前实现仅校验存在性与状态 |

**请求体：**

```json
{ "providerId": 100001 }
```

**响应体：** SingleResponse&lt;SkillProviderInfo&gt;

#### 3.3.8 删除 Provider：`POST /api/v1/skill/provider/delete`

| 项 | 说明 |
|----|------|
| 功能描述 | 软删除 Provider（实际是把 status 置为 `DISABLED`） |

**请求体：**

```json
{ "providerId": 100001 }
```

**响应体：** `data = true`。

#### 3.3.9 Provider 列表：`POST /api/v1/skill/provider/list`

| 项 | 说明 |
|----|------|
| 功能描述 | 当前主体下全部 Provider 列表 |
| 分页 | 否 |

**请求体：**

| 字段 | 说明 |
|------|------|
| providerType | 已声明字段（GIT / ZIP / CUSTOM），当前实现未参与过滤 |
| status | 已声明字段，当前实现未参与过滤 |

**响应体：** ListResponse&lt;SkillProviderInfo&gt;，每条字段：`skillProviderId / providerName / providerType / repoUrl / status / skillCount / gmtCreate`。

#### 3.3.10 Provider 详情：`POST /api/v1/skill/provider/detail`

| 项 | 说明 |
|----|------|
| 功能描述 | 查询单个 Provider 的详情 |

**请求体：**

```json
{ "providerId": 100001 }
```

**响应体：** SingleResponse&lt;SkillProviderInfo&gt;。

---
### 3.4 Skill 版本管理接口（前缀 `/api/v1/skill/version`）

#### 3.4.1 创建版本：`POST /api/v1/skill/version/create`

| 项 | 说明 |
|----|------|
| 功能描述 | 直接为指定 Skill 创建一个版本（不走「草稿→发布」流程，主要用于程序化新增版本） |

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| skillId | 数字 | 是 | 必填 |
| version | 字符串 | 是 | 必填；为空时按 patch 自动递增（基于该 Skill 的最新已发布版本） |

**响应体：** SingleResponse&lt;SkillVersionInfo&gt;，主要字段：`skillVersionId / skillId / skillName / version / description / metadata / skillFiles / cacheSize / accessCount / lastAccessTime / gmtCreate / publishedBy`。

#### 3.4.2 验证版本：`POST /api/v1/skill/version/validate`

| 项 | 说明 |
|----|------|
| 功能描述 | 标记指定版本为最新版本；将 Skill 的 `latest_version_id` 指向该版本 |

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| versionId | 数字 | 是 | 必填 |
| testInputs | 对象 | 否 | 预留的验证测试入参（当前实现不消费） |

**响应体：** SingleResponse&lt;SkillVersionInfo&gt;。

#### 3.4.3 删除版本：`POST /api/v1/skill/version/delete`

| 项 | 说明 |
|----|------|
| 功能描述 | 在指定主体作用域内删除某个版本（当前实现不做物理删除，仅再保存以更新元信息，实际删除由 Skill 删除时级联处理） |

**请求体：**

```json
{ "skillId": 100023, "version": "v0.1.0" }
```

**响应体：** `data = true`。

#### 3.4.4 版本列表：`POST /api/v1/skill/version/list`

| 项 | 说明 |
|----|------|
| 功能描述 | 列出指定 Skill 的全部正式版本（不含 draft），按 `gmt_create DESC` 排列；版本描述为空时回退使用 Skill 描述 |
| 分页 | 否 |
| 排序 | 固定 `gmt_create DESC` |

**请求体：**

```json
{ "skillId": 100023 }
```

**响应体：** ListResponse&lt;SkillVersionInfo&gt;。

#### 3.4.5 版本详情：`POST /api/v1/skill/version/detail`

| 项 | 说明 |
|----|------|
| 功能描述 | 在 `skillId + version + 主体作用域` 维度内查询版本详情 |

**请求体：**

```json
{ "skillId": 100023, "version": "v0.1.0" }
```

**响应体：** SingleResponse&lt;SkillVersionInfo&gt;。

#### 3.4.6 最新版本：`POST /api/v1/skill/version/latest`

| 项 | 说明 |
|----|------|
| 功能描述 | 返回 Skill `latest_version_id` 指向的版本 |

**请求体：**

```json
{ "skillId": 100023 }
```

**响应体：** SingleResponse&lt;SkillVersionInfo&gt;；Skill 不存在或无最新版本时 `data = null`。

#### 3.4.7 版本回滚：`POST /api/v1/skill/version/rollback`

| 项 | 说明 |
|----|------|
| 功能描述 | 以目标历史版本的内容为基础生成一个新的版本（版本号基于历史最新已发布版本递增 patch），同时把 Skill 的展示信息（名称、图标、imageUrl、标签）从目标版本快照恢复，并把快照写入新版本 |

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| skillId | 数字 | 是 | 必填 |
| targetVersion | 字符串 | 是 | 要回滚到的目标版本号 |
| pubVersionType | 字符串 | 否 | 占位字段（patch / minor / major），当前实现固定按 patch 递增 |

**响应体：** SingleResponse&lt;SkillVersionInfo&gt;。

#### 3.4.8 版本导出：`POST /api/v1/skill/version/export`

| 项 | 说明 |
|----|------|
| 功能描述 | 将版本对应的全部文件实时打包为 ZIP 流式下载，文件名格式 `{skillName}-v{version}.zip` |
| 响应类型 | `application/zip` |

**请求体：**

```json
{ "skillId": 100023, "version": "v0.1.0" }
```

**异常返回：** 任意失败均通过 HTTP 500 状态码返回。

---
## 4. 领域模型定义

### 4.1 聚合划分

| 聚合 | 聚合根 | 实体 | 说明 |
|-----|-------|------|------|
| Skill 提供者聚合 | SkillProvider | — | 描述 Skill 来源；与 Skill 通过 `provider_id` 关联（跨聚合 ID 引用） |
| Skill 聚合 | Skill | SkillVersion（同聚合内的实体） | Skill 是聚合根，所有版本归属同一 Skill；通过 `skill_id` 关联 SkillVersion |
| 任务执行聚合（共用） | TaskExecution | — | 承载 ZIP 异步导入任务的状态、阶段、日志、产物 |

### 4.2 值对象

| 值对象 | 含义 | 主要字段 |
|-------|------|---------|
| SkillFile | Skill 单个文件的元数据 | path（相对路径）、checksum（MD5/SHA256）、fileKey（OSS Key）、suffix（扩展名）、size（字节） |
| SkillFileStructure | 前端传入的文件结构条目 | type（`file`/`directory`）、path、name |
| Icon | Skill 图标信息 | icon、iconBackground、iconType |
| ImportLog | 导入过程的日志条目 | time（ISO 8601）、level（INFO/SUCCESS/WARNING/ERROR）、message |
| ScopeType | 作用域 | code 枚举：PLATFORM / TENANT / PROJECT / PERSONAL / APP |

### 4.3 业务状态枚举

| 枚举 | 取值 | 含义 | 数据库字段类型 |
|------|------|------|----------------|
| Skill 状态 | `ACTIVE` / `DISABLED` | 启用 / 禁用 | varchar(32) |
| Skill Provider 状态 | `ACTIVE` / `DISABLED` | 启用 / 禁用 | varchar(32) |
| Skill Provider 类型 | `GIT` / `ZIP` / `CUSTOM` | Git 仓库 / ZIP 包 / 平台内置自定义 | varchar(32) |
| Provider 认证类型 | `NONE` / `TOKEN` / `SSH` | 无认证 / Token / SSH 密钥 | varchar(32) |
| Skill 版本类型 | `draft` / `vMAJOR.MINOR.PATCH` | 草稿 / 语义化正式版本 | varchar(32) |
| Skill Runtime（保留字段） | `PYTHON` / `NODE` / `SHELL` | 运行时类型 | varchar(32) |
| 作用域 ScopeType | `PLATFORM` / `TENANT` / `PROJECT` / `PERSONAL` / `APP` | 多租户作用域 | varchar(32) |
| 任务状态（导入任务） | `PROCESSING` / `COMPLETED` / `FAILED` | 处理中 / 已完成 / 已失败 | varchar(100) |
| 导入任务阶段 | `UPLOAD` / `EXTRACT` / `VALIDATE` / `REGISTER` / `COMPLETED` | 上传 / 解压 / 校验 / 注册 / 完成 | varchar（保存于 task_execution.ext_info.stage） |
| 导入日志级别 | `INFO` / `SUCCESS` / `WARNING` / `ERROR` | 信息 / 成功 / 警告 / 错误 | 仅出现在日志条目内 |

### 4.4 SkillProvider 聚合根

**关键属性：**

| 属性 | 含义 |
|------|------|
| skillProviderId | 主键，由独立发号器分配 |
| providerName | 提供者名称 |
| providerType | GIT / ZIP / CUSTOM |
| repoUrl | 仅 GIT 类型需要 |
| authType | 仅 GIT 类型需要 |
| authConfig | 认证凭据，加密存储 |
| filePath | 仅 ZIP 类型使用 |
| status | ACTIVE / DISABLED |
| scope、subjectId、env、creator、updator、extInfo、gmtCreate、gmtModified | 通用字段 |

**领域内置行为：**

| 行为 | 说明 |
|------|------|
| createCustomProvider(subjectId, scope) | 创建归属指定主体作用域的 CUSTOM Provider，状态固定为 ACTIVE |
| createGitProvider(subjectId, scope, providerName, repoUrl, authType, authConfig) | 创建 GIT Provider，状态固定为 ACTIVE |
| isActive() | 状态是否为 ACTIVE |

**领域校验规则：**

- `providerName + subjectId + scope` 在数据库层面唯一（uk_provider_name_subject_scope）。
- GIT 类型需附带 `repoUrl`、`authType`、`authConfig` 三项。
- ZIP 类型在异步导入场景下，由系统在 ProviderName 前自动加时间戳前缀以避免唯一键冲突。

### 4.5 Skill 聚合根

**关键属性：**

| 属性 | 含义 |
|------|------|
| skillId | 主键，独立发号器分配 |
| providerId | 关联 Provider ID（跨聚合 ID 引用） |
| skillName | Skill 名称（唯一标识） |
| description | Skill 描述 |
| latestVersionId | 最新已验证版本 ID |
| status | ACTIVE / DISABLED |
| scope、subjectId、creator、updator、env、extInfo、gmtCreate、gmtModified | 通用字段 |
| extInfo.icon | 图标 JSON |
| extInfo.imageUrl | 自定义图片 URL |

**领域内置行为：**

| 行为 | 说明 |
|------|------|
| updateLatestVersion(versionId) | 更新最新版本 ID；versionId 不能为空 |
| disable() | 状态置为 DISABLED |
| enable() | 状态置为 ACTIVE |
| isActive() | 状态是否为 ACTIVE |

**领域校验规则：**

- `skillName + subjectId + scope` 在数据库层面唯一（uk_skill_name_subject_scope）。
- `skillName` 不为空，长度 ≤ 120。
- `description` 不为空，长度 ≤ 3000。
- 同主体作用域内同名 Skill 仅允许一份；如已有同名 Skill，但其 `skillId` 与当前编辑的 Skill 不同，则视为重复。

### 4.6 SkillVersion 实体

**关键属性：**

| 属性 | 含义 |
|------|------|
| skillVersionId | 主键，独立发号器分配 |
| skillId | 所属 Skill ID（聚合内引用） |
| version | `draft` 或 `vMAJOR.MINOR.PATCH` |
| description | 版本描述 |
| metadata | SKILL.md 解析后的 JSON |
| skillFiles | 文件清单（List&lt;SkillFile&gt;） |
| cachePath、cacheSize、accessCount、lastAccessTime | LRU 缓存元数据 |
| scope、subjectId、creator、updator、env、extInfo、gmtCreate、gmtModified | 通用字段 |
| extInfo.zipFilePath | 当前版本的 ZIP 包路径 |
| extInfo.snapshotSkillName / snapshotIcon / snapshotImageUrl / snapshotTagIds | 发布时刻快照（用于回滚恢复展示信息） |

**领域校验规则：**

- `skillId + version + subjectId + scope` 唯一（uk_skill_version_subject）。
- 草稿版本号固定为 `draft`，初始正式版本号固定为 `v0.1.0`。
- 正式版本号必须以 `v` 开头并包含三段数字。
- 版本递增按 `patch` 自增（最新已发布版本基础上 +1）。

### 4.7 TaskExecution 实体（用于 Skill 异步导入）

**关键属性：**

| 属性 | 含义 |
|------|------|
| taskId | UUID 格式的任务 ID |
| appId | Skill 导入固定 `SKILL_APP` |
| appType | Skill 导入固定 `SKILL_IMPORT` |
| status | PROCESSING / COMPLETED / FAILED |
| inputs | JSON：fileName、fileSize |
| outputs | JSON：logs（ImportLog 列表）、skillCount、providerId、errorMessage |
| extInfo.stage | 当前阶段 |
| extInfo.progress | 当前进度（字符串形式整数 0-100） |
| gmtArchive | 完成或失败时间 |

**领域内置行为：**

| 行为 | 说明 |
|------|------|
| validateCanComplete() | 仅当当前状态为 PROCESSING 时允许标记完成 |
| markAsCompleted() | 状态置为 COMPLETED，stage = COMPLETED，progress = 100，写入 gmtArchive |
| markAsFailed() | 状态置为 FAILED，写入 gmtArchive |
| updateProgressInfo(stage, progress) | 更新阶段与进度 |

---
## 5. 全量业务规则清单

### 5.1 错误码（Skill 模块）

| 错误码 | 描述 |
|-------|------|
| ILLEGAL_PARAM | 参数不合法（含必填校验、文件结构格式错误等） |
| SYSTEM_ERROR | 系统繁忙 / 兜底异常（含文件复制失败、文件下载失败、发布过程系统异常等） |
| SKILL_NOT_FOUND | Skill 不存在 |
| SKILL_PROVIDER_NOT_FOUND | Skill 提供者不存在 |
| SKILL_VERSION_NOT_FOUND | Skill 版本不存在 |
| SKILL_FILE_MISSING | SKILL.md 为必需文件 / 下载文件不存在 |
| SKILL_PUBLISH_FAILED | Skill 发布失败（如文件复制失败） |
| SKILL_NAME_DUPLICATE | 该 Skill 名称已存在 |
| SKILL_NAME_IS_BLANK | Skill 名称为空 |
| SKILL_NAME_TOO_LONG | Skill 名称过长（>120） |
| SKILL_DESCRIPTION_IS_BLANK | Skill 描述为空 |
| SKILL_DESCRIPTION_TOO_LONG | Skill 描述过长（>3000） |
| SKILL_IMPORT_FILE_INVALID | 压缩包格式无效或解压失败 |
| SKILL_IMPORT_NO_SKILL_FOUND | 压缩包中未发现有效 Skill |
| SKILL_IMPORT_TASK_NOT_FOUND | 导入任务不存在 |

---

### 5.2 数据唯一性约束

| 维度 | 约束 | 触发位置 |
|------|------|---------|
| Skill | `skill_name + subject_id + scope` 唯一 | 创建/草稿/发布/更新校验 + DB 唯一索引 |
| Skill Provider | `provider_name + subject_id + scope` 唯一 | 创建/导入校验 + DB 唯一索引；ZIP 异步导入会自动给 providerName 加时间戳前缀 |
| Skill Version | `skill_id + version + subject_id + scope` 唯一 | 版本创建校验 + DB 唯一索引 |

---

### 5.3 草稿暂存（draft/save）业务规则

**前置参数校验：**

1. `skillFiles` 字段（JSON 字符串）不能为空。
2. 解析后的文件结构列表中必须包含 `name = SKILL.md` 的条目，否则返回 `SKILL_FILE_MISSING`。
3. 解析失败抛 `ILLEGAL_PARAM (文件结构格式错误)`。

**SKILL.md 元数据规则：**

| 场景 | 规则 |
|------|------|
| 新建（skillId 为空） | 必须上传 SKILL.md；解析后的 `name`、`description` 不能为空 |
| 更新（skillId 非空） | 允许不上传 SKILL.md；若上传则按 SKILL.md 的 name、description 覆盖请求 |

**关联数据校验：**

- 更新模式下，按 `skillId + subjectId + scope` 查不到 Skill → `SKILL_NOT_FOUND`。
- 校验 `skillName` 不为空且 ≤ 120；`description` 不为空且 ≤ 3000。
- 校验同主体作用域内不存在同名但 ID 不同的 Skill → 否则 `SKILL_NAME_DUPLICATE`。

**关键流转：**

1. 新建：自动创建/复用归属当前主体作用域的 CUSTOM Provider；新建 Skill；新建 draft 版本；上传文件。
2. 更新：覆盖 Skill 元信息；若已存在 draft 版本则清理已删除文件、合并新旧文件，更新 `description`、`updator`；同步将 Skill 的 `latest_version_id` 置空（提示需要重新发布）。
3. 标签同步：若请求传入 `tagIds`，则在事务内更新 Skill 与系统标签的关联。

**多表事务约束：**

- Skill / SkillVersion / 标签关系更新均在同一事务内执行（`TransactionTemplate`）。
- 文件上传到对象存储不在事务内，发生异常时统一抛 `SYSTEM_ERROR (文件上传失败: ...)`。

**状态流转限制：** 草稿不参与状态流转，固定写入 `draft` 版本号。

---

### 5.4 发布（publish）业务规则

**前置参数校验：** 与草稿保持一致（含 SKILL.md 必需、`skillFiles` 非空、SKILL.md 字段非空）。

**关联数据校验：**

- 新建 + 指定 `providerId`：providerId 不存在 → `SKILL_PROVIDER_NOT_FOUND`。
- 新建 + 不指定 providerId：自动获取/创建当前主体作用域的 CUSTOM Provider。
- 更新 + skillId 在主体作用域下查不到 → `SKILL_NOT_FOUND`。
- 名称、描述长度规则同草稿。

**版本号生成规则：**

- 取该 Skill 下全部 `v` 开头的正式版本号（不含 draft），按三段数字字典序求最大。
- 找不到任何正式版本时使用 `v0.1.0`。
- 否则将最大版本号的 `patch` 段 +1（如 `v1.2.3` → `v1.2.4`）。
- 版本号解析失败时退化为 `v0.1.0`（仅在版本格式异常时才会触发）。

**关键流转：**

1. 准备 draft：和草稿保持流程一致（新建/更新 draft 版本、文件上传、清理已删文件）。
2. 生成新版本号 → 创建正式 SkillVersion 实体（继承 draft 的 metadata、文件清单，并将 fileKey 重写为新版本路径）。
3. 把 draft 版本目录下的文件逐一复制到新版本目录；任意一个失败 → `SKILL_PUBLISH_FAILED`。
4. 保存正式版本，更新 Skill 的 `latest_version_id`。
5. 标签同步：若请求传入 `tagIds` 则更新关联关系。
6. 发布快照：把当前 Skill 的 `skillName`、`icon`、`imageUrl`、最终生效的 `tagIds`（请求传入或当前关联）写入新版本的 `ext_info` 快照字段，用于将来回滚还原。
7. 打包 ZIP：把新版本目录下的文件流式打包为 ZIP，上传到 OSS，将 `zipFilePath` 写入新版本 `ext_info`。

**多表事务约束：**

- Skill / SkillVersion（draft + new）/ 标签关联 / 版本快照 ext_info / ZIP fileKey 的所有写入均在同一事务内。
- OSS 文件复制、ZIP 打包上传是事务内同步调用，若失败则整体回滚事务（`SKILL_PUBLISH_FAILED`）。

**状态流转限制：**

- 同一 Skill 同时刻只允许有 1 份 draft；新版本号必须严格大于历史最新版本（含递增逻辑保证）。

---

### 5.5 简单创建（程序化创建 Skill）业务规则

> 接口：`SkillService.create(SkillCreateRequest)`，由旧入口保留，未通过 Skill Controller 暴露 HTTP，但仍构成业务规则一部分。

- 自动获取/创建 CUSTOM Provider。
- 校验后写入新 Skill + 一条版本号为 `request.version` 或默认 `1.0.0` 的 SkillVersion。
- 立即把 Skill 的 `latest_version_id` 设置为该版本。
- 全部操作在事务内执行。

---

### 5.6 编辑 Skill（update）业务规则

**前置参数校验：**

- `skillId` 不能为空。

**关联数据校验：**

- 按主键查不到 Skill → `SKILL_NOT_FOUND`。

**字段更新规则：**

- 仅当请求字段非 null 时才覆盖：`skillName`、`description`、`status`。
- 不会触发版本变更或文件变更。

**多表事务约束：** 不涉及多表，写一条 Skill 记录。

---

### 5.7 删除 Skill（delete）业务规则

**前置参数校验：**

- `skillId` 不能为空。

**关联数据校验：**

- Skill 存在性校验失败 → `SKILL_NOT_FOUND`。

**关键流转：**

1. 先收集该 Skill 所有版本中的 OSS fileKey。
2. 事务内顺序执行：删除 Skill 与系统标签的关系 → 物理删除该 Skill 全部版本 → 物理删除 Skill。
3. 事务外：批量删除 OSS 文件，失败仅记 warn 日志（尽力而为）。

**多表事务约束：**

- 标签关系、版本表、Skill 表的删除位于同一事务。

**状态流转限制：** 不涉及状态校验（草稿、正式版本同样允许删除）。

---

### 5.8 分页查询 / 全量查询业务规则

**通用规则：**

- 自动按 `scope + subjectId` 进行隔离过滤；`subjectId` 由后端依据 scope 取自请求体（`tenantId` / `appId` / `projectId`）。
- 排序固定为 `gmt_modified DESC, skill_id DESC`，前端 `orderByColumn` 不会被采纳。
- 不区分 draft/正式：本接口返回的是 Skill 维度的列表。

**过滤规则：**

| 字段 | 规则 |
|------|------|
| providerId | 等值过滤 |
| skillName | 模糊匹配 `LIKE %name%`，超过 64 字符截断 |
| keyword | 与 skillName 同字段做 `LIKE %keyword%`，超过 64 字符截断 |
| status | 等值过滤（ACTIVE / DISABLED） |
| categoryTagId | 在系统标签关系表中按 RhsId IN (...) 命中后取 LhsId（即 skillIds），任何一步未命中即直接返回空 |

**分页规则：**

- 仅 `/api/v1/skill/list` 走分页；`/api/v1/skill/listAll` 不分页。
- `/api/v1/skill/list` 当 `categoryTagId` 命中 0 个标签关系时直接返回空分页。

**关联组装：**

- 批量查询关联标签：按 `lhs_source = SKILL`、`relation_key = SKILL` 反向查 `SysTag` 列表。
- 批量加载每个 Skill 的最新版本，组装到 `latestVersion` 字段。

---

### 5.9 详情查询业务规则

**前置参数校验：**

- `skillId` 不能为空。

**关联数据校验：**

- Skill 不存在 → `SKILL_NOT_FOUND`。

**组装规则：**

- 加载 Provider 信息、最新版本（若有）、关联标签列表。
- 解析 `ext_info.icon` 为 Icon 对象，解析 `ext_info.imageUrl` 字符串。
- `versions` 字段：取该 Skill 下全部正式版本（排除 draft），按 `gmtCreate DESC` 排列；`versionCount` 为正式版本个数。
- `files` 字段：当存在最新正式版本且其文件非空 → 返回正式版本文件；否则回退取 draft 版本文件。

---

### 5.10 编辑态选择器（with/version）业务规则

- 不携带业务过滤参数；按当前主体作用域加载全部 Skill。
- 仅保留「至少有一个正式版本」的 Skill；版本数为 0 的 Skill 不返回。
- 版本列表按 `gmt_create DESC` 排列。
- 批量加载关联 Provider 信息以填充 `providerId`、`providerName`。

---

### 5.11 文件下载业务规则

**前置参数校验：**

- `fileKey` 不能为空 → `ILLEGAL_PARAM`。

**关联数据校验：**

- 检查 OSS 中文件是否存在，不存在 → `SKILL_FILE_MISSING`。
- 任何 OSS 异常 → `SYSTEM_ERROR (文件下载失败)`。

**响应规则：**

- `Content-Type` 为 `application/octet-stream`，`Content-Disposition` 携带 URL 编码的文件名（按 fileKey 最后一段截取）。

---

### 5.12 Skill Provider 业务规则

#### 5.12.1 Git 导入

- `repoUrl` 必填。
- 写入一条 GIT 类型 Provider，状态固定 `ACTIVE`，归属当前主体作用域。
- 真正的 Git 仓库扫描与 Skill 注册由 Importer 完成（当前实现仅落库）。

#### 5.12.2 ZIP 异步导入

**前置参数校验：**

- 文件必填，文件名后缀必须为 `.zip` / `.tar.gz` / `.tgz`，否则 `SKILL_IMPORT_FILE_INVALID`。
- 空文件 → `ILLEGAL_PARAM (未上传文件)`。

**关键流转：**

1. 生成 UUID 任务 ID，写入 `task_execution`，状态置 `PROCESSING`，stage `UPLOAD`，progress `0`。
2. 提交异步任务（独立线程池）：UPLOAD → EXTRACT → VALIDATE → REGISTER → COMPLETED。
3. 解压：根据后缀使用 ZIP 或 TAR.GZ 解压器。
4. 扫描：识别每个含 SKILL.md 的目录为一个候选 Skill；找不到任何 SKILL → 任务失败，错误码 `SKILL_IMPORT_NO_SKILL_FOUND` 含义。
5. 注册：为每个候选 Skill 调用 publish 流程（自动复用 ZIP Provider）；同名 Skill 已存在 → 跳过并记 WARNING；其他失败 → 记 WARNING 但继续。
6. 全部失败（无成功 / 无跳过）→ 标记任务失败。
7. 成功完成 → COMPLETED，进度 100，状态 `COMPLETED`，归档时间写入 `gmt_archive`。

**多表事务约束：**

- 单个 Skill 注册仍走 publish 流程（事务原子性以 publish 为单位）；任务级别没有跨 Skill 事务。

**状态流转限制：**

- 仅当任务状态为 `PROCESSING` 时允许 `markAsCompleted`；否则抛系统异常。

#### 5.12.3 ZIP 同步导入（旧）

- `providerName` 必填；当前实现仅落库一条 ZIP Provider，不做实际导入。

#### 5.12.4 Provider 同步

- `providerId` 必填；查不到 → `SKILL_PROVIDER_NOT_FOUND`；非 `ACTIVE` → `ILLEGAL_PARAM (Provider is not active: ...)`。

#### 5.12.5 Provider 删除

- `providerId` 必填；查不到 → `SKILL_PROVIDER_NOT_FOUND`。
- 当前实现把 Provider 状态置为 `DISABLED`（软删除），不级联删除其下的 Skill。

#### 5.12.6 Provider 列表与详情

- 列表按当前主体作用域返回 Provider 列表；每条返回时通过 `findByProviderId` 计算 Skill 数量。
- 详情：providerId 必填；查不到 → `SKILL_PROVIDER_NOT_FOUND`。

#### 5.12.7 导入任务 SSE 订阅

- `taskId` 必填；任务不存在 → `SKILL_IMPORT_TASK_NOT_FOUND`。
- 订阅成功后：先发送当前快照；若任务已 `COMPLETED` 则发送 complete 事件；若已 `FAILED` 则发送 error 事件。
- 订阅期间发送失败会清理 emitter，避免资源泄露。

#### 5.12.8 导入任务列表

- 当前主体作用域 + `app_type = SKILL_IMPORT` 维度返回任务集合，由数据访问层执行默认排序。

#### 5.12.9 导入任务删除

- `taskIds` 必填；批量清理 SSE emitter，按任务 ID 列表删除任务记录。

---

### 5.13 Skill 版本业务规则

#### 5.13.1 创建版本

**前置校验：**

- `skillId`、`version` 必填。

**版本号校验：**

- `skillId + version + subjectId + scope` 不能与已有版本重复，否则 `ILLEGAL_PARAM (版本号已存在: ...)`。

**关键流转：**

- 当请求 `version` 为空（仅在内部调用场景出现）时按 patch 自增；公开接口要求强制传入。
- 直接保存一条 SkillVersion，不做 latestVersionId 联动（区别于 publish）。

#### 5.13.2 验证版本

**前置校验：**

- `versionId` 必填。

**关联数据校验：**

- 版本不存在 → `SKILL_VERSION_NOT_FOUND`。

**关键流转：**

- 找到对应 Skill 后调用 `Skill.updateLatestVersion(versionId)` 并保存（versionId 不能为空，否则抛业务异常）。

#### 5.13.3 删除版本

**前置校验：**

- `skillId`、`version` 必填。

**关联数据校验：**

- 在主体作用域内查不到对应版本 → `SKILL_VERSION_NOT_FOUND`。

**当前实现：**

- 调用 `versionRepository.save(version)` 实质做更新；真正的物理删除由 Skill 删除时级联完成。

#### 5.13.4 版本列表

- `skillId` 必填。
- 查询排除 `draft`，按 `gmt_create DESC` 排序。
- 描述为空时回退使用 Skill 的描述。

#### 5.13.5 版本详情

- `skillId`、`version` 必填；查不到 → `SKILL_VERSION_NOT_FOUND`。

#### 5.13.6 最新版本

- `skillId` 必填。
- Skill 不存在或 `latest_version_id` 为空 → 返回 `null`。

#### 5.13.7 版本回滚

**前置校验：**

- `skillId`、`targetVersion` 必填。

**关联数据校验：**

- 目标版本在主体作用域内不存在 → `SKILL_VERSION_NOT_FOUND`。
- Skill 不存在 → `SKILL_NOT_FOUND`。

**版本号生成：**

- 在主体作用域内取 Skill 下最新已发布版本，按 patch +1；解析失败或没有历史版本 → 使用 `v0.1.0`。

**关键流转：**

1. 创建新版本实体（描述固定为「回滚到版本 X」），继承目标版本的 metadata。
2. 文件复制：从目标版本目录复制到新版本目录，并以新 fileKey 形成新的 SkillFile 列表；任一失败 → `SYSTEM_ERROR (文件复制失败: ...)`。
3. 保存新版本。
4. 从目标版本快照恢复 Skill 的 `skillName` / icon / imageUrl / 标签关联：
   - 若快照中 icon 为空则清除 Skill 当前 icon。
   - 若快照中 imageUrl 为空则清除 Skill 当前 imageUrl。
   - 若快照中 tagIds 为空则不修改标签。
   - 若目标版本无快照（`snapshotSkillName` 为空）则跳过整个恢复流程。
5. 把目标版本的快照 ext_info 复制到新版本，确保未来回滚到新版本时同样能恢复展示信息。
6. 更新 Skill 的 `latest_version_id`。
7. 打包新版本 ZIP，写入 `zipFilePath`；ZIP 打包失败仅记日志，不阻塞回滚。

**状态流转限制：**

- 不区分目标版本是否为 draft（业务侧调用方应保证传入正式版本号）。

#### 5.13.8 版本导出

**前置校验：**

- `skillId`、`version` 必填。

**关联数据校验：**

- 在主体作用域内查不到 → `SKILL_VERSION_NOT_FOUND`。
- 任意 SkillFile 的 `fileKey` 为空 → `SYSTEM_ERROR (文件 fileKey 缺失: ...)`。
- OSS 流为空或读取失败 → `SYSTEM_ERROR (无法读取文件: ... / 导出失败: ...)`。

**响应规则：**

- 文件名：`{skillName}-v{version}.zip`，其中 `skillName` 为 Skill 表的当前名称（非快照），找不到 Skill 时回退 `skill`。
- 通过响应流直接写出 ZIP，IO 异常 → `SYSTEM_ERROR (导出失败)`。

---

### 5.14 OSS 文件存储规则

| 项 | 规则 |
|----|------|
| Bucket | `beedance-skill` |
| 文件路径模板 | `{skillId}/{version}/{path}`（path 为前端给出的相对路径） |
| ZIP 包路径模板（发布） | `{skillId}/{version}/skill-{skillId}-{version}.zip`，`Content-Type` 为 `application/zip` |
| ZIP 包路径模板（回滚） | `{skillId}/{version}/{skillId}-{version}.zip` |
| 数据复制 | 单文件复制使用 OSS 流式 get + uploadStream；多文件批量删除使用 `removeFiles` |
| 校验和 | 上传后通过 OSS 提供的 lower-case MD5 接口写入 `SkillFile.checksum` |

---

### 5.15 多租户与作用域规则

- 所有 Skill / SkillVersion / SkillProvider 查询均在 `subject_id + scope` 维度内隔离。
- `subjectId` 由 BaseRequest 在运行时根据 `scope` 取自 `appId` / `projectId` / `tenantId`，业务对象保存时按当前请求上下文写入。
- 唯一性约束（Skill 名、Provider 名、SkillVersion 三元组）均叠加 `subject_id + scope`，不同主体或不同作用域下允许重名。

---

### 5.16 标签关联规则

- 标签关系通过 `lhs_source = SKILL`、`rhs_source = SYS_TAG`、`relation_key = SKILL` 维度维护。
- 草稿 / 发布接口若传入 `tagIds`（JSON 字符串）→ 在事务内全量覆盖 Skill 与标签的关系。
- 删除 Skill 时清空标签关系。
- 详情、列表接口返回 Skill 时附带关联标签集合。
- 列表接口的 `categoryTagId` 过滤通过反向查 `lhs_id` 实现。

---

### 5.17 ID 生成与发号器规则

| 业务对象 | 序列名 | 起始值 |
|---------|-------|-------|
| Skill Provider | `skill_provider` | 100000 |
| Skill | `skill` | 100000 |
| Skill Version | `skill_version` | 100000 |

发号由独立的发号器按环境标 `env` 分配；非草稿版本 ID 在 Repository 保存时若为空会自动补填。

---

## 附录：业务对象返回字段速查

### A. SkillInfo（Skill 详情/列表/创建/更新/草稿/发布响应）

| 字段 | 含义 |
|------|------|
| skillId | Skill 主键 |
| providerId / providerName | 关联 Provider |
| skillName | Skill 名称 |
| description | Skill 描述 |
| icon | 图标对象（icon、iconBackground、iconType） |
| imageUrl | 自定义图片 URL |
| categoryTags | 分类标签名列表 |
| status | ACTIVE / DISABLED |
| latestVersion | 最新版本（SkillVersionInfo） |
| versionCount | 正式版本数（不含 draft） |
| gmtCreate / gmtModified | 时间字段 |
| tags | 关联标签列表（详情接口） |
| files | 草稿/正式文件列表（详情接口） |
| versions | 正式版本列表（详情接口，倒序） |

### B. SkillVersionInfo（版本详情/列表/创建/验证响应）

| 字段 | 含义 |
|------|------|
| skillVersionId | 版本主键 |
| skillId / skillName | 所属 Skill |
| version | 版本号或 `draft` |
| description | 版本描述 |
| metadata | SKILL.md 解析后的 JSON 字符串 |
| skillFiles | 版本文件列表（SkillFile） |
| cacheSize | 缓存大小 |
| accessCount | 访问次数 |
| lastAccessTime | 最后访问时间 |
| gmtCreate | 创建时间 |
| publishedBy | 发布人（来自 creator 字段，仅版本列表接口填充） |

### C. SkillProviderInfo（Provider 列表/详情/导入响应）

| 字段 | 含义 |
|------|------|
| skillProviderId | Provider 主键 |
| providerName | 提供者名称 |
| providerType | GIT / ZIP / CUSTOM |
| repoUrl | Git 仓库地址 |
| status | ACTIVE / DISABLED |
| skillCount | 该 Provider 下 Skill 数量 |
| gmtCreate | 创建时间 |

### D. SkillImportTaskInfo（导入任务列表响应）

| 字段 | 含义 |
|------|------|
| taskId | 任务 ID |
| fileName | 上传文件名 |
| fileSize | 文件大小（字节） |
| status | PROCESSING / COMPLETED / FAILED |
| stage | 当前阶段（UPLOAD / EXTRACT / VALIDATE / REGISTER / COMPLETED） |
| progress | 0-100 |
| skillCount | 已导入 Skill 数量（处理中为 null） |
| gmtCreate | 创建时间（ISO 8601） |

### E. SkillImportEventData（SSE 事件结构）

| 字段 | 含义 |
|------|------|
| taskId | 任务 ID |
| status | PROCESSING / COMPLETED / FAILED |
| stage | 当前阶段 |
| progress | 0-100 |
| logs | ImportLog 列表（time / level / message） |
| skillCount | 完成事件携带 |
| providerId | 完成事件携带 |
| errorMessage | 失败事件携带 |





