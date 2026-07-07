-- 000034_rebuild_tables_from_newsql.down.sql
-- Rollback: This is a destructive rebuild migration.
-- The down migration cannot restore data that was lost during the up migration.
-- It only drops the tables created by the up migration to allow re-running 000033.

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS public.chunk;
DROP SEQUENCE IF EXISTS public.chunk_seq_id_seq;
DROP TABLE IF EXISTS public.knowledge;
DROP TABLE IF EXISTS public.knowledge_base;
DROP TABLE IF EXISTS public.knowledge_tag;
DROP TABLE IF EXISTS public.knowledge_tag_share;

-- NOTE: To fully restore the previous state, you would need to re-run migrations
-- 000000 through 000033 from scratch, or restore from a database backup.
