// Package handlers — document export endpoint (docs/07 §7).
//
//	POST /api/v1/requests/{id}/export   body: { "format": "docx|pdf|txt",
//	                                            "substitutions": { "[ДАТА]": "…" } }
//	→ binary file (Content-Disposition: attachment; filename=diary_<type>_<date>.<ext>)
//
// РОЛЬ Go (docs/02 §4): «сервис экспорта документов». Источник текста — БД
// обезличенной истории (store.GetGeneration по request_id). Этот один эндпоинт
// покрывает ОБА сценария UX: свежесгенерированный результат (его request_id уже
// сохранён /generate) и просмотр из истории (/requests/{id}) — оба ссылаются на
// одну и ту же запись по id, поэтому отдельный «экспорт из переданного контента»
// не нужен (минимально достаточно и консистентно с docs/07 §6–7).
//
// ПРИВАТНОСТЬ (docs/05 §1, docs/09): рендерится ТОЛЬКО обезличенный
// content_anonymized. substitutions (реальные значения плейсхолдеров) приходят
// с клиента, применяются В ПАМЯТИ и НЕ сохраняются (docs/07 §7). Тело экспорта
// и подстановки НИКОГДА не логируются. Имя файла — без ПДн (тип+дата).
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/aimed/gateway/internal/catalog"
	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/store"
)

// exportRequest is the POST body (docs/07 §7). substitutions is optional.
type exportRequest struct {
	Format        string            `json:"format"`
	Substitutions map[string]string `json:"substitutions,omitempty"`
}

// newExportHandler returns POST /requests/{id}/export (docs/07 §7).
func newExportHandler(repo store.Repository, exporter export.Exporter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id записи")
			return
		}

		var req exportRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}

		format, ok := export.ParseFormat(req.Format)
		if !ok {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"неизвестный формат экспорта (ожидается docx, pdf или txt)")
			return
		}

		// Изоляция по врачу (docs/09 §3): экспортировать можно ТОЛЬКО свою
		// запись. Чужой id → ErrNotFound → 404.
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
			slog.Error("export: load record failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить запись")
			return
		}

		doc := export.Document{
			Title:            titleForExport(detail),
			DocumentTypeCode: detail.DocumentType,
			GeneratedAt:      detail.CreatedAt,
			Content:          detail.Content,
			Answers:          detail.AnswersAnonymized,
			Substitutions:    req.Substitutions,
		}

		data, err := exporter.Export(r.Context(), format, doc)
		if err != nil {
			slog.Error("export: render failed", "format", string(format), "error_type", "render")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось сформировать файл")
			return
		}

		filename := export.Filename(doc, format)
		w.Header().Set("Content-Type", format.ContentType())
		w.Header().Set("Content-Disposition", contentDisposition(filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}

// titleForExport prefers the safe history title; falls back to the document
// type's human label (docs/07 §3). Never contains PII.
func titleForExport(d *store.HistoryDetail) string {
	if strings.TrimSpace(d.TitleSafe) != "" {
		return d.TitleSafe
	}
	for _, dt := range catalog.DocumentTypes() {
		if dt.Code == d.DocumentType {
			return dt.Title
		}
	}
	return "Дневник"
}

// applySubstitutions replaces placeholder→value pairs in the text. Empty map is
// a no-op. Only non-empty keys are applied (defensive).
func applySubstitutions(content string, subs map[string]string) string {
	if len(subs) == 0 {
		return content
	}
	pairs := make([]string, 0, len(subs)*2)
	for k, v := range subs {
		if k == "" {
			continue
		}
		pairs = append(pairs, k, v)
	}
	if len(pairs) == 0 {
		return content
	}
	return strings.NewReplacer(pairs...).Replace(content)
}

// contentDisposition builds an attachment header with both a plain filename and
// an RFC 5987 UTF-8 variant (filename is ASCII-only here, but the UTF-8 form is
// harmless and future-proof). Filename carries no PII (docs/09).
func contentDisposition(filename string) string {
	return "attachment; filename=\"" + filename + "\"; filename*=UTF-8''" + filename
}
