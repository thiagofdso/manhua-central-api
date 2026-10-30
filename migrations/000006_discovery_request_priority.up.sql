ALTER TABLE catalog_discovery_queries ADD COLUMN last_requested_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity';
CREATE INDEX catalog_discovery_priority ON catalog_discovery_queries(last_requested_at DESC,next_attempt_at) WHERE state <> 'ready';
