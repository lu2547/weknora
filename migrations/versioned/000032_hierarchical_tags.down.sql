-- Migration 000032 Down: Revert hierarchical tag columns
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_tags') THEN
        RETURN;
    END IF;

    DROP INDEX IF EXISTS idx_knowledge_tags_path_prefix;
    DROP INDEX IF EXISTS idx_knowledge_tags_kb_depth;
    DROP INDEX IF EXISTS idx_knowledge_tags_parent;
    DROP INDEX IF EXISTS idx_knowledge_tags_kb_path;
    DROP INDEX IF EXISTS idx_knowledge_tags_parent_name;

    -- Restore the original unique (tenant_id, knowledge_base_id, name) index.
    -- NOTE: If duplicate names under different parents exist, this may fail;
    -- operators must dedupe manually before downgrade.
    CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_kb_name
        ON knowledge_tags (tenant_id, knowledge_base_id, name);

    ALTER TABLE knowledge_tags DROP COLUMN IF EXISTS depth;
    ALTER TABLE knowledge_tags DROP COLUMN IF EXISTS path;
    ALTER TABLE knowledge_tags DROP COLUMN IF EXISTS parent_id;
END $$;
