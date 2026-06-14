// Package handlers — request history endpoints (docs/07 §6).
//
//	GET /api/v1/requests?limit=&offset=  — anonymized history list;
//	GET /api/v1/requests/{id}            — full anonymized record;
//	GET /api/v1/history[...]             — aliases (задание Этапа 5 называет
//	                                       эндпоинт /history; контракт docs/07
//	                                       §6 — /requests; поддерживаем оба).
//
// ПРИВАТНОСТЬ: возвращаются ТОЛЬКО обезличенные данные (title_safe, тип, дата,
// модель, id; для детали — answers_anonymized + content с плейсхолдерами).
// Источник — БД, куда писались только обезличенные поля (docs/05 §2.3).
//
// doctor scoping (Этап 9): фильтр по врачу пока выключен (nil) — место под него
// заложено в store.ListFilter.DoctorID / GetGeneration(doctorID).
package handlers

import (
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

		items, total, err := repo.ListGenerations(r.Context(), store.ListFilter{
			DoctorID: nil, // TODO(этап 9): scope by authenticated doctor.
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

		detail, err := repo.GetGeneration(r.Context(), id, nil)
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
