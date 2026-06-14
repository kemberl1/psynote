-- ============================================================================
-- AI MED — PostgreSQL schema (Этап 1, каркас).
--
-- Источник правды: docs/05_data_model.md.
-- ГЛАВНЫЙ ПРИНЦИП (No-PII by design, docs/05 §1): в БД НЕТ персональных данных
-- пациентов. Тексты, привязанные к истории, хранятся уже ОБЕЗЛИЧЕННЫМИ
-- (после гейта анонимизации, docs/04). Все пользовательские данные изолируются
-- по doctor_id (docs/09 §3).
--
-- Этот скрипт выполняется автоматически при первом старте контейнера postgres
-- (docker-entrypoint-initdb.d). Идемпотентность обеспечена IF NOT EXISTS.
--
-- NB: на этапах 2+ миграции переедут в версионируемый инструмент
-- (golang-migrate, one-shot контейнер `migrations` — docs/05 §5). Здесь —
-- стартовая схема для каркаса.
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- для gen_random_uuid()

-- ─── doctor ────────────────────────────────────────────────────────────────
-- Аккаунты врачей. password_hash — ТОЛЬКО Argon2id (docs/09 §2).
-- display_name — собственное имя врача (не пациента), это допустимо (docs/05 §2.2).
CREATE TABLE IF NOT EXISTS doctor (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'doctor',  -- 'doctor' | 'admin'
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

-- ─── session ───────────────────────────────────────────────────────────────
-- Refresh-токены/сессии для JWT-аутентификации (docs/09 §1.3).
-- Хранится ХЭШ токена, не сам токен. user_agent_hash — без сырых данных.
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

-- ─── document_type ───────────────────────────────────────────────────────────
-- Справочник типов документов (docs/05 §2.2). Добавление нового типа
-- не меняет схему (расширяемость, docs/05 §1.3).
CREATE TABLE IF NOT EXISTS document_type (
    code        TEXT PRIMARY KEY,            -- 'daily' | 'exam_10d' | ...
    title       TEXT NOT NULL,
    description TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  INTEGER NOT NULL DEFAULT 0
);

-- ─── questionnaire_schema ────────────────────────────────────────────────────
-- Версионируемая JSON-схема опросника per тип документа (docs/06).
CREATE TABLE IF NOT EXISTS questionnaire_schema (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_type_code TEXT NOT NULL REFERENCES document_type(code),
    version            INTEGER NOT NULL,
    schema_json        JSONB NOT NULL,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_type_code, version)
);

-- ─── generation_request ──────────────────────────────────────────────────────
-- Один запрос врача. answers_anonymized — ответы опросника ПОСЛЕ анонимизации.
-- title_safe — безопасный заголовок для истории. ТОЛЬКО обезличенные данные.
-- (docs/05 §2.2, статусы — docs/05 §2.4).
-- doctor_id NULLABLE до Этапа 9 (аутентификация): на MVP запросы пишутся без
-- привязки к врачу (NULL). Место под scoping заложено (FK сохранён, NULL
-- допускается). После ввода auth поле станет обязательным (см. docs/05 §2,
-- docs/10 Этап 9). ON DELETE SET NULL: удаление врача не теряет обезличенную
-- историю (в ней нет ПДн, docs/05 §2.3).
CREATE TABLE IF NOT EXISTS generation_request (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    doctor_id                UUID REFERENCES doctor(id) ON DELETE SET NULL,
    document_type_code       TEXT NOT NULL REFERENCES document_type(code),
    answers_anonymized       JSONB NOT NULL DEFAULT '{}'::jsonb,
    title_safe               TEXT,
    llm_model_used           TEXT,
    status                   TEXT NOT NULL DEFAULT 'pending',
        -- pending → anonymizing → blocked_pii → retrieving → generating → done → failed
    anonymizer_removed_count INTEGER NOT NULL DEFAULT 0,  -- аудит без значений ПДн
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_genreq_doctor_id ON generation_request(doctor_id);
CREATE INDEX IF NOT EXISTS idx_genreq_created_at ON generation_request(created_at DESC);

-- ─── generated_document ──────────────────────────────────────────────────────
-- Результат генерации. content_anonymized — текст с плейсхолдерами (без ПДн).
-- Реальные даты/ФИО врач подставляет на клиенте при экспорте (docs/05 §2.3).
CREATE TABLE IF NOT EXISTS generated_document (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id         UUID NOT NULL UNIQUE REFERENCES generation_request(id) ON DELETE CASCADE,
    content_anonymized TEXT NOT NULL,
    tokens_used        INTEGER,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ─── attached_document ───────────────────────────────────────────────────────
-- Метаданные приложенных файлов. Сам обезличенный текст идёт в Qdrant;
-- здесь — только метаданные/факт ingestion. Имя файла тоже обезличено (docs/05 §2.2).
CREATE TABLE IF NOT EXISTS attached_document (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id               UUID NOT NULL REFERENCES generation_request(id) ON DELETE CASCADE,
    original_filename_safe   TEXT,
    ingested_to_vector_db    BOOLEAN NOT NULL DEFAULT FALSE,
    vector_collection        TEXT,
    anonymizer_removed_count INTEGER NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_attached_request_id ON attached_document(request_id);
