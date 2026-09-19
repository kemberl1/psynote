package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *PgxRepository) UpsertFeedback(ctx context.Context, rec GenerationFeedback) (*GenerationFeedback, error) {
	var out GenerationFeedback
	// Снимок текста дневника на момент отзыва; прежняя версия отзыва —
	// в историю (перегенерация и правка не затирают замечания врача).
	err := r.pool.QueryRow(ctx, `
		WITH archived AS (
			INSERT INTO generation_feedback_history
				(feedback_id, request_id, doctor_id, rating, comment, quote,
				 content_snapshot, created_at, updated_at)
			SELECT id, request_id, doctor_id, rating, comment, quote,
			       content_snapshot, created_at, updated_at
			FROM generation_feedback
			WHERE request_id = $1 AND doctor_id = $2
		)
		INSERT INTO generation_feedback
			(request_id, doctor_id, rating, comment, quote, content_snapshot)
		VALUES ($1, $2, $3, $4, $5, COALESCE(
			(SELECT content_anonymized FROM generated_document WHERE request_id = $1), ''))
		ON CONFLICT (request_id, doctor_id) DO UPDATE SET
			rating = EXCLUDED.rating,
			comment = EXCLUDED.comment,
			quote = EXCLUDED.quote,
			content_snapshot = EXCLUDED.content_snapshot,
			snapshot_backfilled = FALSE,
			updated_at = now()
		RETURNING id, request_id, doctor_id, rating, comment, quote, created_at, updated_at`,
		rec.RequestID, rec.DoctorID, rec.Rating, rec.Comment, rec.Quote,
	).Scan(
		&out.ID, &out.RequestID, &out.DoctorID, &out.Rating,
		&out.Comment, &out.Quote, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: upsert feedback: %w", err)
	}
	return &out, nil
}

func (r *PgxRepository) GetFeedback(ctx context.Context, requestID, doctorID string) (*GenerationFeedback, error) {
	var out GenerationFeedback
	err := r.pool.QueryRow(ctx, `
		SELECT id, request_id, doctor_id, rating, comment, quote, created_at, updated_at
		FROM generation_feedback
		WHERE request_id = $1 AND doctor_id = $2`, requestID, doctorID,
	).Scan(
		&out.ID, &out.RequestID, &out.DoctorID, &out.Rating,
		&out.Comment, &out.Quote, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get feedback: %w", err)
	}
	return &out, nil
}

func (r *PgxRepository) ListFeedback(ctx context.Context, limit, offset int) ([]AdminFeedbackItem, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM generation_feedback`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count feedback: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT f.id, COALESCE(f.request_id::text, ''), f.doctor_id, f.rating,
		       f.comment, f.quote, f.content_snapshot, f.snapshot_backfilled,
		       f.created_at, f.updated_at,
		       COALESCE(d.email, ''), COALESCE(d.display_name, ''),
		       COALESCE(gr.title_safe, 'Дневник удалён'),
		       COALESCE(gr.document_type_code, '')
		FROM generation_feedback f
		JOIN doctor d ON d.id = f.doctor_id
		LEFT JOIN generation_request gr ON gr.id = f.request_id
		ORDER BY f.updated_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list feedback: %w", err)
	}
	defer rows.Close()

	out := make([]AdminFeedbackItem, 0)
	for rows.Next() {
		var it AdminFeedbackItem
		if err := rows.Scan(
			&it.ID, &it.RequestID, &it.DoctorID, &it.Rating, &it.Comment, &it.Quote,
			&it.ContentSnapshot, &it.SnapshotBackfilled, &it.CreatedAt, &it.UpdatedAt,
			&it.DoctorEmail, &it.DoctorName, &it.TitleSafe, &it.DocumentType,
		); err != nil {
			return nil, 0, fmt.Errorf("store: scan feedback: %w", err)
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

var _ FeedbackRepository = (*PgxRepository)(nil)
