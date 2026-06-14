// Package store — PostgreSQL implementation backed by pgx/v5 pgxpool.
//
// pgx (not database/sql) is chosen as idiomatic, high-performance native
// PostgreSQL driver with first-class JSONB support (docs/05 §2). All writes go
// through a transaction so generation_request + generated_document stay
// consistent (one request ⇒ one document, docs/05 §2.1).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by GetGeneration when no row matches.
var ErrNotFound = errors.New("store: record not found")

// PgxRepository is the production Repository over a pgxpool.Pool.
type PgxRepository struct {
	pool *pgxpool.Pool
}

// NewPgxRepository connects to PostgreSQL using the given DSN and returns a
// ready repository. It pings once to fail fast on misconfiguration.
func NewPgxRepository(ctx context.Context, dsn string) (*PgxRepository, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &PgxRepository{pool: pool}, nil
}

// Close releases the connection pool.
func (r *PgxRepository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

// SaveGeneration writes the anonymized record atomically (docs/05 §2).
func (r *PgxRepository) SaveGeneration(ctx context.Context, rec GenerationRecord) (string, error) {
	answersJSON, err := json.Marshal(rec.AnswersAnonymized)
	if err != nil {
		return "", fmt.Errorf("store: marshal answers: %w", err)
	}
	if len(rec.AnswersAnonymized) == 0 {
		answersJSON = []byte("{}")
	}
	status := rec.Status
	if status == "" {
		status = "done"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var requestID string
	err = tx.QueryRow(ctx, `
		INSERT INTO generation_request
			(doctor_id, document_type_code, answers_anonymized, title_safe,
			 llm_model_used, status, anonymizer_removed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		rec.DoctorID, rec.DocumentType, answersJSON, rec.TitleSafe,
		rec.LLMModelUsed, status, rec.AnonymizerRemovedCount,
	).Scan(&requestID)
	if err != nil {
		return "", fmt.Errorf("store: insert generation_request: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO generated_document (request_id, content_anonymized, tokens_used)
		VALUES ($1, $2, $3)`,
		requestID, rec.ContentAnonymized, rec.TokensUsed,
	)
	if err != nil {
		return "", fmt.Errorf("store: insert generated_document: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("store: commit: %w", err)
	}
	return requestID, nil
}

// ListGenerations returns the anonymized history list and total count.
func (r *PgxRepository) ListGenerations(ctx context.Context, f ListFilter) ([]HistoryItem, int, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Optional doctor scoping (Этап 9). NULL filter ⇒ all rows.
	var total int
	if f.DoctorID != nil {
		if err := r.pool.QueryRow(ctx,
			`SELECT count(*) FROM generation_request WHERE doctor_id = $1`,
			*f.DoctorID).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: count: %w", err)
		}
	} else {
		if err := r.pool.QueryRow(ctx,
			`SELECT count(*) FROM generation_request`).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: count: %w", err)
		}
	}

	var (
		rows pgx.Rows
		err  error
	)
	if f.DoctorID != nil {
		rows, err = r.pool.Query(ctx, `
			SELECT id, document_type_code, COALESCE(title_safe, ''),
			       COALESCE(llm_model_used, ''), status, created_at
			FROM generation_request
			WHERE doctor_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`, *f.DoctorID, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, document_type_code, COALESCE(title_safe, ''),
			       COALESCE(llm_model_used, ''), status, created_at
			FROM generation_request
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("store: query list: %w", err)
	}
	defer rows.Close()

	items := make([]HistoryItem, 0, limit)
	for rows.Next() {
		var it HistoryItem
		if err := rows.Scan(&it.RequestID, &it.DocumentType, &it.TitleSafe,
			&it.LLMModelUsed, &it.Status, &it.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("store: scan list: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: rows: %w", err)
	}
	return items, total, nil
}

// GetGeneration returns the full anonymized record by id (docs/07 §6).
func (r *PgxRepository) GetGeneration(ctx context.Context, id string, doctorID *string) (*HistoryDetail, error) {
	var (
		d          HistoryDetail
		answersRaw []byte
		content    *string
	)

	query := `
		SELECT gr.id, gr.document_type_code, gr.answers_anonymized,
		       COALESCE(gr.title_safe, ''), COALESCE(gr.llm_model_used, ''),
		       gr.status, gr.anonymizer_removed_count, gr.created_at,
		       gd.content_anonymized
		FROM generation_request gr
		LEFT JOIN generated_document gd ON gd.request_id = gr.id
		WHERE gr.id = $1`
	args := []any{id}
	if doctorID != nil {
		query += ` AND gr.doctor_id = $2`
		args = append(args, *doctorID)
	}

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&d.RequestID, &d.DocumentType, &answersRaw, &d.TitleSafe,
		&d.LLMModelUsed, &d.Status, &d.AnonymizerRemovedCount, &d.CreatedAt, &content,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get: %w", err)
	}

	if content != nil {
		d.Content = *content
	}
	if len(answersRaw) > 0 {
		if err := json.Unmarshal(answersRaw, &d.AnswersAnonymized); err != nil {
			return nil, fmt.Errorf("store: unmarshal answers: %w", err)
		}
	}
	return &d, nil
}

// Ensure PgxRepository satisfies Repository.
var _ Repository = (*PgxRepository)(nil)
