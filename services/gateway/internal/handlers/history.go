// Package handlers — request history endpoints (docs/07 §6).
//
//	GET    /api/v1/requests?limit=&offset=  — anonymized history list;
//	GET    /api/v1/requests/{id}            — full anonymized record;
//	POST   /api/v1/requests/pending         — создать запись «Формируется…»;
//	PATCH  /api/v1/requests/{id}            — обновить title/status/answers;
//	DELETE /api/v1/requests/{id}            — удалить запись (+ детей пакета);
//	GET    /api/v1/history[...]             — aliases.
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/aimed/gateway/internal/store"
)

// newHistoryListHandler returns GET /requests (docs/07 §6).
func newHistoryListHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseIntQuery(r, "limit", 20)
		offset := parseIntQuery(r, "offset", 0)

		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		items, total, err := repo.ListGenerations(r.Context(), store.ListFilter{
			DoctorID: &doctorID,
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить историю")
			return
		}

		t := total
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Total: &t},
			Data: items,
		})
	}
}

// newHistoryDetailHandler returns GET /requests/{id} (docs/07 §6).
func newHistoryDetailHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}

		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		detail, err := repo.GetGeneration(r.Context(), id, &doctorID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить запись")
			return
		}

		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{RequestID: detail.RequestID, TS: nowRFC3339()},
			Data: detail,
		})
	}
}

type pendingRequest struct {
	DocumentType    string         `json:"document_type"`
	TitleSafe       string         `json:"title_safe"`
	Answers         map[string]any `json:"answers_anonymized"`
	ParentRequestID string         `json:"parent_request_id,omitempty"`
}

type pendingData struct {
	RequestID    string `json:"request_id"`
	DocumentType string `json:"document_type"`
	TitleSafe    string `json:"title_safe"`
	Status       string `json:"status"`
}

// newPendingHandler creates a history row with status=pending («Формируется…»).
func newPendingHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req pendingRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		if req.DocumentType == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "поле document_type обязательно")
			return
		}
		if !isHistoryDocumentType(req.DocumentType) {
			writeError(w, http.StatusBadRequest, "INVALID_DOCUMENT_TYPE", "неизвестный тип документа")
			return
		}
		title := req.TitleSafe
		if title == "" {
			title = defaultPendingTitle(req.DocumentType)
		}

		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		var parentID *string
		if req.ParentRequestID != "" {
			parentID = &req.ParentRequestID
		}

		id, err := repo.SaveGeneration(r.Context(), store.GenerationRecord{
			DoctorID:          &doctorID,
			DocumentType:      req.DocumentType,
			ParentRequestID:   parentID,
			AnswersAnonymized: req.Answers,
			TitleSafe:         title,
			Status:            "pending",
			ContentAnonymized: "",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось создать запись")
			return
		}

		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{RequestID: id, TS: nowRFC3339()},
			Data: pendingData{
				RequestID:    id,
				DocumentType: req.DocumentType,
				TitleSafe:    title,
				Status:       "pending",
			},
		})
	}
}

type patchRequest struct {
	TitleSafe string         `json:"title_safe"`
	Status    string         `json:"status"`
	Answers   map[string]any `json:"answers_anonymized"`
}

// newHistoryPatchHandler updates title/status/answers of an existing record.
func newHistoryPatchHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}
		var req patchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		if req.Status == "" && req.TitleSafe == "" && req.Answers == nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нечего обновлять")
			return
		}

		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}

		// Load current to merge partial updates.
		cur, err := repo.GetGeneration(r.Context(), id, &doctorID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить запись")
			return
		}
		title := cur.TitleSafe
		if req.TitleSafe != "" {
			title = req.TitleSafe
		}
		status := cur.Status
		if req.Status != "" {
			status = req.Status
		}
		answers := cur.AnswersAnonymized
		if req.Answers != nil {
			answers = req.Answers
		}

		if err := repo.UpdateGenerationMeta(r.Context(), id, &doctorID, title, status, answers); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось обновить запись")
			return
		}

		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{RequestID: id, TS: nowRFC3339()},
			Data: map[string]any{
				"request_id": id,
				"title_safe": title,
				"status":     status,
			},
		})
	}
}

// newHistoryDeleteHandler returns DELETE /requests/{id}.
func newHistoryDeleteHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		if err := repo.DeleteGeneration(r.Context(), id, &doctorID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось удалить запись")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func isHistoryDocumentType(code string) bool {
	switch code {
	case "daily", "exam_10d", "batch":
		return true
	default:
		return false
	}
}

func defaultPendingTitle(docType string) string {
	switch docType {
	case "exam_10d":
		return "Осмотр (раз в 10 дней) · Формируется…"
	case "batch":
		return "Пакет дневников · Формируется…"
	default:
		return "Ежедневный дневник · Формируется…"
	}
}

// parseIntQuery reads a non-negative integer query param with a fallback.
func parseIntQuery(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}
