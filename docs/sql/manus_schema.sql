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