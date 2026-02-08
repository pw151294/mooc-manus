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