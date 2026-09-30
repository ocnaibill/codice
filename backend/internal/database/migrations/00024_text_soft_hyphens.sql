-- +goose Up
-- The text is read again, once, after everything else, without the soft hyphens that a book puts inside
-- its words (#33): searching "retirou-se" found nothing in a text that had "retirou­-se".
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
JOIN text_extractions te ON te.file_id = f.id
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
SELECT 1;
