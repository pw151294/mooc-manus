# 智能体自动化评测机制 — 部署与运维指南

> 目标读者：负责将 mooc-manus 评测系统部署到生产/预发环境的工程师。
> 依赖文档：`docs/evaluation-system-architecture.md`（架构） / `docs/superpowers/specs/2026-07-16-agent-evaluation-design.md`（设计）。

## 1. 前置依赖

| 组件 | 版本 | 用途 |
|------|------|------|
| PostgreSQL | 15+ | 评测数据落库（5 张 `eval_*` 表） |
| Redis | 7+（AOF everysec 建议） | Asynq 消息队列 + CaseTokenGate 令牌桶 |
| Go | 1.22+ | 项目 runtime |

## 2. 配置项

在 `config/config.toml` 中新增 `[evaluation]` 与 `[asynq]` 段。示例：

```toml
[evaluation]
enabled = true
worker_concurrency_default = 5    # 默认队列并发度
worker_concurrency_high = 5       # 高优先级（Retry）队列并发度
case_concurrency_limit = 4        # 单 case 并发上限（令牌桶大小）
instance_total_timeout_sec = 900  # 单 instance 硬超时（含 chat + verify）
verify_script_timeout_sec = 60    # verify_script 单次超时
verify_output_cap_bytes = 65536   # verify stdout/stderr 单侧上限
heartbeat_interval_sec = 15
heartbeat_stale_threshold_sec = 90  # 用于 Cron sweeper
cron_sweeper_interval_sec = 30
cron_reconciler_interval_sec = 60
cron_dlq_archive_interval_sec = 300
upload_content_max_bytes = 262144   # /cases/upload-content 单文件上限 256KB

[asynq]
redis_addr = "127.0.0.1:6379"
redis_password = ""
redis_db = 1  # 建议与业务 redis db 分离
```

如果 `[asynq]` 段的 `redis_addr / redis_password / redis_db` 留空，会自动 fallback 到 `[redis]` 段。

## 3. 首次部署步骤

1. 在目标数据库执行 `AutoMigrate` —— 服务启动即触发（见 `internal/infra/storage/postgres.go`）。首次启动会生成 5 张 `eval_*` 表。
2. Redis 端配置 AOF：`redis-cli CONFIG SET appendonly yes && CONFIG SET appendfsync everysec`；否则 Asynq 任务在意外重启时可能丢失（可容忍，Cron reconciler 会兜底重投）。
3. 启动服务后观察日志：`asynq server started` / `eval cron scheduler started` 三条必现。
4. 可选：`asynqmon`（[github.com/hibiken/asynqmon](https://github.com/hibiken/asynqmon)）作为 Web UI 观察队列状态：
   ```bash
   docker run --rm --name asynqmon -p 8080:8080 hibiken/asynqmon --redis-addr=host.docker.internal:6379 --redis-db=1
   ```

## 4. 关键端点

前缀 `/api/eval`：

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/cases/upload-content` | 上传 init/verify 脚本文本（multipart，256KB 上限） |
| POST | `/cases` | 创建评测用例 |
| GET | `/cases` | 列表（支持 name_like + tags） |
| GET | `/cases/:id` | 单个用例 |
| PUT | `/cases/:id` | 更新（有活引用会 409） |
| DELETE | `/cases/:id` | 删除（有活引用返回 409） |
| POST | `/tasks` | 创建评测任务（自动 M×N 生成 instance + Enqueue） |
| GET | `/tasks` | 列表 |
| GET | `/tasks/:id` | 单个 |
| POST | `/tasks/:id/retry` | 只 retry FAILED/TIMEOUT 实例，走 high 队列 |
| DELETE | `/tasks/:id` | 删除（CASCADE 删实例/结果，snapshot 单独清） |
| GET | `/tasks/:id/instances` | 任务下实例列表 |
| GET | `/instances/:id` | 单个实例（含 result） |
| GET | `/instances/:id/trace` | 拿实例的 trace_id |
| POST | `/instances/:id/retry` | 单实例重跑（high 队列） |
| DELETE | `/instances/:id` | 单实例删除 |
| GET | `/agent-configs` | 可用 agent config 列表（供前端选择） |

## 5. 运维观察

- **队列积压**：`asynq:eval:default` / `asynq:eval:high` 待处理数。默认+高 = worker 总并发；持续积压需要扩 worker_concurrency_*。
- **DLQ 堆积**：`asynq:eval:dlq` 是最终失败任务；Cron `DLQArchiverJob` 会周期把这些死信状态收敛回 `eval_run_instance.status=FAILED`。
- **失联 worker**：Cron `SweeperJob` 每 30s 扫描 `heartbeat_at < now-90s OR deadline_at < now`，把 stale 实例推 TIMEOUT。
- **Task 状态失衡**：Cron `ReconcilerJob` 每 60s 对 PENDING/RUNNING task 跑一次 RecountAndTransit。

## 6. 已知限制

- CreateTask 目前非事务：如果 `snapshot 批量落库成功但后续 case 加载失败`，snapshot 会残留 —— 由 DeleteTask 或离线巡检兜底。
- ArchiveDeadTasks 需要注入 `DLQInspector`（`asynq.Inspector` 的适配器）。当前 route.go 装配位置传入 `nil`，会静默返回 `(0, nil)` —— 需在生产上补齐（M9 nice-to-have）。
- Snapshot 目前未冻结 AgentConfig（MaxIterations/MaxRetries/MaxSearchResults），使用兜底默认（20/3/10）。若评测需要精确复现 agent 循环上限，需要扩 AgentSnapshot 结构并同步更新 FreezeAppConfig。

## 7. 排查手册

**问题**：任务卡在 PENDING 不进队列
- 查看 `asynqSrv` 启动日志是否成功
- 查看 `mq.NewClient` 是否连通 Redis
- 手工 `redis-cli -n 1 LLEN asynq:eval:default` 看待处理数

**问题**：Instance 卡 RUNNING 不推进
- 心跳超时会由 Sweeper 处理，最多等 90s + 30s（sweeper 周期）
- 若仍卡：查 `eval_run_instance.worker_id` 找归属 worker 进程

**问题**：Verify 输出被截断
- 单侧上限 64KB，超出末尾追加 `\n[truncated]`；可以调 `verify_output_cap_bytes`

**问题**：TraceAggregator 返回 `Degraded=true`
- 表示未找到 AGENT_ROOT span；说明 tracer flush 不及时或 conversation 从未启动 AGENT_ROOT
- 检查 `internal/domains/services/agents/agent.go` 里 `StartRootSpan` 是否被调用

## 8. 相关命令

```bash
# 编译 + 全量测试（含新增评测部分）
go build ./...
go test ./... -count=1

# 只测评测领域包
go test ./internal/domains/services/evaluation/... -v -count=1

# 只测 Asynq 基础设施
go test ./internal/infra/mq/... -v -count=1

# 500 条 instance 压测（tag=stress）
go test -tags=stress ./internal/domains/services/evaluation/... -run TestStress -v -count=1
```
