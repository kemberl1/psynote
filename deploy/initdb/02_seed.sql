-- ============================================================================
-- AI MED — seed данных справочников (Этап 1, каркас).
--
-- Только КОНФИГ-справочники (без ПДн). Типы документов MVP — docs/05 §2.2.
-- Идемпотентно: ON CONFLICT DO NOTHING.
-- ============================================================================

INSERT INTO document_type (code, title, description, is_active, sort_order) VALUES
    ('daily',    'Ежедневный дневник',     'Ежедневная запись психического статуса', TRUE, 1),
    ('exam_10d', 'Осмотр (раз в 10 дней)', 'Расширенный осмотр со структурными секциями', TRUE, 2)
ON CONFLICT (code) DO NOTHING;

-- TODO(этап 8): доп. типы — primary_exam, anamnesis, discharge_epicrisis (docs/05 §2.2).
-- TODO(этап 4): наполнение questionnaire_schema (docs/06).
