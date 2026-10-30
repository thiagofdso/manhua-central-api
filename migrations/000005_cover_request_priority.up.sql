ALTER TABLE catalog_sources ADD COLUMN last_requested_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity';
CREATE INDEX catalog_sources_priority ON catalog_sources(last_requested_at DESC, next_attempt_at) WHERE state <> 'ready';

UPDATE catalog_sources s SET original_cover_url=m.cover_url
FROM canonical_manhuas m
WHERE s.canonical_id=m.id AND s.domain IN ('manhuaus.com','ravenscans.org')
AND s.original_cover_url='' AND m.cover_url ~ '^https?://';

UPDATE canonical_manhuas SET cover_url=NULL
WHERE primary_domain IN ('manhuaus.com','ravenscans.org') AND cover_url ~ '^https?://';
