-- ============================================================================
-- История: пакетные записи (parent/child) + тип document_type=batch.
-- parent_request_id: дочерние дневники пакета ссылаются на родительский
-- запрос; при удалении родителя дети удаляются каскадом.
-- ============================================================================

ALTER TABLE generation_request
    ADD COLUMN IF NOT EXISTS parent_request_id UUID
        REFERENCES generation_request(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_genreq_parent_id
    ON generation_request(parent_request_id)
    WHERE parent_request_id IS NOT NULL;

INSERT INTO document_type (code, title, description, is_active, sort_order) VALUES
    ('batch', 'Пакет дневников', 'Агрегированная генерация за период', TRUE, 3)
ON CONFLICT (code) DO NOTHING;
