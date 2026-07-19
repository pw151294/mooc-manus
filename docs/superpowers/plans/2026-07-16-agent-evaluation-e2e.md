# 智能体自动化评测机制 — E2E 功能验证文档

> **用途**：在实施完成后，用来端到端验证「自动化评测机制」的每个业务能力是否真正可用。执行者可以是人肉手动跑 curl，也可以按每节的 `# 期望结果` 里的 SQL/断言写成自动化脚本。

**Spec 依据**：`docs/superpowers/specs/2026-07-16-agent-evaluation-design.md`
**实施计划**：`docs/superpowers/plans/2026-07-16-agent-evaluation-implementation.md`

**前置**：
- 后端已按实施计划完成 M1-M8；
- Redis (`db 1`) 已清空 (`redis-cli -n 1 FLUSHDB`)；
- PostgreSQL 5 张 `eval_*` 表已 AutoMigrate 生成；
- `.env` 里配置有能真实调用的 LLM API key（不建议 mock，E2E 就是要打真链路）；
- 已存在至少 1 个 `appConfig` 记录（NATIVE 工具三件套齐全），记为 `${AGENT_ID}`；
- 前后端服务已启动：
  - 后端服务地址：http://localhost:8080/
  - 前端服务地址：http://localhost:3000/

**约定**：
- 所有 curl 命令写完执行；输出保留到 `.harness/evidence/2026-07-16-eval/*.log`；
- 每步执行后打一个 checkpoint（勾选 `[x]`）；
- 遇到失败 STOP，回读该场景对应的 spec / plan 章节。

---

## E-1 数据层 & 迁移完整性

### E-1.1 五张表齐全

- [ ] `psql` 连本地库执行：

```sql
\dt eval_*
```

**期望结果**：列表出现 `eval_case / eval_task / eval_run_instance / eval_result / eval_agent_snapshot` 五张表，且列 `Type` 均为 `table`。

### E-1.2 索引与约束齐全

- [ ] 执行：

```sql
SELECT indexname, indexdef FROM pg_indexes WHERE tablename LIKE 'eval_%' ORDER BY tablename, indexname;
```

**期望结果**：至少能看到以下索引名（GORM 生成的名字可能有前缀）：

- `uk_task_case_snap`（eval_run_instance 三列唯一）
- `idx_task_status`（task_id, status）
- `idx_status_heartbeat`（status, heartbeat_at）
- `idx_eval_case_tags_gin`（GIN on tags）
- `eval_case.name` UNIQUE

### E-1.3 外键 & CASCADE

- [ ] 执行：

```sql
INSERT INTO eval_case (id, name, task_prompt, verify_script, tags, created_at, updated_at)
VALUES ('c1', 'e2e-test-case', 'echo hi', 'exit 0', '[]', now(), now());

INSERT INTO eval_agent_snapshot (id, source_app_config_id, name, model, system_prompt, tools_config, mcp_config, a2a_config, created_at)
VALUES ('s1', 'app1', 'e2e', 'gpt-4o', 'sys', '{}', '{}', '{}', now());

INSERT INTO eval_task (id, name, status, total_count, case_ids, agent_config_snapshot_ids, created_at)
VALUES ('t1', 'e2e-task', 'PENDING', 1, '["c1"]', '["s1"]', now());

INSERT INTO eval_run_instance (id, task_id, case_id, case_snapshot, agent_config_snapshot_id, status, attempt)
VALUES ('i1', 't1', 'c1', '{}', 's1', 'PENDING', 1);

INSERT INTO eval_result (id, instance_id, passed, verify_exit_code, finished_at)
VALUES ('r1', 'i1', false, 1, now());

-- 删 task 应级联删 instance + result
DELETE FROM eval_task WHERE id = 't1';
SELECT COUNT(*) FROM eval_run_instance WHERE id = 'i1';  -- 期望 0
SELECT COUNT(*) FROM eval_result WHERE id = 'r1';         -- 期望 0

-- snapshot 未随 task 自动删（RESTRICT），需手动清
DELETE FROM eval_agent_snapshot WHERE id = 's1';
DELETE FROM eval_case WHERE id = 'c1';
```

**期望结果**：两次 COUNT(*) 都返回 0，即 CASCADE 生效。

---

## E-2 用例 CRUD & 上传

### E-2.1 上传解析 UTF-8 文本

- [ ] 准备 `/tmp/verify.sh`（内容如 `#!/bin/bash\ntest -f /tmp/out.txt`），执行：

```bash
curl -F 'file=@/tmp/verify.sh' http://localhost:8080/api/eval/cases/upload-content
```

**期望**：`{"code":0,"data":{"content":"#!/bin/bash\ntest -f /tmp/out.txt\n"}}`

### E-2.2 上传超限拒绝

- [ ] 生成一个 300KB 大文件 `/tmp/big.txt`，`curl -F 'file=@/tmp/big.txt'`。

**期望**：HTTP 413 或 `{"code":40001,"message":"upload too large"}`。

### E-2.3 创建 / 查询 / 更新 / 删除用例

- [ ] 创建：

```bash
curl -X POST http://localhost:8080/api/eval/cases \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"e2e-case-1",
    "description":"验证文件读写",
    "init_script":"echo hello > input.txt",
    "task_prompt":"读 input.txt 内容并写到 output.txt",
    "verify_script":"test -f output.txt && grep -q hello output.txt",
    "tags":["file-io"]
  }'
```

**期望**：返回 `{"data":{"id":"<uuid>","name":"e2e-case-1",...}}`。记 `${CASE_ID}`。

