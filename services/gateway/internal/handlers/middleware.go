// Package handlers — authentication middleware (docs/09 §1, §3).
//
// requireAuth защищает приватные роуты: требует валидный access-JWT в заголовке
// Authorization: Bearer <token>, извлекает doctor_id (+ role) из проверенных
// claims и кладёт их в request context. Хендлеры ниже по стеку читают владельца
// через doctorIDFromContext — это и есть точка изоляции данных по врачу
// (docs/09 §3: фильтрация на уровне репозитория, не только UI).
//
// Любой сбой проверки (нет заголовка / битый токен / просрочен / неверная
// подпись) → 401 UNAUTHORIZED с конвертом ошибки (docs/07 §1). Тело и токен
// НИКОГДА не логируются (docs/09 §4).
package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/aimed/gateway/internal/auth"
)

// ctxKey is a private context key type (avoids collisions, idiomatic Go).
type ctxKey int

const (
	ctxKeyDoctorID ctxKey = iota
	ctxKeyRole
)

// requireAuth wraps a handler, enforcing a valid access token. On success the
// verified doctor_id and role are injected into the context.
func requireAuth(ts *auth.TokenService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED",
				"требуется авторизация")
			return
		}
		claims, err := ts.ParseAccessToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED",
				"недействительный или истёкший токен")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyDoctorID, claims.DoctorID)
		ctx = context.WithValue(ctx, ctxKeyRole, claims.Role)
		next(w, r.WithContext(ctx))
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// doctorIDFromContext returns the authenticated doctor_id (and true) when the
// request passed requireAuth. Used by scoped handlers (history/detail/export/
// generate) to enforce per-doctor isolation (docs/09 §3).
func doctorIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyDoctorID).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
