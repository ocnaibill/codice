-- +goose Up
-- An EPUB now says, through the epub:type of its documents and the landmarks of its navigation document, which of its
-- parts are the front matter, the story and the back matter (#40), where it used to be guessed from the titles. The
-- EPUBs that are already here are read again, once and after everything else. The worker reads again only the files of
-- this format that were read before this version, so the other files of a work (its PDF, its comic) are not touched.
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
WHERE lower(f.format) = 'epub'
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
SELECT 1;