- [ ] 列表：`curl 'http://localhost:8080/api/eval/cases?name_like=e2e'` → 出现该 case。
- [ ] 详情：`curl http://localhost:8080/api/eval/cases/${CASE_ID}` → 三要素完整。
- [ ] 更新 description → 200 且 DB 内 `updated_at` 前进。
- [ ] 删除：`curl -X DELETE http://localhost:8080/api/eval/cases/${CASE_ID}` → 200。列表再查已消失。

### E-2.4 有活 task 时删除拒绝

- [ ] 先跑一次 E-3.1 到 `eval_run_instance` 存在阶段，此时 `curl -X DELETE .../cases/${CASE_ID}` → HTTP 409，body 提示引用中。

---

## E-3 任务创建 & M×N 展开

### E-3.1 创建 2 case × 2 agent 任务

- [ ] 造两个 case `${C1}, ${C2}`；准备两个 agent config `${A1}, ${A2}`；执行：

```bash
curl -X POST http://localhost:8080/api/eval/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"e2e-task-2x2",
    "case_ids":["'${C1}'","'${C2}'"],
    "agent_config_ids":["'${A1}'","'${A2}'"]
  }'
```

**期望**：
- 返回 `{"data":{"id":"<task_uuid>","total_count":4,"status":"PENDING",...}}`
- DB：`SELECT COUNT(*) FROM eval_run_instance WHERE task_id=?` → 4
- DB：`SELECT COUNT(*) FROM eval_agent_snapshot WHERE id = ANY(?)` → 2
- Redis：`redis-cli -n 1 LLEN 'asynq:{eval:default}:pending'` → 4（或已被 worker 消费为空）

### E-3.2 引用不存在拒绝

- [ ] 用一个不存在的 case_id 发同样请求：

**期望**：HTTP 400，body 提示 `case_id XXX not found`；DB 无任何 task/snapshot/instance 新增（事务回滚）。

---

## E-4 单实例完整生命周期（PENDING → PASSED）

**准备**：创建一个绝对能通过的 case：`init: echo hi > x`，`prompt: 什么都不做，直接结束`，`verify: exit 0`。

### E-4.1 状态机时间线

- [ ] 创建任务后立刻开始每 500ms 轮询：

```bash
watch -n 0.5 "psql -c \"SELECT status, attempt, conversation_id, heartbeat_at FROM eval_run_instance WHERE task_id='${TASK_ID}'\""
```

**期望依次看到**：
`PENDING` → `QUEUED` → `INITIALIZING` → `RUNNING` → `VERIFYING` → `PASSED`

### E-4.2 结果落库

- [ ] `curl http://localhost:8080/api/eval/instances/${INST_ID}` → `passed=true, verify_exit_code=0, error_log=""`
- [ ] `eval_result.total_tokens > 0`（LLM_CALL span 采集到）
- [ ] `eval_result.agent_latency_ms > 0`
- [ ] `eval_run_instance.trace_id` 非空

### E-4.3 Trace 转发

- [ ] `curl http://localhost:8080/api/eval/instances/${INST_ID}/trace` → 返回 span 列表；至少含 1 条 `AGENT_ROOT` + N 条 `LLM_CALL` + M 条 `TOOL_CALL`。每条 LLM_CALL 有 `tags["llm.usage.prompt_tokens"] > 0`。

---

## E-5 失败与超时路径

### E-5.1 verify 失败 → FAILED

- [ ] 造 case：verify_script = `exit 1`。跑完后：
- 实例最终 status = `FAILED`；result `passed=false, verify_exit_code=1`
- 父 task status = `PARTIAL_FAILED`

### E-5.2 init_script 失败 → FAILED

- [ ] 造 case：init_script = `false`。跑完后：
- 实例 status = `FAILED`；error_log 含 "init_script failed"
- 智能体从未被调起（`eval_result.total_tokens == 0`）

### E-5.3 智能体超时 → TIMEOUT

- [ ] 造 case：prompt = `请无限循环调用 bashExec 执行 sleep 30`；配置 `instance_total_timeout_sec = 60`。
- 60s 后由巡检器强推 TIMEOUT
- error_log 含 "worker_heartbeat_stale_or_deadline_reached"

### E-5.4 verify 脚本 60s 超时 → FAILED（非 TIMEOUT）

- [ ] 造 case：verify_script = `sleep 120`。60s 后：
- 实例 status = `FAILED`（**不是** TIMEOUT，spec §3.3 R2-N4）
- error_log 含 `verify_script_timeout`

---

## E-6 重试语义（就地重跑、保留 attempt 历史）

### E-6.1 单实例重试

- [ ] E-5.1 的 FAILED 实例执行：

```bash
curl -X POST http://localhost:8080/api/eval/instances/${INST_ID}/retry
```

**期望**：
- HTTP 200
- 实例 `status=PENDING, attempt=2, conversation_id=<新 uuid>, message_id=<新 uuid>, started_at=NULL, finished_at=NULL`
- 老的 `eval_result` 保持（供历史查询），但新一轮完成后被 upsert 覆盖
- 观察 Asynq 进入 `eval:high` 队列：`redis-cli -n 1 LLEN 'asynq:{eval:high}:pending'`

### E-6.2 任务级重试

- [ ] `curl -X POST /api/eval/tasks/${TASK_ID}/retry` —— 只重投 FAILED/TIMEOUT，PASSED 实例不动。DB 里 `attempt` 只有失败实例自增。

---

## E-7 并发与限流（Asynq + case 令牌桶）

### E-7.1 单 case 并发上限

