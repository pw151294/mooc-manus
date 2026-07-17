CREATE TABLE app_config
(
    id                 VARCHAR(36) PRIMARY KEY,
    base_url           VARCHAR(255)  NOT NULL,
    api_key            VARCHAR(255)  NOT NULL,
    model_name         VARCHAR(100)  NOT NULL,
    temperature        DECIMAL(3, 2) NOT NULL,
    max_tokens         INTEGER       NOT NULL,
    max_iterations     INTEGER       NOT NULL,
    max_retries        INTEGER       NOT NULL,
    max_search_results INTEGER       NOT NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE app_config IS '应用配置表';
COMMENT ON COLUMN app_config.id IS '主键ID';
COMMENT ON COLUMN app_config.base_url IS '基础URL';
COMMENT ON COLUMN app_config.api_key IS 'API密钥';
COMMENT ON COLUMN app_config.model_name IS '模型名称';
COMMENT ON COLUMN app_config.temperature IS '温度参数';
COMMENT ON COLUMN app_config.max_tokens IS '最大Token数';
COMMENT ON COLUMN app_config.max_iterations IS '最大迭代次数';
COMMENT ON COLUMN app_config.max_retries IS '最大重试次数';
COMMENT ON COLUMN app_config.max_search_results IS '最大搜索结果数';
COMMENT ON COLUMN app_config.created_at IS '创建时间';
COMMENT ON COLUMN app_config.updated_at IS '更新时间';

CREATE TABLE tool_provider
(
    id                 VARCHAR(36) PRIMARY KEY,
    provider_name      VARCHAR(255) NOT NULL,
    provider_type      VARCHAR(100) NOT NULL,
    provider_desc      TEXT,
    provider_url       VARCHAR(255),
    provider_transport VARCHAR(100),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (provider_name)
);

COMMENT ON TABLE tool_provider IS '工具提供者表';
COMMENT ON COLUMN tool_provider.id IS '主键ID';
COMMENT ON COLUMN tool_provider.provider_name IS '提供者名称，唯一';
COMMENT ON COLUMN tool_provider.provider_type IS '提供者类型';
COMMENT ON COLUMN tool_provider.provider_desc IS '提供者描述';
COMMENT ON COLUMN tool_provider.provider_url IS '提供者URL';
COMMENT ON COLUMN tool_provider.provider_transport IS '提供者传输方式';
COMMENT ON COLUMN tool_provider.created_at IS '创建时间';
COMMENT ON COLUMN tool_provider.updated_at IS '更新时间';

CREATE TABLE tool_function
(
    id            VARCHAR(36) PRIMARY KEY,
    provider_id   VARCHAR(36)  NOT NULL,
    function_name VARCHAR(255) NOT NULL,
    function_desc TEXT,
    parameters    JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_tool_provider
        FOREIGN KEY (provider_id)
            REFERENCES tool_provider (id)
            ON DELETE CASCADE
);

CREATE INDEX idx_tool_function_provider_id ON tool_function (provider_id);

COMMENT ON TABLE tool_function IS '工具函数表';
COMMENT ON COLUMN tool_function.id IS '主键ID';
COMMENT ON COLUMN tool_function.provider_id IS '外键，关联 tool_provider.id';
COMMENT ON COLUMN tool_function.function_name IS '函数名称';
COMMENT ON COLUMN tool_function.function_desc IS '函数描述';
COMMENT ON COLUMN tool_function.parameters IS '函数参数，JSONB格式';
COMMENT ON COLUMN tool_function.created_at IS '创建时间';
COMMENT ON COLUMN tool_function.updated_at IS '更新时间';

CREATE TABLE a2a_server_config
(
    id            VARCHAR(36) PRIMARY KEY,
    app_config_id VARCHAR(36)  NOT NULL,
    name          VARCHAR(36)  NOT NULL,
    description   VARCHAR(36)  NOT NULL,
    base_url      VARCHAR(255) NOT NULL,
    enabled       BOOLEAN      NOT NULL DEFAULT TRUE,
    ext_info      JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_app_config
        FOREIGN KEY (app_config_id)
            REFERENCES app_config (id)
            ON DELETE CASCADE
);

CREATE INDEX idx_a2a_server_config_app_config_id ON a2a_server_config (app_config_id);

COMMENT ON TABLE a2a_server_config IS 'A2A服务配置表';
COMMENT ON COLUMN a2a_server_config.id IS '主键ID';
COMMENT ON COLUMN a2a_server_config.app_config_id IS '外键，关联 app_config.id';
COMMENT ON COLUMN a2a_server_config.name IS 'A2A服务名称';
COMMENT ON COLUMN a2a_server_config.description IS 'A2A服务描述';
COMMENT ON COLUMN a2a_server_config.base_url IS 'A2A服务的端点';
COMMENT ON COLUMN a2a_server_config.enabled IS '该服务的状态启用/禁用';
COMMENT ON COLUMN a2a_server_config.ext_info IS '扩展信息，JSON字符串';
COMMENT ON COLUMN a2a_server_config.created_at IS '创建时间';
COMMENT ON COLUMN a2a_server_config.updated_at IS '更新时间';

CREATE TABLE a2a_server_functions
(
    id                   VARCHAR(36) PRIMARY KEY,
    a2a_server_config_id VARCHAR(36) NOT NULL,
    function_id          VARCHAR(36) NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_a2a_server_config
        FOREIGN KEY (a2a_server_config_id)
            REFERENCES a2a_server_config (id)
            ON DELETE CASCADE,
    CONSTRAINT fk_tool_function
        FOREIGN KEY (function_id)
            REFERENCES tool_function (id)
            ON DELETE CASCADE
);

CREATE INDEX idx_a2a_server_functions_a2a_server_config_id ON a2a_server_functions (a2a_server_config_id);
CREATE INDEX idx_a2a_server_functions_function_id ON a2a_server_functions (function_id);

COMMENT ON TABLE a2a_server_functions IS 'A2A服务功能表，维护a2a服务与工具调用的关联关系';
COMMENT ON COLUMN a2a_server_functions.id IS '主键ID';
COMMENT ON COLUMN a2a_server_functions.a2a_server_config_id IS '外键，关联 a2a_server_config.id';
COMMENT ON COLUMN a2a_server_functions.function_id IS '外键，关联 tool_function.id';
COMMENT ON COLUMN a2a_server_functions.created_at IS '创建时间';
COMMENT ON COLUMN a2a_server_functions.updated_at IS '更新时间';


CREATE TABLE sessions
(
    id                   VARCHAR(255) NOT NULL DEFAULT (gen_random_uuid()::VARCHAR(255)),
    sandbox_id           VARCHAR(255),
    task_id              VARCHAR(255),
    title                VARCHAR(255) NOT NULL DEFAULT ''::character varying,
    unread_message_count INTEGER      NOT NULL DEFAULT 0,
    latest_message       TEXT         NOT NULL DEFAULT ''::text,
    latest_message_at    TIMESTAMP,
    events               JSONB        NOT NULL DEFAULT '[]'::jsonb,
    files                JSONB        NOT NULL DEFAULT '[]'::jsonb,
    memories             JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status               VARCHAR(255) NOT NULL DEFAULT ''::character varying,
    updated_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP(0),
    created_at           TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP(0),

    CONSTRAINT pk_sessions_id PRIMARY KEY (id)
);


CREATE TABLE files
(
    id         VARCHAR(255)                NOT NULL DEFAULT (uuid_generate_v4()::VARCHAR(255)),
    filename   VARCHAR(255)                NOT NULL DEFAULT ''::character varying,
    filepath   VARCHAR(255)                NOT NULL DEFAULT ''::character varying,
    key        VARCHAR(255)                NOT NULL DEFAULT ''::character varying,
    extension  VARCHAR(255)                NOT NULL DEFAULT ''::character varying,
    mime_type  VARCHAR(255)                NOT NULL DEFAULT ''::character varying,
    size       INTEGER                     NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP(0),
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP(0),

    CONSTRAINT pk_files_id PRIMARY KEY (id)
);

-- ============================================================
-- Skill 模块（迁移自 Beedance Skill 配置 & 版本管理）
-- 详见 docs/mooc-manus-code-standards.md §3.2 与
--      docs/mooc-manus-code-standards-supplement.md §1
-- ============================================================

CREATE TABLE skill_provider
(
    skill_provider_id VARCHAR(36) PRIMARY KEY,
    provider_name     VARCHAR(128) NOT NULL UNIQUE,
    provider_type     VARCHAR(32)  NOT NULL,
    auth_type         VARCHAR(32),
    repo_url          VARCHAR(512),
    status            VARCHAR(32)  NOT NULL DEFAULT 'ACTIVE',
    creator           VARCHAR(64),
    updator           VARCHAR(64),
    ext_info          JSONB,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_skill_provider_status ON skill_provider (status);
CREATE INDEX idx_skill_provider_created_at ON skill_provider (created_at);

COMMENT ON TABLE skill_provider IS 'Skill 提供者表';
COMMENT ON COLUMN skill_provider.skill_provider_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill_provider.provider_name IS '提供者名称（全局唯一）';
COMMENT ON COLUMN skill_provider.provider_type IS '提供者类型：GIT / ZIP / CUSTOM';
COMMENT ON COLUMN skill_provider.auth_type IS '认证类型：HTTP_TOKEN / NONE';
COMMENT ON COLUMN skill_provider.repo_url IS 'Git 仓库地址（provider_type=GIT 时必填）';
COMMENT ON COLUMN skill_provider.status IS '状态：ACTIVE / DISABLED';
COMMENT ON COLUMN skill_provider.ext_info IS '扩展信息（JSONB）';
COMMENT ON COLUMN skill_provider.created_at IS '创建时间';
COMMENT ON COLUMN skill_provider.updated_at IS '更新时间';

CREATE TABLE skill
(
    skill_id          VARCHAR(36) PRIMARY KEY,
    skill_name        VARCHAR(120) NOT NULL UNIQUE,
    skill_provider_id VARCHAR(36)  NOT NULL REFERENCES skill_provider (skill_provider_id) ON DELETE RESTRICT,
    description       VARCHAR(3000),
    latest_version_id VARCHAR(36),
    status            VARCHAR(32)  NOT NULL DEFAULT 'ACTIVE',
    creator           VARCHAR(64),
    updator           VARCHAR(64),
    ext_info          JSONB,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_skill_provider_id ON skill (skill_provider_id);
CREATE INDEX idx_skill_status ON skill (status);
CREATE INDEX idx_skill_created_at ON skill (created_at);

COMMENT ON TABLE skill IS 'Skill 配置表';
COMMENT ON COLUMN skill.skill_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill.skill_name IS 'Skill 名称（全局唯一）';
COMMENT ON COLUMN skill.skill_provider_id IS '所属 Provider（外键，删除 Provider 时若有 Skill 会拒绝）';
COMMENT ON COLUMN skill.description IS 'Skill 描述';
COMMENT ON COLUMN skill.latest_version_id IS '最新已发布版本 ID（指向 skill_version.skill_version_id）';
COMMENT ON COLUMN skill.status IS '状态：ACTIVE / DISABLED';
COMMENT ON COLUMN skill.ext_info IS '扩展信息（icon / imageUrl 的 JSON）';
COMMENT ON COLUMN skill.created_at IS '创建时间';
COMMENT ON COLUMN skill.updated_at IS '更新时间';

CREATE TABLE skill_version
(
    skill_version_id VARCHAR(36) PRIMARY KEY,
    skill_id         VARCHAR(36)  NOT NULL REFERENCES skill (skill_id) ON DELETE CASCADE,
    version          VARCHAR(32)  NOT NULL,
    description      VARCHAR(3000),
    metadata         JSONB,
    skill_files      JSONB,
    ext_info         JSONB,
    creator          VARCHAR(64),
    updator          VARCHAR(64),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (skill_id, version)
);

CREATE INDEX idx_skill_version_skill_id ON skill_version (skill_id);
CREATE INDEX idx_skill_version_created_at ON skill_version (created_at);

COMMENT ON TABLE skill_version IS 'Skill 版本表';
COMMENT ON COLUMN skill_version.skill_version_id IS '主键 ID (UUID)';
COMMENT ON COLUMN skill_version.skill_id IS '所属 Skill（外键，删除 Skill 时级联删除版本）';
COMMENT ON COLUMN skill_version.version IS '版本号（draft 或 vX.Y.Z，与 skill_id 联合唯一）';
COMMENT ON COLUMN skill_version.description IS '版本描述';
COMMENT ON COLUMN skill_version.metadata IS 'SKILL.md 解析后的 JSON';
COMMENT ON COLUMN skill_version.skill_files IS '版本文件列表（JSONB 数组：文件名/大小/校验和/OSS Key）';
COMMENT ON COLUMN skill_version.ext_info IS '扩展信息（zipFilePath / 快照字段）';
COMMENT ON COLUMN skill_version.created_at IS '创建时间';
COMMENT ON COLUMN skill_version.updated_at IS '更新时间';

CREATE TABLE task_execution
(
    task_id     VARCHAR(100) PRIMARY KEY,
    app_id      VARCHAR(64)  NOT NULL,
    app_type    VARCHAR(64)  NOT NULL,
    status      VARCHAR(32)  NOT NULL DEFAULT 'PROCESSING',
    stage       VARCHAR(32),
    progress    INTEGER      NOT NULL DEFAULT 0,
    ext_info    JSONB,
    creator     VARCHAR(64),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at TIMESTAMPTZ
);

CREATE INDEX idx_task_app_id ON task_execution (app_id);
CREATE INDEX idx_task_status ON task_execution (status);
CREATE INDEX idx_task_created_at ON task_execution (created_at);

COMMENT ON TABLE task_execution IS '异步任务执行记录表（跨模块共用）';
COMMENT ON COLUMN task_execution.task_id IS '任务 ID（业务生成，最长 100 字符）';
COMMENT ON COLUMN task_execution.app_id IS '应用 ID（Skill 模块固定 SKILL_APP）';
COMMENT ON COLUMN task_execution.app_type IS '任务类型（Skill 模块固定 SKILL_IMPORT）';
COMMENT ON COLUMN task_execution.status IS '任务状态：PROCESSING / COMPLETED / FAILED';
COMMENT ON COLUMN task_execution.stage IS '当前阶段（仅 Skill 导入任务有效：UPLOAD/EXTRACT/VALIDATE/REGISTER/COMPLETED）';
COMMENT ON COLUMN task_execution.progress IS '进度（0-100）';
COMMENT ON COLUMN task_execution.ext_info IS '扩展信息（logs / skillCount / providerId / errorMessage 的 JSON）';
COMMENT ON COLUMN task_execution.created_at IS '创建时间';
COMMENT ON COLUMN task_execution.updated_at IS '更新时间';

-- ============================================================
-- 智能体链路追踪表（Agent Tracing）
-- 关联 spec：docs/superpowers/specs/2026-07-14-agent-tracing-design.md
-- ============================================================
CREATE TABLE ai_span
(
    id              BIGSERIAL PRIMARY KEY,
    trace_id        VARCHAR(64)  NOT NULL,
    span_id         INTEGER      NOT NULL,
    parent_span_id  INTEGER      NOT NULL,
    span_type       VARCHAR(32)  NOT NULL,
    operation_name  VARCHAR(128) NOT NULL DEFAULT '',
    conversation_id VARCHAR(64)  NOT NULL DEFAULT '',
    agent_name      VARCHAR(64)  NOT NULL DEFAULT '',
    start_time      BIGINT       NOT NULL,
    end_time        BIGINT       NOT NULL DEFAULT 0,
    latency_ms      INTEGER      NOT NULL DEFAULT 0,
    is_error        BOOLEAN      NOT NULL DEFAULT FALSE,
    tags            JSONB,
    logs            JSONB,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uk_trace_span UNIQUE (trace_id, span_id)
);

CREATE INDEX idx_ai_span_trace ON ai_span (trace_id);
CREATE INDEX idx_ai_span_conv ON ai_span (conversation_id, created_at DESC);
CREATE INDEX idx_ai_span_error ON ai_span (is_error, created_at DESC);

COMMENT ON TABLE  ai_span                IS '智能体链路 span';
COMMENT ON COLUMN ai_span.trace_id       IS '链路 ID = messageId';
COMMENT ON COLUMN ai_span.span_id        IS 'trace 内自增，root=0';
COMMENT ON COLUMN ai_span.parent_span_id IS 'root=-1';
COMMENT ON COLUMN ai_span.span_type      IS 'AGENT_ROOT/AGENT_ROUND/LLM_CALL/TOOL_BATCH/TOOL_CALL/SUBAGENT_CALL';
COMMENT ON COLUMN ai_span.operation_name IS 'tool 名（其他类型为空）';
COMMENT ON COLUMN ai_span.conversation_id IS '会话 ID（冗余存独立列，便于筛选）';
COMMENT ON COLUMN ai_span.agent_name     IS 'agent 名称';
COMMENT ON COLUMN ai_span.start_time     IS '纳秒时间戳';
COMMENT ON COLUMN ai_span.end_time       IS '纳秒时间戳';
COMMENT ON COLUMN ai_span.latency_ms     IS '毫秒时延';
COMMENT ON COLUMN ai_span.is_error       IS '当前 span 是否错误（不冒泡）';
COMMENT ON COLUMN ai_span.tags           IS '扩展 kv';
COMMENT ON COLUMN ai_span.logs           IS '过程日志 [{ts, level, msg, extra}]';
COMMENT ON COLUMN task_execution.archived_at IS '归档时间（完成后 7 天可标记归档）';

-- ============================================================================
-- 评测域数据模型 (5 表)
-- 依据: docs/superpowers/specs/2026-07-16-agent-evaluation-design.md §2
-- 约定:
--   1. 主键 VARCHAR(36) 存 uuid 字符串,应用层生成。
--   2. 所有 timestamp 使用 TIMESTAMPTZ,应用层以 UTC 写入。
--   3. 表前缀 eval_,共 5 张:
--        eval_case              -- 评测用例 (物理删)
--        eval_agent_snapshot    -- Agent 配置快照
--        eval_task              -- 父任务
--        eval_run_instance      -- M×N 运行实例
--        eval_result            -- 1:1 to instance
--   4. 外键级联 (§2.6):
--        eval_task ─ CASCADE ─▶ eval_run_instance ─ CASCADE ─▶ eval_result
--        eval_agent_snapshot ◀─ RESTRICT ─ eval_run_instance
--        eval_case 无 FK, case_snapshot(jsonb) 保历史
--   5. 状态枚举字符串,应用层 state_machine.go 白名单守门;DB 用 CHECK 兜底。
--   6. 全部 CREATE 语句使用 IF NOT EXISTS,脚本可重复执行。
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 1. eval_case  评测用例 (物理删除)
-- ----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS eval_case
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

CREATE INDEX IF NOT EXISTS idx_eval_case_tags_gin ON eval_case USING GIN (tags);

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
CREATE TABLE IF NOT EXISTS eval_agent_snapshot
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

CREATE INDEX IF NOT EXISTS idx_eval_agent_snapshot_src ON eval_agent_snapshot (source_app_config_id);

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
CREATE TABLE IF NOT EXISTS eval_task
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

CREATE INDEX IF NOT EXISTS idx_eval_task_status_created ON eval_task (status, created_at DESC);

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
CREATE TABLE IF NOT EXISTS eval_run_instance
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

CREATE INDEX IF NOT EXISTS idx_eval_run_instance_task_status  ON eval_run_instance (task_id, status);
CREATE INDEX IF NOT EXISTS idx_eval_run_instance_status_hb    ON eval_run_instance (status, heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_eval_run_instance_status_queue ON eval_run_instance (status, queued_at);

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
CREATE TABLE IF NOT EXISTS eval_result
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
-- 评测域删除路径提示 (仅注释,不写入 DB):
--   删单 case: 前置查非终态引用非空则 409,通过后直接物理删。
--   删单实例: DB 级 CASCADE 自动删 eval_result;应用层同事务重算 task 4 count + CAS 推进状态。
--   删单任务: 顺序敏感,先 DELETE eval_task 触发 CASCADE 清 instance,
--             再 DELETE eval_agent_snapshot WHERE id IN (...) 释放 RESTRICT。两步同事务。
-- ============================================================================