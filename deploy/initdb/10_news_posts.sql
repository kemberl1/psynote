-- ============================================================================
-- Новости и релизы: посты, которые админ пишет в админке, врачи читают в /news.
--
-- ИДЕМПОТЕНТНОСТЬ: IF NOT EXISTS. Безопасно на пустом томе и при повторном
-- прогоне. На уже существующем томе те же DDL выполняет gateway при старте
-- (store.EnsureNewsSchema).
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS news_post (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT NOT NULL,
    summary        TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL,
    post_type      TEXT NOT NULL DEFAULT 'release',
    version_label  TEXT NOT NULL DEFAULT '',
    is_published   BOOLEAN NOT NULL DEFAULT FALSE,
    published_at   TIMESTAMPTZ,
    author_id      UUID REFERENCES doctor(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT news_post_type_chk CHECK (post_type IN ('release', 'news'))
);

CREATE INDEX IF NOT EXISTS idx_news_post_published
    ON news_post (published_at DESC NULLS LAST, created_at DESC)
    WHERE is_published = TRUE;

CREATE INDEX IF NOT EXISTS idx_news_post_updated
    ON news_post (updated_at DESC);
