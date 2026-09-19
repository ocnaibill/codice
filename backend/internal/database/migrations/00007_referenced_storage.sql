-- +goose Up
-- Referenced library (DEC-032, DEC-035): files that stay where they are, in
-- directories the owner has authorised. The catalog only points at them.
CREATE TABLE storage_roots (
	id SERIAL PRIMARY KEY,
	path TEXT NOT NULL UNIQUE,
	created_by UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The view now also says how the primary file is stored, so a caller can build
-- its absolute path: managed files are relative to the storage directory,
-- referenced files to their root. Columns are only appended.
CREATE OR REPLACE VIEW work_primary AS
SELECT w.id AS work_id,
       e.id AS edition_id,
       e.cover_url,
       f.id AS file_id,
       f.format AS file_format,
       l.path AS file_path,
       l.mode AS file_mode,
       l.root AS file_root,
       l.state AS file_state
FROM works w
LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
LEFT JOIN LATERAL (
	SELECT id, format FROM files WHERE edition_id = e.id ORDER BY id LIMIT 1
) f ON TRUE
LEFT JOIN LATERAL (
	SELECT path, mode, root, state FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
) l ON TRUE;

-- +goose Down
DROP VIEW work_primary;
CREATE VIEW work_primary AS
SELECT w.id AS work_id,
       e.id AS edition_id,
       e.cover_url,
       f.id AS file_id,
       f.format AS file_format,
       l.path AS file_path
FROM works w
LEFT JOIN editions e ON e.work_id = w.id AND e.is_primary
LEFT JOIN LATERAL (
	SELECT id, format FROM files WHERE edition_id = e.id ORDER BY id LIMIT 1
) f ON TRUE
LEFT JOIN LATERAL (
	SELECT path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1
) l ON TRUE;
DROP TABLE IF EXISTS storage_roots;
