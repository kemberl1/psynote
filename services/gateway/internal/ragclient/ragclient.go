// Package ragclient is the gateway's client for the Python RAG service.
//
// The gateway sends ONLY anonymized context to RAG (см. docs/02 §2 sequence):
// build_context(обезличенные ответы) и ingest(обезличенный документ).
//
// Этап 1 (каркас): только интерфейс и заглушка. HTTP/gRPC-вызовы к FastAPI
// (services/rag) реализуются на Этапе 2–3 роадмапа.
package ragclient

import "context"

// Sample is one retrieved anonymized few-shot example from the corpus.
type Sample struct {
	Text    string
	DocType string
	Section string
}

// Client talks to the RAG service.
type Client interface {
	// BuildContext retrieves top-k anonymized samples for the given query.
	BuildContext(ctx context.Context, query, docType string) ([]Sample, error)
	// Ingest pushes an already-anonymized document into the vector DB.
	Ingest(ctx context.Context, anonymizedText, docType string) error
}

// Stub is a no-op placeholder.
type Stub struct{}

// BuildContext is a placeholder. TODO(этап 2): retrieval из Qdrant через RAG.
func (Stub) BuildContext(_ context.Context, _, _ string) ([]Sample, error) { return nil, nil }

// Ingest is a placeholder. TODO(этап 2): ingestion обезличенного документа.
func (Stub) Ingest(_ context.Context, _, _ string) error { return nil }
