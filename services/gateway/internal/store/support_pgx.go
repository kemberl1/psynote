package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const messagePreviewLimit = 140

func previewMessage(body string) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if utf8.RuneCountInString(s) <= messagePreviewLimit {
		return s
	}
	runes := []rune(s)
	return string(runes[:messagePreviewLimit]) + "…"
}

func scanThread(row pgx.Row) (*SupportThread, error) {
	var t SupportThread
	err := row.Scan(
		&t.ID, &t.DoctorID, &t.Status, &t.LastMessageAt,
		&t.LastMessagePreview, &t.UnreadByAdmin, &t.UnreadByUser, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *PgxRepository) GetThreadByDoctor(ctx context.Context, doctorID string) (*SupportThread, error) {
	t, err := scanThread(r.pool.QueryRow(ctx, `
		SELECT id, doctor_id, status, last_message_at, last_message_preview,
		       unread_by_admin, unread_by_user, created_at
		FROM support_thread WHERE doctor_id = $1`, doctorID))
	if err != nil {
		if err == ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get support thread by doctor: %w", err)
	}
	return t, nil
}

func (r *PgxRepository) GetThreadByID(ctx context.Context, threadID string) (*SupportThread, error) {
	t, err := scanThread(r.pool.QueryRow(ctx, `
		SELECT id, doctor_id, status, last_message_at, last_message_preview,
		       unread_by_admin, unread_by_user, created_at
		FROM support_thread WHERE id = $1`, threadID))
	if err != nil {
		if err == ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get support thread: %w", err)
	}
	return t, nil
}

func (r *PgxRepository) GetThreadInboxItem(ctx context.Context, threadID string) (*SupportThreadListItem, error) {
	var it SupportThreadListItem
	err := r.pool.QueryRow(ctx, `
		SELECT t.id, t.doctor_id, t.status, t.last_message_at, t.last_message_preview,
		       t.unread_by_admin, t.unread_by_user, t.created_at,
		       COALESCE(d.email, ''), COALESCE(d.display_name, '')
		FROM support_thread t
		JOIN doctor d ON d.id = t.doctor_id
		WHERE t.id = $1`, threadID,
	).Scan(
		&it.ID, &it.DoctorID, &it.Status, &it.LastMessageAt,
		&it.LastMessagePreview, &it.UnreadByAdmin, &it.UnreadByUser, &it.CreatedAt,
		&it.DoctorEmail, &it.DoctorName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get support inbox item: %w", err)
	}
	return &it, nil
}

func (r *PgxRepository) GetOrCreateThread(ctx context.Context, doctorID string) (*SupportThread, error) {
	t, err := scanThread(r.pool.QueryRow(ctx, `
		INSERT INTO support_thread (doctor_id) VALUES ($1)
		ON CONFLICT (doctor_id) DO UPDATE SET doctor_id = EXCLUDED.doctor_id
		RETURNING id, doctor_id, status, last_message_at, last_message_preview,
		          unread_by_admin, unread_by_user, created_at`, doctorID))
	if err != nil {
		return nil, fmt.Errorf("store: get-or-create support thread: %w", err)
	}
	return t, nil
}

func (r *PgxRepository) ListThreads(ctx context.Context, limit, offset int) ([]SupportThreadListItem, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM support_thread
		WHERE last_message_preview <> ''`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count support threads: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.doctor_id, t.status, t.last_message_at, t.last_message_preview,
		       t.unread_by_admin, t.unread_by_user, t.created_at,
		       COALESCE(d.email, ''), COALESCE(d.display_name, '')
		FROM support_thread t
		JOIN doctor d ON d.id = t.doctor_id
		WHERE t.last_message_preview <> ''
		ORDER BY t.unread_by_admin DESC, t.last_message_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list support threads: %w", err)
	}
	defer rows.Close()

	out := make([]SupportThreadListItem, 0)
	for rows.Next() {
		var it SupportThreadListItem
		if err := rows.Scan(
			&it.ID, &it.DoctorID, &it.Status, &it.LastMessageAt,
			&it.LastMessagePreview, &it.UnreadByAdmin, &it.UnreadByUser, &it.CreatedAt,
			&it.DoctorEmail, &it.DoctorName,
		); err != nil {
			return nil, 0, fmt.Errorf("store: scan support thread: %w", err)
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (r *PgxRepository) ListMessages(ctx context.Context, threadID string) ([]SupportMessage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.thread_id, m.sender_id, m.sender_role,
		       COALESCE(d.display_name, ''), m.body, m.created_at
		FROM support_message m
		JOIN doctor d ON d.id = m.sender_id
		WHERE m.thread_id = $1
		ORDER BY m.created_at ASC`, threadID)
	if err != nil {
		return nil, fmt.Errorf("store: list support messages: %w", err)
	}
	defer rows.Close()

	out := make([]SupportMessage, 0)
	for rows.Next() {
		var m SupportMessage
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.SenderID, &m.SenderRole,
			&m.SenderName, &m.Body, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan support message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PgxRepository) AddMessage(ctx context.Context, threadID, senderID, senderRole, body string) (*SupportMessage, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin support tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var msg SupportMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO support_message (thread_id, sender_id, sender_role, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, thread_id, sender_id, sender_role, body, created_at`,
		threadID, senderID, senderRole, body,
	).Scan(&msg.ID, &msg.ThreadID, &msg.SenderID, &msg.SenderRole, &msg.Body, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("store: insert support message: %w", err)
	}

	incAdmin := 0
	incUser := 0
	if senderRole == "user" {
		incAdmin = 1
	} else {
		incUser = 1
	}
	_, err = tx.Exec(ctx, `
		UPDATE support_thread SET
			last_message_at = $2,
			last_message_preview = $3,
			unread_by_admin = unread_by_admin + $4,
			unread_by_user = unread_by_user + $5
		WHERE id = $1`,
		threadID, msg.CreatedAt, previewMessage(body), incAdmin, incUser)
	if err != nil {
		return nil, fmt.Errorf("store: update support thread: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(display_name, '') FROM doctor WHERE id = $1`, senderID,
	).Scan(&msg.SenderName); err != nil {
		return nil, fmt.Errorf("store: sender name: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit support tx: %w", err)
	}
	return &msg, nil
}

func (r *PgxRepository) MarkRead(ctx context.Context, threadID, who string) error {
	var q string
	switch who {
	case "admin":
		q = `UPDATE support_thread SET unread_by_admin = 0 WHERE id = $1`
	case "user":
		q = `UPDATE support_thread SET unread_by_user = 0 WHERE id = $1`
	default:
		return fmt.Errorf("store: mark read: unknown who %q", who)
	}
	tag, err := r.pool.Exec(ctx, q, threadID)
	if err != nil {
		return fmt.Errorf("store: mark support read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PgxRepository) SupportSummary(ctx context.Context) (*SupportSummary, error) {
	var s SupportSummary
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(unread_by_admin), 0),
		       COUNT(*) FILTER (WHERE unread_by_admin > 0)
		FROM support_thread`).Scan(&s.UnreadMessages, &s.UnreadThreads)
	if err != nil {
		return nil, fmt.Errorf("store: support summary: %w", err)
	}
	return &s, nil
}

var _ SupportRepository = (*PgxRepository)(nil)
