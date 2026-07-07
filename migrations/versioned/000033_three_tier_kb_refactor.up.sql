-- 000033_three_tier_kb_refactor.up.sql
-- Three-tier Knowledge Base refactoring: table renames, column renames, new fields, new tables.
-- Aligns with DDL from WeKnora/docs/newsql/*.

-- ============================================================
-- 1. Rename tables to singular form
-- ============================================================
ALTER TABLE IF EXISTS knowledge_bases RENAME TO knowledge_base;
ALTER TABLE IF EXISTS knowledges RENAME TO knowledge;
ALTER TABLE IF EXISTS chunks RENAME TO chunk;
ALTER TABLE IF EXISTS knowledge_tags RENAME TO knowledge_tag;

-- ============================================================
-- 2. knowledge_base: rename PK + add new columns
-- ============================================================
ALTER TABLE knowledge_base RENAME COLUMN id TO id_knowledge_base;

-- Add category (personal/public/enterprise) and owner columns
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS category VARCHAR(32) NOT NULL DEFAULT 'personal';
ALTER TABLE knowledge_base ADD COLUMN IF NOT EXISTS owner VARCHAR(64);

COMMENT ON COLUMN knowledge_base.category IS 'personal 个人; public 公共; enterprise 企业';
COMMENT ON COLUMN knowledge_base.owner IS '知识库所有者标识';

-- ============================================================
-- 3. knowledge: rename PK + FK columns, adjust enable_status type
-- ============================================================
ALTER TABLE knowledge RENAME COLUMN id TO id_knowledge;
ALTER TABLE knowledge RENAME COLUMN knowledge_base_id TO id_knowledge_base;

-- Ensure enable_status has correct type and default
-- 旧值 'enabled'/'disabled' 字符串映射为新语义：'0' 启用、'1' 禁用
ALTER TABLE knowledge
    ALTER COLUMN enable_status DROP DEFAULT;
ALTER TABLE knowledge
    ALTER COLUMN enable_status TYPE CHAR(1) USING (
        CASE
            WHEN enable_status IN ('enabled', '0', 'true', 't') THEN '0'
            WHEN enable_status IN ('disabled', '1', 'false', 'f') THEN '1'
            ELSE '0'
        END
    );
ALTER TABLE knowledge ALTER COLUMN enable_status SET DEFAULT '0';

COMMENT ON COLUMN knowledge.enable_status IS '0 启用; 1 禁用';

-- ============================================================
-- 4. chunk: rename PK + FK columns, widen tag_id
-- ============================================================
ALTER TABLE chunk RENAME COLUMN id TO id_chunk;
ALTER TABLE chunk RENAME COLUMN knowledge_id TO id_knowledge;
ALTER TABLE chunk RENAME COLUMN knowledge_base_id TO id_knowledge_base;

-- Widen tag_id from varchar(36) to varchar(64) to match new DDL
ALTER TABLE chunk ALTER COLUMN tag_id TYPE VARCHAR(64);

-- Create global sequence for seq_id if not exists (START 100000000 leaves room for legacy IDs)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_sequences WHERE schemaname = 'public' AND sequencename = 'chunk_seq_id_seq') THEN
        CREATE SEQUENCE chunk_seq_id_seq START WITH 100000000;
    END IF;
END $$;

-- Add composite index for knowledge+enabled queries (if not exists)
CREATE INDEX IF NOT EXISTS idx_chunk_knowledge_enabled ON chunk (id_knowledge, is_enabled, deleted_at);

-- ============================================================
-- 5. knowledge_tag: rename PK + FK + parent column, add owner
-- ============================================================
ALTER TABLE knowledge_tag RENAME COLUMN id TO id_knowledge_tag;
ALTER TABLE knowledge_tag RENAME COLUMN knowledge_base_id TO id_knowledge_base;
ALTER TABLE knowledge_tag RENAME COLUMN parent_id TO parent_id_knowledge_tag;

ALTER TABLE knowledge_tag ADD COLUMN IF NOT EXISTS owner VARCHAR(64);

-- ============================================================
-- 6. Create knowledge_tag_share table
-- ============================================================
CREATE TABLE IF NOT EXISTS knowledge_tag_share (
    id_knowledge_tag_share VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    id_knowledge_tag       VARCHAR(36) NOT NULL,
    group_key              VARCHAR(64) NOT NULL,
    shared_by_user_id      VARCHAR(64),
    permission             VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kts_tag_id ON knowledge_tag_share (id_knowledge_tag);
CREATE INDEX IF NOT EXISTS idx_kts_group_key ON knowledge_tag_share (group_key);

COMMENT ON TABLE knowledge_tag_share IS '标签分享记录';
COMMENT ON COLUMN knowledge_tag_share.permission IS 'viewer 查看; editor 编辑; admin 管理';

-- ============================================================
-- 7. Update existing indexes that reference old column names
-- ============================================================
-- Drop old indexes that reference renamed columns (ignore if not exist)
DROP INDEX IF EXISTS idx_knowledges_knowledge_base_id;
DROP INDEX IF EXISTS idx_chunks_knowledge_id;
DROP INDEX IF EXISTS idx_chunks_knowledge_base_id;
DROP INDEX IF EXISTS idx_knowledge_tags_knowledge_base_id;
DROP INDEX IF EXISTS idx_knowledge_tags_parent_id;

-- Recreate with new column names
CREATE INDEX IF NOT EXISTS idx_knowledge_base_id ON knowledge (id_knowledge_base);
CREATE INDEX IF NOT EXISTS idx_knowledge_enable_status ON knowledge (enable_status);
CREATE INDEX IF NOT EXISTS idx_chunk_knowledge_base ON chunk (id_knowledge_base);
CREATE INDEX IF NOT EXISTS idx_chunk_knowledge ON chunk (id_knowledge);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag_base ON knowledge_tag (id_knowledge_base);
CREATE INDEX IF NOT EXISTS idx_knowledge_tag_parent ON knowledge_tag (parent_id_knowledge_tag);