**准备**：`case_concurrency_limit = 2`；造 6 条同 case 的 instance（同 case, 6 个 agent）。

- [ ] 观察日志和 Redis：

```bash
redis-cli -n 1 GET 'eval:concurrency:case:${CASE_ID}'
```

- 值应始终 ≤ 2
- worker 日志出现 `case slot busy, retry in 30s`（拿不到令牌者被 Asynq scheduled）

### E-7.2 全局 worker pool 并发

- [ ] 12 并发上限 —— 一次批量入队 100 条 instance，`ps -T -p <pid> | grep -c mooc-manus` 或 pprof `goroutine` 断言活跃 worker goroutine 峰值 ≤ 12（+ 一定框架开销，宽容判定 ≤ 20）。

### E-7.3 goroutine 泄漏压测

- [ ] 用 M8.2 里的 `TestGoroutineLeak_500Instances` 跑一遍；断言前后 goroutine 差 < 20。

---

## E-8 巡检器 3 个 job

### E-8.1 InstanceHeartbeatSweeper（30s）

- [ ] 手工把 DB 里某活跃实例的 `heartbeat_at = now() - 200s` 且 `deadline_at = now() - 10s`；等 30s 内被 sweeper 扫到；实例 status 变 `TIMEOUT`，父 task 计数刷新。

### E-8.2 TaskStatusReconciler（60s）

- [ ] 手工制造脑裂：所有 instance 已 `PASSED` 但 task 仍 `RUNNING`；等 60s 内 reconciler 收敛为 `SUCCEEDED`。

### E-8.3 AsynqDeadTaskArchiver（5min）

- [ ] 通过 asynqmon 手工把某个 task 移入 archive；等 5min 内该实例被同步为 `FAILED`。

---

## E-9 数据可回溯：case 删除 & snapshot 冻结

### E-9.1 case 删除后历史结果仍可读

- [ ] 跑完一次评测（PASSED），然后物理删 case：`curl -X DELETE .../cases/${CASE_ID}` → 200；
- 再查 `curl .../instances/${INST_ID}` → 依然返回完整 `case_snapshot.task_prompt / verify_script / init_script`（从 `eval_run_instance.case_snapshot` 里读）
- Trace 依然可访问

### E-9.2 appConfig 修改后历史评测不受影响

- [ ] 修改 `${AGENT_ID}` 的 system_prompt；重新调 `curl .../instances/${INST_ID}` → snapshot 中的 system_prompt 仍是旧值。

---

## E-10 UI 冒烟（若前端已实现）

在 `mooc-manus-web`：

- [ ] 用例管理页：可创建/编辑/删除；上传 `.sh` 文件后 textarea 自动填入内容；手动/上传互斥 UI 生效
- [ ] 任务创建页：可多选 case（≥1）+ 多选 agent（≥1）；提交后跳转任务详情
- [ ] 任务详情页：实时显示 4 count（成功/失败/进行中/总数）；点击某实例进入实例详情
- [ ] 实例详情页：能展开火焰图（复用现有 trace 组件）；显示 token / latency / verify stdout/stderr
- [ ] 重试按钮：任务级 + 实例级，点击后 status 回 PENDING

---

## E-11 部署健康检查（生产化前置）

**说明（PlanReview-Blk8 修复）**：本项目暂无独立 `/health` 端点整合 asynq/cron 探针。改用启动日志 + Redis 直查作为替代健康信号，不额外增开发工作量。

- [ ] 启动日志包含 `Asynq connected redis db=1`（在 `mq.NewClient` 内 zap info log）
- [ ] 启动日志包含 `Cron scheduler started, 3 jobs registered`
- [ ] Redis db=1 隔离验证：
  - `redis-cli -n 1 KEYS 'asynq:*' | wc -l` > 0（连上后至少有 queue 元数据）
  - `redis-cli -n 0 KEYS 'asynq:*' | wc -l` == 0（业务库无污染）
- [ ] `pprof /debug/pprof/goroutine?debug=1` 采样：无 goroutine 卡在 `chan send` / `select` 超 10 分钟

---

## E-12 冒烟 checklist（发布前最后 5 分钟）

| # | 场景 | 结果 |
|---|---|---|
| 1 | 建 case + 上传脚本 | [ ] |
| 2 | 建 2×2 task | [ ] |
| 3 | 4 条 instance 全部跑到 PASSED | [ ] |
| 4 | 任务态 → SUCCEEDED | [ ] |
| 5 | LLM_CALL span 带 token | [ ] |
| 6 | trace 火焰图完整 | [ ] |
| 7 | 单实例 retry 生效 | [ ] |
| 8 | 强杀一条实例 → TIMEOUT 收敛 | [ ] |
| 9 | 删除 case 后历史仍可读 | [ ] |
| 10 | Redis db=1 隔离 | [ ] |

以上 10 条冒烟全部 ✅，评测机制 v1.0 视为可交付。

---

## E-13 主流程埋点日志矩阵（问题定位入口）

> 前置：pkg/logger 的全局 lumberjack sink 会把评测子系统的 zap.Logger 落盘到
> `mooc-manus/logs/manus.log`。route.go 已把 `evalZap` 切到 `logger.Zap().Named("eval")`；
> mq handler 内部单独 `Named("eval.mq")`。所有埋点日志的 msg 字段以 `EVAL_` 前缀
> 打头，可用 `grep 'EVAL_' logs/manus.log` 一次性拉出评测主流水。

### E-13.1 埋点节点矩阵

