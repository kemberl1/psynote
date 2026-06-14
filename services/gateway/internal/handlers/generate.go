// Package handlers — generation orchestration endpoint.
//
// POST /api/v1/generate (docs/07 §5) is the heart of the Go orchestrator role
// (docs/02 §3, §4). Flow:
//
//	decode → (1) GATEWAY anonymization gate over free-text answers (fail-closed,
//	first line of defence, docs/04 §1) → (2) ragclient.Generate (RAG runs its
//	own anonymization + retrieval + LLM, docs/03) → (3) persist ANONYMIZED
//	history (docs/05) → (4) return {meta,data} envelope (docs/07 §1).
//
// ДВОЙНАЯ ЗАЩИТА (обоснование по docs/02 §4, docs/04 §1): Go — «привратник
// данных», поэтому свободный текст ответов прогоняется через СВОЙ анонимайзер
// ПЕРЕД отправкой в RAG, даже несмотря на то, что RAG-пайплайн анонимизирует
// повторно (Этап 4). Это fail-closed на границе периметра: если gateway-гейт
// не уверен — запрос блокируется (422) до того, как байт ПДн уйдёт за пределы Go.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/aimed/gateway/internal/anonymizer"
	"github.com/aimed/gateway/internal/catalog"
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// generateRequest is the public request body (docs/07 §5).
type generateRequest struct {
	DocumentType string         `json:"document_type"`
	Answers      map[string]any `json:"answers"`
	Options      map[string]any `json:"options,omitempty"`
}

