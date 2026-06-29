# 替换 Repository 实现（如 MySQL → Mongo）

把现有某个 Repository 实现换成另一种存储（如 `SkillRepository` 由 GORM/MySQL 改 MongoDB、`AppConfigRepository` 加 Redis 二级缓存）。架构由 R-40（DDD 分层）锁定：Domain 仅依赖 Repository 接口，实现替换不应影响 Domain Service。

## 前置条件

1. 理由清晰（性能 / schema 灵活性 / 部署形态），且产品方与运维方都同意切换
2. 数据迁移方案落 ADR：双写期 / 切流方式 / 回滚预案
3. 现有 Repository 接口已稳定（接口签名变更需先开 PR 与下游对齐）
4. 阅读 `knowledge/ddd-examples.md` 与目标 Repository 既有实现（`internal/infra/repositories/<entity>.go`）

## 步骤

```bash
cd /path/to/mooc-manus-all/mooc-manus
git switch -c refactor/repo-<entity>-<new-impl>
```

### 1. 确认接口稳定

- Repository 接口定义在 `internal/domains/services/<domain>/repository.go` 或 `internal/domains/services/<entity>.go`
- 列出所有方法：`grep -rn "type <Entity>Repository interface" internal/`
- 如接口需要改 → 先开独立 PR 改接口（保持旧实现可编译），再做实现替换；不要在同个 PR 里既改接口又换实现

### 2. 新实现落 infra 层

新建 `internal/infra/repositories/<entity>_<impl>.go`（如 `skill_mongo.go`）：

```go
package repositories

// ⚠️ R-40 第 3 条：PO 类型仅 repository 内部用；不暴露给 domain

type <Entity>MongoRepository struct { coll *mongo.Collection }

func New<Entity>MongoRepository(c *mongo.Client, db string) domainSvc.<Entity>Repository {
    return &<Entity>MongoRepository{coll: c.Database(db).Collection("<entity>")}
}
// 实现所有接口方法：入参/出参为 DO（domain object），内部转 PO（infra 持久化对象）
```

PO 结构（`XxxPO` / `XxxDocument`）放 `internal/infra/models/`，仅 repository 内部使用（R-40 第 3 条）。

### 3. 双写 / 切流（如需在线迁移）

灰度路径：旧实现保留兜底 → 新实现上线 → 通过 `app_config` feature flag（如 `skill_repo_impl = "mongo"`）切流 → DI 处按 flag 二选一：

```go
if cfg.SkillRepoImpl == "mongo" {
    skillRepo = repositories.NewSkillMongoRepository(...)
} else {
    skillRepo = repositories.NewSkillRepository(db) // 原 GORM
}
```

灰度比例升满后下旧实现；新 PR 删旧文件 + 改 DI 默认路径。

### 4. 数据迁移脚本

- 新建 `scripts/migrate-<entity>-to-<impl>.go`（或独立 binary）：批量从旧库读 → 转换 → 写新库 → 校验 count
- 校验脚本：随机抽样 N 条对比新旧库
- 不要把迁移逻辑塞进 Repository 实现里污染读写路径

### 5. 测试

- 接口契约测试：用同一组 Domain Service 测试同时跑两个实现（参考既有 `<entity>_test.go`）
- 新实现单测：`internal/infra/repositories/<entity>_<impl>_test.go`
- 必测：事务边界 / 并发写 / not found / 索引缺失

### 6. 构建 & commit

```bash
go build ./... && go test ./internal/infra/repositories/... ./internal/domains/services/...
go vet ./...
git add -A
git commit -m "refactor(repo): <Entity>Repository 实现切换至 <impl>"
git push -u origin refactor/repo-<entity>-<new-impl>
```

## 常见坑

1. **PO 类型泄漏到 domain**：在 domain service 内 `import "mooc-manus/internal/infra/models"` 用 PO → R-40 第 3 条违反；转换在 repository 内完成。
2. **接口与实现一把改**：同 PR 改方法签名 + 换实现 + 改 DI → review 失控、回滚困难，必须拆 PR。
3. **事务语义不一致**：GORM `db.Transaction(...)` 与 MongoDB session 语义不同；接口若暗含跨方法事务需先改接口。
4. **索引缺失致慢查询**：迁移前在新库建好与旧库等效索引，否则上线后慢 10x。
5. **回滚预案缺**：feature flag 必备；切流任一侧故障可立刻切回旧实现。
6. **测试只跑新实现**：契约层用同一 fixture 跑新旧两实现确保行为等价。

## 验证

```bash
go build ./...
go test ./internal/infra/repositories/... ./internal/domains/services/... -v
go vet ./...
HARNESS_ROOT=.harness ./.harness/scripts/validate-harness.sh

# 在线灰度
# 1. flag 切 5% 流量到新实现
# 2. 监控错误率 / 延迟
# 3. 逐步升 25% / 50% / 100%
# 4. 比对旧库新库数据一致性脚本
```

## Agent 行为

- 用户说"换个存储" → 先问数据迁移方案、回滚预案；没有方案直接拒绝起手
- 接口改动与实现替换在同一 PR → 强制拆分
- 看到 domain service 内 `import "mooc-manus/internal/infra/models"` → reject（R-40 第 3 条）
- 看到 PO 类型出现在 Repository 接口签名 → reject（R-40 要求行为）
- 灰度切流时未通过 feature flag 而是改 DI 默认值 → 提示加 flag
- ⚠️ 注意 R-40：禁止 domains 依赖 infra 实现包；只依赖 Repository 接口
- 数据迁移脚本未做抽样校验 → 提示补
