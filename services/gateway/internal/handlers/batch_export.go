// Package handlers — batch document export (docs/07 §7).
//
//	POST /api/v1/export/batch   body: { "format": "docx|txt",
//	                                   "request_ids": ["uuid", ...],
//	                                   "substitutions": { "[ДАТА]": "…" } }
//	→ one combined binary file
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"

	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/store"
)

const maxBatchExport = 31

type batchExportRequest struct {
	Format        string            `json:"format"`
	RequestIDs    []string          `json:"request_ids"`
	Substitutions map[string]string `json:"substitutions,omitempty"`
}

func newBatchExportHandler(repo store.Repository, exporter export.Exporter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req batchExportRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}

		format, ok := export.ParseFormat(req.Format)
		if !ok {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"неизвестный формат экспорта (ожидается docx или txt)")
			return
		}

		if len(req.RequestIDs) == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "укажите request_ids")
			return
		}
		if len(req.RequestIDs) > maxBatchExport {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"слишком много записей (максимум 31)")
			return
		}

		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}

		docs := make([]export.Document, 0, len(req.RequestIDs))
		for _, id := range req.RequestIDs {
			if id == "" {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "пустой request_id")
				return
			}
			detail, err := repo.GetGeneration(r.Context(), id, &doctorID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusNotFound, "NOT_FOUND", "запись не найдена: "+id)
					return
				}
				slog.Error("batch export: load record failed", "error_type", "store")
				writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить запись")
				return
			}
			docs = append(docs, export.Document{
				Title:            titleForExport(detail),
				DocumentTypeCode: detail.DocumentType,
				GeneratedAt:      detail.CreatedAt,
				Content:          detail.Content,
				Answers:          detail.AnswersAnonymized,
				Substitutions:    req.Substitutions,
			})
		}

		sort.Slice(docs, func(i, j int) bool {
			return docs[i].GeneratedAt.Before(docs[j].GeneratedAt)
		})

		data, err := exporter.ExportBatch(r.Context(), format, docs)
		if err != nil {
			slog.Error("batch export: render failed", "format", string(format), "error_type", "render")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось сформировать файл")
			return
		}

		filename := export.BatchFilename(docs, format)
		w.Header().Set("Content-Type", format.ContentType())
		w.Header().Set("Content-Disposition", contentDisposition(filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}
}
