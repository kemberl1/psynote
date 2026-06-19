// Package handlers — admin-role guard (Этап 10, docs/09 §3).
//
// requireAdmin wraps a handler, enforcing that the authenticated user has
// role="admin". The request must already have passed requireAuth (which puts
// role into the context).
package handlers

import (
	"context"
	"net/http"
)

// requireAdmin wraps a handler, requiring role=admin. Must be composed after
// requireAuth (which injects ctxKeyRole).
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, ok := roleFromContext(r.Context())
		if !ok || role != "admin" {
			writeError(w, http.StatusForbidden, "FORBIDDEN",
				"доступ только для администраторов")
			return
		}
		next(w, r)
	}
}

// roleFromContext returns the authenticated role from the context (set by requireAuth).
func roleFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyRole).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
