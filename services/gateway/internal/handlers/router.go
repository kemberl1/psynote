// Package handlers wires HTTP routes for the gateway.
//
// Этап 1 (каркас): здесь только health-эндпоинт. Будущие роуты
// (auth, generate, attachments, export, document-types, questionnaire —
// см. docs/07_api_contract.md) добавляются как отдельные обработчики,
// что отражает принцип «каждый тип документа = отдельный роут» (docs/02 AD-8).
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aimed/gateway/internal/config"
)

// NewRouter builds the HTTP handler tree using the standard library
// net/http router (Go 1.22+ pattern routing). chi can be introduced later
// without changing this contract (см. docs/02 §5 — chi или net/http).
func NewRouter(cfg config.Config) http.Handler {
	mux := http.NewServeMux()

	// Health endpoint. Доступен и по корню, и под префиксом /api/v1
	// (проверка из docs/10 Этап 0: `GET /api/v1/health`).
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET "+config.APIPrefix+"/health", healthHandler)

	// TODO(этап 5): auth-роуты — POST /api/v1/auth/{register,login,refresh,logout}, GET /me.
	// TODO(этап 4): GET /api/v1/document-types, GET /api/v1/questionnaire.
	// TODO(этап 3): POST /api/v1/generate (анонимизация-гейт → RAG → LLM с fallback).
	// TODO(этап 4): POST /api/v1/attachments (анонимизация + ingestion).
	// TODO(этап 6): POST /api/v1/export (docx|pdf).

	return withCommonMiddleware(cfg, mux)
}

type healthResponse struct {
	Status string `json:"status"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// withCommonMiddleware applies cross-cutting concerns (CORS placeholder,
// security headers). Полноценные middleware (rate-limit, JWT-проверка)
// — на следующих этапах, см. docs/09_security_privacy.md §6.
func withCommonMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Минимальные security-заголовки (docs/09 §6).
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// CORS (белый список origin фронтенда). TODO(этап 4): preflight, методы, заголовки.
		if cfg.CORSAllowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", cfg.CORSAllowedOrigin)
		}

		_ = time.Now() // placeholder для будущего request-logging middleware
		next.ServeHTTP(w, r)
	})
}