| # | 节点 | 触发点（文件:行号语义） | msg 关键字 | 关键字段 |
|---|---|---|---|---|
| 1 | 任务创建入口 | `applications/services/eval.go CreateTask` | `EVAL_TASK_CREATE_START` / `EVAL_TASK_CREATE_DONE` / `EVAL_TASK_CREATE_ERR` | `name / m_cases / n_agents / total_expect / task_id / instance_count` |
| 2 | MQ 入队 | `applications/services/eval.go enqueueInstances` | `EVAL_MQ_ENQUEUE_START` / `EVAL_MQ_ENQUEUE_DONE` / `EVAL_MQ_ENQUEUE_ITEM_ERR` / `EVAL_MQ_NOT_WIRED_SKIP_ENQUEUE` | `queue(default\|high) / count / ok / fail` |
| 3 | MQ 消费 | `infra/mq/run_instance_handler.go ProcessTask` | `EVAL_MQ_CONSUME_START` / `EVAL_MQ_CONSUME_DONE` / `EVAL_MQ_PAYLOAD_UNMARSHAL_ERR` / `EVAL_MQ_INSTANCE_MISS` | `instance_id / attempt / queue_lag_ms / consume_duration_ms / execute_err` |
| 4 | Case 令牌桶 | 同上 | `EVAL_MQ_TOKEN_ACQUIRED` / `EVAL_MQ_TOKEN_BUSY` / `EVAL_MQ_TOKEN_ACQUIRE_ERR` / `EVAL_MQ_TOKEN_RELEASE_ERR` | `instance_id / case_id` |
| 5 | Executor 入口 & CAS | `domains/services/evaluation/executor.go Execute` | `EVAL_STAGE_INIT` / `EVAL_STAGE_INIT_CAS_MISS` / `EVAL_STAGE_INIT_LOAD_ERR` | `instance_id / task_id / case_id / conversation_id / message_id / attempt / worker_id / has_init_script` |
| 6 | init_script 阶段 | `executor.go executeStages` | `EVAL_STAGE_INIT_SCRIPT_DONE` | `workdir / exit_code / duration_ms / run_err` |
| 7 | Snapshot 加载 | 同上 | `EVAL_STAGE_SNAPSHOT_LOAD_ERR` | `snapshot_id` |
| 8 | Chat 阶段 | 同上 | `EVAL_STAGE_RUN_ENTER` / `EVAL_STAGE_RUN_DONE` / `EVAL_STAGE_RUN_ERR` / `EVAL_STAGE_RUN_TIMEOUT` / `EVAL_STAGE_RUN_AGENT_ERR` / `EVAL_STAGE_RUN_CAS_MISS` | `snapshot_id / model / workdir / prompt_len / duration_ms / last_msg_len` |
| 9 | verify_script 阶段 | 同上 | `EVAL_STAGE_VERIFY_DONE` / `EVAL_STAGE_VERIFY_CAS_MISS` | `passed / exit_code / stdout_bytes / stderr_bytes / duration_ms / run_err` |
| 10 | 指标聚合 | `executor.go finalizeVerify` | `EVAL_STAGE_AGGREGATE` | `trace_id / degraded / prompt_tokens / completion_tokens / total_tokens / agent_latency_ms` |
| 11 | 终态收敛 | 同上 | `EVAL_STAGE_FINALIZE` / `EVAL_STAGE_FINALIZE_CAS_MISS` / `EVAL_STAGE_FINALIZE_RESULT_UPSERT_ERR` / `EVAL_STAGE_FINALIZE_RECOUNT_ERR` | `final_status / passed / verify_exit_code / total_tokens / agent_latency_ms / error_log_bytes` |
| 12 | 错误收敛（init/chat 失败路径） | `executor.go finalizeError` | `EVAL_STAGE_FINALIZE_ERROR` / `EVAL_STAGE_FINALIZE_ERROR_CAS_MISS` | `from / to / error_msg` |

### E-13.2 日志查询秘籍

一条实例的完整链路（15 条上下顺序稳定的埋点）：
```bash
INST_ID=<your-instance-id>
grep "$INST_ID" mooc-manus/logs/manus.log | grep -E 'EVAL_(MQ|STAGE|TASK)' | less
```

一次任务的所有实例（按 task_id 聚合）：
```bash
TASK_ID=<your-task-id>
grep "$TASK_ID" mooc-manus/logs/manus.log | grep 'EVAL_STAGE_FINALIZE'
```

### E-13.3 task_prompt 占位符

executor 在 chat 前会对 `task_prompt` 做一次简单字符串替换，让评测用例的 prompt
能够拿到当前实例的 workspace 绝对路径：

| 占位符 | 展开为 | 用途 |
|---|---|---|
| `${EVAL_WORKSPACE}` | `nativeToolsProvider.MessageWorkspaceDir(messageId)` 的绝对路径 | 让 agent 把绝对路径传给 `fileRead` / `bashExec`；`fileEdit` / `fileWrite` 仍用相对路径（相对该 workspace） |

替换发生的位置：`executor.go executeStages` chat 阶段前；日志字段
`workspace_placeholder_used=true` 标记该实例是否使用了占位符。init_script /
verify_script **不做**占位符替换，它们本身就以 workspace 为 cwd。

---

## E-14 高质量评测用例集（覆盖 NATIVE 三件套 + fileWrite）

### E-14.1 用例设计原则

1. **三要素互相绑定**：init_script 铺设的 fixture 必须能被 task_prompt 里的指令使用，
   verify_script 又必须能对 agent 产出做确定性判定；三者任何一个改动都要连带调另外两个。
2. **黑盒判定**：verify_script 只看文件产物（exit 0 = 通过），不依赖 LLM 输出文本。
   避免"agent 说自己做完了"但实际没做的假通过。
