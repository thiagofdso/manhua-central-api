DROP INDEX IF EXISTS catalog_discovery_priority;
ALTER TABLE catalog_discovery_queries DROP COLUMN IF EXISTS last_requested_at;
