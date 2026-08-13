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
	repo := &PgxRepository{pool: pool}
	if err := repo.EnsureSupportFeedbackSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := repo.EnsureDoctorProfileSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return repo, nil
}

// Close releases the connection pool.
func (r *PgxRepository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func marshalAnswers(answers map[string]any) ([]byte, error) {
	if len(answers) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(answers)
	if err != nil {
		return nil, fmt.Errorf("store: marshal answers: %w", err)
	}
	return b, nil
}

// SaveGeneration writes the anonymized record atomically (docs/05 §2).
func (r *PgxRepository) SaveGeneration(ctx context.Context, rec GenerationRecord) (string, error) {
	answersJSON, err := marshalAnswers(rec.AnswersAnonymized)
	if err != nil {
		return "", err
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
			(doctor_id, document_type_code, parent_request_id, answers_anonymized,
			 title_safe, llm_model_used, status, anonymizer_removed_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		rec.DoctorID, rec.DocumentType, rec.ParentRequestID, answersJSON,
		rec.TitleSafe, rec.LLMModelUsed, status, rec.AnonymizerRemovedCount,
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

// CompleteGeneration overwrites an existing record with final generation data.
func (r *PgxRepository) CompleteGeneration(ctx context.Context, id string, doctorID *string, rec GenerationRecord) error {
	answersJSON, err := marshalAnswers(rec.AnswersAnonymized)
	if err != nil {
		return err
	}
	status := rec.Status
	if status == "" {
		status = "done"
	}
	docType := rec.DocumentType

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tag interface{ RowsAffected() int64 }
	if doctorID != nil {
		tag, err = tx.Exec(ctx, `
			UPDATE generation_request
			SET answers_anonymized = $2,
			    title_safe = $3,
			    llm_model_used = $4,
			    status = $5,
			    anonymizer_removed_count = $6,
			    document_type_code = CASE WHEN $7 = '' THEN document_type_code ELSE $7 END
			WHERE id = $1 AND doctor_id = $8`,
			id, answersJSON, rec.TitleSafe, rec.LLMModelUsed, status,
			rec.AnonymizerRemovedCount, docType, *doctorID,
		)
	} else {
		tag, err = tx.Exec(ctx, `
			UPDATE generation_request
			SET answers_anonymized = $2,
			    title_safe = $3,
			    llm_model_used = $4,
			    status = $5,
			    anonymizer_removed_count = $6,
			    document_type_code = CASE WHEN $7 = '' THEN document_type_code ELSE $7 END
			WHERE id = $1`,
			id, answersJSON, rec.TitleSafe, rec.LLMModelUsed, status,
			rec.AnonymizerRemovedCount, docType,
		)
	}
	if err != nil {
		return fmt.Errorf("store: complete generation_request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO generated_document (request_id, content_anonymized, tokens_used)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_id) DO UPDATE
		SET content_anonymized = EXCLUDED.content_anonymized,
		    tokens_used = EXCLUDED.tokens_used`,
		id, rec.ContentAnonymized, rec.TokensUsed,
	)
	if err != nil {
		return fmt.Errorf("store: upsert generated_document: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// UpdateGenerationMeta updates title/status/answers without rewriting content.
func (r *PgxRepository) UpdateGenerationMeta(
	ctx context.Context, id string, doctorID *string, titleSafe, status string, answers map[string]any,
) error {
	answersJSON, err := marshalAnswers(answers)
	if err != nil {
		return err
	}

	var tag interface{ RowsAffected() int64 }
	if doctorID != nil {
		tag, err = r.pool.Exec(ctx, `
			UPDATE generation_request
			SET title_safe = $2, status = $3, answers_anonymized = $4
			WHERE id = $1 AND doctor_id = $5`,
			id, titleSafe, status, answersJSON, *doctorID,
		)
	} else {
		tag, err = r.pool.Exec(ctx, `
			UPDATE generation_request
			SET title_safe = $2, status = $3, answers_anonymized = $4
			WHERE id = $1`,
			id, titleSafe, status, answersJSON,
		)
	}
	if err != nil {
		return fmt.Errorf("store: update meta: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteGeneration removes a top-level record; children cascade via FK.
func (r *PgxRepository) DeleteGeneration(ctx context.Context, id string, doctorID *string) error {
	var (
		tag interface{ RowsAffected() int64 }
		err error
	)
	if doctorID != nil {
		tag, err = r.pool.Exec(ctx,
			`DELETE FROM generation_request WHERE id = $1 AND doctor_id = $2 AND parent_request_id IS NULL`,
			id, *doctorID)
	} else {
		tag, err = r.pool.Exec(ctx,
			`DELETE FROM generation_request WHERE id = $1 AND parent_request_id IS NULL`,
			id)
	}
	if err != nil {
		return fmt.Errorf("store: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListGenerations returns top-level anonymized history + total count.
func (r *PgxRepository) ListGenerations(ctx context.Context, f ListFilter) ([]HistoryItem, int, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	if f.DoctorID != nil {
		if err := r.pool.QueryRow(ctx, `
			SELECT count(*) FROM generation_request
			WHERE doctor_id = $1 AND parent_request_id IS NULL`,
			*f.DoctorID).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: count: %w", err)
		}
	} else {
		if err := r.pool.QueryRow(ctx, `
			SELECT count(*) FROM generation_request
			WHERE parent_request_id IS NULL`).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("store: count: %w", err)
		}
	}

	var (
		rows pgx.Rows
		err  error
	)
	if f.DoctorID != nil {
		rows, err = r.pool.Query(ctx, `
			SELECT gr.id, gr.document_type_code, COALESCE(gr.title_safe, ''),
			       COALESCE(gr.llm_model_used, ''), gr.status, gr.created_at,
			       (SELECT count(*) FROM generation_request c WHERE c.parent_request_id = gr.id)
			FROM generation_request gr
			WHERE gr.doctor_id = $1 AND gr.parent_request_id IS NULL
			ORDER BY gr.created_at DESC
			LIMIT $2 OFFSET $3`, *f.DoctorID, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT gr.id, gr.document_type_code, COALESCE(gr.title_safe, ''),
			       COALESCE(gr.llm_model_used, ''), gr.status, gr.created_at,
			       (SELECT count(*) FROM generation_request c WHERE c.parent_request_id = gr.id)
			FROM generation_request gr
			WHERE gr.parent_request_id IS NULL
			ORDER BY gr.created_at DESC
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
			&it.LLMModelUsed, &it.Status, &it.CreatedAt, &it.ChildrenCount); err != nil {
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

	children, err := r.listChildren(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(children) > 0 {
		d.Children = children
	}
	return &d, nil
}

func (r *PgxRepository) listChildren(ctx context.Context, parentID string) ([]HistoryChild, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT gr.id, gr.document_type_code, COALESCE(gr.title_safe, ''),
		       gr.status, gr.created_at, COALESCE(gd.content_anonymized, '')
		FROM generation_request gr
		LEFT JOIN generated_document gd ON gd.request_id = gr.id
		WHERE gr.parent_request_id = $1
		ORDER BY gr.created_at ASC`, parentID)
	if err != nil {
		return nil, fmt.Errorf("store: list children: %w", err)
	}
	defer rows.Close()

	out := make([]HistoryChild, 0)
	for rows.Next() {
		var c HistoryChild
		if err := rows.Scan(&c.RequestID, &c.DocumentType, &c.TitleSafe,
			&c.Status, &c.CreatedAt, &c.Content); err != nil {
			return nil, fmt.Errorf("store: scan child: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Ensure PgxRepository satisfies Repository.
var _ Repository = (*PgxRepository)(nil)
