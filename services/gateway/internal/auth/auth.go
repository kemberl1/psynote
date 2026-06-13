// Package auth handles doctor authentication (JWT access + opaque refresh).
//
// See docs/09_security_privacy.md §1 (JWT + Argon2id) and docs/05_data_model.md
// (таблицы doctor/session). Изоляция данных по doctor_id обязательна.
//
// Этап 1 (каркас): только интерфейсы и заглушки. Реальная реализация
// (Argon2id-хэши, выпуск/валидация JWT, ротация refresh) — Этап 5 роадмапа.
package auth

import "context"

// Claims carries the verified identity extracted from an access token.
type Claims struct {
	DoctorID string
	Role     string
}

// Service issues and verifies authentication tokens.
type Service interface {
	// HashPassword hashes a plaintext password with Argon2id.
	HashPassword(ctx context.Context, plaintext string) (string, error)
	// VerifyPassword checks a plaintext password against an Argon2id hash.
	VerifyPassword(ctx context.Context, hash, plaintext string) (bool, error)
	// IssueAccessToken mints a short-lived signed JWT for the doctor.
	IssueAccessToken(ctx context.Context, doctorID, role string) (string, error)
	// ParseAccessToken validates a JWT and returns its claims.
	ParseAccessToken(ctx context.Context, token string) (Claims, error)
}

// Stub is a no-op placeholder.
type Stub struct{}

// HashPassword is a placeholder. TODO(этап 5): Argon2id.
func (Stub) HashPassword(_ context.Context, _ string) (string, error) { return "", nil }

// VerifyPassword is a placeholder. TODO(этап 5): Argon2id verify.
func (Stub) VerifyPassword(_ context.Context, _, _ string) (bool, error) { return false, nil }

// IssueAccessToken is a placeholder. TODO(этап 5): подпись JWT (HS256).
func (Stub) IssueAccessToken(_ context.Context, _, _ string) (string, error) { return "", nil }

// ParseAccessToken is a placeholder. TODO(этап 5): валидация подписи и exp.
func (Stub) ParseAccessToken(_ context.Context, _ string) (Claims, error) { return Claims{}, nil }
