// Package handlers — authentication endpoints (docs/07 §2, docs/09 §1).
//
//	POST /api/v1/auth/register  {email,password,display_name} → 201 {doctor_id,email}
//	POST /api/v1/auth/login     {email,password}              → 200 {access_token,refresh_token,expires_in}
//	POST /api/v1/auth/refresh   {refresh_token}               → 200 {access_token,refresh_token,expires_in}
//	POST /api/v1/auth/logout    {refresh_token}               → 204 (отзыв сессии)
//	GET  /api/v1/auth/me                                      → 200 {doctor_id,email,display_name,role}
//
// Все ответы — конверт {meta,data|error} (docs/07 §1). Коды: 400 валидация,
// 401 неверный логин/пароль или токен, 409 занятый email.
//
// ПРИВАТНОСТЬ (docs/09 §2, §4): пароль хешируется Argon2id и НИКОГДА не
// логируется/не хранится в открытом виде. refresh-токен — opaque; в БД лежит
// только его SHA-256-хэш. Логи без значений (только тип события).
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aimed/gateway/internal/auth"
	"github.com/aimed/gateway/internal/store"
)

// minPasswordLength — базовая политика паролей (docs/09 §2: мин. длина).
const minPasswordLength = 8

// authDeps bundles what the auth handlers need.
type authDeps struct {
	repo   store.Repository
	tokens *auth.TokenService
	params auth.Argon2Params
}

// ─── request/response bodies (docs/07 §2) ────────────────────────────────────

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type registerData struct {
	DoctorID string `json:"doctor_id"`
	Email    string `json:"email"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // access TTL в секундах (docs/07 §2)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type meData struct {
	DoctorID    string `json:"doctor_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// ─── handlers ─────────────────────────────────────────────────────────────────

// newRegisterHandler — POST /auth/register (docs/07 §2). 409 на дубль email.
func newRegisterHandler(d authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		email := normalizeEmail(req.Email)
		if !validEmail(email) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "некорректный email")
			return
		}
		if len([]rune(req.Password)) < minPasswordLength {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"пароль слишком короткий (минимум 8 символов)")
			return
		}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = email // разумный дефолт, без ПДн пациента
		}

		hash, err := auth.HashPassword(req.Password, d.params)
		if err != nil {
			slog.Error("register: hash failed", "error_type", "argon2")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось создать аккаунт")
			return
		}

		id, err := d.repo.CreateDoctor(r.Context(), email, hash, displayName, "doctor")
		if err != nil {
			if errors.Is(err, store.ErrEmailTaken) {
				writeError(w, http.StatusConflict, "EMAIL_TAKEN", "этот email уже зарегистрирован")
				return
			}
			slog.Error("register: create doctor failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось создать аккаунт")
			return
		}

		writeEnvelope(w, http.StatusCreated, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: registerData{DoctorID: id, Email: email},
		})
	}
}

// newLoginHandler — POST /auth/login (docs/07 §2). 401 при неверной паре.
func newLoginHandler(d authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		email := normalizeEmail(req.Email)

		doc, err := d.repo.GetDoctorByEmail(r.Context(), email)
		if err != nil {
			// Не раскрываем, существует ли email (одинаковый ответ — anti-enumeration).
			if !errors.Is(err, store.ErrNotFound) {
				slog.Error("login: lookup failed", "error_type", "store")
			}
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "неверный email или пароль")
			return
		}
		ok, verr := auth.VerifyPassword(req.Password, doc.PasswordHash)
		if verr != nil || !ok || !doc.IsActive {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "неверный email или пароль")
			return
		}

		td, ok2 := issueTokenPair(w, r, d, doc.ID, doc.Role)
		if !ok2 {
			return
		}
		_ = d.repo.TouchLastLogin(r.Context(), doc.ID) // best-effort

		writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: td})
	}
}

