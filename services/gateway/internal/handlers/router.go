// Package handlers wires HTTP routes for the gateway.
//
// Этап 5 (оркестратор): помимо health и служебного /anonymize здесь
// регистрируются публичные роуты контракта docs/07:
//
//	GET  /api/v1/document-types         (docs/07 §3)
//	GET  /api/v1/questionnaire          (docs/07 §3)
//	POST /api/v1/generate               (docs/07 §5) — анонимизация→RAG→store
//	GET  /api/v1/requests               (docs/07 §6) — история (обезличенная)
//	GET  /api/v1/requests/{id}          (docs/07 §6) — деталь (обезличенная)
//	GET  /api/v1/history[, /{id}]       — алиасы под имена из задания Этапа 5
//
// Принцип «каждый тип документа = отдельный роут/конфиг» (docs/02 AD-8)
// сохранён: тип передаётся в теле/квери, оркестрация едина.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/aimed/gateway/internal/anonymizer"
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// Deps holds the orchestration dependencies injected into the router.
// rag/repo may be nil on a degraded boot (e.g. no DB) — affected routes then
// return 503; health still works (graceful degradation, docs/02 §7).
type Deps struct {
	Anonymizer anonymizer.Anonymizer
	RAG        ragclient.Client
	Repo       store.Repository
}

// NewRouter builds the HTTP handler tree using the standard library net/http
// router (Go 1.22+ pattern routing, method+path patterns). chi can be
// introduced later without changing this contract (docs/02 §5).
func NewRouter(cfg config.Config, deps Deps) http.Handler {
	mux := http.NewServeMux()

	// ─── Health (docs/07 §8) — доступен и по корню, и под префиксом ─────────
	healthH := newHealthHandler(deps.RAG)
	mux.HandleFunc("GET /health", healthH)
	mux.HandleFunc("GET "+config.APIPrefix+"/health", healthH)

	// ─── Служебный PII-гейт (docs/04) ──────────────────────────────────────
	if deps.Anonymizer != nil {
		mux.HandleFunc("POST "+config.APIPrefix+"/anonymize", newAnonymizeHandler(deps.Anonymizer))
	}

	// ─── Справочники и схема опросника (docs/07 §3) ─────────────────────────
	mux.HandleFunc("GET "+config.APIPrefix+"/document-types", newDocumentTypesHandler())
	mux.HandleFunc("GET "+config.APIPrefix+"/questionnaire", newQuestionnaireHandler())

	// ─── Генерация (docs/07 §5) ─────────────────────────────────────────────
	if deps.RAG != nil && deps.Repo != nil {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate",
			newGenerateHandler(cfg, deps.Anonymizer, deps.RAG, deps.Repo))
	} else {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate", newUnavailableHandler())
	}

	// ─── История запросов (docs/07 §6) + алиасы /history (задание Этапа 5) ──
	if deps.Repo != nil {
		listH := newHistoryListHandler(deps.Repo)
		detailH := newHistoryDetailHandler(deps.Repo)
		mux.HandleFunc("GET "+config.APIPrefix+"/requests", listH)
		mux.HandleFunc("GET "+config.APIPrefix+"/requests/{id}", detailH)
		mux.HandleFunc("GET "+config.APIPrefix+"/history", listH)
		mux.HandleFunc("GET "+config.APIPrefix+"/history/{id}", detailH)

		// ─── Экспорт документа (docs/07 §7) — сервис экспорта Go (docs/02 §4) ──
		exportH := newExportHandler(deps.Repo, export.New())
		mux.HandleFunc("POST "+config.APIPrefix+"/requests/{id}/export", exportH)
	}

	return withCommonMiddleware(cfg, mux)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// newUnavailableHandler returns 503 for routes whose backends failed to init.
func newUnavailableHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE",
			"подсистема временно недоступна")
	}
}

// withCommonMiddleware applies CORS (preflight + headers) and security headers
// (docs/09 §6). CORS origin is the configured frontend (env CORS_ALLOWED_ORIGIN).
func withCommonMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers (docs/09 §6).
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// CORS (white-listed frontend origin).
		if cfg.CORSAllowedOrigin != "" {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", cfg.CORSAllowedOrigin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}

		// Preflight: respond immediately without hitting business handlers.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
