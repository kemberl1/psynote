-- Подпись типа batch в справочнике: «Период», не «Пакет».
-- init-скрипты docker-entrypoint выполняются только при первом создании volume.
-- На уже живой БД: psql "$DATABASE_URL" < deploy/initdb/09_period_label.sql
UPDATE document_type
SET title = 'Период дневников'
WHERE code = 'batch' AND title <> 'Период дневников';