// newRefreshHandler — POST /auth/refresh (docs/07 §2). Ротация: старая сессия
// отзывается, выдаётся новая пара (docs/09 §1.3).
func newRefreshHandler(d authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "невалидное тело запроса")
			return
		}
		if strings.TrimSpace(req.RefreshToken) == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "отсутствует refresh-токен")
			return
		}

		hash := auth.HashRefreshToken(req.RefreshToken)
		sess, err := d.repo.GetSessionByHash(r.Context(), hash)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "недействительный refresh-токен")
			return
		}
		if sess.Revoked || !time.Now().UTC().Before(sess.ExpiresAt) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "сессия истекла, войдите заново")
			return
		}

		// Rotation: revoke the used session BEFORE issuing a new one (docs/09 §1.3).
		if err := d.repo.RevokeSession(r.Context(), sess.ID); err != nil {
			slog.Error("refresh: revoke failed", "error_type", "store")
			writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось обновить сессию")
			return
		}

		doc, err := d.repo.GetDoctorByID(r.Context(), sess.DoctorID)
		if err != nil || !doc.IsActive {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "аккаунт недоступен")
			return
		}

		td, ok := issueTokenPair(w, r, d, doc.ID, doc.Role)
		if !ok {
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{Meta: meta{TS: nowRFC3339()}, Data: td})
	}
}

// newLogoutHandler — POST /auth/logout (docs/07 §2). Отзыв сессии → 204.
// Идемпотентно: неизвестный/отозванный токен тоже даёт 204 (нечего раскрывать).
func newLogoutHandler(d authDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logoutRequest
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
		if strings.TrimSpace(req.RefreshToken) != "" {
			hash := auth.HashRefreshToken(req.RefreshToken)
			if sess, err := d.repo.GetSessionByHash(r.Context(), hash); err == nil {
				_ = d.repo.RevokeSession(r.Context(), sess.ID)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// newMeHandler — GET /auth/me (docs/07 §2). Профиль текущего врача из access.
func newMeHandler(repo store.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		doctorID, ok := doctorIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "требуется авторизация")
			return
		}
		doc, err := repo.GetDoctorByID(r.Context(), doctorID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "аккаунт не найден")
			return
		}
		writeEnvelope(w, http.StatusOK, envelope{
			Meta: meta{TS: nowRFC3339()},
			Data: meData{
				DoctorID:    doc.ID,
				Email:       doc.Email,
				DisplayName: doc.DisplayName,
				Role:        doc.Role,
			},
		})
	}
}

// issueTokenPair mints an access JWT + a fresh opaque refresh token, persisting
// the refresh session (hash only). On error it writes a 500 envelope and
// returns ok=false. Возвращаемый tokenData содержит САМ refresh-токен (его
// получает клиент), а в БД ушёл только хэш (docs/09 §1.3).
func issueTokenPair(w http.ResponseWriter, r *http.Request, d authDeps, doctorID, role string) (tokenData, bool) {
	access, err := d.tokens.IssueAccessToken(doctorID, role)
	if err != nil {
		slog.Error("auth: issue access failed", "error_type", "jwt")
		writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось выпустить токен")
		return tokenData{}, false
	}
	refresh, refreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		slog.Error("auth: generate refresh failed", "error_type", "rand")
		writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось выпустить токен")
		return tokenData{}, false
	}
	expiresAt := time.Now().UTC().Add(d.tokens.RefreshTTL())
	if _, err := d.repo.CreateSession(r.Context(), doctorID, refreshHash, expiresAt); err != nil {
		slog.Error("auth: persist session failed", "error_type", "store")
		writeError(w, http.StatusInternalServerError, "INTERNAL", "не удалось создать сессию")
		return tokenData{}, false
	}
	return tokenData{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int(d.tokens.AccessTTL().Seconds()),
	}, true
}

// normalizeEmail lowercases and trims an email for consistent uniqueness.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validEmail is a minimal, dependency-free sanity check (docs/09 §6 валидация
// входа). Не RFC-полный — для MVP достаточно «есть @ и точка после неё».
func validEmail(email string) bool {
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	domain := email[at+1:]
	return strings.Contains(domain, ".") && !strings.HasSuffix(domain, ".")
}
