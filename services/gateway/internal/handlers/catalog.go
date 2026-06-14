// Package handlers — reference/catalog endpoints (docs/07 §3).
//
//	GET /api/v1/document-types                       — supported document types;
//	GET /api/v1/questionnaire?document_type=daily    — questionnaire JSON schema.
//
// Источник данных — пакет internal/catalog (статическая каноническая схема по
// docs/06, обоснование размещения — там же). Чистый конфиг, без ПДн.
package handlers

import (
	"net/http"

	"github.com/aimed/gateway/internal/catalog"
)

// newDocumentTypesHandler returns GET /document-types (docs/07 §3).
func newDocumentTypesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: catalog.DocumentTypes(),
		})
	}
}

// newQuestionnaireHandler returns GET /questionnaire?document_type=... (docs/07 §3).
func newQuestionnaireHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		docType := r.URL.Query().Get("document_type")
		if docType == "" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"параметр document_type обязателен")
			return
		}
		schema, ok := catalog.Questionnaire(docType)
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND",
				"схема опросника для типа не найдена")
			return
		}
		v := schema.Version
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339(), Version: &v},
			Data: schema,
		})
	}
}
