// Package handlers — admin document upload endpoints (Этап 10, docs/07 §8).
//
//	POST /api/v1/admin/documents   — multipart upload .docx/.odt/.doc → RAG /ingest
//	GET  /api/v1/admin/documents   — list upload history
//	GET  /api/v1/admin/documents/{id} — single upload detail
//
// Flow: Gateway validates auth+role+file → sends raw file to RAG /ingest →
// RAG extracts text → calls gateway /anonymize → chunks → embeds → upserts
// to Qdrant. Original file is NOT stored in DB (docs/09).
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// allowedExtensions defines accepted file extensions for upload.
var allowedExtensions = map[string]bool{
	".docx": true,
	".odt":  true,
	".doc":  true,
}

// MaxUploadSize is the maximum file size for admin upload (~15 MB).
const MaxUploadSize int64 = 15 << 20

// adminDeps is the dependency bundle for admin handlers.
type adminDeps struct {
	repo store.AdminRepository
	rag  ragclient.AdminIngestClient
}

// newAdminUploadHandler — POST /admin/documents (multipart/form-data).
func newAdminUploadHandler(d adminDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}

		// Limit body size.
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize+1024)

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"файл не найден в запросе (поле 'file')")
			return
		}
		defer func() { _ = file.Close() }()

		// Validate extension.
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if !allowedExtensions[ext] {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				fmt.Sprintf("неподдерживаемый формат %s (допустимы .docx, .odt, .doc)", ext))
			return
		}

		// Read file content.
		fileData, err := io.ReadAll(io.LimitReader(file, MaxUploadSize+1))
		if err != nil || int64(len(fileData)) > MaxUploadSize {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"файл слишком большой (максимум 15 МБ)")
			return
		}
		if len(fileData) == 0 {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "пустой файл")
			return
		}

		// Save metadata with status=processing.
		fname := storeSafeFilename(header.Filename)
		docRec := store.AdminDocument{
			UploadedBy:       &doctorID,
			OriginalFilename: fname,
			Status:           "processing",
		}
		docID, err := d.repo.SaveAdminDocument(r.Context(), docRec)
		if err != nil {
			slog.Error("admin: save metadata failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось сохранить метаданные")
			return
		}

		// Send file to RAG /ingest (blocks until processing completes).
		contentType := header.Header.Get("Content-Type")
		ingestResult, err := d.rag.IngestFile(r.Context(), fname, fileData, contentType)
		if err != nil {
			slog.Error("admin: RAG ingest failed", "error_type", "rag")
			_ = d.repo.UpdateAdminDocumentResult(r.Context(), docID,
				"failed", 0, nil, 0, nil, "ошибка RAG-сервиса")
			writeError(w, http.StatusBadGateway, "RAG_ERROR", "ошибка обработки документа")
			return
		}

		// Update record with result.
		errMsg := ""
		if ingestResult.ErrorMessage != "" {
			errMsg = ingestResult.ErrorMessage
		}
		err = d.repo.UpdateAdminDocumentResult(r.Context(), docID,
			ingestResult.Status,
			ingestResult.AnonymizerRemovedCount,
			ingestResult.RemovedByType,
			ingestResult.ChunksCount,
			ingestResult.QdrantIDs,
			errMsg,
		)
		if err != nil {
			slog.Error("admin: update result failed", "error_type", "store")
		}

		total := 1
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Total: &total},
			Data: map[string]any{
				"doc_id":                   docID,
				"status":                   ingestResult.Status,
				"original_filename":        fname,
				"anonymizer_removed_count": ingestResult.AnonymizerRemovedCount,
				"removed_by_type":          ingestResult.RemovedByType,
				"chunks_count":             ingestResult.ChunksCount,
				"qdrant_ids":               ingestResult.QdrantIDs,
				"error_message":            errMsg,
			},
		})
	}
}

// newAdminDocumentListHandler — GET /admin/documents.
func newAdminDocumentListHandler(d adminDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docs, total, err := d.repo.ListAdminDocuments(r.Context(), 20, 0)
		if err != nil {
			slog.Error("admin: list documents failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось загрузить список")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Total: &total},
			Data: docs,
		})
	}
}

// newAdminDocumentDetailHandler — GET /admin/documents/{id}.
func newAdminDocumentDetailHandler(d adminDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "не указан id")
			return
		}
		doc, err := d.repo.GetAdminDocument(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "документ не найден")
				return
			}
			slog.Error("admin: get document failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось получить документ")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: doc,
		})
	}
}

// storeSafeFilename sanitizes a filename: strips path components and trims.
func storeSafeFilename(name string) string {
	name = filepath.Base(name)
	if name == "." || name == "/" {
		return "unknown.docx"
	}
	return name
}

// Ensure admin handler functions reference json to avoid unused import.
var _ = json.Marshal
