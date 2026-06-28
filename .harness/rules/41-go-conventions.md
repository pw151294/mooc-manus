---
rule_id: R-41-go
severity: medium
---

# Go 编码规范

迁移自 `.harness/archive/cursorrules-pre-harness-v1` 与 `.harness/archive/conventions-pre-harness-v1.md`。本规则覆盖错误处理、日志、命名、测试四项后端日常编码风格。

## 禁止行为

1. **禁止吞掉错误**
   - 禁止 `_ = err` / `_, _ = fn()` 等丢弃 error
   - 禁止裸 `return err` 时无上下文（顶层入口处的兜底转换除外）
   - 禁止 `panic(err)` 入仓（除 `init()` 阶段的不可恢复错误）

2. **禁止用 `fmt.Println` / `log.Printf` 入仓**
   - 调试日志走项目统一 zap logger（见 `pkg/logger/`），不得遗留 `fmt.Println`、`println`、标准库 `log` 包调用

3. **禁止违反命名约定**
   - 包名禁止驼峰或下划线（如 `appConfig` / `app_config`）；统一小写无分隔（`appconfig` / `agents`）
   - 文件名禁止驼峰或复数（如 `SkillProvider.go` / `skills.go`）；统一蛇形单数（`skill_provider.go`）
   - 接口名禁止以 `I` 前缀；统一以 `-er` 或 `Service` / `Repository` 后缀（`SkillRepository` / `SkillDomainService`）
   - 时间字段禁止 `gmtCreate` / `gmtModified`，统一 `CreatedAt` / `UpdatedAt`

4. **禁止测试副作用**
   - `*_test.go` 禁止 `os.Exit` / `os.Setenv` 跨用例污染 / 直接连真实 PostgreSQL / 真实 Docker
   - 禁止依赖测试执行顺序

## 要求行为

1. **错误处理 wrap with `%w`**
   ```go
   if err != nil {
       return fmt.Errorf("convert skill request: %w", err)
   }
   ```
   - 哨兵错误统一定义在 `pkg/<domain>err/errors.go`（如 `pkg/skillerr`），避免领域层反向依赖应用层
   - Handler 层通过 `errors.Is` 映射 HTTP 状态码（参考 `api/handlers/skill.go::writeError`）

2. **日志使用项目 zap logger**
   - 统一 logger 入口（参见 `pkg/logger/`）
   - 错误日志带上下文字段：`logger.Error("import skill failed", zap.String("skillId", id), zap.Error(err))`

3. **命名（汇总，详见 `.harness/archive/cursorrules-pre-harness-v1`）**
   | 维度 | 规则 | 示例 |
   |------|------|------|
   | 文件 | 蛇形单数 | `skill_provider.go` |
   | 包 | 复数小写 | `handlers` / `services` / `repositories` |
   | PO | `XxxPO` | `SkillProviderPO` |
   | DO | `XxxDO` | `SkillDO` |
   | DTO | `XxxRequest` / `XxxDTO` | `SkillDraftSaveRequest` |
   | 接口 | `XxxService` / `XxxRepository` | `SkillDomainService` |
   | 实现 | 接口名 + `Impl`（Service/Repository） | `SkillRepositoryImpl` |
   | 构造 | `NewXxx` 返回接口 | `NewSkillRepository() SkillRepository` |
   | 转换 | `ConvertSrc2Dst` | `ConvertSkillPO2DO` |

4. **测试使用 `testing` + `testify`**
   - 单元测试用 `testify/assert` / `testify/require`
   - 跨进程依赖用 mock / fake，不连真实基础设施
   - 集成测试单独标记（`//go:build integration`）

## Agent 行为

- 生成代码时按上表命名；检测到 `gmt_create` / `delete_flag` / `Response[T]` 等禁用标识 → 拒绝并按规范重写
- 错误处理：补全 `%w` wrap 与 sentinel 检查；不主动添加 `panic`
- 日志：替换 `fmt.Println` 为 logger 调用

## 可验证性

- `golangci-lint` 启用 `errcheck` / `errorlint` / `revive` / `goimports`
- `pre-commit` hook 跑 `go vet` + `golangci-lint run --new`
- grep 检查：
  - `grep -rn "fmt.Println\|fmt.Print(" internal/ api/` 应为空
  - `grep -rn "_ = err" internal/ api/` 应为空
  - `grep -rn "gmt_create\|gmtCreate\|delete_flag" .` 应为空