3. **工具覆盖完备**：三条用例并集覆盖 `fileRead` / `fileWrite` / `fileEdit` / `bashExec`
   四个 NATIVE 内置工具的核心能力（读、写、精确替换、shell 执行）。
4. **宿主依赖最小化**：init / verify 脚本只依赖 `bash / grep / awk / sed / python3`
   这些 macOS/Linux 都默认自带的工具；不引入 `jq / node` 等可能未安装的依赖。
5. **能力递进**：单工具 → 双工具组合 → 三工具流水线，复杂度从 L1 递增到 L3，便于
   分层排查智能体在哪一层能力上出现回归。

### E-14.2 用例覆盖矩阵

| 用例 | 复杂度 | fileRead | fileEdit | fileWrite | bashExec | tags | 典型失败模式 |
|---|---|---|---|---|---|---|---|
| CASE-1 常量修正 | L1 | ✓ | ✓ | · | · | `["file-io","edit","python"]` | agent 直接照抄 PI 值 / old_string 不唯一 |
| CASE-2 日志统计 | L2 | · | · | ✓ | ✓ | `["bash","aggregation","log"]` | 数错行数 / 输出格式跑偏 / 忘记 fileWrite 落盘 |
| CASE-3 JSON 流水线 | L3 | ✓ | ✓ | ✓ | ✓ | `["multi-tool","json","pipeline"]` | 中间产物未落盘 / metadata 头未加 / JSON 结构破坏 |

### E-14.3 CASE-1 常量修正（fileRead + fileEdit）

**能力目标**：验证智能体能"先读、再精确改"——先用 `fileRead` 定位到需要修改的行，
再用 `fileEdit` 做唯一 old_string 的精确替换，理解"字符串精确匹配 + 唯一性"契约。

**name**：`native-const-fix-pi`

**description**：修复 Python 源文件中错误的 π 常量，保留函数签名与其他代码不变。

**init_script**（在 workspace 内布置一个含错误常量的源文件）：

```bash
#!/usr/bin/env bash
set -euo pipefail

cat > geometry.py <<'PY'
"""基础几何计算工具。
本文件由评测用例 native-const-fix-pi 生成。
"""

# WARN: 下面的 PI 常量因历史原因被错误改成了 3.0，需要修正为标准 float 精度值 3.14159。
PI = 3.0


def circle_area(r: float) -> float:
    """计算半径为 r 的圆的面积。"""
    return PI * r * r


def circle_circumference(r: float) -> float:
    """计算半径为 r 的圆的周长。"""
    return 2 * PI * r


if __name__ == "__main__":
    print(f"area(2) = {circle_area(2)}")
    print(f"circumference(2) = {circle_circumference(2)}")
PY

echo "[init] geometry.py 已生成"
```

**task_prompt**：

```
你当前的工作目录（workspace）绝对路径是：${EVAL_WORKSPACE}

该目录下有一个 Python 源文件 geometry.py，其中的 PI 常量被错误设置为 3.0。
请完成以下两步：

1. 使用 fileRead 读取 ${EVAL_WORKSPACE}/geometry.py 的完整内容，理解上下文；
2. 使用 fileEdit（path 使用相对路径 "geometry.py"，persistent 保持默认 false）
   把 `PI = 3.0` 精确替换为 `PI = 3.14159`；其他代码、注释、空行必须原样保留。

完成后请简短说明你修改的位置。禁止使用 bashExec 直接跑 sed / awk 完成此题，
必须走 fileRead → fileEdit 的能力路径。
```

**verify_script**（`exit 0` 即通过）：

```bash
#!/usr/bin/env bash
set -euo pipefail

FILE="geometry.py"

# 1. 文件必须存在
[[ -f "$FILE" ]] || { echo "verify: geometry.py 不存在"; exit 1; }

# 2. PI 必须已被改为 3.14159（允许结尾空白/注释）
grep -Eq '^PI[[:space:]]*=[[:space:]]*3\.14159[[:space:]]*(#.*)?$' "$FILE" \
  || { echo "verify: 未检测到 PI = 3.14159"; grep -n '^PI' "$FILE"; exit 2; }

# 3. 错误值必须被清除
if grep -Eq '^PI[[:space:]]*=[[:space:]]*3\.0[[:space:]]*(#.*)?$' "$FILE"; then
  echo "verify: 旧值 PI = 3.0 仍存在"
  exit 3
fi

# 4. 函数签名必须保留（防止 agent 顺手重构）
grep -q 'def circle_area(r: float) -> float:' "$FILE" \
  || { echo "verify: circle_area 签名被改动"; exit 4; }
grep -q 'def circle_circumference(r: float) -> float:' "$FILE" \
  || { echo "verify: circle_circumference 签名被改动"; exit 5; }

# 5. 执行一次 python，输出 area(2) 应≈12.56636，圆周长应≈12.56636
if command -v python3 >/dev/null 2>&1; then
  OUT=$(python3 "$FILE")
  echo "$OUT" | grep -Eq 'area\(2\)[[:space:]]*=[[:space:]]*12\.56[0-9]+' \
    || { echo "verify: python3 执行输出未包含期望 area(2) ≈ 12.56"; echo "$OUT"; exit 6; }
fi

echo "verify: PASSED"
```

### E-14.4 CASE-2 日志统计（bashExec + fileWrite）

**能力目标**：验证智能体能用 `bashExec` 做数据处理、并用 `fileWrite` 把最终报告
落盘到 workspace（bashExec 的 cwd 是 manus 进程目录，不是 workspace，所以 agent
必须显式用 fileWrite 而不是重定向到某个"当前目录"）。

