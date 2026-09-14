package store

import (
	"context"
	"fmt"
)

// newsDDL creates the news_post table on an already-initialized volume
// (initdb scripts run only once). Idempotent.
const newsDDL = `
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
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'news_post_type_chk'
    ) THEN
        ALTER TABLE news_post
            ADD CONSTRAINT news_post_type_chk
            CHECK (post_type IN ('release', 'news'));
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_news_post_published
    ON news_post (published_at DESC NULLS LAST, created_at DESC)
    WHERE is_published = TRUE;
CREATE INDEX IF NOT EXISTS idx_news_post_updated
    ON news_post (updated_at DESC);
`

// EnsureNewsSchema applies news/release DDL if missing.
func (r *PgxRepository) EnsureNewsSchema(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, newsDDL); err != nil {
		return fmt.Errorf("store: ensure news schema: %w", err)
	}
	return nil
}
