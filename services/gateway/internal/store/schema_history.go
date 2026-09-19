package store

import (
	"context"
	"fmt"
)

// historyDDL keeps diaries and doctors' feedback from being lost (idempotent).
//
//   - generated_document_version: a regenerated diary no longer overwrites
//     the previous text/brief without a trace;
//   - generation_feedback.content_snapshot: the diary text the doctor rated;
//   - generation_feedback_history: previous comment when a doctor edits it;
//   - feedback survives deletion of the diary (FK SET NULL, not CASCADE).
//
// Backfill: existing feedback gets the CURRENT diary text as its snapshot,
// marked snapshot_backfilled — the text at rating time is only recoverable
// from a DB backup.
const historyDDL = `
CREATE TABLE IF NOT EXISTS generated_document_version (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id         UUID NOT NULL REFERENCES generation_request(id) ON DELETE CASCADE,
    content_anonymized TEXT NOT NULL,
    answers_anonymized JSONB NOT NULL DEFAULT '{}'::jsonb,
    llm_model_used     TEXT,
    tokens_used        INTEGER,
    archived_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gen_doc_version_request
    ON generated_document_version(request_id, archived_at DESC);

ALTER TABLE generation_feedback
    ADD COLUMN IF NOT EXISTS content_snapshot TEXT NOT NULL DEFAULT '';
-- TRUE: снимок взят задним числом (текст на момент миграции, дневник мог
-- быть уже перегенерирован) — не то, что врач видел при оценке.
ALTER TABLE generation_feedback
    ADD COLUMN IF NOT EXISTS snapshot_backfilled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE generation_feedback ALTER COLUMN request_id DROP NOT NULL;
-- FK → SET NULL; пересоздаём только если он ещё не SET NULL (без
-- блокировки таблицы на каждом старте gateway).
DO $$
DECLARE c record;
BEGIN
    FOR c IN
        SELECT conname FROM pg_constraint
        WHERE conrelid = 'generation_feedback'::regclass
          AND confrelid = 'generation_request'::regclass
          AND contype = 'f' AND confdeltype <> 'n'
    LOOP
        EXECUTE format('ALTER TABLE generation_feedback DROP CONSTRAINT %I', c.conname);
    END LOOP;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'generation_feedback'::regclass
          AND confrelid = 'generation_request'::regclass
          AND contype = 'f'
    ) THEN
        ALTER TABLE generation_feedback
            ADD CONSTRAINT generation_feedback_request_id_fkey
            FOREIGN KEY (request_id) REFERENCES generation_request(id) ON DELETE SET NULL;
    END IF;
END $$;

UPDATE generation_feedback f
SET content_snapshot = gd.content_anonymized, snapshot_backfilled = TRUE
FROM generated_document gd
WHERE gd.request_id = f.request_id AND f.content_snapshot = '';

CREATE TABLE IF NOT EXISTS generation_feedback_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    feedback_id      UUID NOT NULL,
    request_id       UUID,
    doctor_id        UUID,
    rating           INTEGER NOT NULL,
    comment          TEXT NOT NULL DEFAULT '',
    quote            TEXT NOT NULL DEFAULT '',
    content_snapshot TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,
    archived_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gen_feedback_history_feedback
    ON generation_feedback_history(feedback_id, archived_at DESC);
`

// EnsureHistorySchema applies diary-version / feedback-history DDL if missing.
func (r *PgxRepository) EnsureHistorySchema(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, historyDDL); err != nil {
		return fmt.Errorf("store: ensure history schema: %w", err)
	}
	return nil
}
