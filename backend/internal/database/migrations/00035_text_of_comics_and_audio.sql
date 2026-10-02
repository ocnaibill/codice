-- +goose Up
-- A comic or an audio file now has text: what its metadata says (the ComicInfo.xml, the chapters, the description),
-- read as segments (#25). The files of this kind that are already here are read again, once and after everything
-- else. The worker reads again only the files of these formats that were read before this version, so the other files
-- of a work (its EPUB, its PDF) are not touched by this.
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
WHERE lower(f.format) IN ('cbz', 'cbr', 'mp3', 'm4a', 'm4b', 'ogg', 'wav', 'flac')
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
SELECT 1;
