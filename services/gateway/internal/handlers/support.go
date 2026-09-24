// Package handlers — чат поддержки: виджет врача и админ-инбокс.
//
//	GET  /api/v1/support/thread              — свой диалог (+ сообщения)
//	POST /api/v1/support/messages            — написать в поддержку
//	POST /api/v1/support/thread/read         — пометить прочитанным
//	GET  /api/v1/support/attachments/{id}    — вложение из своего диалога
//	GET  /api/v1/admin/support/summary       — счётчик непрочитанных
//	GET  /api/v1/admin/support/threads       — список диалогов
//	GET  /api/v1/admin/support/threads/{id}  — диалог + сообщения
//	POST /api/v1/admin/support/threads/{id}/messages
//	POST /api/v1/admin/support/threads/{id}/read
//	GET  /api/v1/admin/support/attachments/{id}
//
// Сообщения принимаются как JSON {"body": "..."} или multipart/form-data
// (поле body + файлы в поле files) — так в чат прикладывают скриншоты.
package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aimed/gateway/internal/store"
)

const (
	supportMaxBody     = 4000
	supportMaxFiles    = 5
	supportMaxFileSize = 10 << 20
	supportMaxUpload   = 25 << 20
	supportRoleUser    = "user"
	supportRoleStaff   = "support"
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

func sanitizeSupportBody(raw string, hasFiles bool) (string, string) {
	body := strings.TrimSpace(raw)
	if body == "" && !hasFiles {
		return "", "напишите сообщение"
	}
	if utf8.RuneCountInString(body) > supportMaxBody {
		return "", "сообщение слишком длинное (максимум 4000 символов)"
	}
	return body, ""
}

// supportInlineImages are the only types a browser gets to render inline;
// everything else (including SVG) is served as a download.
var supportInlineImages = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// supportContentType trusts the bytes, not the client-declared type.
func supportContentType(data []byte) string {
	ct := http.DetectContentType(data)
	if base, _, err := mime.ParseMediaType(ct); err == nil {
		ct = base
	}
	switch {
	case supportInlineImages[ct], ct == "application/pdf", ct == "text/plain":
		return ct
	case ct == "application/zip":
		return ct // docx/xlsx тоже zip — отдаём как файл
	default:
		return "application/octet-stream"
	}
}

func sanitizeSupportFilename(raw string) string {
	name := filepath.Base(strings.ReplaceAll(raw, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "файл"
	}
	if runes := []rune(name); len(runes) > 200 {
		ext := filepath.Ext(name)
		if utf8.RuneCountInString(ext) > 20 {
			ext = ""
		}
		name = string(runes[:200-utf8.RuneCountInString(ext)]) + ext
	}
	return name
}

// readSupportMessage parses a JSON or multipart message. Returns the body,
// attachments and a user-facing error (empty on success).
func readSupportMessage(w http.ResponseWriter, r *http.Request) (string, []store.SupportAttachmentUpload, string) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		var req supportMessageBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			return "", nil, "невалидное тело запроса"
		}
		body, verr := sanitizeSupportBody(req.Body, false)
		return body, nil, verr
	}

	r.Body = http.MaxBytesReader(w, r.Body, supportMaxUpload+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return "", nil, "вложения слишком большие (максимум 25 МБ за сообщение)"
		}
		return "", nil, "невалидное тело запроса"
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	headers := r.MultipartForm.File["files"]
	if len(headers) > supportMaxFiles {
		return "", nil, "не больше " + strconv.Itoa(supportMaxFiles) + " файлов за раз"
	}
	files := make([]store.SupportAttachmentUpload, 0, len(headers))
	for _, fh := range headers {
		f, err := readSupportFile(fh)
		if err != "" {
			return "", nil, err
		}
		files = append(files, f)
	}

	body, verr := sanitizeSupportBody(r.FormValue("body"), len(files) > 0)
	return body, files, verr
}

func readSupportFile(fh *multipart.FileHeader) (store.SupportAttachmentUpload, string) {
	name := sanitizeSupportFilename(fh.Filename)
	if fh.Size > supportMaxFileSize {
		return store.SupportAttachmentUpload{}, "файл «" + name + "» больше 10 МБ"
	}
	src, err := fh.Open()
	if err != nil {
		return store.SupportAttachmentUpload{}, "не удалось прочитать файл «" + name + "»"
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, supportMaxFileSize+1))
	if err != nil {
		return store.SupportAttachmentUpload{}, "не удалось прочитать файл «" + name + "»"
	}
	if len(data) > supportMaxFileSize {
		return store.SupportAttachmentUpload{}, "файл «" + name + "» больше 10 МБ"
	}
	if len(data) == 0 {
		return store.SupportAttachmentUpload{}, "файл «" + name + "» пустой"
	}
	return store.SupportAttachmentUpload{
		Filename:    name,
		ContentType: supportContentType(data),
		Data:        data,
	}, ""
}

func writeSupportAttachment(w http.ResponseWriter, f *store.SupportAttachmentFile) {
	disposition := "attachment"
	if supportInlineImages[f.ContentType] {
		disposition = "inline"
	}
	cd := mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename})
	if cd == "" {
		cd = disposition
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", cd)
	w.Header().Set("Content-Length", strconv.Itoa(len(f.Data)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Data)
}

// newSupportAttachmentHandler serves an attachment. Doctors see only their
// own thread; the admin route (adminScope) sees every thread.
func newSupportAttachmentHandler(repo store.SupportRepository, adminScope bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id вложения")
			return
		}
		f, err := repo.GetAttachment(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "вложение не найдено")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить вложение")
			return
		}
		if !adminScope && f.DoctorID != doctorID {
			// не палим существование чужих вложений
			writeError(w, http.StatusNotFound, "NOT_FOUND", "вложение не найдено")
			return
		}
		writeSupportAttachment(w, f)
	}
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

		body, files, verr := readSupportMessage(w, r)
		if verr != "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", verr)
			return
		}

		thread, err := repo.GetOrCreateThread(r.Context(), doctorID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось открыть диалог")
			return
		}

		msg, err := repo.AddMessage(r.Context(), thread.ID, doctorID, supportRoleUser, body, files)
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

		body, files, verr := readSupportMessage(w, r)
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

		msg, err := repo.AddMessage(r.Context(), id, adminID, supportRoleStaff, body, files)
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
