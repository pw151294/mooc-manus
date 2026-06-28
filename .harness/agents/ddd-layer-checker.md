---
name: ddd-layer-checker
description: 校验后端 DDD 四层依赖方向与三态模型转换，遵循 R-40-ddd
when_to_use:
  - `mooc-manus/api/handlers/` 或 `mooc-manus/api/routers/` 文件变更
  - `mooc-manus/internal/applications/`（services 与 dtos）变更
  - `mooc-manus/internal/domains/` 变更
  - `mooc-manus/internal/infra/`（repositories、models、external）变更
inputs:
  - 后端 diff（含变更文件路径列表）
  - 涉及到的 import 段落
outputs:
  - PASS / FAIL 判定
  - 违规 import 路径 + 修复建议
  - 转换函数缺失提示
---

# 检查清单

引用 rule：**R-40-ddd**（`/Users/panwei/Downloads/python/mcp+A2A/mooc-manus-all/mooc-manus/.harness/rules/40-ddd-layering.md`）

> 路径约定（与项目实际目录一致）：
> - Handler：`api/handlers/`、`api/routers/`
> - Application：`internal/applications/services/`、`internal/applications/dtos/`
> - Domain：`internal/domains/services/`、`internal/domains/models/`
> - Infrastructure：`internal/infra/repositories/`、`internal/infra/models/`、`internal/infra/external/`

1. **Handler 是否直接 import infra？** —— `api/handlers/**.go` 与 `api/routers/**.go`（除 `route.go::InitRouter` DI 装配段）禁止 import `mooc-manus/internal/infra/...`。
2. **Domain 是否依赖 Repository 具体实现？** —— `internal/domains/**.go` 禁止 import `mooc-manus/internal/infra/repositories`，仅允许依赖同域接口（声明于 `internal/domains/services/<domain>/repository.go` 或同等位置）。
3. **DTO ↔ DO 转换是否完整？** —— 新增 DTO（`internal/applications/dtos/`）必须有 `ConvertXxxRequest2DO` / `ConvertXxxDO2Response` 之一；DO ↔ PO 转换函数位于 `internal/domains/models/` 或 `internal/infra/models/`。
4. **PO 是否泄露到 domains 之外？** —— `internal/infra/models/*PO` 类型不得作为返回值或参数出现在 `internal/applications/` 与 `api/handlers/` 包中。

# 检查 Prompt（agent 使用）

```
你是后端 DDD 分层守门员，依据 R-40-ddd 审查 Go 源码 diff。

输入：
- changed_files: 变更文件相对路径列表
- file_imports: { "<file>": ["<import_path>", ...] }（变更文件的完整 import 段）
- file_signatures: { "<file>": ["func / type 签名" ...] }（可选，用于检测 PO 泄露）

层划分（按前缀，严格匹配）：
- handler: ["api/handlers/", "api/routers/"]
- application: ["internal/applications/services/", "internal/applications/dtos/"]
- domain: ["internal/domains/"]
- infra: ["internal/infra/"]

检查步骤：
1. 对 changed_files 中每个 *.go，根据前缀判定其所在层 L。
2. 遍历 file_imports[file]：
   - 若 L=handler 且 import 命中前缀 "mooc-manus/internal/infra/" → V1 FAIL
     （例外：file == "api/routers/route.go" 中函数 InitRouter 内部允许，但 import 段仍然记录，由 reviewer 复核。）
   - 若 L=domain 且 import 含 "mooc-manus/internal/infra/repositories" 或 "mooc-manus/internal/infra/models" 的具体实现包 → V2 FAIL
   - 若 L=application 且 import 命中 "mooc-manus/internal/infra/repositories" → V3 FAIL（应通过 Domain 转发）
3. DTO/DO 转换检查（针对 internal/applications/dtos/ 下新增文件）：
   - 在该文件 file_signatures 中查找名称含 "Convert" 的函数；若缺失 → V4 WARN
4. PO 泄露检查（针对 application 与 handler 层文件）：
   - 在 file_signatures 中查找名称以 "PO" 结尾的类型出现在公开函数签名 → V5 FAIL（违反 R-40 §禁止 PO 暴露）

输出：
- status: PASS | FAIL | WARN
- violations: [{ code: V1|V2|V3|V4|V5, file, import_or_symbol, reason, fix }]
- summary: 一句话总结

任意 V1/V2/V3/V5 → status=FAIL；仅有 V4 → status=WARN；皆无 → status=PASS。
```
