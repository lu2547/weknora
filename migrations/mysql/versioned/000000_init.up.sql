-- WeKnora OceanBase/MySQL Complete Schema
-- Combined from all PG versioned migrations (000000 ~ 000020)

-- ============================================================
-- Table: tenants
-- ============================================================
CREATE TABLE IF NOT EXISTS tenants (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    api_key VARCHAR(256) NOT NULL,
    retriever_engines JSON NOT NULL DEFAULT ('[]'),
    status VARCHAR(50) DEFAULT 'active',
    business VARCHAR(255) NOT NULL DEFAULT '',
    storage_quota BIGINT NOT NULL DEFAULT 10737418240,
    storage_used BIGINT NOT NULL DEFAULT 0,
    agent_config JSON DEFAULT NULL,
    context_config JSON DEFAULT NULL,
    conversation_config JSON DEFAULT NULL,
    web_search_config JSON DEFAULT NULL,
    parser_engine_config JSON DEFAULT NULL,
    storage_engine_config JSON DEFAULT NULL,
    chat_history_config JSON DEFAULT NULL,
    retrieval_config JSON DEFAULT NULL,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 AUTO_INCREMENT=10000;

CREATE INDEX idx_tenants_api_key ON tenants(api_key);
CREATE INDEX idx_tenants_status ON tenants(status);

-- ============================================================
-- Table: users
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    username VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    avatar VARCHAR(512) DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    can_access_all_tenants BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL,
    UNIQUE KEY users_username_key (username),
    UNIQUE KEY users_email_key (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_users_tenant_id ON users(tenant_id);

-- ============================================================
-- Table: auth_tokens
-- ============================================================
CREATE TABLE IF NOT EXISTS auth_tokens (
    id VARCHAR(36) PRIMARY KEY,
    user_id VARCHAR(36) NOT NULL,
    token TEXT NOT NULL,
    token_type VARCHAR(50) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_auth_tokens_user_id ON auth_tokens(user_id);

-- ============================================================
-- Table: models
-- ============================================================
CREATE TABLE IF NOT EXISTS models (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    source VARCHAR(50) NOT NULL,
    description TEXT,
    parameters JSON NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_models_tenant_source_type ON models(tenant_id, source, type);
CREATE INDEX idx_models_type ON models(type);
CREATE INDEX idx_models_source ON models(source);

-- ============================================================
-- Table: knowledge_bases
-- ============================================================
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    tenant_id BIGINT NOT NULL,
    type VARCHAR(32) NOT NULL DEFAULT 'document',
    is_temporary BOOLEAN NOT NULL DEFAULT FALSE,
    chunking_config JSON NOT NULL DEFAULT ('{"chunk_size":512,"chunk_overlap":50,"split_markers":["\n\n","\n","。"],"keep_separator":true}'),
    image_processing_config JSON NOT NULL DEFAULT ('{"enable_multimodal":false,"model_id":""}'),
    embedding_model_id VARCHAR(64) NOT NULL DEFAULT '',
    summary_model_id VARCHAR(64) NOT NULL DEFAULT '',
    cos_config JSON NOT NULL DEFAULT ('{}'),
    vlm_config JSON NOT NULL DEFAULT ('{}'),
    extract_config JSON NULL DEFAULT NULL,
    faq_config JSON DEFAULT NULL,
    question_generation_config JSON NULL DEFAULT NULL,
    storage_provider_config JSON DEFAULT NULL,
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    pinned_at DATETIME(6) NULL DEFAULT NULL,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_knowledge_bases_tenant_id ON knowledge_bases(tenant_id);
CREATE INDEX idx_knowledge_bases_tenant_name ON knowledge_bases(tenant_id, name);

-- ============================================================
-- Table: knowledges
-- ============================================================
CREATE TABLE IF NOT EXISTS knowledges (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    source VARCHAR(128) NOT NULL,
    parse_status VARCHAR(50) NOT NULL DEFAULT 'unprocessed',
    enable_status VARCHAR(50) NOT NULL DEFAULT 'enabled',
    embedding_model_id VARCHAR(64),
    file_name VARCHAR(255),
    file_type VARCHAR(50),
    file_size BIGINT,
    file_path TEXT,
    file_hash VARCHAR(64),
    storage_size BIGINT NOT NULL DEFAULT 0,
    metadata JSON,
    tag_id VARCHAR(36),
    summary_status VARCHAR(32) DEFAULT 'none',
    last_faq_import_result JSON DEFAULT NULL,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL,
    processed_at DATETIME(6),
    error_message TEXT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_knowledges_tenant_id ON knowledges(tenant_id);
CREATE INDEX idx_knowledges_base_id ON knowledges(knowledge_base_id);
CREATE INDEX idx_knowledges_parse_status ON knowledges(parse_status);
CREATE INDEX idx_knowledges_enable_status ON knowledges(enable_status);

-- ============================================================
-- Table: sessions
-- ============================================================
CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    title VARCHAR(255),
    description TEXT,
    knowledge_base_id VARCHAR(36),
    max_rounds INT NOT NULL DEFAULT 5,
    enable_rewrite BOOLEAN NOT NULL DEFAULT TRUE,
    fallback_strategy VARCHAR(255) NOT NULL DEFAULT 'fixed',
    fallback_response TEXT NOT NULL DEFAULT '很抱歉，我暂时无法回答这个问题。',
    keyword_threshold FLOAT NOT NULL DEFAULT 0.5,
    vector_threshold FLOAT NOT NULL DEFAULT 0.5,
    rerank_model_id VARCHAR(64),
    embedding_top_k INT NOT NULL DEFAULT 10,
    rerank_top_k INT NOT NULL DEFAULT 10,
    rerank_threshold FLOAT NOT NULL DEFAULT 0.65,
    summary_model_id VARCHAR(64),
    summary_parameters JSON NOT NULL DEFAULT ('{}'),
    agent_config JSON DEFAULT NULL,
    context_config JSON DEFAULT NULL,
    agent_id VARCHAR(36),
    is_fallback BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_sessions_tenant_id ON sessions(tenant_id);

-- ============================================================
-- Table: messages
-- ============================================================
CREATE TABLE IF NOT EXISTS messages (
    id VARCHAR(36) PRIMARY KEY,
    request_id VARCHAR(36) NOT NULL,
    session_id VARCHAR(36) NOT NULL,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    knowledge_references JSON NOT NULL DEFAULT ('[]'),
    agent_steps JSON DEFAULT NULL,
    mentioned_items JSON DEFAULT NULL,
    is_completed BOOLEAN NOT NULL DEFAULT FALSE,
    is_fallback BOOLEAN DEFAULT FALSE,
    agent_duration_ms BIGINT DEFAULT 0,
    knowledge_id VARCHAR(36),
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_messages_session_id ON messages(session_id);
CREATE INDEX idx_messages_session_role ON messages(session_id, role);
CREATE INDEX idx_messages_knowledge_id ON messages(knowledge_id);

-- ============================================================
-- Table: chunks
-- ============================================================
CREATE TABLE IF NOT EXISTS chunks (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    content TEXT NOT NULL,
    chunk_index INT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    start_at INT NOT NULL,
    end_at INT NOT NULL,
    pre_chunk_id VARCHAR(36),
    next_chunk_id VARCHAR(36),
    chunk_type VARCHAR(20) NOT NULL DEFAULT 'text',
    parent_chunk_id VARCHAR(36),
    image_info TEXT,
    relation_chunks JSON,
    indirect_relation_chunks JSON,
    metadata JSON,
    tag_id VARCHAR(36),
    status INT NOT NULL DEFAULT 0,
    content_hash VARCHAR(64),
    flags INT NOT NULL DEFAULT 1,
    seq_id BIGINT AUTO_INCREMENT UNIQUE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 AUTO_INCREMENT=100000000;

CREATE INDEX idx_chunks_tenant_knowledge ON chunks(tenant_id, knowledge_id);
CREATE INDEX idx_chunks_parent_id ON chunks(parent_chunk_id);
CREATE INDEX idx_chunks_chunk_type ON chunks(chunk_type);
CREATE INDEX idx_chunks_tenant_kg ON chunks(tenant_id, knowledge_id);

-- ============================================================
-- Table: knowledge_tags
-- ============================================================
CREATE TABLE IF NOT EXISTS knowledge_tags (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    name VARCHAR(255) NOT NULL,
    seq_id BIGINT AUTO_INCREMENT UNIQUE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 AUTO_INCREMENT=10000000;

CREATE INDEX idx_knowledge_tags_kb_id ON knowledge_tags(knowledge_base_id);
CREATE INDEX idx_knowledge_tags_tenant_id ON knowledge_tags(tenant_id);

-- ============================================================
-- Table: mcp_services
-- ============================================================
CREATE TABLE IF NOT EXISTS mcp_services (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    transport_type VARCHAR(50) NOT NULL,
    url VARCHAR(512),
    headers JSON,
    auth_config JSON,
    advanced_config JSON,
    stdio_config JSON,
    env_vars JSON,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_mcp_services_tenant_id ON mcp_services(tenant_id);
CREATE INDEX idx_mcp_services_enabled ON mcp_services(enabled);
CREATE INDEX idx_mcp_services_is_builtin ON mcp_services(is_builtin);

-- ============================================================
-- Table: custom_agents
-- ============================================================
CREATE TABLE IF NOT EXISTS custom_agents (
    id VARCHAR(36) NOT NULL,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config JSON NOT NULL DEFAULT ('{}'),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL,
    PRIMARY KEY (id, tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_custom_agents_tenant_id ON custom_agents(tenant_id);

-- ============================================================
-- Table: organizations
-- ============================================================
CREATE TABLE IF NOT EXISTS organizations (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id VARCHAR(36) NOT NULL,
    owner_tenant_id BIGINT NOT NULL DEFAULT 0,
    visibility VARCHAR(32) NOT NULL DEFAULT 'private',
    join_policy VARCHAR(32) NOT NULL DEFAULT 'approval',
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_organizations_owner_id ON organizations(owner_id);

-- ============================================================
-- Table: organization_members
-- ============================================================
CREATE TABLE IF NOT EXISTS organization_members (
    id VARCHAR(36) PRIMARY KEY,
    organization_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    role VARCHAR(50) NOT NULL DEFAULT 'viewer',
    invited_by VARCHAR(36),
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL,
    UNIQUE KEY idx_org_members_org_user (organization_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_org_members_org_id ON organization_members(organization_id);
CREATE INDEX idx_org_members_user_id ON organization_members(user_id);

-- ============================================================
-- Table: kb_shares
-- ============================================================
CREATE TABLE IF NOT EXISTS kb_shares (
    id VARCHAR(36) PRIMARY KEY,
    knowledge_base_id VARCHAR(36) NOT NULL,
    organization_id VARCHAR(36) NOT NULL,
    shared_by_user_id VARCHAR(36) NOT NULL,
    source_tenant_id BIGINT NOT NULL,
    permission VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_kb_shares_kb_id ON kb_shares(knowledge_base_id);
CREATE INDEX idx_kb_shares_org_id ON kb_shares(organization_id);
CREATE INDEX idx_kb_shares_source_tenant ON kb_shares(source_tenant_id);

-- ============================================================
-- Table: organization_join_requests
-- ============================================================
CREATE TABLE IF NOT EXISTS organization_join_requests (
    id VARCHAR(36) PRIMARY KEY,
    organization_id VARCHAR(36) NOT NULL,
    user_id VARCHAR(36) NOT NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_role VARCHAR(32) NOT NULL DEFAULT 'viewer',
    request_type VARCHAR(32) NOT NULL DEFAULT 'join',
    prev_role VARCHAR(32),
    message TEXT,
    reviewed_by VARCHAR(36),
    reviewed_at DATETIME(6),
    review_message TEXT,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_org_join_requests_org_id ON organization_join_requests(organization_id);
CREATE INDEX idx_org_join_requests_user_id ON organization_join_requests(user_id);
CREATE INDEX idx_org_join_requests_status ON organization_join_requests(status);

-- ============================================================
-- Table: agent_shares
-- ============================================================
CREATE TABLE IF NOT EXISTS agent_shares (
    id VARCHAR(36) PRIMARY KEY,
    agent_id VARCHAR(36) NOT NULL,
    organization_id VARCHAR(36) NOT NULL,
    shared_by_user_id VARCHAR(36) NOT NULL,
    source_tenant_id BIGINT NOT NULL,
    permission VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    deleted_at DATETIME(6) NULL DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_agent_shares_agent_id ON agent_shares(agent_id);
CREATE INDEX idx_agent_shares_org_id ON agent_shares(organization_id);
CREATE INDEX idx_agent_shares_source_tenant ON agent_shares(source_tenant_id);

-- ============================================================
-- Table: tenant_disabled_shared_agents
-- ============================================================
CREATE TABLE IF NOT EXISTS tenant_disabled_shared_agents (
    tenant_id BIGINT NOT NULL,
    agent_id VARCHAR(36) NOT NULL,
    source_tenant_id BIGINT NOT NULL,
    created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (tenant_id, agent_id, source_tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_tenant_disabled_shared_agents_tenant_id ON tenant_disabled_shared_agents(tenant_id);
