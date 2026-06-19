// Package handlers wires HTTP routes for the gateway.
//
// Этап 10: admin-роуты (загрузка документов в RAG через UI).
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"

	"github.com/aimed/gateway/internal/anonymizer"
	"github.com/aimed/gateway/internal/auth"
)

// Deps holds the orchestration dependencies injected into the router.
type Deps struct {
	Anonymizer anonymizer.Anonymizer
	RAG        ragclient.Client
	AdminRAG   ragclient.AdminIngestClient
	Repo       store.Repository
	AdminRepo  store.AdminRepository
	Tokens     *auth.TokenService
}

// NewRouter builds the HTTP handler tree.
func NewRouter(cfg config.Config, deps Deps) http.Handler {
	mux := http.NewServeMux()

	// ─── Health (docs/07 §8)
	healthH := newHealthHandler(deps.RAG)
	mux.HandleFunc("GET /health", healthH)
	mux.HandleFunc("GET "+config.APIPrefix+"/health", healthH)

	// ─── Служебный PII-гейт (docs/04)
	if deps.Anonymizer != nil {
		mux.HandleFunc("POST "+config.APIPrefix+"/anonymize", newAnonymizeHandler(deps.Anonymizer))
	}

	// protect wraps a handler with requireAuth.
	protect := func(h http.HandlerFunc) http.HandlerFunc {
		if deps.Tokens == nil {
			return newUnavailableHandler()
		}
		return requireAuth(deps.Tokens, h)
	}

	// protectAdmin wraps a handler with requireAuth + requireAdmin.
	protectAdmin := func(h http.HandlerFunc) http.HandlerFunc {
		if deps.Tokens == nil {
			return newUnavailableHandler()
		}
		return requireAuth(deps.Tokens, requireAdmin(h))
	}

	// ─── Аутентификация (docs/07 §2, docs/09 §1)
	if deps.Tokens != nil && deps.Repo != nil {
		ad := authDeps{repo: deps.Repo, tokens: deps.Tokens, params: auth.DefaultArgon2Params}
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/register", newRegisterHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/login", newLoginHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/refresh", newRefreshHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/logout", newLogoutHandler(ad))
		mux.HandleFunc("GET "+config.APIPrefix+"/auth/me", protect(newMeHandler(deps.Repo)))
	}

	// ─── Справочники и схема опросника (docs/07 §3)
	mux.HandleFunc("GET "+config.APIPrefix+"/document-types", protect(newDocumentTypesHandler()))
	mux.HandleFunc("GET "+config.APIPrefix+"/questionnaire", protect(newQuestionnaireHandler()))

	// ─── Генерация (docs/07 §5)
	if deps.RAG != nil && deps.Repo != nil {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate",
			protect(newGenerateHandler(cfg, deps.Anonymizer, deps.RAG, deps.Repo)))
	} else {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate", newUnavailableHandler())
	}

	// ─── История запросов (docs/07 §6)
	if deps.Repo != nil {
		listH := newHistoryListHandler(deps.Repo)
		detailH := newHistoryDetailHandler(deps.Repo)
		mux.HandleFunc("GET "+config.APIPrefix+"/requests", protect(listH))
		mux.HandleFunc("GET "+config.APIPrefix+"/requests/{id}", protect(detailH))
		mux.HandleFunc("GET "+config.APIPrefix+"/history", protect(listH))
		mux.HandleFunc("GET "+config.APIPrefix+"/history/{id}", protect(detailH))

		// ─── Экспорт документа (docs/07 §7)
		exportH := newExportHandler(deps.Repo, export.New())
		mux.HandleFunc("POST "+config.APIPrefix+"/requests/{id}/export", protect(exportH))
	}

	// ─── Админка: загрузка документов (Этап 10, docs/07 §8)
	if deps.AdminRepo != nil && deps.AdminRAG != nil {
		ad := adminDeps{repo: deps.AdminRepo, rag: deps.AdminRAG}
		mux.HandleFunc("POST "+config.APIPrefix+"/admin/documents", protectAdmin(newAdminUploadHandler(ad)))
		mux.HandleFunc("GET "+config.APIPrefix+"/admin/documents", protectAdmin(newAdminDocumentListHandler(ad)))
		mux.HandleFunc("GET "+config.APIPrefix+"/admin/documents/{id}", protectAdmin(newAdminDocumentDetailHandler(ad)))
	} else {
		unavail := newUnavailableHandler()
		mux.HandleFunc("POST "+config.APIPrefix+"/admin/documents", unavail)
		mux.HandleFunc("GET "+config.APIPrefix+"/admin/documents", unavail)
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

// withCommonMiddleware applies CORS and security headers.
func withCommonMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if cfg.CORSAllowedOrigin != "" {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", cfg.CORSAllowedOrigin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
