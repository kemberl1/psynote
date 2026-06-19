// Package store — PostgreSQL implementation of admin document CRUD (Этап 10).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SaveAdminDocument inserts a new admin_document record (status=processing).
func (r *PgxRepository) SaveAdminDocument(ctx context.Context, rec AdminDocument) (string, error) {
	removedJSON, err := json.Marshal(rec.RemovedByType)
	if err != nil {
		return "", fmt.Errorf("store: marshal removed_by_type: %w", err)
	}
	if len(rec.RemovedByType) == 0 {
		removedJSON = []byte("{}")
	}
	// Guard against pgx nil → SQL NULL vs NOT NULL DEFAULT '{}'
	if rec.QdrantIDs == nil {
		rec.QdrantIDs = []string{}
	}

	var id string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO admin_document
			(uploaded_by, original_filename, status, anonymizer_removed_count,
			 removed_by_type, chunks_count, qdrant_ids, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		rec.UploadedBy, rec.OriginalFilename, rec.Status,
		rec.AnonymizerRemovedCount, removedJSON,
		rec.ChunksCount, rec.QdrantIDs, rec.ErrorMessage,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store: insert admin_document: %w", err)
	}
	return id, nil
}

// UpdateAdminDocumentResult updates an admin_document after processing completes.
func (r *PgxRepository) UpdateAdminDocumentResult(
	ctx context.Context, id string,
	status string, removedCount int, removedByType map[string]int,
	chunksCount int, qdrantIDs []string, errMsg string,
) error {
	removedJSON, err := json.Marshal(removedByType)
	if err != nil {
		removedJSON = []byte("{}")
	}
	if qdrantIDs == nil {
		qdrantIDs = []string{}
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE admin_document
		SET status = $2,
		    anonymizer_removed_count = $3,
		    removed_by_type = $4,
		    chunks_count = $5,
		    qdrant_ids = $6,
		    error_message = $7,
		    updated_at = now()
		WHERE id = $1`,
		id, status, removedCount, removedJSON, chunksCount, qdrantIDs, errMsg,
	)
	if err != nil {
		return fmt.Errorf("store: update admin_document: %w", err)
	}
	return nil
}

// ListAdminDocuments returns all admin documents, newest first.
func (r *PgxRepository) ListAdminDocuments(ctx context.Context, limit, offset int) ([]AdminDocument, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM admin_document`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count admin_document: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, uploaded_by, original_filename, status,
		       anonymizer_removed_count, removed_by_type,
		       chunks_count, qdrant_ids, COALESCE(error_message, ''),
		       created_at, updated_at
		FROM admin_document
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list admin_document: %w", err)
	}
	defer rows.Close()

	docs := make([]AdminDocument, 0, limit)
	for rows.Next() {
		var d AdminDocument
		var removedRaw []byte
		if err := rows.Scan(
			&d.ID, &d.UploadedBy, &d.OriginalFilename, &d.Status,
			&d.AnonymizerRemovedCount, &removedRaw,
			&d.ChunksCount, &d.QdrantIDs, &d.ErrorMessage,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("store: scan admin_document: %w", err)
		}
		if len(removedRaw) > 0 {
			_ = json.Unmarshal(removedRaw, &d.RemovedByType)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: rows admin_document: %w", err)
	}
	return docs, total, nil
}

// GetAdminDocument returns one admin document by id.
func (r *PgxRepository) GetAdminDocument(ctx context.Context, id string) (*AdminDocument, error) {
	var d AdminDocument
	var removedRaw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, uploaded_by, original_filename, status,
		       anonymizer_removed_count, removed_by_type,
		       chunks_count, qdrant_ids, COALESCE(error_message, ''),
		       created_at, updated_at
		FROM admin_document
		WHERE id = $1`, id).Scan(
		&d.ID, &d.UploadedBy, &d.OriginalFilename, &d.Status,
		&d.AnonymizerRemovedCount, &removedRaw,
		&d.ChunksCount, &d.QdrantIDs, &d.ErrorMessage,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get admin_document: %w", err)
	}
	if len(removedRaw) > 0 {
		_ = json.Unmarshal(removedRaw, &d.RemovedByType)
	}
	return &d, nil
}
