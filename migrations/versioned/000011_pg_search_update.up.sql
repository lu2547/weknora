-- Migration 000011: Update pg_search extension to latest version
-- Equivalent to: psql -c 'ALTER EXTENSION pg_search UPDATE;'
-- Only runs if pg_search is installed (ParadeDB environments only)

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search') THEN
        ALTER EXTENSION pg_search UPDATE;
    END IF;
END $$;
