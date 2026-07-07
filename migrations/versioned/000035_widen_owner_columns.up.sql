-- Migration: 000035_widen_owner_columns
-- Description: Widen owner / id_knowledge_base columns from varchar(32) to varchar(64).
-- Rationale: users.id is UUID varchar(36); writing it into owner varchar(32) triggers
--   ERROR: value too long for type character varying(32) (SQLSTATE 22001)
-- Affected columns:
--   - knowledge_base.owner            varchar(32) -> varchar(64)
--   - knowledge_tag.owner             varchar(32) -> varchar(64)
--   - knowledge_tag.id_knowledge_base varchar(32) -> varchar(64)
-- Idempotent: ALTER COLUMN TYPE is safe to re-run.

DO $$ BEGIN RAISE NOTICE '[Migration 000035] Widening owner/id_knowledge_base columns to varchar(64)...'; END $$;

ALTER TABLE public.knowledge_base
    ALTER COLUMN owner TYPE varchar(64);

ALTER TABLE public.knowledge_tag
    ALTER COLUMN owner TYPE varchar(64);

ALTER TABLE public.knowledge_tag
    ALTER COLUMN id_knowledge_base TYPE varchar(64);

DO $$ BEGIN RAISE NOTICE '[Migration 000035] Done.'; END $$;
