// Package auth — JWT access tokens (HS256) и opaque refresh-токены (docs/09 §1.3).
//
// ⭐ ОТКЛОНЕНИЕ ОТ docs/09 (обосновано): docs/09 §1.2 называет в качестве примера
// github.com/golang-jwt/jwt/v5. Мы реализуем компактный HS256-JWT СВОИМИ
// средствами на crypto/hmac + crypto/sha256 (stdlib), БЕЗ внешней зависимости.
// Причины (полностью в духе docs/09 §1.2 «минимум зависимостей; демонстрирует
// навыки Go»):
//   - HS256-JWT — это header.payload.signature, где signature = HMAC-SHA256;
//     реализация тривиальна и полностью покрывается тестами;
//   - меньше зависимостей → меньше поверхность атаки и проще offline-сборка
//     образа (docs/09 §6 «минимальные контейнеры»);
//   - для диплома по Go своя реализация — ещё один защищаемый Go-компонент.
//
// Access-token несёт claims { sub=doctor_id, role, iat, exp } и подписан
// секретом из ENV JWT_SECRET (config.Config.JWTSecret). Refresh-token —
// OPAQUE (случайные 32 байта, base64url), в БД хранится только его SHA-256-хэш
// (таблица session), что даёт отзыв и ротацию (docs/09 §1.3).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by token verification.
var (
	// ErrTokenMalformed — структура токена не является валидным JWT.
	ErrTokenMalformed = errors.New("auth: malformed token")
	// ErrTokenSignature — подпись не совпала (подделка/неверный секрет).
	ErrTokenSignature = errors.New("auth: invalid token signature")
	// ErrTokenExpired — срок действия истёк (claim exp в прошлом).
	ErrTokenExpired = errors.New("auth: token expired")
)

// Claims carries the verified identity extracted from an access token
// (docs/09 §1.3: doctor_id, role, exp).
type Claims struct {
	DoctorID string
	Role     string
	IssuedAt time.Time
	Expires  time.Time
}

// jwtHeader is the fixed HS256 JWT header.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtPayload is the registered+private claim set we sign.
type jwtPayload struct {
	Sub  string `json:"sub"`  // doctor_id
	Role string `json:"role"` // 'doctor' | 'admin'
	Iat  int64  `json:"iat"`  // issued-at (unix)
	Exp  int64  `json:"exp"`  // expiry (unix)
}

// TokenService issues and verifies tokens for one signing secret and TTL set.
type TokenService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time // injectable clock for tests
}

// NewTokenService builds a token service. secret must be non-empty (loaded from
// ENV JWT_SECRET, docs/09 §6). accessTTL ~15m, refreshTTL ~7–30d (docs/09 §1.3).
func NewTokenService(secret string, accessTTL, refreshTTL time.Duration) (*TokenService, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("auth: JWT secret is empty (set JWT_SECRET)")
	}
	return &TokenService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}, nil
}

// AccessTTL exposes the configured access-token lifetime (for expires_in).
func (s *TokenService) AccessTTL() time.Duration { return s.accessTTL }

// RefreshTTL exposes the configured refresh-token lifetime.
func (s *TokenService) RefreshTTL() time.Duration { return s.refreshTTL }

// IssueAccessToken mints a signed HS256 JWT for the doctor (docs/09 §1.3).
func (s *TokenService) IssueAccessToken(doctorID, role string) (string, error) {
	now := s.now().UTC()
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	payload := jwtPayload{
		Sub:  doctorID,
		Role: role,
		Iat:  now.Unix(),
		Exp:  now.Add(s.accessTTL).Unix(),
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("auth: marshal header: %w", err)
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("auth: marshal payload: %w", err)
	}

	b64 := base64.RawURLEncoding
	signingInput := b64.EncodeToString(hb) + "." + b64.EncodeToString(pb)
	sig := s.sign(signingInput)
	return signingInput + "." + b64.EncodeToString(sig), nil
}

// ParseAccessToken validates signature + expiry and returns the claims.
func (s *TokenService) ParseAccessToken(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrTokenMalformed
	}
	b64 := base64.RawURLEncoding

	// Verify signature first (constant-time) — never trust unverified payload.
	signingInput := parts[0] + "." + parts[1]
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrTokenMalformed
	}
	expected := s.sign(signingInput)
	if subtle.ConstantTimeCompare(sig, expected) != 1 {
		return Claims{}, ErrTokenSignature
	}

	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrTokenMalformed
	}
	var payload jwtPayload
	if err := json.Unmarshal(pb, &payload); err != nil {
		return Claims{}, ErrTokenMalformed
	}

	exp := time.Unix(payload.Exp, 0).UTC()
	if !s.now().UTC().Before(exp) {
		return Claims{}, ErrTokenExpired
	}
	if payload.Sub == "" {
		return Claims{}, ErrTokenMalformed
	}

	return Claims{
		DoctorID: payload.Sub,
		Role:     payload.Role,
		IssuedAt: time.Unix(payload.Iat, 0).UTC(),
		Expires:  exp,
	}, nil
}

// sign computes HMAC-SHA256 over the signing input with the service secret.
func (s *TokenService) sign(signingInput string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

// ─── Opaque refresh tokens (docs/09 §1.3) ────────────────────────────────────

// GenerateRefreshToken returns a cryptographically-random opaque token (the
// value handed to the client) и его SHA-256-хэш (то, что хранится в session).
// Сам токен в БД НЕ попадает — только хэш (docs/05 §2.2, docs/09 §1.3).
func GenerateRefreshToken() (token string, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("auth: read refresh entropy: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	hash = HashRefreshToken(token)
	return token, hash, nil
}

// HashRefreshToken returns the hex SHA-256 of an opaque refresh token. Used both
// when persisting (Create) and when looking a token up (refresh/logout). Это
// детерминированный хэш (не Argon2) — refresh-токен имеет высокую энтропию (256
// бит), поэтому брутфорс по хэшу неосуществим, а быстрый lookup по индексу
// в БД обязателен.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
