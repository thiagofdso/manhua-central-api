CREATE SEQUENCE catalog_collection_ids;
SELECT setval('catalog_collection_ids', GREATEST(COALESCE(MAX(id), 0) + 1, 8000000000000000)) FROM canonical_manhuas;

CREATE TABLE catalog_sources (
    id BIGSERIAL PRIMARY KEY,
    canonical_id BIGINT NOT NULL REFERENCES canonical_manhuas(id) ON DELETE CASCADE,
    domain TEXT NOT NULL,
    source_url TEXT NOT NULL,
    original_cover_url TEXT NOT NULL DEFAULT '',
    file_key TEXT,
    content_hash TEXT,
    content_type TEXT,
    byte_size BIGINT,
    downloaded_at TIMESTAMPTZ,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'processing', 'ready', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(domain, source_url)
);
CREATE INDEX catalog_sources_jobs ON catalog_sources(next_attempt_at) WHERE state <> 'ready';
INSERT INTO catalog_sources(canonical_id, domain, source_url, original_cover_url, next_attempt_at)
SELECT id, primary_domain, url_template, COALESCE(cover_url, ''), 'infinity'::TIMESTAMPTZ FROM canonical_manhuas
WHERE primary_domain IN ('manhuaus.com', 'ravenscans.org')
ON CONFLICT(domain, source_url) DO NOTHING;

CREATE TABLE catalog_discovery_queries (
    domain TEXT NOT NULL,
    query TEXT NOT NULL,
    page INTEGER NOT NULL,
    payload JSONB,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'processing', 'ready', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    refreshed_at TIMESTAMPTZ,
    last_error TEXT,
    PRIMARY KEY(domain, query, page)
);
