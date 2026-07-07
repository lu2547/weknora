-- 000034_rebuild_tables_from_newsql.up.sql
-- Rebuild all knowledge-related tables from scratch per WeKnora/docs/newsql DDL definitions.
-- WARNING: This migration drops and recreates tables. All existing data in these tables will be lost.
-- NOTE: OWNER/GRANT statements (production-only roles aidata/r_aidata_*/aiopr) are wrapped
--       in conditional blocks at the end so they are skipped on local/dev environments.

-- ============================================================
-- 1. knowledge_tag_share
-- ============================================================
DROP TABLE IF EXISTS public.knowledge_tag_share;
CREATE TABLE public.knowledge_tag_share (
    id_knowledge_tag_share varchar(36) DEFAULT uuid_generate_v4() NOT NULL,
    id_knowledge_tag varchar(36) NOT NULL,
    group_key varchar(36) NOT NULL,
    shared_by_user_id varchar(36) NOT NULL,
    "permission" varchar(32) DEFAULT 'viewer'::character varying NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    deleted_at timestamptz NULL,
    CONSTRAINT knowledge_tag_share_pkey PRIMARY KEY (id_knowledge_tag_share)
);

COMMENT ON TABLE public.knowledge_tag_share IS '知识库标签授权记录表';
COMMENT ON COLUMN public.knowledge_tag_share.id_knowledge_tag_share IS '授权唯一标识';
COMMENT ON COLUMN public.knowledge_tag_share.id_knowledge_tag IS '知识库标签ID';
COMMENT ON COLUMN public.knowledge_tag_share.group_key IS 'PMS权限组';
COMMENT ON COLUMN public.knowledge_tag_share.shared_by_user_id IS '授权操作用户ID';
COMMENT ON COLUMN public.knowledge_tag_share."permission" IS '权限: viewer、editor等';
COMMENT ON COLUMN public.knowledge_tag_share.created_at IS '创建时间';
COMMENT ON COLUMN public.knowledge_tag_share.updated_at IS '更新时间';
COMMENT ON COLUMN public.knowledge_tag_share.deleted_at IS '软删除时间';

CREATE INDEX IF NOT EXISTS idx_knowledge_tag_share_id_knowledge_tag ON public.knowledge_tag_share USING btree (id_knowledge_tag);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag_share_group_key ON public.knowledge_tag_share USING btree (group_key);

-- ============================================================
-- 2. knowledge_tag
-- ============================================================
DROP TABLE IF EXISTS public.knowledge_tag;
CREATE TABLE public.knowledge_tag (
    id_knowledge_tag varchar(36) NOT NULL,
    "name" varchar(255) NOT NULL,
    id_knowledge_base varchar(32) NOT NULL,
    owner varchar(32) NOT NULL,
    parent_id_knowledge_tag varchar(36) NULL,
    sort int4 NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    deleted_at timestamptz NULL,
    CONSTRAINT knowledge_tag_pkey PRIMARY KEY (id_knowledge_tag)
);

COMMENT ON TABLE public.knowledge_tag IS '知识库标签';
COMMENT ON COLUMN public.knowledge_tag.id_knowledge_tag IS '标签ID';
COMMENT ON COLUMN public.knowledge_tag."name" IS '标签名称';
COMMENT ON COLUMN public.knowledge_tag.id_knowledge_base IS '知识库ID';
COMMENT ON COLUMN public.knowledge_tag.owner IS '知识库所有者';
COMMENT ON COLUMN public.knowledge_tag.parent_id_knowledge_tag IS '父标签';
COMMENT ON COLUMN public.knowledge_tag.sort IS '标签排序';
COMMENT ON COLUMN public.knowledge_tag.created_at IS '创建时间';
COMMENT ON COLUMN public.knowledge_tag.updated_at IS '更新时间';
COMMENT ON COLUMN public.knowledge_tag.deleted_at IS '删除时间';

CREATE INDEX IF NOT EXISTS idx_knowledge_tag_parent_id_knowledge_tag ON public.knowledge_tag USING btree (parent_id_knowledge_tag);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag_id_knowledge_base ON public.knowledge_tag USING btree (id_knowledge_base);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag_owner ON public.knowledge_tag USING btree (owner);

