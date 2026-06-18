// Package store is the gateway's persistence layer for the ANONYMIZED request
// history (docs/05 — No-PII by design).
//
// ГЛАВНЫЙ ПРИНЦИП ПРИВАТНОСТИ (docs/05 §1, docs/09): в PostgreSQL попадают
// ТОЛЬКО обезличенные данные. Этот слой физически не принимает сырой ввод —
// его API оперирует уже-обезличенными answers (answers_anonymized из RAG),
// title_safe и content_anonymized с плейсхолдерами. Никаких ФИО/дат/адресов.
//
// Архитектура: Repository — интерфейс (для юнит-тестов handlers через фейк),
// PgxRepository — реализация на pgxpool (database/sql не используется — pgx
// идиоматичнее и быстрее для Postgres, docs/05 §2). Запись генерации
// атомарна (transaction: generation_request + generated_document).
package store

import (
	"context"
	"errors"
	"time"
)

// GenerationRecord is the ANONYMIZED record to persist after a successful
// generation (docs/05 §2.2). Every field here is already PII-free.
type GenerationRecord struct {
	// DoctorID — владелец записи. Nil до появления аутентификации (Этап 9):
	// MVP пишет NULL (схема допускает nullable doctor_id), место под scoping
	// заложено (docs/05 §2 «Изоляция по врачу»).
	DoctorID *string
	// DocumentType — код типа документа (daily | exam_10d), FK document_type.
	DocumentType string
	// AnswersAnonymized — ответы опросника ПОСЛЕ анонимизации (из RAG).
	AnswersAnonymized map[string]any
	// TitleSafe — безопасный заголовок истории (без ПДн).
	TitleSafe string
	// LLMModelUsed — какая модель X5 сгенерировала документ.
	LLMModelUsed string
	// Status — статус запроса (docs/05 §2.4), для MVP обычно "done".
	Status string
	// AnonymizerRemovedCount — счётчик удалённых ПДн (аудит, без значений).
	AnonymizerRemovedCount int
	// ContentAnonymized — сгенерированный текст с плейсхолдерами.
	ContentAnonymized string
	// TokensUsed — токены LLM (метаданное генерации).
	TokensUsed int
}

// HistoryItem is one row of the history list (docs/07 §6, GET /requests).
type HistoryItem struct {
	RequestID    string `json:"request_id"`
	DocumentType string `json:"document_type"`
	TitleSafe    string `json:"title_safe"`
	LLMModelUsed string `json:"llm_model_used"`
	// Status — статус запроса (docs/05 §2.4). Раньше колонка не выбиралась из БД,
	// поэтому поле приходило пустым/null на фронт — БАГ приёмки, теперь маппится.
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// HistoryDetail is the full anonymized record (docs/07 §6, GET /requests/{id}).
type HistoryDetail struct {
	RequestID         string         `json:"request_id"`
	DocumentType      string         `json:"document_type"`
	AnswersAnonymized map[string]any `json:"answers_anonymized"`
	Content           string         `json:"content"`
	TitleSafe         string         `json:"title_safe"`
	LLMModelUsed      string         `json:"llm_model_used"`
	Status            string         `json:"status"`
	// AnonymizerRemovedCount — счётчик удалённых ПДн (аудит, без значений).
	// Раньше колонка не выбиралась из БД, поэтому поле приходило null на фронт —
	// БАГ приёмки, теперь выбирается и маппится.
	AnonymizerRemovedCount int       `json:"anonymizer_removed_count"`
	CreatedAt              time.Time `json:"created_at"`
}

// ListFilter scopes / paginates the history list (docs/07 §6).
type ListFilter struct {
	// DoctorID, when non-nil, restricts to one owner (Этап 9 auth). Nil = all.
	DoctorID *string
	Limit    int
	Offset   int
}

// Doctor is a registered doctor account (docs/05 §2.2 «doctor»). password_hash
// is ALWAYS an Argon2id PHC string (docs/09 §2) — никогда не плейнтекст.
type Doctor struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
	IsActive     bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// Session is a refresh-token record (docs/05 §2.2 «session», docs/09 §1.3).
// RefreshTokenHash хранит ХЭШ opaque-токена (SHA-256 hex), не сам токен.
type Session struct {
	ID               string
	DoctorID         string
	RefreshTokenHash string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	Revoked          bool
}

// ErrEmailTaken is returned by CreateDoctor on a duplicate email (→ 409, docs/07).
var ErrEmailTaken = errors.New("store: email already registered")

// Repository abstracts persistence so handlers can be unit-tested with a fake
// (no real DB in unit tests, см. задание). docs/05 §2.
type Repository interface {
	// SaveGeneration persists an anonymized generation atomically and returns
	// the new request_id.
	SaveGeneration(ctx context.Context, rec GenerationRecord) (string, error)
	// ListGenerations returns the anonymized history list + total count.
	ListGenerations(ctx context.Context, f ListFilter) ([]HistoryItem, int, error)
	// GetGeneration returns one full anonymized record by id (scoped by doctor
	// when filter.DoctorID set). Returns ErrNotFound when absent.
	GetGeneration(ctx context.Context, id string, doctorID *string) (*HistoryDetail, error)

	// ─── Auth (Этап 9, docs/09) ──────────────────────────────────────────────

	// CreateDoctor inserts a new doctor and returns the new id. Returns
	// ErrEmailTaken when email уже существует (unique violation → 409).
	CreateDoctor(ctx context.Context, email, passwordHash, displayName, role string) (string, error)
	// GetDoctorByEmail looks a doctor up by email (login). ErrNotFound if absent.
	GetDoctorByEmail(ctx context.Context, email string) (*Doctor, error)
	// GetDoctorByID looks a doctor up by id (/me). ErrNotFound if absent.
	GetDoctorByID(ctx context.Context, id string) (*Doctor, error)
	// TouchLastLogin updates doctor.last_login_at to now (best-effort on login).
	TouchLastLogin(ctx context.Context, id string) error

	// CreateSession persists a refresh-token session (hash only) and returns id.
	CreateSession(ctx context.Context, doctorID, refreshTokenHash string, expiresAt time.Time) (string, error)
	// GetSessionByHash finds an active (non-revoked, non-expired enforced by
	// caller) session by refresh-token hash. ErrNotFound if absent.
	GetSessionByHash(ctx context.Context, refreshTokenHash string) (*Session, error)
	// RevokeSession marks a session revoked by id (logout / rotation). Idempotent.
	RevokeSession(ctx context.Context, id string) error
}
