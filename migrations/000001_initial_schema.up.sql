CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Tabela de Usuários Multi-Tenant (Suporte a E-mail/Senha e Google OAuth)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NULL, -- Nulo para contas autenticadas exclusivamente via Google OAuth
    google_id VARCHAR(128) NULL UNIQUE, -- Identificador único imutável do Google (sub)
    display_name VARCHAR(100) NOT NULL,
    avatar_url TEXT NULL,
    auth_provider VARCHAR(32) NOT NULL DEFAULT 'email', -- 'email', 'google'
    role VARCHAR(32) NOT NULL DEFAULT 'reader', -- 'admin', 'reader'
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 2. Tabela de Obras Canônicas (Catálogo Global Compartilhado)
CREATE TABLE IF NOT EXISTS canonical_manhuas (
    id BIGINT PRIMARY KEY, -- Compatibilidade com IDs numéricos legados
    slug VARCHAR(255) NOT NULL UNIQUE,
    title VARCHAR(255) NOT NULL,
    alternative_titles TEXT NULL,
    synopsis TEXT NULL,
    cover_url TEXT NULL, -- URL na CDN/R2
    primary_domain VARCHAR(64) NOT NULL,
    url_template TEXT NOT NULL,
    last_chapter INTEGER NULL,
    update_day SMALLINT NULL, -- 0 (Segunda) a 6 (Domingo)
    is_hiatus BOOLEAN NOT NULL DEFAULT FALSE,
    bayesian_rating NUMERIC(4, 2) NOT NULL DEFAULT 0.00,
    total_votes INTEGER NOT NULL DEFAULT 0,
    total_views BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_canonical_ranking_views ON canonical_manhuas (total_views DESC);
CREATE INDEX IF NOT EXISTS idx_canonical_ranking_rating ON canonical_manhuas (bayesian_rating DESC, total_votes DESC);
CREATE INDEX IF NOT EXISTS idx_canonical_routine_check ON canonical_manhuas (update_day, is_hiatus, last_chapter) WHERE is_hiatus = FALSE;

-- 3. Biblioteca Pessoal do Usuário (Multi-Tenant Isolado)
CREATE TABLE IF NOT EXISTS user_libraries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    canonical_manhua_id BIGINT NOT NULL REFERENCES canonical_manhuas(id) ON DELETE CASCADE,
    current_chapter INTEGER NOT NULL DEFAULT 1,
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    custom_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at BIGINT NOT NULL, -- Timestamp epoch em segundos para ordenação canônica
    CONSTRAINT uq_user_manhua UNIQUE (user_id, canonical_manhua_id)
);

-- Índice ultra-otimizado para a Regra de Ouro (RN-04)
CREATE INDEX IF NOT EXISTS idx_user_library_sort 
ON user_libraries (user_id, is_favorite DESC, custom_order ASC, updated_at DESC);

-- 4. Avaliações Comunitárias (Rating de 1 a 10)
CREATE TABLE IF NOT EXISTS user_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    canonical_manhua_id BIGINT NOT NULL REFERENCES canonical_manhuas(id) ON DELETE CASCADE,
    score SMALLINT NOT NULL CHECK (score >= 1 AND score <= 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_user_rating UNIQUE (user_id, canonical_manhua_id)
);

-- 5. Registro de Tokens FCM para Push Notifications
CREATE TABLE IF NOT EXISTS fcm_device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token VARCHAR(512) NOT NULL UNIQUE,
    platform VARCHAR(32) NOT NULL DEFAULT 'android', -- 'android', 'ios', 'web'
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
