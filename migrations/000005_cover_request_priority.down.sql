DROP INDEX IF EXISTS catalog_sources_priority;
ALTER TABLE catalog_sources DROP COLUMN last_requested_at;
