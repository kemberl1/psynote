// Package handlers — отзывы на генерации (оценка + комментарий + цитата).
//
//	GET  /api/v1/requests/{id}/feedback  — свой отзыв (или null)
//	PUT  /api/v1/requests/{id}/feedback  — создать / обновить
//	GET  /api/v1/admin/feedback          — все отзывы (админ)
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/aimed/gateway/internal/store"
)

const (
	feedbackMaxComment = 4000
	feedbackMaxQuote   = 2000
)

type feedbackBody struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
	Quote   string `json:"quote"`
}

func sanitizeFeedback(req feedbackBody) (store.GenerationFeedback, string) {
	if req.Rating < 1 || req.Rating > 5 {
		return store.GenerationFeedback{}, "поставьте оценку от 1 до 5 звёзд"
	}
	comment := strings.TrimSpace(req.Comment)
	quote := strings.TrimSpace(req.Quote)
	if utf8.RuneCountInString(comment) > feedbackMaxComment {
		return store.GenerationFeedback{}, "комментарий слишком длинный (максимум 4000 символов)"
	}
	if utf8.RuneCountInString(quote) > feedbackMaxQuote {
		return store.GenerationFeedback{}, "цитата слишком длинная (максимум 2000 символов)"
	}
	return store.GenerationFeedback{
		Rating:  req.Rating,
		Comment: comment,
		Quote:   quote,
	}, ""
}

type feedbackDeps struct {
	history  store.Repository
	feedback store.FeedbackRepository
}

func newFeedbackGetHandler(d feedbackDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}
		if _, err := d.history.GetGeneration(r.Context(), id, &doctorID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось проверить запись")
			return
		}

		fb, err := d.feedback.GetFeedback(r.Context(), id, doctorID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeEnvelope(w, http.StatusOK, envelope{
					Meta: meta{TS: nowRFC3339(), RequestID: id},
					Data: map[string]any{"feedback": nil},
				})
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить отзыв")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), RequestID: id},
			Data: map[string]any{"feedback": fb},
		})
	}
}

func newFeedbackPutHandler(d feedbackDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}

		var req feedbackBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		rec, verr := sanitizeFeedback(req)
		if verr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
			return
		}

		if _, err := d.history.GetGeneration(r.Context(), id, &doctorID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось проверить запись")
			return
		}

		rec.RequestID = id
		rec.DoctorID = doctorID
		saved, err := d.feedback.UpsertFeedback(r.Context(), rec)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось сохранить отзыв")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), RequestID: id},
			Data: saved,
		})
	}
}

func newAdminFeedbackListHandler(repo store.FeedbackRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseIntQuery(r, "limit", 50)
		offset := parseIntQuery(r, "offset", 0)
		items, total, err := repo.ListFeedback(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить отзывы")
			return
		}
		if items == nil {
			items = []store.AdminFeedbackItem{}
		}
		t := total
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Total: &t},
			Data: items,
		})
	}
}
