-- ============================================================================
-- 评测域数据模型初始化脚本
-- 依据: docs/superpowers/specs/2026-07-16-agent-evaluation-design.md §2
-- 目标数据库: PostgreSQL 12+ (需要 JSONB / GIN / TIMESTAMPTZ)
-- 约定:
--   1. 所有主键使用 VARCHAR(36) 存 uuid 字符串,与既有 manus_schema 对齐,
--      应用层负责生成 uuid。
--   2. 所有时间字段使用 TIMESTAMPTZ,应用层以 UTC 写入。
--   3. 表前缀统一 eval_,共 5 张表:
--        eval_case              -- 评测用例
--        eval_agent_snapshot    -- Agent 配置快照
--        eval_task              -- 父任务
--        eval_run_instance      -- M×N 运行实例
--        eval_result            -- 实例执行结果 (1:1 to instance)
--   4. 外键级联关系 (§2.6):
--        eval_task ─ CASCADE ─▶ eval_run_instance ─ CASCADE ─▶ eval_result
--        eval_agent_snapshot ◀─ RESTRICT ─ eval_run_instance
--        eval_case 无 FK, 由 case_snapshot(jsonb) 冻结历史
--   5. 状态枚举以字符串存储,由应用层 state_machine.go 白名单守门;
--      DB 层用 CHECK 兜底避免非法写入。
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. eval_case  评测用例 (物理删除)
-- ----------------------------------------------------------------------------
CREATE TABLE eval_case
(
    id            VARCHAR(36) PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    description   TEXT,
    init_script   TEXT,
    task_prompt   TEXT         NOT NULL,
    verify_script TEXT         NOT NULL,
    tags          JSONB        NOT NULL DEFAULT '[]'::JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_eval_case_name UNIQUE (name)
);

CREATE INDEX idx_eval_case_tags_gin ON eval_case USING GIN (tags);

COMMENT ON TABLE  eval_case               IS '评测用例;物理删除,删除前置检查非终态引用';
COMMENT ON COLUMN eval_case.id            IS '主键,uuid 字符串';
COMMENT ON COLUMN eval_case.name          IS '用例名,全局唯一';
COMMENT ON COLUMN eval_case.description   IS '用例描述,可空';
COMMENT ON COLUMN eval_case.init_script   IS '工作目录初始化脚本,负责在沙盒 workspace 写待编辑源文件/fixture,可空';
COMMENT ON COLUMN eval_case.task_prompt   IS '给智能体的评测指令';
COMMENT ON COLUMN eval_case.verify_script IS '验证脚本,exit_code=0 视为通过';
COMMENT ON COLUMN eval_case.tags          IS '标签数组,便于筛选,例如 ["file-io","bash","edit"]';
COMMENT ON COLUMN eval_case.created_at    IS '创建时间 (UTC)';
COMMENT ON COLUMN eval_case.updated_at    IS '更新时间 (UTC)';

