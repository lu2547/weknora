-- Migration 000032: Add hierarchical support to knowledge_tags
-- Tags become a tree: parent_id + materialized path + depth.
-- A knowledge/chunk is tagged with a single tag_id which can point to any
-- node of the tree (root, intermediate, or leaf).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_tags') THEN
        RAISE NOTICE '[Migration 000032] knowledge_tags table does not exist, skipping';
        RETURN;
    END IF;

    -- 1) Add new columns (nullable first for backfill)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'knowledge_tags' AND column_name = 'parent_id'
    ) THEN
        ALTER TABLE knowledge_tags ADD COLUMN parent_id VARCHAR(36);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'knowledge_tags' AND column_name = 'path'
    ) THEN
        ALTER TABLE knowledge_tags ADD COLUMN path VARCHAR(1024);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'knowledge_tags' AND column_name = 'depth'
    ) THEN
        ALTER TABLE knowledge_tags ADD COLUMN depth INTEGER NOT NULL DEFAULT 0;
    END IF;

    -- 2) Backfill existing rows: treat them as root level tags
    UPDATE knowledge_tags
    SET path = '/' || name
    WHERE path IS NULL OR path = '';

    UPDATE knowledge_tags
    SET depth = 0
    WHERE depth IS NULL;

    -- 3) Enforce NOT NULL on path
    ALTER TABLE knowledge_tags ALTER COLUMN path SET NOT NULL;

    -- 4) Drop old unique index (tenant + kb + name), replaced by
    --    unique (tenant + kb + parent_id + name) via an expression index.
    DROP INDEX IF EXISTS idx_knowledge_tags_kb_name;

    -- Unique per-parent name (NULL parent means root)
    CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_parent_name
        ON knowledge_tags (tenant_id, knowledge_base_id, COALESCE(parent_id, ''), name);

    -- Unique full path within a KB
    CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_tags_kb_path
        ON knowledge_tags (tenant_id, knowledge_base_id, path);

    -- Secondary indexes for tree queries
    CREATE INDEX IF NOT EXISTS idx_knowledge_tags_parent
        ON knowledge_tags (parent_id);
    CREATE INDEX IF NOT EXISTS idx_knowledge_tags_kb_depth
        ON knowledge_tags (tenant_id, knowledge_base_id, depth);
    -- Prefix search support for descendant queries (LIKE 'path/%')
    CREATE INDEX IF NOT EXISTS idx_knowledge_tags_path_prefix
        ON knowledge_tags (path text_pattern_ops);

    RAISE NOTICE '[Migration 000032] knowledge_tags hierarchical columns ready';
END $$;
