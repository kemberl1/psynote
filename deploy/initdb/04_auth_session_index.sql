-- ============================================================================
-- AI MED — миграция (Этап 9): аутентификация врачей.
--
-- Источник правды: docs/09_security_privacy.md §1–§3, docs/05 §2.2.
--
-- Таблицы doctor и session уже созданы в 01_schema.sql (каркас Этапа 1).
-- Эта миграция:
--   1) гарантирует наличие doctor/session (на случай очень старого тома);
--   2) добавляет ИНДЕКС по session.refresh_token_hash — горячий путь /refresh и
--      /logout ищет сессию по хэшу токена (docs/09 §1.3); без индекса это
--      seq-scan. UNIQUE: один хэш токена = одна сессия (коллизии SHA-256
--      неосуществимы; UNIQUE также защищает от дублей при ретраях).
--
-- ИДЕМПОТЕНТНОСТЬ: безопасно выполнять повторно (IF NOT EXISTS). На ЧИСТОМ томе
-- 01_schema.sql уже всё создал — здесь до-создаётся только индекс.
--
-- ВАЖНО про docker-entrypoint-initdb.d: init-скрипты выполняются ТОЛЬКО при
-- первом старте с пустым томом. Для уже работающей БД примените миграцию
-- вручную:
--   docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
--     < deploy/initdb/04_auth_session_index.sql
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Страховка: таблицы аутентификации (идентичны 01_schema.sql) на старом томе.
CREATE TABLE IF NOT EXISTS doctor (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'doctor',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS session (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id          UUID NOT NULL REFERENCES doctor(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    user_agent_hash    TEXT,
    issued_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked            BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_session_doctor_id ON session(doctor_id);

-- Горячий lookup по хэшу refresh-токена (refresh/logout), UNIQUE на хэш.
CREATE UNIQUE INDEX IF NOT EXISTS uq_session_refresh_token_hash
    ON session(refresh_token_hash);
