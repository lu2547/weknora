-- 000033_three_tier_kb_refactor.down.sql
-- Rollback: reverse all renames and drop new objects.

-- Drop new table
DROP TABLE IF EXISTS knowledge_tag_share;

-- Drop new indexes
DROP INDEX IF EXISTS idx_knowledge_base_id;
DROP INDEX IF EXISTS idx_knowledge_enable_status;
DROP INDEX IF EXISTS idx_chunk_knowledge_base;
DROP INDEX IF EXISTS idx_chunk_knowledge;
DROP INDEX IF EXISTS idx_chunk_knowledge_enabled;
DROP INDEX IF EXISTS idx_knowledge_tag_base;
DROP INDEX IF EXISTS idx_knowledge_tag_parent;

-- ============================================================
-- Reverse knowledge_tag column renames
-- ============================================================
ALTER TABLE knowledge_tag DROP COLUMN IF EXISTS owner;
ALTER TABLE knowledge_tag RENAME COLUMN parent_id_knowledge_tag TO parent_id;
ALTER TABLE knowledge_tag RENAME COLUMN id_knowledge_base TO knowledge_base_id;
ALTER TABLE knowledge_tag RENAME COLUMN id_knowledge_tag TO id;

-- ============================================================
-- Reverse chunk column renames
-- ============================================================
ALTER TABLE chunk ALTER COLUMN tag_id TYPE VARCHAR(36);
ALTER TABLE chunk RENAME COLUMN id_knowledge_base TO knowledge_base_id;
ALTER TABLE chunk RENAME COLUMN id_knowledge TO knowledge_id;
ALTER TABLE chunk RENAME COLUMN id_chunk TO id;

-- ============================================================
-- Reverse knowledge column renames
-- ============================================================
ALTER TABLE knowledge ALTER COLUMN enable_status TYPE VARCHAR(255);
ALTER TABLE knowledge ALTER COLUMN enable_status DROP DEFAULT;
ALTER TABLE knowledge RENAME COLUMN id_knowledge_base TO knowledge_base_id;
ALTER TABLE knowledge RENAME COLUMN id_knowledge TO id;

-- ============================================================
-- Reverse knowledge_base changes
-- ============================================================
ALTER TABLE knowledge_base DROP COLUMN IF EXISTS owner;
ALTER TABLE knowledge_base DROP COLUMN IF EXISTS category;
ALTER TABLE knowledge_base RENAME COLUMN id_knowledge_base TO id;

-- ============================================================
-- Reverse table renames
-- ============================================================
ALTER TABLE IF EXISTS knowledge_tag RENAME TO knowledge_tags;
ALTER TABLE IF EXISTS chunk RENAME TO chunks;
ALTER TABLE IF EXISTS knowledge RENAME TO knowledges;
ALTER TABLE IF EXISTS knowledge_base RENAME TO knowledge_bases;

-- Recreate original indexes
CREATE INDEX IF NOT EXISTS idx_knowledges_knowledge_base_id ON knowledges (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_chunks_knowledge_id ON chunks (knowledge_id);
CREATE INDEX IF NOT EXISTS idx_chunks_knowledge_base_id ON chunks (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_tags_knowledge_base_id ON knowledge_tags (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_tags_parent_id ON knowledge_tags (parent_id);
