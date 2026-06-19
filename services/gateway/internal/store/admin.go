// Package store — admin document metadata (Этап 10, docs/05, docs/07 §8).
//
// admin_document хранит МЕТАДАННЫЕ загруженных через админку документов.
// Оригинальный файл НЕ хранится (docs/09 — приватность). Только:
//   - имя файла (для UI), дата загрузки, статус обработки,
//   - счётчик анонимизации, номера чанков в Qdrant.
//
// НИКОГДА не сохраняем содержимое файла / ФИО / ПДн.
package store

import (
	"context"
	"time"
)

// AdminDocument is metadata for a document uploaded via the admin UI.
type AdminDocument struct {
	ID                     string         `json:"id"`
	UploadedBy             *string        `json:"uploaded_by,omitempty"`
	OriginalFilename       string         `json:"original_filename"`
	Status                 string         `json:"status"`
	AnonymizerRemovedCount int            `json:"anonymizer_removed_count"`
	RemovedByType          map[string]int `json:"removed_by_type"`
	ChunksCount            int            `json:"chunks_count"`
	QdrantIDs              []string       `json:"qdrant_ids"`
	ErrorMessage           string         `json:"error_message,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// AdminRepository abstracts admin document persistence (for test mocks).
type AdminRepository interface {
	SaveAdminDocument(ctx context.Context, rec AdminDocument) (string, error)
	UpdateAdminDocumentResult(ctx context.Context, id string, status string, removedCount int, removedByType map[string]int, chunksCount int, qdrantIDs []string, errMsg string) error
	ListAdminDocuments(ctx context.Context, limit, offset int) ([]AdminDocument, int, error)
	GetAdminDocument(ctx context.Context, id string) (*AdminDocument, error)
}