**name**：`native-log-stat-http`

**description**：统计一份 access 日志中各 HTTP 状态码出现次数，输出到 report.txt。

**init_script**：

```bash
#!/usr/bin/env bash
set -euo pipefail

cat > access.log <<'LOG'
127.0.0.1 - - [18/Jul/2026:10:00:00 +0800] "GET /api/health HTTP/1.1" 200 5
127.0.0.1 - - [18/Jul/2026:10:00:01 +0800] "GET /api/users HTTP/1.1" 200 128
127.0.0.1 - - [18/Jul/2026:10:00:02 +0800] "POST /api/login HTTP/1.1" 401 47
127.0.0.1 - - [18/Jul/2026:10:00:03 +0800] "GET /favicon.ico HTTP/1.1" 404 0
127.0.0.1 - - [18/Jul/2026:10:00:04 +0800] "GET /api/users/9 HTTP/1.1" 200 96
127.0.0.1 - - [18/Jul/2026:10:00:05 +0800] "GET /api/users/x HTTP/1.1" 404 0
127.0.0.1 - - [18/Jul/2026:10:00:06 +0800] "POST /api/orders HTTP/1.1" 500 220
127.0.0.1 - - [18/Jul/2026:10:00:07 +0800] "POST /api/login HTTP/1.1" 200 64
127.0.0.1 - - [18/Jul/2026:10:00:08 +0800] "GET /api/orders HTTP/1.1" 200 512
127.0.0.1 - - [18/Jul/2026:10:00:09 +0800] "POST /api/login HTTP/1.1" 401 47
LOG

echo "[init] access.log 已生成，共 $(wc -l < access.log) 条记录"
```

**task_prompt**：

```
你当前的工作目录（workspace）绝对路径是：${EVAL_WORKSPACE}

该目录下有一个 nginx access 风格的日志文件 access.log。请你：

1. 使用 bashExec 分析 ${EVAL_WORKSPACE}/access.log，按 HTTP 状态码分组统计出现次数；
   命令可参考：awk '{print $9}' ${EVAL_WORKSPACE}/access.log | sort | uniq -c | sort -rn
   （风险等级 safe，本命令只读、不修改文件）；
2. 使用 fileWrite（path="report.txt"，persistent 保持默认 false，overwrite=true）
   把统计结果写入 workspace 下的 report.txt。文件内容格式（严格）：
       每一行： "<状态码> <次数>"
       两个字段用一个空格分隔；
       按次数从多到少排序，次数相同按状态码升序；
       不要有多余的表头、总计行、空行。
   本用例中的 access.log 期望产出：
       200 5
       401 2
       404 2
       500 1

只允许通过 bashExec + fileWrite 两个工具完成，禁止 fileEdit / fileRead。
```

**verify_script**：

```bash
#!/usr/bin/env bash
set -euo pipefail

REPORT="report.txt"
[[ -f "$REPORT" ]] || { echo "verify: report.txt 不存在"; exit 1; }

# 期望内容（顺序 + 值都必须严格匹配）
EXPECT=$(cat <<'EOF'
200 5
401 2
404 2
500 1
EOF
)

ACTUAL=$(cat "$REPORT")

if [[ "$ACTUAL" != "$EXPECT" ]]; then
  echo "verify: report.txt 内容不匹配"
  echo "--- expect ---"; printf '%s\n' "$EXPECT"
  echo "--- actual ---"; printf '%s\n' "$ACTUAL"
  # 常见错误定位
  diff <(printf '%s\n' "$EXPECT") <(printf '%s\n' "$ACTUAL") || true
  exit 2
fi

# access.log 不能被误删
[[ -f "access.log" ]] || { echo "verify: access.log 被误删"; exit 3; }

echo "verify: PASSED"
```

### E-14.5 CASE-3 JSON 流水线（四工具串联）

**能力目标**：验证智能体在"读取 → 计算 → 精确改结构 → 追加新文件"多步任务里的
工具选型和产物落盘能力，覆盖 fileRead / bashExec / fileEdit / fileWrite 四件套。

**name**：`native-json-summary-pipeline`

**description**：读取一份订单 JSON，用 bashExec 计算聚合值，用 fileEdit 修正原文件
的 `meta.total_amount` 字段，用 fileWrite 追加一份摘要 summary.md。

**init_script**：

```bash
#!/usr/bin/env bash
set -euo pipefail

cat > orders.json <<'JSON'
{
  "meta": {
    "generated_at": "2026-07-18T10:00:00+08:00",
    "currency": "CNY",
    "total_amount": 0
  },
  "orders": [
    { "id": "O-1001", "user": "alice",   "amount": 128.50, "status": "PAID"    },
    { "id": "O-1002", "user": "bob",     "amount":  49.90, "status": "PAID"    },
    { "id": "O-1003", "user": "carol",   "amount": 320.00, "status": "REFUND"  },
    { "id": "O-1004", "user": "alice",   "amount":  15.00, "status": "PAID"    },
    { "id": "O-1005", "user": "dave",    "amount": 799.00, "status": "PAID"    },
    { "id": "O-1006", "user": "bob",     "amount":  20.00, "status": "CANCEL"  }
  ]
}
JSON

echo "[init] orders.json 已生成"
```

**task_prompt**：