-- ============================================================
-- 3. knowledge_base
-- ============================================================
DROP TABLE IF EXISTS public.knowledge_base;
CREATE TABLE public.knowledge_base (
    id_knowledge_base varchar(36) NOT NULL,
    "name" varchar(255) NOT NULL,
    category varchar(32) DEFAULT 'personal'::character varying NOT NULL,
    "type" varchar(32) DEFAULT 'document'::character varying NOT NULL,
    owner varchar(32) NOT NULL,
    description text NULL,
    chunking_config jsonb DEFAULT '{"chunk_size": 512, "chunk_overlap": 50, "split_markers": ["\n\n", "\n", "。"], "keep_separator": true}'::jsonb,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    deleted_at timestamptz NULL,
    faq_config jsonb NULL,
    question_generation_config jsonb NULL,
    is_pinned bool DEFAULT false NOT NULL,
    pinned_at timestamptz NULL,
    CONSTRAINT knowledge_base_pkey PRIMARY KEY (id_knowledge_base)
);

COMMENT ON TABLE public.knowledge_base IS '知识库（Knowledge Base）表，管理用户上传的知识集合';
COMMENT ON COLUMN public.knowledge_base.id_knowledge_base IS '知识库ID';
COMMENT ON COLUMN public.knowledge_base."name" IS '知识库名称';
COMMENT ON COLUMN public.knowledge_base.category IS '知识库类型: personal 个人; enterprise 企业';
COMMENT ON COLUMN public.knowledge_base."type" IS '类型: document（文档）、faq（问答）';
COMMENT ON COLUMN public.knowledge_base.owner IS '知识库属主';
COMMENT ON COLUMN public.knowledge_base.description IS '描述';
COMMENT ON COLUMN public.knowledge_base.chunking_config IS '分块配置: chunk_size（大小）、overlap（重叠）、split_markers（分隔）';
COMMENT ON COLUMN public.knowledge_base.created_at IS '创建时间';
COMMENT ON COLUMN public.knowledge_base.updated_at IS '更新时间';
COMMENT ON COLUMN public.knowledge_base.deleted_at IS '删除时间';
COMMENT ON COLUMN public.knowledge_base.faq_config IS 'FAQ专属配置';
COMMENT ON COLUMN public.knowledge_base.question_generation_config IS '自动问答生成配置';
COMMENT ON COLUMN public.knowledge_base.is_pinned IS '是否置顶，true=置顶';
COMMENT ON COLUMN public.knowledge_base.pinned_at IS '置顶时间';

-- ============================================================
-- 4. knowledge
-- ============================================================
DROP TABLE IF EXISTS public.knowledge;
CREATE TABLE public.knowledge (
    id_knowledge varchar(36) NOT NULL,
    id_knowledge_base varchar(36) NOT NULL,
    "type" varchar(50) NOT NULL,
    title varchar(255) NOT NULL,
    description text NULL,
    parse_status varchar(50) DEFAULT 'pending'::character varying NOT NULL,
    enable_status char(1) DEFAULT '0' NOT NULL,
    embedding_model_id varchar(64) NULL,
    file_name varchar(255) NULL,
    file_type varchar(50) NULL,
    file_size int8 NULL,
    file_path text NULL,
    file_hash varchar(64) NULL,
    storage_size int8 DEFAULT 0 NOT NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    processed_at timestamptz NULL,
    error_message text NULL,
    deleted_at timestamptz NULL,
    tag_id varchar(36) NULL,
    summary_status varchar(32) DEFAULT 'none'::character varying NULL,
    collection_name varchar(128) NULL,
    metadata jsonb NULL,
    CONSTRAINT knowledge_pkey PRIMARY KEY (id_knowledge)
);

