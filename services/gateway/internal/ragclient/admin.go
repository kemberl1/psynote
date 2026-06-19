// Package ragclient — RAG service admin ingest client (Этап 10).
package ragclient

import "context"

// AdminIngestClient abstracts the RAG /ingest call for admin use (test mock).
type AdminIngestClient interface {
	IngestFile(ctx context.Context, filename string, fileData []byte, contentType string) (*IngestResult, error)
}
