package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *PgxRepository) UpsertFeedback(ctx context.Context, rec GenerationFeedback) (*GenerationFeedback, error) {
	var out GenerationFeedback
	err := r.pool.QueryRow(ctx, `
		INSERT INTO generation_feedback (request_id, doctor_id, rating, comment, quote)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (request_id, doctor_id) DO UPDATE SET
			rating = EXCLUDED.rating,
			comment = EXCLUDED.comment,
			quote = EXCLUDED.quote,
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
		SELECT f.id, f.request_id, f.doctor_id, f.rating, f.comment, f.quote,
		       f.created_at, f.updated_at,
		       COALESCE(d.email, ''), COALESCE(d.display_name, ''),
		       COALESCE(gr.title_safe, ''), gr.document_type_code
		FROM generation_feedback f
		JOIN doctor d ON d.id = f.doctor_id
		JOIN generation_request gr ON gr.id = f.request_id
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
			&it.CreatedAt, &it.UpdatedAt,
			&it.DoctorEmail, &it.DoctorName, &it.TitleSafe, &it.DocumentType,
		); err != nil {
			return nil, 0, fmt.Errorf("store: scan feedback: %w", err)
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

var _ FeedbackRepository = (*PgxRepository)(nil)