```
你当前的工作目录（workspace）绝对路径是：${EVAL_WORKSPACE}

该目录下有一份订单 JSON 文件 orders.json（顶层字段 meta / orders）。请完成以下流水线：

1. 使用 fileRead 读取 ${EVAL_WORKSPACE}/orders.json 的完整内容，理解结构；
2. 使用 bashExec 计算 status 为 "PAID" 的订单 amount 之和（保留两位小数）。
   推荐用 python3 -c 一次算完，例如：
       python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print(f"{sum(o[\"amount\"] for o in d[\"orders\"] if o[\"status\"]==\"PAID\"):.2f}")' ${EVAL_WORKSPACE}/orders.json
   （风险等级 safe，纯只读计算）；
3. 使用 fileEdit 把 orders.json 中的 `"total_amount": 0` 精确替换成
   `"total_amount": <你算出的和>`（保留两位小数，如 992.40）。其他字段 / 缩进 / 引号
   必须原样保留；
4. 使用 fileWrite（path="summary.md"，persistent 保持默认 false，overwrite=true）
   写入一份 Markdown 摘要，格式（严格）：
       # Orders Summary
       - total_paid_amount: <值>
       - paid_order_count: <值>
       - top_user: <PAID 中 amount 累计最多的 user>
   本用例期望值：total_paid_amount = 992.40，paid_order_count = 4，top_user = dave。

四个工具都要用到；顺序建议 fileRead → bashExec → fileEdit → fileWrite。
```

**verify_script**：

```bash
#!/usr/bin/env bash
set -euo pipefail

command -v python3 >/dev/null 2>&1 || { echo "verify: python3 不可用，无法校验 JSON"; exit 90; }

[[ -f "orders.json" ]] || { echo "verify: orders.json 不存在"; exit 1; }
[[ -f "summary.md"  ]] || { echo "verify: summary.md 不存在";  exit 2; }

# 1. orders.json 必须是合法 JSON 且 meta.total_amount = 992.40
python3 - <<'PY'
import json, sys
d = json.load(open("orders.json"))
meta = d.get("meta", {})
ta = meta.get("total_amount")
if not isinstance(ta, (int, float)):
    print("verify: meta.total_amount 不是数字:", ta); sys.exit(11)
if round(float(ta), 2) != 992.40:
    print(f"verify: meta.total_amount 期望 992.40，实际 {ta}"); sys.exit(12)
# 订单数与结构不能被破坏
orders = d.get("orders", [])
if len(orders) != 6:
    print(f"verify: orders 数量应为 6，实际 {len(orders)}"); sys.exit(13)
required_ids = {"O-1001","O-1002","O-1003","O-1004","O-1005","O-1006"}
if {o.get("id") for o in orders} != required_ids:
    print("verify: orders.id 集合被改动"); sys.exit(14)
PY

# 2. summary.md 内容按行严格匹配
EXPECT=$(cat <<'EOF'
# Orders Summary
- total_paid_amount: 992.40
- paid_order_count: 4
- top_user: dave
EOF
)
ACTUAL=$(cat summary.md)
if [[ "$ACTUAL" != "$EXPECT" ]]; then
  echo "verify: summary.md 内容不匹配"
  echo "--- expect ---"; printf '%s\n' "$EXPECT"
  echo "--- actual ---"; printf '%s\n' "$ACTUAL"
  diff <(printf '%s\n' "$EXPECT") <(printf '%s\n' "$ACTUAL") || true
  exit 3
fi

echo "verify: PASSED"
```

---

## E-15 端到端验证 SOP（用例集 + 埋点联动）

> 本节把 E-14 三条用例与 E-13 埋点串联，得到一条"从建用例到问题定位"的完整动线。
> 前置：后端已按 E-13 加载新的埋点，服务已 `go run ./main.go`，前端 `pnpm dev`
> 起在 :3000，浏览器打开 `http://localhost:3000/eval/cases`。

### E-15.1 录入三条用例（前端 UI 或 REST 直调）

前端路径：Eval → 用例管理 → "创建用例" → 分别录入 CASE-1 / CASE-2 / CASE-3
（name / description / tags / init_script / task_prompt / verify_script 各字段一一对应）。
上传 `.sh` 文件时 ScriptInput 会自动填入内容，注意最终点击的是"手动"模式提交
（防止 UI 保存的是文件对象而非文本）。

REST 兜底：
```bash
BASE=http://localhost:8080
curl -s -XPOST $BASE/api/eval/cases -H 'Content-Type: application/json' \
  -d @docs/superpowers/plans/eval-fixtures/case-1.json | jq -r '.id'
```
（fixture 文件可选择性生成，主要用于跑 CI；本文档不强制）

**验收**：
- [ ] 三条用例出现在列表页；tags 展示为 chip；点击行可预览三段脚本
- [ ] `psql`：`SELECT name, tags FROM eval_case ORDER BY created_at DESC LIMIT 3;`
      返回三条记录，name 分别为 `native-const-fix-pi` / `native-log-stat-http` /
      `native-json-summary-pipeline`

### E-15.2 建一次 3×1 评测任务并观察埋点

前端：Eval → 评测任务 → "创建任务"
- 任务名：`e2e-native-tools-smoke`
- 用例：勾选 CASE-1 / CASE-2 / CASE-3（M=3）
- Agent：任选 1 个 NATIVE 三件套齐全的 appConfig（N=1）
- 提交后跳转任务详情，应立刻看到 3 条 instance，状态由 PENDING → QUEUED → INITIALIZING 依次推进

**同时在另一个终端 tail 日志**：
```bash
tail -F mooc-manus/logs/manus.log | grep -E 'EVAL_(TASK|MQ|STAGE)'
```

