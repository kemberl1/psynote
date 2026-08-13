-- Подпись врача и заведующего в бланке МИС (настройки аккаунта).
-- Идемпотентно: IF NOT EXISTS. На существующем томе те же DDL
-- выполняет gateway при старте (store.EnsureDoctorProfileSchema).

ALTER TABLE doctor ADD COLUMN IF NOT EXISTS full_name TEXT NOT NULL DEFAULT '';
ALTER TABLE doctor ADD COLUMN IF NOT EXISTS position TEXT NOT NULL DEFAULT '';
ALTER TABLE doctor ADD COLUMN IF NOT EXISTS head_full_name TEXT NOT NULL DEFAULT '';
ALTER TABLE doctor ADD COLUMN IF NOT EXISTS head_position TEXT NOT NULL DEFAULT '';
ALTER TABLE doctor ADD COLUMN IF NOT EXISTS head_institution TEXT NOT NULL DEFAULT '';
