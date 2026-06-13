// Package anonymizer is the PII-stripping gate (привратник данных).
//
// This is the MOST critical component for privacy (см. docs/04_anonymization.md
// и docs/02_system_architecture.md §4): ни один байт ПДн не должен попасть
// в БД / Qdrant / лог / LLM до прохождения этого гейта.
//
// Этап 1 (каркас): только интерфейс и заглушка. Реальный многоуровневый
// пайплайн (нормализация → regex → словари → NER → морфология → whitelist →
// валидатор-гейт) реализуется на Этапе 1 роадмапа (docs/10).
package anonymizer

import "context"

// Result is the outcome of an anonymization pass.
type Result struct {
	// Text is the anonymized text with typed placeholders ([ДАТА], [ФИО_ВРАЧА], ...).
	Text string
	// RemovedCount is an audit counter of removed PII entities (без значений!).
	RemovedCount int
	// Clean reports whether the validation gate confirms no residual PII.
	Clean bool
}

// Anonymizer strips personal data from arbitrary medical text.
type Anonymizer interface {
	// Anonymize runs the multi-level pipeline and validation gate.
	Anonymize(ctx context.Context, raw string) (Result, error)
}

// Stub is a no-op placeholder. It intentionally reports Clean=false so that
// the gate cannot be accidentally bypassed before the real implementation lands.
type Stub struct{}

// Anonymize is a placeholder. TODO(этап 1 роадмапа): реализовать уровни 1–7.
func (Stub) Anonymize(_ context.Context, _ string) (Result, error) {
	return Result{Text: "", RemovedCount: 0, Clean: false}, nil
}