COMMENT ON TABLE public.knowledge IS '知识（文档）表，表示一个完整上传的文档或知识源';
COMMENT ON COLUMN public.knowledge.id_knowledge IS '知识ID';
COMMENT ON COLUMN public.knowledge.id_knowledge_base IS '所属知识库ID';
COMMENT ON COLUMN public.knowledge."type" IS '类型: file（文件）、url（网页）、api（API）等';
COMMENT ON COLUMN public.knowledge.title IS '标题';
COMMENT ON COLUMN public.knowledge.description IS '描述';
COMMENT ON COLUMN public.knowledge.parse_status IS '解析状态: pending, processing, completed, failed, deleting';
COMMENT ON COLUMN public.knowledge.enable_status IS '启用状态: 0（启用）、1（禁用）';
COMMENT ON COLUMN public.knowledge.embedding_model_id IS '嵌入模型ID（可覆盖知识库配置）';
COMMENT ON COLUMN public.knowledge.file_name IS '文件名';
COMMENT ON COLUMN public.knowledge.file_type IS '文件类型（如pdf、docx）';
COMMENT ON COLUMN public.knowledge.file_size IS '文件大小（字节）';
COMMENT ON COLUMN public.knowledge.file_path IS '文件路径（iobs文件名）';
COMMENT ON COLUMN public.knowledge.file_hash IS '文件哈希值';
COMMENT ON COLUMN public.knowledge.storage_size IS '向量数据所占存储大小（字节）';
COMMENT ON COLUMN public.knowledge.created_at IS '创建时间';
COMMENT ON COLUMN public.knowledge.updated_at IS '更新时间';
COMMENT ON COLUMN public.knowledge.processed_at IS '处理完成时间';
COMMENT ON COLUMN public.knowledge.error_message IS '处理失败时的错误信息';
COMMENT ON COLUMN public.knowledge.deleted_at IS '软删除时间';
COMMENT ON COLUMN public.knowledge.tag_id IS '关联标签ID';
COMMENT ON COLUMN public.knowledge.summary_status IS '摘要状态: none（无）、generating（生成中）、completed（完成）';
COMMENT ON COLUMN public.knowledge.collection_name IS 'Milvus collection名称，避免运行时拼接';
COMMENT ON COLUMN public.knowledge.metadata IS '知识级元数据（manual内容/FAQ导入结果等），JSONB格式';

CREATE INDEX IF NOT EXISTS idx_knowledge_base ON public.knowledge USING btree (id_knowledge_base);
CREATE INDEX IF NOT EXISTS idx_knowledge_enable_status ON public.knowledge USING btree (enable_status);
CREATE INDEX IF NOT EXISTS idx_knowledge_parse_status ON public.knowledge USING btree (parse_status);
CREATE INDEX IF NOT EXISTS idx_knowledge_summary_status ON public.knowledge USING btree (summary_status);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag ON public.knowledge USING btree (tag_id);

-- ============================================================
-- 5. chunk (with sequence)
-- ============================================================
DROP TABLE IF EXISTS public.chunk;
DROP SEQUENCE IF EXISTS public.chunk_seq_id_seq;

CREATE SEQUENCE public.chunk_seq_id_seq
    INCREMENT BY 1
    MINVALUE 1
    MAXVALUE 9223372036854775807
    START 100000000
    CACHE 1
    NO CYCLE;

CREATE TABLE public.chunk (
    id_chunk varchar(36) DEFAULT uuid_generate_v4() NOT NULL,
    id_knowledge_base varchar(36) NOT NULL,
    id_knowledge varchar(36) NOT NULL,
    "content" text NOT NULL,
    chunk_index int4 NOT NULL,
    is_enabled bool DEFAULT true NOT NULL,
    start_at int4 NOT NULL,
    end_at int4 NOT NULL,
    pre_chunk_id varchar(36) NULL,
    next_chunk_id varchar(36) NULL,
    chunk_type varchar(20) DEFAULT 'text'::character varying NOT NULL,
    parent_chunk_id varchar(36) NULL,
    image_info text NULL,
    created_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    updated_at timestamptz DEFAULT CURRENT_TIMESTAMP NULL,
    deleted_at timestamptz NULL,
    metadata jsonb NULL,
    tag_id varchar(64) NULL,
    status int4 DEFAULT 0 NOT NULL,
    content_hash varchar(64) NULL,
    flags int4 DEFAULT 1 NOT NULL,
    seq_id int8 NOT NULL DEFAULT nextval('chunk_seq_id_seq'::regclass),
    relation_chunks jsonb NULL,
    indirect_relation_chunks jsonb NULL,
    CONSTRAINT chunk_pkey PRIMARY KEY (id_chunk)
);

