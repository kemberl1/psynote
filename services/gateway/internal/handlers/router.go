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
	"github.com/aimed/gateway/internal/auth"
	"github.com/aimed/gateway/internal/config"
	"github.com/aimed/gateway/internal/export"
	"github.com/aimed/gateway/internal/ragclient"
	"github.com/aimed/gateway/internal/store"
)

// Deps holds the orchestration dependencies injected into the router.
// rag/repo may be nil on a degraded boot (e.g. no DB) — affected routes then
// return 503; health still works (graceful degradation, docs/02 §7).
//
// Tokens (Этап 9) — сервис JWT/refresh. Nil => auth-роуты и защита выключены
// (например, пустой JWT_SECRET): приватные роуты тогда возвращают 503, а
// /auth/* не регистрируются. На рабочем стенде JWT_SECRET обязателен (docs/09).
type Deps struct {
	Anonymizer anonymizer.Anonymizer
	RAG        ragclient.Client
	Repo       store.Repository
	Tokens     *auth.TokenService
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

	// protect wraps a private handler with requireAuth when a TokenService is
	// available; otherwise the route is 503 (auth выключен — пустой JWT_SECRET).
	// Это и есть точка изоляции по врачу (docs/09 §3): без валидного access
	// в context не попадёт doctor_id.
	protect := func(h http.HandlerFunc) http.HandlerFunc {
		if deps.Tokens == nil {
			return newUnavailableHandler()
		}
		return requireAuth(deps.Tokens, h)
	}

	// ─── Аутентификация (docs/07 §2, docs/09 §1) ────────────────────────────
	// Публичные: /auth/register, /auth/login, /auth/refresh, /auth/logout.
	// /auth/me — под access-токеном.
	if deps.Tokens != nil && deps.Repo != nil {
		ad := authDeps{repo: deps.Repo, tokens: deps.Tokens, params: auth.DefaultArgon2Params}
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/register", newRegisterHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/login", newLoginHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/refresh", newRefreshHandler(ad))
		mux.HandleFunc("POST "+config.APIPrefix+"/auth/logout", newLogoutHandler(ad))
		mux.HandleFunc("GET "+config.APIPrefix+"/auth/me", protect(newMeHandler(deps.Repo)))
	}

	// ─── Справочники и схема опросника (docs/07 §3) ─────────────────────────
	// ПОД AUTH (обоснование, docs/07 §9 таблица — оба помечены ✅): это рабочее
	// приложение врача, справочник типов/схема опросника — часть рабочего
	// интерфейса, не публичный контент. Защищаем единообразно с остальным API,
	// уменьшая поверхность анонимного доступа (docs/09 §6 «минимизация»).
	mux.HandleFunc("GET "+config.APIPrefix+"/document-types", protect(newDocumentTypesHandler()))
	mux.HandleFunc("GET "+config.APIPrefix+"/questionnaire", protect(newQuestionnaireHandler()))

	// ─── Генерация (docs/07 §5) — под auth, doctor_id из context ─────────────
	if deps.RAG != nil && deps.Repo != nil {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate",
			protect(newGenerateHandler(cfg, deps.Anonymizer, deps.RAG, deps.Repo)))
	} else {
		mux.HandleFunc("POST "+config.APIPrefix+"/generate", newUnavailableHandler())
	}

	// ─── История запросов (docs/07 §6) + алиасы /history (задание Этапа 5) ──
	// Все под auth + изоляция по doctor_id (docs/09 §3).
	if deps.Repo != nil {
		listH := newHistoryListHandler(deps.Repo)
		detailH := newHistoryDetailHandler(deps.Repo)
		mux.HandleFunc("GET "+config.APIPrefix+"/requests", protect(listH))
		mux.HandleFunc("GET "+config.APIPrefix+"/requests/{id}", protect(detailH))
		mux.HandleFunc("GET "+config.APIPrefix+"/history", protect(listH))
		mux.HandleFunc("GET "+config.APIPrefix+"/history/{id}", protect(detailH))

		// ─── Экспорт документа (docs/07 §7) — сервис экспорта Go (docs/02 §4) ──
		exportH := newExportHandler(deps.Repo, export.New())
		mux.HandleFunc("POST "+config.APIPrefix+"/requests/{id}/export", protect(exportH))
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
