// Package store — PostgreSQL implementation of the auth-related repository
// methods (docs/05 §2 «doctor», «session»; docs/09 §1–§3).
//
// ПРИВАТНОСТЬ: doctor.password_hash — ТОЛЬКО Argon2id PHC-строка (никогда не
// плейнтекст). session хранит лишь ХЭШ refresh-токена (docs/09 §1.3). Эти данные
// (email/имя врача) — собственные данные врача, не ПДн пациента (docs/05 §2.2).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateDoctor inserts a new doctor. A unique-violation on email maps to
// ErrEmailTaken (→ 409, docs/07 §2).
func (r *PgxRepository) CreateDoctor(ctx context.Context, email, passwordHash, displayName, role string) (string, error) {
	if role == "" {
		role = "doctor"
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO doctor (email, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		email, passwordHash, displayName, role,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return "", ErrEmailTaken
		}
		return "", fmt.Errorf("store: insert doctor: %w", err)
	}
	return id, nil
}

// GetDoctorByEmail fetches a doctor by email (login). ErrNotFound if absent.
func (r *PgxRepository) GetDoctorByEmail(ctx context.Context, email string) (*Doctor, error) {
	return r.scanDoctor(ctx, `
		SELECT id, email, password_hash, display_name, role, is_active,
		       created_at, last_login_at,
		       COALESCE(full_name, ''), COALESCE(position, ''), COALESCE(head_full_name, ''),
		       COALESCE(head_position, ''), COALESCE(head_institution, '')
		FROM doctor WHERE email = $1`, email)
}

// GetDoctorByID fetches a doctor by id (/me). ErrNotFound if absent.
func (r *PgxRepository) GetDoctorByID(ctx context.Context, id string) (*Doctor, error) {
	return r.scanDoctor(ctx, `
		SELECT id, email, password_hash, display_name, role, is_active,
		       created_at, last_login_at,
		       COALESCE(full_name, ''), COALESCE(position, ''), COALESCE(head_full_name, ''),
		       COALESCE(head_position, ''), COALESCE(head_institution, '')
		FROM doctor WHERE id = $1`, id)
}

// scanDoctor runs a single-row doctor query and maps it (or ErrNotFound).
func (r *PgxRepository) scanDoctor(ctx context.Context, query string, arg any) (*Doctor, error) {
	var d Doctor
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&d.ID, &d.Email, &d.PasswordHash, &d.DisplayName, &d.Role,
		&d.IsActive, &d.CreatedAt, &d.LastLoginAt,
		&d.FullName, &d.Position, &d.HeadFullName,
		&d.HeadPosition, &d.HeadInstitution,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get doctor: %w", err)
	}
	return &d, nil
}

// TouchLastLogin updates last_login_at to now (best-effort on login).
func (r *PgxRepository) TouchLastLogin(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE doctor SET last_login_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: touch last_login: %w", err)
	}
	return nil
}

// UpdateDoctorProfile stores signature fields used in the MIS footer.
// Non-empty full_name also becomes display_name (top bar / fallback).
func (r *PgxRepository) UpdateDoctorProfile(ctx context.Context, id string, p SignatureProfile) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE doctor
		SET full_name = $2,
		    position = $3,
		    head_full_name = $4,
		    head_position = $5,
		    head_institution = $6,
		    display_name = CASE WHEN $2 <> '' THEN $2 ELSE display_name END
		WHERE id = $1`,
		id, p.FullName, p.Position, p.HeadFullName, p.HeadPosition, p.HeadInstitution,
	)
	if err != nil {
		return fmt.Errorf("store: update doctor profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSession persists a refresh-token session (hash only) and returns its id.
func (r *PgxRepository) CreateSession(ctx context.Context, doctorID, refreshTokenHash string, expiresAt time.Time) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO session (doctor_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id`,
		doctorID, refreshTokenHash, expiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store: insert session: %w", err)
	}
	return id, nil
}

// GetSessionByHash finds a session by refresh-token hash. Revocation/expiry are
// enforced by the caller (refresh handler) so that the reason can be logged.
func (r *PgxRepository) GetSessionByHash(ctx context.Context, refreshTokenHash string) (*Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx, `
		SELECT id, doctor_id, refresh_token_hash, issued_at, expires_at, revoked
		FROM session WHERE refresh_token_hash = $1`, refreshTokenHash).Scan(
		&s.ID, &s.DoctorID, &s.RefreshTokenHash, &s.IssuedAt, &s.ExpiresAt, &s.Revoked,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get session: %w", err)
	}
	return &s, nil
}

// RevokeSession marks a session revoked (logout / rotation). Idempotent.
func (r *PgxRepository) RevokeSession(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE session SET revoked = TRUE WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: revoke session: %w", err)
	}
	return nil
}