COMMENT ON TABLE public.chunk IS '知识块（Chunk）表，存储文档切分后的语义单元，用于检索与生成';
COMMENT ON COLUMN public.chunk.id_chunk IS '唯一标识，UUID';
COMMENT ON COLUMN public.chunk.id_knowledge_base IS '所属知识库ID';
COMMENT ON COLUMN public.chunk.id_knowledge IS '所属知识ID（文档ID）';
COMMENT ON COLUMN public.chunk."content" IS '文本内容，实际语义块';
COMMENT ON COLUMN public.chunk.chunk_index IS '在文档中的顺序索引（从0开始）';
COMMENT ON COLUMN public.chunk.is_enabled IS '是否启用，true=启用，false=禁用';
COMMENT ON COLUMN public.chunk.start_at IS '在原文中起始字符位置（偏移量）';
COMMENT ON COLUMN public.chunk.end_at IS '在原文中结束字符位置（偏移量）';
COMMENT ON COLUMN public.chunk.pre_chunk_id IS '前一个块ID，用于链式结构（如长文档）';
COMMENT ON COLUMN public.chunk.next_chunk_id IS '后一个块ID，用于链式结构';
COMMENT ON COLUMN public.chunk.chunk_type IS '块类型: text（文本）、image（图片）、table（表格）等';
COMMENT ON COLUMN public.chunk.parent_chunk_id IS '父级块ID，用于父子结构（如标题-段落）';
COMMENT ON COLUMN public.chunk.image_info IS '图片相关信息（如OCR结果、元数据），JSON格式';
COMMENT ON COLUMN public.chunk.created_at IS '创建时间';
COMMENT ON COLUMN public.chunk.updated_at IS '最后更新时间';
COMMENT ON COLUMN public.chunk.deleted_at IS '软删除时间，非空表示已删除';
COMMENT ON COLUMN public.chunk.metadata IS '附加元数据（如来源URL、作者、时间戳等），JSON格式';
COMMENT ON COLUMN public.chunk.tag_id IS '标签ID，用于分类';
COMMENT ON COLUMN public.chunk.status IS '状态码: 0=正常，其他预留';
COMMENT ON COLUMN public.chunk.content_hash IS '内容哈希值，用于去重和校验';
COMMENT ON COLUMN public.chunk.flags IS '位标志位，用于控制行为（如是否可编辑、是否摘要）';
COMMENT ON COLUMN public.chunk.seq_id IS '全局顺序ID，用于保证唯一性和排序';
COMMENT ON COLUMN public.chunk.relation_chunks IS '直接关联Chunk ID列表（图片/关系类Chunk使用），JSON格式';
COMMENT ON COLUMN public.chunk.indirect_relation_chunks IS '间接关联Chunk ID列表，JSON格式';

CREATE INDEX IF NOT EXISTS idx_chunk_chunk_type ON public.chunk USING btree (chunk_type);
CREATE INDEX IF NOT EXISTS idx_chunk_content_hash ON public.chunk USING btree (content_hash);
CREATE INDEX IF NOT EXISTS idx_chunk_knowledge_enabled ON public.chunk USING btree (id_knowledge, is_enabled, deleted_at);
CREATE INDEX IF NOT EXISTS idx_chunk_parent_id ON public.chunk USING btree (parent_chunk_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_chunk_seq_id ON public.chunk USING btree (seq_id);
CREATE INDEX IF NOT EXISTS idx_chunk_tag ON public.chunk USING btree (tag_id);

-- ============================================================
-- Production-only OWNER/GRANT statements (skipped if roles don't exist on this env)
-- ============================================================
DO $$
DECLARE
    tbl text;
    tables text[] := ARRAY['knowledge_tag_share','knowledge_tag','knowledge_base','knowledge','chunk'];
BEGIN
    -- OWNER
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aidata') THEN
        FOREACH tbl IN ARRAY tables LOOP
            EXECUTE format('ALTER TABLE public.%I OWNER TO aidata', tbl);
        END LOOP;
    END IF;
    -- GRANTs
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'r_aidata_qry') THEN
        FOREACH tbl IN ARRAY tables LOOP
            EXECUTE format('GRANT SELECT ON TABLE public.%I TO r_aidata_qry', tbl);
        END LOOP;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'r_aidata_dml') THEN
        FOREACH tbl IN ARRAY tables LOOP
            EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.%I TO r_aidata_dml', tbl);
        END LOOP;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'r_aidata_dev_qry') THEN
        FOREACH tbl IN ARRAY tables LOOP
            EXECUTE format('GRANT SELECT ON TABLE public.%I TO r_aidata_dev_qry', tbl);
        END LOOP;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'aiopr') THEN
        EXECUTE 'GRANT SELECT ON TABLE public.knowledge_base TO aiopr';
        EXECUTE 'GRANT SELECT ON TABLE public.knowledge TO aiopr';
        EXECUTE 'GRANT SELECT ON TABLE public.chunk TO aiopr';
    END IF;
END $$;
