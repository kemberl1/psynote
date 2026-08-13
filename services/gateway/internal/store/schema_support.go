package store

import (
	"context"
	"fmt"
)

// supportFeedbackDDL creates chat + feedback tables on an already-initialized
// volume (initdb scripts run only once). Idempotent.
const supportFeedbackDDL = `
CREATE TABLE IF NOT EXISTS support_thread (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id             UUID NOT NULL UNIQUE REFERENCES doctor(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'open',
    last_message_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_preview  TEXT NOT NULL DEFAULT '',
    unread_by_admin       INTEGER NOT NULL DEFAULT 0,
    unread_by_user        INTEGER NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_support_thread_last_msg
    ON support_thread(last_message_at DESC);
CREATE INDEX IF NOT EXISTS idx_support_thread_unread_admin
    ON support_thread(unread_by_admin DESC)
    WHERE unread_by_admin > 0;

CREATE TABLE IF NOT EXISTS support_message (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   UUID NOT NULL REFERENCES support_thread(id) ON DELETE CASCADE,
    sender_id   UUID NOT NULL REFERENCES doctor(id) ON DELETE CASCADE,
    sender_role TEXT NOT NULL,
    body        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_support_message_thread
    ON support_message(thread_id, created_at ASC);

CREATE TABLE IF NOT EXISTS generation_feedback (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id  UUID NOT NULL REFERENCES generation_request(id) ON DELETE CASCADE,
    doctor_id   UUID NOT NULL REFERENCES doctor(id) ON DELETE CASCADE,
    rating      INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment     TEXT NOT NULL DEFAULT '',
    quote       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, doctor_id)
);
CREATE INDEX IF NOT EXISTS idx_gen_feedback_created
    ON generation_feedback(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_gen_feedback_request
    ON generation_feedback(request_id);
`

// EnsureSupportFeedbackSchema applies chat/feedback DDL if missing.
func (r *PgxRepository) EnsureSupportFeedbackSchema(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, supportFeedbackDDL); err != nil {
		return fmt.Errorf("store: ensure support/feedback schema: %w", err)
	}
	return nil
}
