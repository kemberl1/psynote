// Package anonymizer is the PII-stripping gate (привратник данных).
//
// This is the MOST critical component for privacy (см. docs/04_anonymization.md
// и docs/02_system_architecture.md §4): ни один байт ПДн не должен попасть
// в БД / Qdrant / лог / LLM до прохождения этого гейта.
//
// Pipeline (docs/04 §3, defence in depth):
//
//	L1 нормализация → L2 regex-детекторы → L3 словари (учреждения) →
//	L4 NER (опц. локальный Python-сайдкар) → L5 морфология ФИО →
//	L6 whitelist (МКБ/препараты не трогаем) → L7 валидатор-гейт.
//
// Замена НЕОБРАТИМАЯ; карта «плейсхолдер → значение» нигде не сохраняется
// (docs/04 §5). Логируются только счётчики категорий, не значения (docs/04 §7).
package anonymizer

import (
	"context"
	"errors"
	"fmt"
)

// Result is the outcome of an anonymization pass.
type Result struct {
	// Text is the anonymized text with typed placeholders ([ДАТА], [ФИО_ВРАЧА], ...).
	Text string
	// RemovedCount is an audit counter of removed PII entities (без значений!).
	RemovedCount int
	// RemovedByType breaks the counter down per category for audit/metrics.
	RemovedByType map[EntityType]int
	// Clean reports whether the validation gate confirms no residual PII.
	Clean bool
	// Suspicions lists residual findings when Clean is false (категории, не значения).
	Suspicions []Suspicion
}

// Anonymizer strips personal data from arbitrary medical text.
type Anonymizer interface {
	// Anonymize runs the multi-level pipeline and validation gate.
	Anonymize(ctx context.Context, raw string) (Result, error)
}

// ErrPIIDetected is returned by Gate when the validation gate is not satisfied.
// Maps to HTTP 422 / error code PII_DETECTED (docs/07_api_contract.md §1).
var ErrPIIDetected = errors.New("anonymizer: residual PII detected, operation blocked")

// Options configures the pipeline. Zero value is safe (Go-only MVP, no NER).
type Options struct {
	// DictionaryDir, if set, loads gazetteers from disk instead of the embedded
	// copies — позволяет пополнять словари без перекомпиляции (docs/04 §3).
	DictionaryDir string
	// NER is the optional level-4 backend (local Python Natasha side-car). When
	// nil, a no-op backend is used and the Go-only layers act as the MVP.
	NER NERClient
	// FailClosed, when true (default), forces Anonymize to surface gate failures
	// via Result.Clean=false. The Gate method always enforces fail-closed.
	FailClosed bool
}

// Pipeline is the production multi-level anonymizer (replaces Stub).
type Pipeline struct {
	dict      *dictionaries
	detectors []regexDetector
	inst      *institutionDetector
	fio       *fioDetector
	wl        *whitelist
	gate      *gate
	ner       NERClient
}

// New constructs a Pipeline, loading dictionaries (embedded or from disk).
func New(opts Options) (*Pipeline, error) {
	d, err := loadDictionaries(opts.DictionaryDir)
	if err != nil {
		return nil, fmt.Errorf("anonymizer: %w", err)
	}
	ner := opts.NER
	if ner == nil {
		ner = noopNER{}
	}
	wl := newWhitelist()
	return &Pipeline{
		dict:      d,
		detectors: buildRegexDetectors(),
		inst:      newInstitutionDetector(d),
		fio:       newFIODetector(d),
		wl:        wl,
		gate:      newGate(wl, d),
		ner:       ner,
	}, nil
}

// Anonymize runs all levels and the validation gate. It never returns the
// original text on a soft failure; instead Result.Clean=false signals the
// caller to block (see Gate). A non-nil error is reserved for hard failures
// (e.g. NER backend error) — fail-closed: in that case Clean is false too.
func (p *Pipeline) Anonymize(ctx context.Context, raw string) (Result, error) {
	// L1 — нормализация.
	text := normalizeText(raw)

	set := &spanSet{}

	// L6 (предварительно) — whitelist как «маска неприкосновенности».
	// Регистрируем до резолва, чтобы защитить МКБ/препараты от замены.
	p.wl.mark(text, set)

	// L2 — regex-детекторы структурированных ПДн.
	runRegexDetectors(p.detectors, text, set)

	// L3 — словари учреждений.
	p.inst.detect(text, set)

	// L5 — морфология ФИО (Go MVP).
	p.fio.detect(text, set)

	// L4 — внешний NER (опционально, локально). Ошибка backend → fail-closed.
	ents, err := p.ner.Recognize(ctx, text)
	if err != nil {
		return Result{Text: "", Clean: false}, fmt.Errorf("anonymizer: NER backend: %w", err)
	}
	for _, e := range ents {
		set.add(e.Start, e.End, e.Type)
	}

	// Резолв пересечений (whitelist выигрывает) и замена на плейсхолдеры.
	resolved := set.resolve()
	cleaned, counts := apply(text, resolved)

	total := 0
	for _, c := range counts {
		total += c
	}

	// L7 — валидатор-гейт (независимый параноидальный проход).
	suspicions := p.gate.inspect(cleaned)

	return Result{
		Text:          cleaned,
		RemovedCount:  total,
		RemovedByType: counts,
		Clean:         len(suspicions) == 0,
		Suspicions:    suspicions,
	}, nil
}

// Gate enforces the fail-closed decision on an anonymization Result. It returns
// ErrPIIDetected when the gate is not satisfied, so callers can map it to HTTP
// 422 before any write to DB / Qdrant / LLM (docs/04 §1, docs/07 §1).
func Gate(r Result) error {
	if !r.Clean {
		return fmt.Errorf("%w: %d residual finding(s)", ErrPIIDetected, len(r.Suspicions))
	}
	return nil
}

// Ensure Pipeline satisfies the interface.
var _ Anonymizer = (*Pipeline)(nil)