// generateData is the success payload (docs/07 §5).
type generateData struct {
	RequestID string `json:"request_id"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	// Anonymization summarises HOW MUCH PII the gateway stripped from the
	// doctor's free-text input, powering the UX «мы убрали X ПДн» plate
	// (docs/04 §7). Обратно-совместимое ДОБАВЛЕНИЕ поля в существующий конверт.
	Anonymization anonymizationSummary `json:"anonymization"`
}

// anonymizationSummary is the privacy-safe audit of input anonymization shown
// to the doctor (плашка «мы убрали X ПДн»). It carries ONLY counts and
// human-readable categories — НИКОГДА сами значения убранных ПДн (docs/04 §5, §7).
// removed_by_type maps a category label (ФИО / ДАТА / АДРЕС / ...) to its count.
type anonymizationSummary struct {
	RemovedCount  int            `json:"removed_count"`
	RemovedByType map[string]int `json:"removed_by_type"`
}

// newGenerateHandler wires the orchestration dependencies.
func newGenerateHandler(cfg config.Config, anon anonymizer.Anonymizer, rag ragclient.Client, repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req generateRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		if req.DocumentType == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "поле document_type обязательно")
			return
		}
		if !catalog.IsKnownDocumentType(req.DocumentType) {
			writeError(w, http.StatusBadRequest, "INVALID_DOCUMENT_TYPE", "неизвестный тип документа")
			return
		}

		// (1) Gateway-side anonymization gate over free-text answers. removed is
		// the total count of PII stripped FROM THE DOCTOR'S INPUT; removedByType
		// breaks it down per internal category (без значений) for the UX summary.
		anonAnswers, removed, removedByType, err := anonymizeAnswers(r.Context(), anon, req.Answers)
		if err != nil {
			if errors.Is(err, anonymizer.ErrPIIDetected) {
				writeError(w, http.StatusUnprocessableEntity, "PII_DETECTED",
					"во входных данных обнаружены ПДн, операция заблокирована")
				return
			}
			slog.Error("generate: anonymization failed", "error_type", "internal")
			writeError(w, http.StatusUnprocessableEntity, "PII_DETECTED",
				"не удалось безопасно обезличить ввод")
			return
		}

		// (2) Orchestrate generation through RAG with a generous timeout.
		ctx, cancel := context.WithTimeout(r.Context(), cfg.RAGGenerateTimeout)
		defer cancel()

		res, err := rag.Generate(ctx, ragclient.GenerateRequest{
			DocumentType: req.DocumentType,
			Answers:      anonAnswers,
			Options:      req.Options,
		})
		if err != nil {
			writeRAGError(w, err)
			return
		}

		// (3) Persist ANONYMIZED history (docs/05). Generation already succeeded;
		// a persistence failure must not lose the user's result, so we log
		// (no PII) and still return the content. doctor_id is nil until auth
		// (Этап 9) — schema allows NULL.
		// anonymizer_removed_count persisted to history is the SUM of what the
		// gateway stripped from the doctor's input (removed) plus what RAG's own
		// anonymization pass stripped downstream (res.AnonymizerRemovedCount) —
		// a full audit total (docs/05 §2.2). The /generate response summary below
		// reports ONLY the gateway-side input count, which is exactly the part
		// relevant to the doctor's «мы убрали X ПДн» plate.
		removedTotal := removed + res.AnonymizerRemovedCount
		requestID := res.RequestID
		newID, perr := repo.SaveGeneration(ctx, store.GenerationRecord{
			DoctorID:               nil,
			DocumentType:           req.DocumentType,
			AnswersAnonymized:      res.AnswersAnonymized,
			TitleSafe:              res.TitleSafe,
			LLMModelUsed:           res.LLMModelUsed,
			Status:                 res.Status,
			AnonymizerRemovedCount: removedTotal,
			ContentAnonymized:      res.Content,
			TokensUsed:             res.TokensUsed,
		})
		if perr != nil {
			slog.Error("generate: persist history failed", "error_type", "store")
		} else if newID != "" {
			requestID = newID
		}

		// (4) Envelope (docs/07 §1, §5).
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{
				RequestID:    requestID,
				TS:           nowRFC3339(),
				LLMModelUsed: res.LLMModelUsed,
				TokensUsed:   res.TokensUsed,
			},
			Data: generateData{
				RequestID: requestID,
				Content:   res.Content,
				Status:    res.Status,
				Anonymization: anonymizationSummary{
					RemovedCount:  removed,
					RemovedByType: anonymizer.LabelCounts(removedByType),
				},
			},
		})
	}
}

// anonymizeAnswers runs the gateway PII gate over every free-text fragment in
// the answers map (custom_text values, plain string values, list items),
// returning a NEW map with anonymized text and the total removed-PII count.
// Fail-closed: if any fragment's gate is not satisfied, returns ErrPIIDetected
// and the original answers are never forwarded (docs/04 §1).
//
// Select/enum codes (e.g. "no_change") carry no PII and pass through unchanged;
// running them through the gate is harmless and keeps the rule simple/uniform.
func anonymizeAnswers(ctx context.Context, anon anonymizer.Anonymizer, answers map[string]any) (map[string]any, int, map[anonymizer.EntityType]int, error) {
	if anon == nil || len(answers) == 0 {
		return answers, 0, nil, nil
	}
	total := 0
	byType := map[anonymizer.EntityType]int{}
	out := make(map[string]any, len(answers))
	for k, v := range answers {
		nv, n, err := anonymizeValue(ctx, anon, v, byType)
		if err != nil {
			return nil, 0, nil, err
		}
		total += n
		out[k] = nv
	}
	return out, total, byType, nil
}

// anonymizeValue recursively anonymizes strings, custom_text dicts and lists,
// accumulating per-category counts into byType (без значений).
func anonymizeValue(ctx context.Context, anon anonymizer.Anonymizer, v any, byType map[anonymizer.EntityType]int) (any, int, error) {
	switch val := v.(type) {
	case string:
		return anonymizeString(ctx, anon, val, byType)
	case []any:
		total := 0
		out := make([]any, len(val))
		for i, item := range val {
			ni, n, err := anonymizeValue(ctx, anon, item, byType)
			if err != nil {
				return nil, 0, err
			}
			total += n
			out[i] = ni
		}
		return out, total, nil
	case map[string]any:
		total := 0
		out := make(map[string]any, len(val))
		for k, item := range val {
			// custom_text is the canonical free-text carrier (docs/06 §1.4).
			ni, n, err := anonymizeValue(ctx, anon, item, byType)
			if err != nil {
				return nil, 0, err
			}
			total += n
			out[k] = ni
		}
		return out, total, nil
	default:
		return v, 0, nil
	}
}

// anonymizeString runs the gate over one string and returns the cleaned text,
// folding its per-category removal counts into byType (audit, без значений).
func anonymizeString(ctx context.Context, anon anonymizer.Anonymizer, s string, byType map[anonymizer.EntityType]int) (string, int, error) {
	if s == "" {
		return s, 0, nil
	}
	res, err := anon.Anonymize(ctx, s)
	if err != nil {
		// Hard failure → fail-closed.
		return "", 0, anonymizer.ErrPIIDetected
	}
	if gateErr := anonymizer.Gate(res); gateErr != nil {
		return "", 0, gateErr
	}
	for t, c := range res.RemovedByType {
		byType[t] += c
	}
	return res.Text, res.RemovedCount, nil
}

// writeRAGError maps a ragclient error to the contract HTTP status (docs/07 §1).
func writeRAGError(w http.ResponseWriter, err error) {
	if errors.Is(err, ragclient.ErrUnavailable) {
		slog.Error("generate: rag unavailable")
		writeError(w, http.StatusServiceUnavailable, "LLM_UNAVAILABLE",
			"сервис генерации временно недоступен")
		return
	}
	var rerr *ragclient.Error
	if errors.As(err, &rerr) {
		// Pass the upstream contract code through with the same HTTP status,
		// constraining to the known range (docs/07 §1).
		status := rerr.HTTPStatus
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		slog.Error("generate: rag error", "status", status, "code", rerr.Code)
		writeError(w, status, rerr.Code, rerr.Message)
		return
	}
	slog.Error("generate: unexpected orchestration error")
	writeError(w, http.StatusInternalServerError, "INTERNAL", "внутренняя ошибка оркестрации")
}
