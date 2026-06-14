-- ============================================================================
-- AI MED — миграция (Этап 5): doctor_id в generation_request → NULLABLE.
--
-- ЗАЧЕМ: до Этапа 9 (аутентификация) gateway-оркестратор сохраняет
-- ОБЕЗЛИЧЕННУЮ историю генераций БЕЗ привязки к врачу (doctor_id = NULL).
-- Место под scoping по врачу заложено (FK сохранён). См. docs/05 §2, docs/10.
--
-- ИДЕМПОТЕНТНОСТЬ: скрипт безопасно выполнять повторно. На ЧИСТОМ томе
-- 01_schema.sql уже создаёт колонку nullable — тогда ALTER ниже ничего не
-- меняет. На СУЩЕСТВУЮЩЕМ томе (БД, созданная до Этапа 5) этот скрипт снимает
-- NOT NULL и переключает FK на ON DELETE SET NULL.
--
-- ВАЖНО про docker-entrypoint-initdb.d: init-скрипты выполняются ТОЛЬКО при
-- первом старте с пустым томом. Для уже работающей БД примените миграцию
-- вручную (см. docs e2e-приёмки / README), например:
--   docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
--     < deploy/initdb/03_migration_doctor_nullable.sql
-- ============================================================================

DO $$
BEGIN
    -- Снять NOT NULL, если он ещё стоит (идемпотентно).
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'generation_request'
          AND column_name = 'doctor_id'
          AND is_nullable = 'NO'
    ) THEN
        ALTER TABLE generation_request ALTER COLUMN doctor_id DROP NOT NULL;
    END IF;
END $$;

-- Переключить FK на ON DELETE SET NULL (идемпотентно: пересоздаём именованный
-- constraint). Имя constraint по умолчанию: generation_request_doctor_id_fkey.
ALTER TABLE generation_request
    DROP CONSTRAINT IF EXISTS generation_request_doctor_id_fkey;
ALTER TABLE generation_request
    ADD CONSTRAINT generation_request_doctor_id_fkey
    FOREIGN KEY (doctor_id) REFERENCES doctor(id) ON DELETE SET NULL;
