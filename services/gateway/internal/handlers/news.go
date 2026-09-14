// Package handlers — новости и релизы.
//
//	GET    /api/v1/news                 — опубликованные посты (врач)
//	GET    /api/v1/news/{id}            — один опубликованный пост
//	GET    /api/v1/admin/news           — все посты, включая черновики
//	GET    /api/v1/admin/news/{id}      — любой пост
//	POST   /api/v1/admin/news           — создать
//	PATCH  /api/v1/admin/news/{id}      — обновить
//	DELETE /api/v1/admin/news/{id}      — удалить
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
	newsMaxTitle   = 200
	newsMaxSummary = 500
	newsMaxBody    = 20000
	newsMaxVersion = 40
	newsMaxJSON    = 64 << 10
)

type newsCreateBody struct {
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Body         string `json:"body"`
	PostType     string `json:"post_type"`
	VersionLabel string `json:"version_label"`
	IsPublished  bool   `json:"is_published"`
}

type newsPatchBody struct {
	Title        *string `json:"title"`
	Summary      *string `json:"summary"`
	Body         *string `json:"body"`
	PostType     *string `json:"post_type"`
	VersionLabel *string `json:"version_label"`
	IsPublished  *bool   `json:"is_published"`
}

func normalizeNewsType(raw string, allowEmpty bool) (string, string) {
	t := strings.TrimSpace(raw)
	if t == "" {
		if allowEmpty {
			return "", ""
		}
		return store.NewsTypeRelease, ""
	}
	if t != store.NewsTypeRelease && t != store.NewsTypeNews {
		return "", "тип поста: release или news"
	}
	return t, ""
}

func validateNewsFields(title, summary, body, version string, requireTitle, requireBody bool) string {
	if requireTitle && title == "" {
		return "укажите заголовок"
	}
	if requireBody && body == "" {
		return "напишите текст поста"
	}
	if utf8.RuneCountInString(title) > newsMaxTitle {
		return "заголовок слишком длинный (максимум 200 символов)"
	}
	if utf8.RuneCountInString(summary) > newsMaxSummary {
		return "краткое описание слишком длинное (максимум 500 символов)"
	}
	if utf8.RuneCountInString(body) > newsMaxBody {
		return "текст слишком длинный (максимум 20 000 символов)"
	}
	if utf8.RuneCountInString(version) > newsMaxVersion {
		return "версия слишком длинная (максимум 40 символов)"
	}
	return ""
}

func newsPostTypeFilter(r *http.Request) (string, string) {
	raw := strings.TrimSpace(r.URL.Query().Get("type"))
	if raw == "" || raw == "all" {
		return "", ""
	}
	if raw != store.NewsTypeRelease && raw != store.NewsTypeNews {
		return "", "фильтр type: release, news или all"
	}
	return raw, ""
}

func writeNewsList(w http.ResponseWriter, items []store.NewsPost, total int) {
	if items == nil {
		items = []store.NewsPost{}
	}
	t := total
	writeEnvelope(w, http.StatusOK, envelope{
		Meta: meta{TS: nowRFC3339(), Total: &t},
		Data: items,
	})
}

func writeNewsPost(w http.ResponseWriter, status int, post *store.NewsPost) {
	writeEnvelope(w, status, envelope{
		Meta: meta{TS: nowRFC3339()},
		Data: post,
	})
}

func newsIDOrBadRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id поста")
		return "", false
	}
	return id, true
}

func decodeJSON[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, newsMaxJSON)).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
		return false
	}
	return true
}

func newNewsListHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postType, ferr := newsPostTypeFilter(r)
		if ferr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", ferr)
			return
		}
		items, total, err := repo.ListNewsPosts(r.Context(), store.NewsListFilter{
			PublishedOnly: true,
			IncludeBody:   false,
			PostType:      postType,
			Limit:         parseIntQuery(r, "limit", 50),
			Offset:        parseIntQuery(r, "offset", 0),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить новости")
			return
		}
		writeNewsList(w, items, total)
	}
}

func newNewsDetailHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := newsIDOrBadRequest(w, r)
		if !ok {
			return
		}
		post, err := repo.GetNewsPost(r.Context(), id, true)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "пост не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить пост")
			return
		}
		writeNewsPost(w, http.StatusOK, post)
	}
}

func newAdminNewsListHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postType, ferr := newsPostTypeFilter(r)
		if ferr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", ferr)
			return
		}
		items, total, err := repo.ListNewsPosts(r.Context(), store.NewsListFilter{
			PublishedOnly: false,
			IncludeBody:   true,
			PostType:      postType,
			Limit:         parseIntQuery(r, "limit", 80),
			Offset:        parseIntQuery(r, "offset", 0),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить посты")
			return
		}
		writeNewsList(w, items, total)
	}
}

func newAdminNewsDetailHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := newsIDOrBadRequest(w, r)
		if !ok {
			return
		}
		post, err := repo.GetNewsPost(r.Context(), id, false)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "пост не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить пост")
			return
		}
		writeNewsPost(w, http.StatusOK, post)
	}
}

func newAdminNewsCreateHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		var req newsCreateBody
		if !decodeJSON(w, r, &req) {
			return
		}
		title := strings.TrimSpace(req.Title)
		summary := strings.TrimSpace(req.Summary)
		body := strings.TrimSpace(req.Body)
		version := strings.TrimSpace(req.VersionLabel)
		postType, typeErr := normalizeNewsType(req.PostType, false)
		if typeErr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", typeErr)
			return
		}
		if verr := validateNewsFields(title, summary, body, version, true, true); verr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
			return
		}
		post, err := repo.CreateNewsPost(r.Context(), store.NewsPostInput{
			Title:        title,
			Summary:      summary,
			Body:         body,
			PostType:     postType,
			VersionLabel: version,
			IsPublished:  req.IsPublished,
			AuthorID:     doctorID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось сохранить пост")
			return
		}
		writeNewsPost(w, http.StatusCreated, post)
	}
}

func newAdminNewsPatchHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := newsIDOrBadRequest(w, r)
		if !ok {
			return
		}
		var req newsPatchBody
		if !decodeJSON(w, r, &req) {
			return
		}
		patch := store.NewsPostPatch{}
		if req.Title != nil {
			title := strings.TrimSpace(*req.Title)
			if verr := validateNewsFields(title, "", "", "", true, false); verr != "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
				return
			}
			patch.Title = &title
		}
		if req.Summary != nil {
			summary := strings.TrimSpace(*req.Summary)
			if verr := validateNewsFields("", summary, "", "", false, false); verr != "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
				return
			}
			patch.Summary = &summary
		}
		if req.Body != nil {
			body := strings.TrimSpace(*req.Body)
			if verr := validateNewsFields("", "", body, "", false, true); verr != "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
				return
			}
			patch.Body = &body
		}
		if req.PostType != nil {
			postType, typeErr := normalizeNewsType(*req.PostType, false)
			if typeErr != "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", typeErr)
				return
			}
			patch.PostType = &postType
		}
		if req.VersionLabel != nil {
			version := strings.TrimSpace(*req.VersionLabel)
			if verr := validateNewsFields("", "", "", version, false, false); verr != "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
				return
			}
			patch.VersionLabel = &version
		}
		patch.IsPublished = req.IsPublished

		if patch.Title == nil && patch.Summary == nil && patch.Body == nil &&
			patch.PostType == nil && patch.VersionLabel == nil && patch.IsPublished == nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "нечего обновлять")
			return
		}

		post, err := repo.UpdateNewsPost(r.Context(), id, patch)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "пост не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось обновить пост")
			return
		}
		writeNewsPost(w, http.StatusOK, post)
	}
}

func newAdminNewsDeleteHandler(repo store.NewsRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := newsIDOrBadRequest(w, r)
		if !ok {
			return
		}
		if err := repo.DeleteNewsPost(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "пост не найден")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось удалить пост")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: map[string]bool{"ok": true},
		})
	}
}
