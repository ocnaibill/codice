-- +goose Up
-- Contract step of the model change started in 00002. Every writer now records
-- editions, files, locations and contributors directly, so the compatibility
-- triggers and the legacy columns of `works` are removed. Descriptive facts that
-- belong to an edition live on the edition; the work keeps what is true of the
-- work itself (title, series, description, locks, status).

-- 1. Safety backfill. The triggers kept the new tables in step, so this normally
--    changes nothing; it makes sure no legacy value is dropped without a home.
UPDATE editions e
SET language = COALESCE(e.language, w.language),
    publisher = COALESCE(e.publisher, w.publisher),
    publication_date = COALESCE(e.publication_date, w.publication_date),
    isbn = COALESCE(e.isbn, w.isbn)
FROM works w WHERE w.id = e.work_id AND e.is_primary;

INSERT INTO work_contributors (work_id, person_id, role, position)
SELECT id, author_id, 'author', 0 FROM works WHERE author_id IS NOT NULL
ON CONFLICT DO NOTHING;

INSERT INTO files (edition_id, format)
SELECT e.id, NULLIF(w.format, '')
FROM works w JOIN editions e ON e.work_id = w.id AND e.is_primary
WHERE COALESCE(w.file_path, '') <> ''
  AND NOT EXISTS (SELECT 1 FROM files f WHERE f.edition_id = e.id);

INSERT INTO storage_locations (file_id, mode, path)
SELECT f.id, 'managed', w.file_path
FROM works w JOIN editions e ON e.work_id = w.id AND e.is_primary
JOIN files f ON f.id = (SELECT id FROM files WHERE edition_id = e.id ORDER BY id LIMIT 1)
WHERE COALESCE(w.file_path, '') <> ''
  AND NOT EXISTS (SELECT 1 FROM storage_locations l WHERE l.file_id = f.id);

-- 2. The notes trigger read the author from the legacy column.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notes_fill_source() RETURNS trigger AS $$
BEGIN
	IF NEW.work_id IS NOT NULL THEN
		NEW.source_work_id := COALESCE(NEW.source_work_id, NEW.work_id);
		IF NEW.source_title IS NULL THEN
			SELECT w.original_title,
			       NULLIF((SELECT p.name FROM work_contributors c JOIN person p ON p.id = c.person_id
			               WHERE c.work_id = w.id AND c.role = 'author'
			               ORDER BY c.position, p.name LIMIT 1), 'Unknown Author')
			INTO NEW.source_title, NEW.source_author
			FROM works w WHERE w.id = NEW.work_id;
		END IF;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- 3. Remove the compatibility triggers and the legacy columns.
DROP TRIGGER IF EXISTS works_project_insert ON works;
DROP TRIGGER IF EXISTS works_project_update ON works;
DROP FUNCTION IF EXISTS works_project_insert();
DROP FUNCTION IF EXISTS works_project_update();

ALTER TABLE works DROP COLUMN file_path;
ALTER TABLE works DROP COLUMN format;
ALTER TABLE works DROP COLUMN language;
ALTER TABLE works DROP COLUMN publisher;
ALTER TABLE works DROP COLUMN publication_date;
ALTER TABLE works DROP COLUMN isbn;
ALTER TABLE works DROP COLUMN author_id;

-- +goose Down
-- Restores the legacy columns from the new tables and recreates the triggers.
-- With several editions or files per work only the primary ones are copied back.
ALTER TABLE works ADD COLUMN file_path VARCHAR(255);
ALTER TABLE works ADD COLUMN format VARCHAR(16);
ALTER TABLE works ADD COLUMN language VARCHAR(16);
ALTER TABLE works ADD COLUMN publisher VARCHAR(256);
ALTER TABLE works ADD COLUMN publication_date VARCHAR(32);
ALTER TABLE works ADD COLUMN isbn VARCHAR(64);
ALTER TABLE works ADD COLUMN author_id INTEGER REFERENCES person(id) ON DELETE SET NULL;

UPDATE works w
SET language = e.language, publisher = e.publisher, publication_date = e.publication_date, isbn = e.isbn
FROM editions e WHERE e.work_id = w.id AND e.is_primary;

