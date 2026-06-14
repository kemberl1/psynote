// Package handlers — anonymize endpoint.
//
// POST /api/v1/anonymize is a SERVICE/DEBUG endpoint that exposes the PII gate
// directly (docs/04). docs/07_api_contract.md does not define a dedicated
// anonymize route — anonymization is an internal step of /generate and
// /attachments — so this endpoint is marked internal and reuses the contract's
// envelope and the 422/PII_DETECTED error code (docs/07 §1).
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/aimed/gateway/internal/anonymizer"
)

// anonymizeRequest is the input body for the service endpoint.
type anonymizeRequest struct {
	Text string `json:"text"`
}

// anonymizeData is the success payload (mirrors docs/05 anonymizer_removed_count
// and docs/04 typed placeholders). Реальные значения НЕ возвращаются.
type anonymizeData struct {
	Content       string         `json:"content"`
	RemovedCount  int            `json:"anonymizer_removed_count"`
	RemovedByType map[string]int `json:"removed_by_type"`
	GatePassed    bool           `json:"gate_passed"`
}

type meta struct {
	RequestID string `json:"request_id,omitempty"`
	TS        string `json:"ts"`
	// Optional generation metadata (docs/07 §5). Omitted for non-generate routes.
	LLMModelUsed string `json:"llm_model_used,omitempty"`
	TokensUsed   int    `json:"tokens_used,omitempty"`
	// Optional list metadata (docs/07 §6 — total for pagination).
	Total *int `json:"total,omitempty"`
	// Optional schema version (docs/07 §3 — questionnaire meta.version).
	Version *int `json:"version,omitempty"`
}

// nowRFC3339 is the canonical timestamp for envelope meta.ts (docs/07 §1).
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

type envelope struct {
	Meta  meta `json:"meta"`
	Data  any  `json:"data,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// newAnonymizeHandler returns a handler bound to the given pipeline.
func newAnonymizeHandler(p anonymizer.Anonymizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req anonymizeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "поле text обязательно")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		res, err := p.Anonymize(ctx, req.Text)
		if err != nil {
			// Fail-closed: внутренняя ошибка пайплайна = блокировка.
			writeError(w, http.StatusUnprocessableEntity, "PII_DETECTED", "не удалось безопасно обезличить текст")
			return
		}

		// Гейт fail-closed: при остаточных ПДн — 422, тело без значений.
		if gateErr := anonymizer.Gate(res); gateErr != nil {
			if errors.Is(gateErr, anonymizer.ErrPIIDetected) {
				writeError(w, http.StatusUnprocessableEntity, "PII_DETECTED",
					"во входных данных обнаружены ПДн, операция заблокирована")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "ошибка валидатора")
			return
		}

		byType := make(map[string]int, len(res.RemovedByType))
		for t, c := range res.RemovedByType {
			byType[string(t)] = c
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: time.Now().UTC().Format(time.RFC3339)},
			Data: anonymizeData{
				Content:       res.Text,
				RemovedCount:  res.RemovedCount,
				RemovedByType: byType,
				GatePassed:    res.Clean,
			},
		})
	}
}

func writeEnvelope(w http.ResponseWriter, status int, env envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	env := envelope{Meta: meta{TS: time.Now().UTC().Format(time.RFC3339)}}
	env.Error = &struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: msg}
	writeEnvelope(w, status, env)
}
