# 智能体自动化评测机制 — E2E 功能验证文档

> **用途**：在实施完成后，用来端到端验证「自动化评测机制」的每个业务能力是否真正可用。执行者可以是人肉手动跑 curl，也可以按每节的 `# 期望结果` 里的 SQL/断言写成自动化脚本。

**Spec 依据**：`docs/superpowers/specs/2026-07-16-agent-evaluation-design.md`
**实施计划**：`docs/superpowers/plans/2026-07-16-agent-evaluation-implementation.md`

**前置**：
- 后端已按实施计划完成 M1-M8；
- 服务在本机 `:8080` 可访问；
- Redis (`db 1`) 已清空 (`redis-cli -n 1 FLUSHDB`)；
- PostgreSQL 5 张 `eval_*` 表已 AutoMigrate 生成；
- `.env` 里配置有能真实调用的 LLM API key（不建议 mock，E2E 就是要打真链路）；
- 已存在至少 1 个 `appConfig` 记录（NATIVE 工具三件套齐全），记为 `${AGENT_ID}`；
- 前端 `mooc-manus-web` 已跑起来（可选，若只走 API 层验证可省略）。

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

- [ ] `curl http://localhost:8080/health` 或类似 endpoint 应含 `asynq: connected, cron: running` 子项
- [ ] Redis db=1 隔离验证：`redis-cli -n 1 KEYS 'asynq:*' | wc -l` > 0；`redis-cli -n 0 KEYS 'asynq:*' | wc -l` == 0
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

**End of E2E Verification Document**
