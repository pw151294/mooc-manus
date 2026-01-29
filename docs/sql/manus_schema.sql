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

CREATE TABLE tool_provider (
                               id VARCHAR(36) PRIMARY KEY,
                               provider_name VARCHAR(255) NOT NULL,
                               provider_type VARCHAR(100) NOT NULL,
                               provider_desc TEXT,
                               provider_url VARCHAR(255),
                               provider_transport VARCHAR(100),
                               created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               UNIQUE(provider_name)
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

CREATE TABLE tool_function (
                               id VARCHAR(36) PRIMARY KEY,
                               provider_id VARCHAR(36) NOT NULL,
                               function_name VARCHAR(255) NOT NULL,
                               function_desc TEXT,
                               parameters JSONB,
                               created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                               CONSTRAINT fk_tool_provider
                                   FOREIGN KEY(provider_id)
                                       REFERENCES tool_provider(id)
                                       ON DELETE CASCADE
);

CREATE INDEX idx_tool_function_provider_id ON tool_function(provider_id);

COMMENT ON TABLE tool_function IS '工具函数表';
COMMENT ON COLUMN tool_function.id IS '主键ID';
COMMENT ON COLUMN tool_function.provider_id IS '外键，关联 tool_provider.id';
COMMENT ON COLUMN tool_function.function_name IS '函数名称';
COMMENT ON COLUMN tool_function.function_desc IS '函数描述';
COMMENT ON COLUMN tool_function.parameters IS '函数参数，JSONB格式';
COMMENT ON COLUMN tool_function.created_at IS '创建时间';
COMMENT ON COLUMN tool_function.updated_at IS '更新时间';