-- ----------------------------------------------------------------------------
-- 2. eval_agent_snapshot  Agent 配置快照 (冻结 appConfig 关键字段)
-- ----------------------------------------------------------------------------
CREATE TABLE eval_agent_snapshot
(
    id                    VARCHAR(36) PRIMARY KEY,
    source_app_config_id  VARCHAR(64)  NOT NULL,
    name                  VARCHAR(128) NOT NULL,
    model                 VARCHAR(64)  NOT NULL,
    system_prompt         TEXT,
    tools_config          JSONB        NOT NULL DEFAULT '{}'::JSONB,
    mcp_config            JSONB        NOT NULL DEFAULT '{}'::JSONB,
    a2a_config            JSONB        NOT NULL DEFAULT '{}'::JSONB,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_eval_agent_snapshot_src ON eval_agent_snapshot (source_app_config_id);

COMMENT ON TABLE  eval_agent_snapshot                      IS 'Agent 配置快照,保证历史评测的模型/prompt/工具集不受活对象变更影响';
COMMENT ON COLUMN eval_agent_snapshot.id                   IS '主键,uuid 字符串';
COMMENT ON COLUMN eval_agent_snapshot.source_app_config_id IS '反向溯源到活的 app_config.id';
COMMENT ON COLUMN eval_agent_snapshot.name                 IS '快照瞬间的 appConfig 名';
COMMENT ON COLUMN eval_agent_snapshot.model                IS '模型名,例如 gpt-4o';
COMMENT ON COLUMN eval_agent_snapshot.system_prompt        IS '冻结的 system prompt';
COMMENT ON COLUMN eval_agent_snapshot.tools_config         IS '冻结的 tools 定义 (JSONB)';
COMMENT ON COLUMN eval_agent_snapshot.mcp_config           IS '冻结的 MCP 定义 (JSONB)';
COMMENT ON COLUMN eval_agent_snapshot.a2a_config           IS '冻结的 A2A 定义 (JSONB)';
COMMENT ON COLUMN eval_agent_snapshot.created_at           IS '创建时间 (UTC)';

-- ----------------------------------------------------------------------------
-- 3. eval_task  父任务 (M×N)
-- ----------------------------------------------------------------------------
CREATE TABLE eval_task
(
    id                        VARCHAR(36) PRIMARY KEY,
    name                      VARCHAR(128) NOT NULL,
    status                    VARCHAR(24)  NOT NULL DEFAULT 'PENDING',
    total_count               INTEGER      NOT NULL DEFAULT 0,
    succeeded_count           INTEGER      NOT NULL DEFAULT 0,
    failed_count              INTEGER      NOT NULL DEFAULT 0,
    running_count             INTEGER      NOT NULL DEFAULT 0,
    case_ids                  JSONB        NOT NULL DEFAULT '[]'::JSONB,
    agent_config_snapshot_ids JSONB        NOT NULL DEFAULT '[]'::JSONB,
    created_by                VARCHAR(64),
    created_at                TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at                TIMESTAMPTZ,
    finished_at               TIMESTAMPTZ,
    CONSTRAINT ck_eval_task_status
        CHECK (status IN ('PENDING', 'RUNNING', 'SUCCEEDED', 'PARTIAL_FAILED'))
);

CREATE INDEX idx_eval_task_status_created ON eval_task (status, created_at DESC);

COMMENT ON TABLE  eval_task                           IS '评测父任务;每个任务生成 M×N 条运行实例';
COMMENT ON COLUMN eval_task.id                        IS '主键,uuid 字符串';
COMMENT ON COLUMN eval_task.name                      IS '任务名';
COMMENT ON COLUMN eval_task.status                    IS '状态: PENDING/RUNNING/SUCCEEDED/PARTIAL_FAILED';
COMMENT ON COLUMN eval_task.total_count               IS '实例总数 = M × N';
COMMENT ON COLUMN eval_task.succeeded_count           IS 'passed=true 实例汇总,冗余列避免全表扫';
COMMENT ON COLUMN eval_task.failed_count              IS 'FAILED/TIMEOUT/passed=false 实例汇总';
COMMENT ON COLUMN eval_task.running_count             IS '非终态实例汇总';
COMMENT ON COLUMN eval_task.case_ids                  IS '选中的 M 个 case_id 数组';
COMMENT ON COLUMN eval_task.agent_config_snapshot_ids IS '生成的 N 个 agent snapshot_id 数组';
COMMENT ON COLUMN eval_task.created_by                IS '创建者标识,预留';
COMMENT ON COLUMN eval_task.created_at                IS '创建时间 (UTC)';
COMMENT ON COLUMN eval_task.started_at                IS '首个实例开始时间 (UTC)';
COMMENT ON COLUMN eval_task.finished_at               IS '任务终态时间 (UTC)';

-- ----------------------------------------------------------------------------
-- 4. eval_run_instance  M×N 运行实例
-- ----------------------------------------------------------------------------
CREATE TABLE eval_run_instance
(
    id                       VARCHAR(36) PRIMARY KEY,
    task_id                  VARCHAR(36)  NOT NULL,
    case_id                  VARCHAR(36)  NOT NULL,
    case_snapshot            JSONB        NOT NULL,
    agent_config_snapshot_id VARCHAR(36)  NOT NULL,
    status                   VARCHAR(24)  NOT NULL DEFAULT 'PENDING',
    attempt                  INTEGER      NOT NULL DEFAULT 1,
    conversation_id          VARCHAR(64)  NOT NULL DEFAULT '',
    message_id               VARCHAR(64)  NOT NULL DEFAULT '',
    trace_id                 VARCHAR(64)  NOT NULL DEFAULT '',
    queued_at                TIMESTAMPTZ,
    started_at               TIMESTAMPTZ,
    finished_at              TIMESTAMPTZ,
    heartbeat_at             TIMESTAMPTZ,
    deadline_at              TIMESTAMPTZ,
    worker_id                VARCHAR(64)  NOT NULL DEFAULT '',
    error_message            TEXT,
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ck_eval_run_instance_status
        CHECK (status IN ('PENDING', 'QUEUED', 'INITIALIZING', 'RUNNING',
                          'VERIFYING', 'PASSED', 'FAILED', 'TIMEOUT', 'CANCELED')),
    CONSTRAINT ck_eval_run_instance_attempt CHECK (attempt >= 1),
    CONSTRAINT uk_eval_run_instance_triplet
        UNIQUE (task_id, case_id, agent_config_snapshot_id),
    CONSTRAINT fk_eval_run_instance_task
        FOREIGN KEY (task_id) REFERENCES eval_task (id) ON DELETE CASCADE,
    CONSTRAINT fk_eval_run_instance_snapshot
        FOREIGN KEY (agent_config_snapshot_id) REFERENCES eval_agent_snapshot (id) ON DELETE RESTRICT
);

CREATE INDEX idx_eval_run_instance_task_status  ON eval_run_instance (task_id, status);
CREATE INDEX idx_eval_run_instance_status_hb    ON eval_run_instance (status, heartbeat_at);
CREATE INDEX idx_eval_run_instance_status_queue ON eval_run_instance (status, queued_at);

COMMENT ON TABLE  eval_run_instance                          IS 'M×N 运行实例;每次 attempt 重置 conversation/message_id;删除任务级联清空';
COMMENT ON COLUMN eval_run_instance.id                       IS '主键,uuid 字符串';
COMMENT ON COLUMN eval_run_instance.task_id                  IS '外键 eval_task.id,ON DELETE CASCADE';
COMMENT ON COLUMN eval_run_instance.case_id                  IS '逻辑关联 eval_case.id,不设 FK (case 允许物理删)';
COMMENT ON COLUMN eval_run_instance.case_snapshot            IS '冻结用例三要素 + name + tags 的 JSON;case 删除后仍可回溯';
COMMENT ON COLUMN eval_run_instance.agent_config_snapshot_id IS '外键 eval_agent_snapshot.id,ON DELETE RESTRICT (由 task 删除时同事务清理)';
COMMENT ON COLUMN eval_run_instance.status                   IS '状态: PENDING/QUEUED/INITIALIZING/RUNNING/VERIFYING/PASSED/FAILED/TIMEOUT/CANCELED';
COMMENT ON COLUMN eval_run_instance.attempt                  IS '尝试次数,从 1 起,重试时递增';
COMMENT ON COLUMN eval_run_instance.conversation_id          IS '会话 ID,每次 attempt 重置';
COMMENT ON COLUMN eval_run_instance.message_id               IS '消息 ID,每次 attempt 重置';
COMMENT ON COLUMN eval_run_instance.trace_id                 IS '首个 AGENT_ROOT span 的 trace_id;收敛后一次性回填';
COMMENT ON COLUMN eval_run_instance.queued_at                IS '入队时间 (UTC)';
COMMENT ON COLUMN eval_run_instance.started_at               IS '本次 attempt 开始时间 (UTC)';
COMMENT ON COLUMN eval_run_instance.finished_at              IS '本次 attempt 终态时间 (UTC)';
COMMENT ON COLUMN eval_run_instance.heartbeat_at             IS '心跳更新时间,巡检器判活依据';
COMMENT ON COLUMN eval_run_instance.deadline_at              IS 'started_at + instance_total_timeout;巡检直接比较,不依赖运行期配置';
COMMENT ON COLUMN eval_run_instance.worker_id                IS '处理该实例的 worker 标识';
COMMENT ON COLUMN eval_run_instance.error_message            IS '排队/初始化阶段失败摘要,超 4KB 截断';
COMMENT ON COLUMN eval_run_instance.created_at               IS '创建时间 (UTC)';

-- ----------------------------------------------------------------------------
-- 5. eval_result  实例执行结果 (1:1 to instance)
-- ----------------------------------------------------------------------------
CREATE TABLE eval_result
(
    id                VARCHAR(36) PRIMARY KEY,
    instance_id       VARCHAR(36) NOT NULL,
    passed            BOOLEAN     NOT NULL,
    verify_exit_code  INTEGER     NOT NULL DEFAULT 0,
    verify_stdout     TEXT,
    verify_stderr     TEXT,
    error_log         TEXT,
    prompt_tokens     BIGINT      NOT NULL DEFAULT 0,
    completion_tokens BIGINT      NOT NULL DEFAULT 0,
    total_tokens      BIGINT      NOT NULL DEFAULT 0,
    agent_latency_ms  BIGINT      NOT NULL DEFAULT 0,
    finished_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_eval_result_instance UNIQUE (instance_id),
    CONSTRAINT fk_eval_result_instance
        FOREIGN KEY (instance_id) REFERENCES eval_run_instance (id) ON DELETE CASCADE
);

COMMENT ON TABLE  eval_result                   IS '实例执行结果,与 eval_run_instance 一一对应;verify_stdout/stderr/error_log 单字段上限 64KB 由应用层截断';
COMMENT ON COLUMN eval_result.id                IS '主键,uuid 字符串';
COMMENT ON COLUMN eval_result.instance_id       IS '外键 eval_run_instance.id,ON DELETE CASCADE,UNIQUE';
COMMENT ON COLUMN eval_result.passed            IS '是否通过验证';
COMMENT ON COLUMN eval_result.verify_exit_code  IS 'verify_script 退出码,0 通过';
COMMENT ON COLUMN eval_result.verify_stdout     IS 'verify_script stdout,应用层截断 64KB';
COMMENT ON COLUMN eval_result.verify_stderr     IS 'verify_script stderr,应用层截断 64KB';
COMMENT ON COLUMN eval_result.error_log         IS '智能体侧错误 + 初始化脚本错误,应用层截断 64KB';
COMMENT ON COLUMN eval_result.prompt_tokens     IS 'Σ LLM_CALL span 的 prompt tokens';
COMMENT ON COLUMN eval_result.completion_tokens IS 'Σ LLM_CALL span 的 completion tokens';
COMMENT ON COLUMN eval_result.total_tokens      IS 'Σ LLM_CALL span 的 total tokens';
COMMENT ON COLUMN eval_result.agent_latency_ms  IS 'AGENT_ROOT span 的 latency_ms';
COMMENT ON COLUMN eval_result.finished_at       IS '结果落盘时间 (UTC)';
COMMENT ON COLUMN eval_result.created_at        IS '创建时间 (UTC)';

-- ============================================================================
-- 删除路径提示 (仅注释,不写入 DB):
--   删单 case: 前置查非终态引用非空则 409,通过后直接物理删。
--   删单实例: DB 级 CASCADE 自动删 eval_result;应用层同事务重算 task 4 count + CAS 推进状态。
--   删单任务: 顺序敏感,先 DELETE eval_task 触发 CASCADE 清 instance,
--             再 DELETE eval_agent_snapshot WHERE id IN (...) 释放 RESTRICT。两步同事务。
-- ============================================================================