UPDATE works w
SET file_path = wp.file_path, format = wp.file_format
FROM work_primary wp WHERE wp.work_id = w.id;

UPDATE works w
SET author_id = (SELECT c.person_id FROM work_contributors c
                 WHERE c.work_id = w.id AND c.role = 'author' ORDER BY c.position, c.person_id LIMIT 1);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notes_fill_source() RETURNS trigger AS $$
BEGIN
	IF NEW.work_id IS NOT NULL THEN
		NEW.source_work_id := COALESCE(NEW.source_work_id, NEW.work_id);
		IF NEW.source_title IS NULL THEN
			SELECT w.original_title, NULLIF(p.name, 'Unknown Author')
			INTO NEW.source_title, NEW.source_author
			FROM works w LEFT JOIN person p ON p.id = w.author_id
			WHERE w.id = NEW.work_id;
		END IF;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION works_project_insert() RETURNS trigger AS $$
DECLARE
	ed_id INTEGER;
	file_id BIGINT;
BEGIN
	INSERT INTO editions (work_id, title, language, publisher, publication_date, isbn, is_primary)
	VALUES (NEW.id, NEW.original_title, NEW.language, NEW.publisher, NEW.publication_date, NEW.isbn, TRUE)
	RETURNING id INTO ed_id;

	IF COALESCE(NEW.file_path, '') <> '' THEN
		INSERT INTO files (edition_id, format) VALUES (ed_id, NULLIF(NEW.format, ''))
		RETURNING id INTO file_id;
		INSERT INTO storage_locations (file_id, mode, path) VALUES (file_id, 'managed', NEW.file_path);
	END IF;

	IF NEW.author_id IS NOT NULL THEN
		INSERT INTO work_contributors (work_id, person_id, role, position)
		VALUES (NEW.id, NEW.author_id, 'author', 0);
	END IF;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER works_project_insert AFTER INSERT ON works
FOR EACH ROW EXECUTE FUNCTION works_project_insert();

-- +goose StatementBegin
CREATE FUNCTION works_project_update() RETURNS trigger AS $$
DECLARE
	ed_id INTEGER;
	primary_file BIGINT;
BEGIN
	SELECT id INTO ed_id FROM editions WHERE work_id = NEW.id AND is_primary;
	IF ed_id IS NULL THEN
		RETURN NULL;
	END IF;

	UPDATE editions
	SET language = NEW.language, publisher = NEW.publisher,
	    publication_date = NEW.publication_date, isbn = NEW.isbn
	WHERE id = ed_id;

	SELECT id INTO primary_file FROM files WHERE edition_id = ed_id ORDER BY id LIMIT 1;

	IF COALESCE(NEW.file_path, '') <> '' THEN
		IF primary_file IS NULL THEN
			INSERT INTO files (edition_id, format) VALUES (ed_id, NULLIF(NEW.format, ''))
			RETURNING id INTO primary_file;
			INSERT INTO storage_locations (file_id, mode, path) VALUES (primary_file, 'managed', NEW.file_path);
		ELSIF NEW.file_path IS DISTINCT FROM OLD.file_path THEN
			UPDATE storage_locations SET path = NEW.file_path
			WHERE id = (SELECT id FROM storage_locations WHERE file_id = primary_file ORDER BY id LIMIT 1);
		END IF;
	END IF;
	IF primary_file IS NOT NULL THEN
		UPDATE files SET format = NULLIF(NEW.format, '') WHERE id = primary_file;
	END IF;

	IF NEW.author_id IS DISTINCT FROM OLD.author_id THEN
		DELETE FROM work_contributors WHERE work_id = NEW.id AND role = 'author' AND position = 0;
		IF NEW.author_id IS NOT NULL THEN
			INSERT INTO work_contributors (work_id, person_id, role, position)
			VALUES (NEW.id, NEW.author_id, 'author', 0)
			ON CONFLICT DO NOTHING;
		END IF;
	END IF;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER works_project_update AFTER UPDATE OF
	language, publisher, publication_date, isbn, format, file_path, author_id ON works
FOR EACH ROW EXECUTE FUNCTION works_project_update();
