# 智能体自动化评测机制 — E2E 手工验证记录（占位）

> 使用说明：按 `docs/superpowers/plans/2026-07-16-agent-evaluation-e2e.md` 里的 E-1 ~ E-10 逐节手工过一遍，把每步的实际输出/断言结果登记在这里。若发现与 spec 不符 → 记录到 issue 追踪。

**执行环境**：
- 本机：
- Postgres 版本：
- Redis 版本：
- 服务端口：`:8080`
- Redis DB：1
- 前端：`mooc-manus-web` 起在 `:3000`（可选）
- 执行时间：待填

---

## E-1 数据层 & 迁移完整性

- E-1.1 五张表齐全：待验证
- E-1.2 索引齐全：待验证
- E-1.3 外键 & CASCADE：待验证

## E-2 用例 (Case) CRUD

- E-2.1 上传内容：待验证
- E-2.2 创建：待验证
- E-2.3 列表 + 过滤：待验证
- E-2.4 更新：待验证
- E-2.5 删除（无活引用）：待验证

## E-3 任务 (Task) 创建与 M×N 生成

- E-3.1 CreateTask：待验证
- E-3.2 Instance 数 = M×N：待验证
- E-3.3 Asynq 队列积压情况：待验证

## E-4 实例执行主流程

- E-4.1 PASSED 分支：待验证
- E-4.2 FAILED 分支（verify exit != 0）：待验证
- E-4.3 TIMEOUT 分支：待验证

## E-5 Task 状态推进

- E-5.1 SUCCEEDED：待验证
- E-5.2 PARTIAL_FAILED：待验证

## E-6 Retry 逻辑

- E-6.1 RetryInstance：待验证
- E-6.2 RetryTask（仅 FAILED/TIMEOUT）：待验证

## E-7 Delete 逻辑

- E-7.1 DeleteInstance：待验证
- E-7.2 DeleteTask（CASCADE）：待验证
- E-7.3 DeleteCase（活引用返回 409）：待验证

## E-8 Cron 巡检

- E-8.1 Sweeper：待验证
- E-8.2 Reconciler：待验证
- E-8.3 DLQ Archiver：待验证（依赖 DLQInspector 实现）

## E-9 Trace 与指标

- E-9.1 GetInstanceTrace：待验证
- E-9.2 TotalTokens 聚合：待验证
- E-9.3 AgentLatencyMs 聚合：待验证

## E-10 UploadContent 上限校验

- E-10.1 正常上传：待验证
- E-10.2 超过 256KB 返回 413：待验证
- E-10.3 非 UTF-8 返回 400：待验证

---

## 汇总

- 通过：0 / X
- 失败：0 / X
- 阻塞项：无
- 附件：`.harness/evidence/2026-07-16-eval/*.log`（待生成）
