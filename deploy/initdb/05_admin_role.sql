-- ============================================================================
-- AI MED — миграция (Этап 10): admin-роль + таблица загруженных документов.
--
-- Источник: docs/05_data_model.md, docs/07 §8 (Admin API).
--
-- 1) Таблица admin_document — метаданные загруженных через админку документов.
--    Поле doctor.role уже существует (см. 01_schema.sql); здесь — страховка
--    проверки DEFAULT 'doctor' и UNIQUE INDEX на doctor(role) для seed'а.
--
-- 2) Seed admin-пользователя (если не существует) — для первоначальной
--    настройки. Пароль = 'admin123456' (Argon2id), сменить на проде!
--
-- ИДЕМПОТЕНТНОСТЬ: безопасно выполнять повторно (IF NOT EXISTS / ON CONFLICT).
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Страховка: гарантируем наличие колонки role (на случай старого тома).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'doctor' AND column_name = 'role'
    ) THEN
        ALTER TABLE doctor ADD COLUMN role TEXT NOT NULL DEFAULT 'doctor';
    END IF;
END $$;

-- ─── admin_document ─────────────────────────────────────────────────────
-- Метаданные загруженных через UI документов (оригинал НЕ хранится — docs/09).
CREATE TABLE IF NOT EXISTS admin_document (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploaded_by              UUID REFERENCES doctor(id) ON DELETE SET NULL,
    original_filename        TEXT NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'processing',
        -- processing → ingested / failed / pii_blocked
    anonymizer_removed_count INTEGER NOT NULL DEFAULT 0,
    removed_by_type          JSONB NOT NULL DEFAULT '{}'::jsonb,
    chunks_count             INTEGER NOT NULL DEFAULT 0,
    qdrant_ids               TEXT[] NOT NULL DEFAULT '{}',
    error_message            TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_admin_doc_uploaded_by ON admin_document(uploaded_by);
CREATE INDEX IF NOT EXISTS idx_admin_doc_created_at ON admin_document(created_at DESC);

-- ─── Seed admin-пользователя ────────────────────────────────────────────
-- Логин: admin@aimed.local / admin123456 (Argon2id, сменить!).
-- id стараемся стабильным (ON CONFLICT DO NOTHING — безопасно).
-- Argon2id PHC hash for 'admin123456' (OWASP defaults: m=64MB, t=3, p=2);
-- generated via gateway's HashPassword() function.
INSERT INTO doctor (id, email, password_hash, display_name, role)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'admin@aimed.local',
    '$argon2id$v=19$m=65536,t=3,p=2$HtzuQ/mZ85WI7HD3JjZ1zw$BwJemnrP0lVhTkNAQ09JPfJ40N40MIkg5zajaZaq24I',
    'Администратор',
    'admin'
)
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash;
