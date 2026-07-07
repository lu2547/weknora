-- Down migration for 000035_widen_owner_columns
-- Reverts varchar(64) back to varchar(32). NOTE: rows with owner length > 32 will fail.

DO $$ BEGIN RAISE NOTICE '[Migration 000035 DOWN] Shrinking owner/id_knowledge_base columns back to varchar(32)...'; END $$;

ALTER TABLE public.knowledge_base
    ALTER COLUMN owner TYPE varchar(32);

ALTER TABLE public.knowledge_tag
    ALTER COLUMN owner TYPE varchar(32);

ALTER TABLE public.knowledge_tag
    ALTER COLUMN id_knowledge_base TYPE varchar(32);

DO $$ BEGIN RAISE NOTICE '[Migration 000035 DOWN] Done.'; END $$;