**期望在 10s 内至少看到（按顺序）**：
```
EVAL_TASK_CREATE_START   name=e2e-native-tools-smoke m_cases=3 n_agents=1 total_expect=3
EVAL_TASK_CREATE_DONE    task_id=<T> instance_count=3
EVAL_MQ_ENQUEUE_START    queue=default count=3
EVAL_MQ_ENQUEUE_DONE     ok=3 fail=0
EVAL_MQ_CONSUME_START   ×3   （instance_id 逐个出现）
EVAL_MQ_TOKEN_ACQUIRED  ×3
EVAL_STAGE_INIT         ×3   has_init_script=true
EVAL_STAGE_INIT_SCRIPT_DONE ×3   exit_code=0
EVAL_STAGE_RUN_ENTER    ×3   workspace_placeholder_used=true model=<...>
EVAL_STAGE_RUN_DONE     ×3
EVAL_STAGE_VERIFY_DONE  ×3   passed=true exit_code=0
EVAL_STAGE_AGGREGATE    ×3   degraded=false total_tokens>0
EVAL_STAGE_FINALIZE     ×3   final_status=PASSED
EVAL_MQ_CONSUME_DONE    ×3
```

### E-15.3 单实例失败模式复现（负例）

选一条实例（例如 CASE-1）人为破坏 verify_script 或 task_prompt，重跑一次：

- **模式 A：写错 old_string**（把 task_prompt 里 `PI = 3.0` 换成 `PI = 3.00`）
  → 预期看到 `EVAL_STAGE_VERIFY_DONE passed=false exit_code=2` 和
     `EVAL_STAGE_FINALIZE final_status=FAILED`；前端 InstanceDrawer 的
     verify_stdout 应显示 `verify: 未检测到 PI = 3.14159`。

- **模式 B：init_script 故意 exit 1**（在 CASE-2 init 结尾加 `exit 1`）
  → 预期看到 `EVAL_STAGE_INIT_SCRIPT_DONE exit_code=1` 紧接
     `EVAL_STAGE_FINALIZE_ERROR from=INITIALIZING to=FAILED`；实例状态直接跳 FAILED，
     不进入 RUNNING。

- **模式 C：把 CaseConcurrencyLimit 设为 1，同时投递 4 条**（Case-1 × 4 副本）
  → 预期看到多条 `EVAL_MQ_TOKEN_BUSY`，asynq 30s 后重投；最终仍全部 PASSED。

### E-15.4 通过日志定位问题的推荐动线

发现某条 instance 停在 RUNNING 或直接 FAILED：

```bash
INST=<instance_id>

# 1. 拉出该实例的完整链路（一次实例通常 12~20 行）
grep "$INST" mooc-manus/logs/manus.log | grep 'EVAL_'

# 2. 看最后一次 stage：出错在 init / run / verify / aggregate 哪一段
grep "$INST" mooc-manus/logs/manus.log | grep 'EVAL_STAGE_' | tail -5

# 3. run 段失败：结合 conversation_id 拉 baseAgent 的原始 LLM/工具日志
CONV=$(grep "$INST" mooc-manus/logs/manus.log | grep EVAL_STAGE_RUN_ENTER \
        | sed -E 's/.*"conversation_id": "([^"]+)".*/\1/' | head -1)
grep "$CONV" mooc-manus/logs/manus.log | tail -80

# 4. verify 段失败：直接看 verify_stdout（前端 InstanceDrawer）或
#    在 DB 里查 eval_result.verify_stderr
psql -c "SELECT verify_exit_code, verify_stderr FROM eval_result WHERE instance_id='$INST';"

# 5. token 埋点失败：ai_span 表按 trace_id 查完整火焰图
TRACE=$(grep "$INST" mooc-manus/logs/manus.log | grep EVAL_STAGE_AGGREGATE \
        | sed -E 's/.*"trace_id": "([^"]+)".*/\1/' | head -1)
psql -c "SELECT span_type, operation_name, latency_ms, is_error FROM ai_span WHERE trace_id='$TRACE' ORDER BY start_time;"
```

### E-15.5 用例集回归 checklist（跑通即视为验收）

| # | 项 | 命令 / 观察点 | 结果 |
|---|---|---|---|
| 1 | CASE-1 单跑 → PASSED | 任务详情该实例 status=PASSED | [ ] |
| 2 | CASE-2 单跑 → PASSED | 同上 | [ ] |
| 3 | CASE-3 单跑 → PASSED | 同上 | [ ] |
| 4 | 三条用例共 1 Agent 3×1 任务全 PASSED | `EVAL_STAGE_FINALIZE final_status=PASSED` 出现 3 次 | [ ] |
| 5 | manus.log 内 `EVAL_` 埋点 15 类全部出现过 | `grep -oE 'EVAL_[A-Z_]+' logs/manus.log \| sort -u` ≥ 15 行 | [ ] |
| 6 | trace 火焰图非 degraded | `EVAL_STAGE_AGGREGATE degraded=false` | [ ] |
| 7 | 负例 A（改错 old_string）落 FAILED，日志能直接指向 verify | 见 E-15.3 | [ ] |
| 8 | 负例 B（init exit 1）落 FAILED，from=INITIALIZING | 见 E-15.3 | [ ] |
| 9 | 负例 C（case token busy）出现 `EVAL_MQ_TOKEN_BUSY` 且最终全 PASSED | 见 E-15.3 | [ ] |
| 10 | evalZap 落盘到 manus.log（不再是 stderr）| `grep '"logger":"eval' logs/manus.log \| head -1` 非空 | [ ] |

全部 ✅ → 评测系统 v1.0 端到端验证收敛完成，可交付。

---

**End of E2E Verification Document**
