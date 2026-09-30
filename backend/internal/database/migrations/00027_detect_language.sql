-- +goose Up
-- The files whose edition has no language are read again, once and after everything else, to tell it from
-- their text (#35). A file that declares its language is not touched by this.
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
WHERE COALESCE(e.language, '') = ''
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
SELECT 1;
