-- ============================================================================
-- Вложения в чате поддержки: картинки и файлы к сообщениям.
--
-- ИДЕМПОТЕНТНОСТЬ: IF NOT EXISTS. Безопасно на пустом томе и при повторном
-- прогоне. На уже существующем томе те же DDL выполняет gateway при старте
-- (store.EnsureSupportFeedbackSchema).
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Содержимое лежит прямо в BYTEA: файлов мало и они небольшие (≤ 10 МБ),
-- отдельное объектное хранилище ради этого не заводим.
CREATE TABLE IF NOT EXISTS support_attachment (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id   UUID NOT NULL REFERENCES support_message(id) ON DELETE CASCADE,
    thread_id    UUID NOT NULL REFERENCES support_thread(id) ON DELETE CASCADE,
    filename     TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL,
    data         BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_support_attachment_message
    ON support_attachment(message_id);
CREATE INDEX IF NOT EXISTS idx_support_attachment_thread
    ON support_attachment(thread_id);
