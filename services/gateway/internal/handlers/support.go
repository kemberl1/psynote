// Package handlers — чат поддержки: виджет врача и админ-инбокс.
//
//	GET  /api/v1/support/thread              — свой диалог (+ сообщения)
//	POST /api/v1/support/messages            — написать в поддержку
//	POST /api/v1/support/thread/read         — пометить прочитанным
//	GET  /api/v1/admin/support/summary       — счётчик непрочитанных
//	GET  /api/v1/admin/support/threads       — список диалогов
//	GET  /api/v1/admin/support/threads/{id}  — диалог + сообщения
//	POST /api/v1/admin/support/threads/{id}/messages
//	POST /api/v1/admin/support/threads/{id}/read
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
	supportMaxBody   = 4000
	supportRoleUser  = "user"
	supportRoleStaff = "support"
)

type supportThreadView struct {
	ThreadID string                 `json:"thread_id,omitempty"`
	Status   string                 `json:"status"`
	Unread   int                    `json:"unread"`
	Messages []store.SupportMessage `json:"messages"`
}

type supportMessageBody struct {
	Body string `json:"body"`
}

func sanitizeSupportBody(raw string) (string, string) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return "", "напишите сообщение"
	}
	if utf8.RuneCountInString(body) > supportMaxBody {
		return "", "сообщение слишком длинное (максимум 4000 символов)"
	}
	return body, ""
}

func newSupportThreadHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}

		thread, err := repo.GetThreadByDoctor(r.Context(), doctorID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeEnvelope(w, http.StatusOK, envelope{
					Meta: meta{TS: nowRFC3339()},
					Data: supportThreadView{Status: "none", Messages: []store.SupportMessage{}},
				})
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить чат")
			return
		}

		msgs, err := repo.ListMessages(r.Context(), thread.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить сообщения")
			return
		}
		if msgs == nil {
			msgs = []store.SupportMessage{}
		}

		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: supportThreadView{
				ThreadID: thread.ID,
				Status:   thread.Status,
				Unread:   thread.UnreadByUser,
				Messages: msgs,
			},
		})
	}
}

func newSupportSendHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}

		var req supportMessageBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		body, verr := sanitizeSupportBody(req.Body)
		if verr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
			return
		}

		thread, err := repo.GetOrCreateThread(r.Context(), doctorID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось открыть диалог")
			return
		}

		msg, err := repo.AddMessage(r.Context(), thread.ID, doctorID, supportRoleUser, body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось отправить сообщение")
			return
		}
		_ = repo.MarkRead(r.Context(), thread.ID, "user")

		writeEnvelope(w, http.StatusCreated, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: msg,
		})
	}
}

func newSupportReadHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		thread, err := repo.GetThreadByDoctor(r.Context(), doctorID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: map[string]bool{"ok": true}})
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось обновить диалог")
			return
		}
		if err := repo.MarkRead(r.Context(), thread.ID, "user"); err != nil && !errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось отметить прочитанным")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: map[string]bool{"ok": true}})
	}
}

func newAdminSupportSummaryHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sum, err := repo.SupportSummary(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить сводку")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: sum})
	}
}

func newAdminSupportListHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := parseIntQuery(r, "limit", 50)
		offset := parseIntQuery(r, "offset", 0)
		items, total, err := repo.ListThreads(r.Context(), limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить диалоги")
			return
		}
		if items == nil {
			items = []store.SupportThreadListItem{}
		}
		t := total
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Total: &t},
			Data: items,
		})
	}
}

type adminSupportThreadView struct {
	Thread   store.SupportThreadListItem `json:"thread"`
	Messages []store.SupportMessage      `json:"messages"`
}

func newAdminSupportDetailHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id диалога")
			return
		}
		item, err := repo.GetThreadInboxItem(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "диалог не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить диалог")
			return
		}
		msgs, err := repo.ListMessages(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить сообщения")
			return
		}
		if msgs == nil {
			msgs = []store.SupportMessage{}
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: adminSupportThreadView{Thread: *item, Messages: msgs},
		})
	}
}

func newAdminSupportReplyHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id диалога")
			return
		}

		var req supportMessageBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		body, verr := sanitizeSupportBody(req.Body)
		if verr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
			return
		}

		if _, err := repo.GetThreadByID(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "диалог не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить диалог")
			return
		}

		msg, err := repo.AddMessage(r.Context(), id, adminID, supportRoleStaff, body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось отправить ответ")
			return
		}
		_ = repo.MarkRead(r.Context(), id, "admin")

		writeEnvelope(w, http.StatusCreated, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: msg,
		})
	}
}

func newAdminSupportReadHandler(repo store.SupportRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id диалога")
			return
		}
		if err := repo.MarkRead(r.Context(), id, "admin"); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "диалог не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось отметить прочитанным")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: map[string]bool{"ok": true}})
	}
}
