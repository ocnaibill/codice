-- +goose Up
-- Target data model, step "expand" (spec sections 5.1 and 5.4, plan phase 2).
--
-- Nothing is dropped or rewritten: the legacy columns of `works` and the table
-- `user_progress` stay, so the worker and older code keep working. Existing data
-- is copied into the new tables. Triggers keep the new tables in step with the
-- legacy columns the worker still writes; they are removed in phase 3, once the
-- worker writes the new model directly.

-- 1. Works: reversible retirement from the catalog (DEC-038). Retiring hides a
-- work; files and notes stay.
ALTER TABLE works ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ;
ALTER TABLE works ADD COLUMN IF NOT EXISTS retired_by UUID REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_works_retired ON works(retired_at) WHERE retired_at IS NOT NULL;

-- 2. Editions: several per work (RF-005). Exactly one is the primary edition,
-- the one the legacy columns and covers describe.
ALTER TABLE editions DROP CONSTRAINT IF EXISTS editions_work_id_key;
-- NOT VALID: applies to new rows without failing on odd legacy data.
ALTER TABLE editions ADD CONSTRAINT editions_work_required CHECK (work_id IS NOT NULL) NOT VALID;
ALTER TABLE editions ADD COLUMN language VARCHAR(16);
ALTER TABLE editions ADD COLUMN publisher VARCHAR(256);
ALTER TABLE editions ADD COLUMN publication_date VARCHAR(32);
ALTER TABLE editions ADD COLUMN isbn VARCHAR(64);
ALTER TABLE editions ADD COLUMN is_primary BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE editions ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE UNIQUE INDEX editions_one_primary_per_work ON editions(work_id) WHERE is_primary;

-- Works that never got a cover have no edition row yet.
INSERT INTO editions (work_id, title)
SELECT w.id, w.original_title FROM works w
WHERE NOT EXISTS (SELECT 1 FROM editions e WHERE e.work_id = w.id);

UPDATE editions e
SET language = w.language, publisher = w.publisher, publication_date = w.publication_date, isbn = w.isbn
FROM works w WHERE w.id = e.work_id;

-- 3. Files: one digital manifestation of an edition. Identical bytes are one
-- stored file (DEC-027): the hash is unique when known.
CREATE TABLE files (
	id BIGSERIAL PRIMARY KEY,
	edition_id INTEGER NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
	format VARCHAR(16),
	size_bytes BIGINT,
	sha256 CHAR(64),
	version INTEGER NOT NULL DEFAULT 1,
	availability VARCHAR(16) NOT NULL DEFAULT 'available'
		CHECK (availability IN ('available', 'missing', 'blocked')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_files_edition_id ON files(edition_id);
CREATE UNIQUE INDEX files_sha256_key ON files(sha256) WHERE sha256 IS NOT NULL;

-- 4. Storage locations: where the bytes live. A path is a location, never an
-- identity (DEC-036).
CREATE TABLE storage_locations (
	id BIGSERIAL PRIMARY KEY,
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	mode VARCHAR(12) NOT NULL CHECK (mode IN ('managed', 'referenced')),
	root TEXT,
	path TEXT NOT NULL,
	state VARCHAR(16) NOT NULL DEFAULT 'ok' CHECK (state IN ('ok', 'missing', 'trashed')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_storage_locations_file_id ON storage_locations(file_id);
CREATE UNIQUE INDEX storage_locations_path_key ON storage_locations(mode, COALESCE(root, ''), path);

-- Each legacy work with a file becomes one file in its primary edition.
INSERT INTO files (edition_id, format)
SELECT e.id, NULLIF(w.format, '')
FROM works w JOIN editions e ON e.work_id = w.id AND e.is_primary
WHERE COALESCE(w.file_path, '') <> '';

INSERT INTO storage_locations (file_id, mode, path)
SELECT f.id, 'managed', w.file_path
FROM works w
JOIN editions e ON e.work_id = w.id AND e.is_primary
JOIN files f ON f.edition_id = e.id
WHERE COALESCE(w.file_path, '') <> '';

-- 5. Contributors: many per work, each with a role (RF-005). The existing table
-- `person` is the contributor table.
CREATE TABLE work_contributors (
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	person_id INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE,
	role VARCHAR(16) NOT NULL CHECK (role IN ('author', 'translator', 'narrator', 'editor', 'illustrator')),
	position SMALLINT NOT NULL DEFAULT 0,
	PRIMARY KEY (work_id, person_id, role)
);
CREATE INDEX idx_work_contributors_person ON work_contributors(person_id);

INSERT INTO work_contributors (work_id, person_id, role, position)
SELECT id, author_id, 'author', 0 FROM works WHERE author_id IS NOT NULL;

-- 6. What "the file and edition of a work" means for code that still thinks in
-- one file per work: the primary edition and its first file.
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

-- 7. Legacy writers (the worker and the upload handlers) still write `works`.
-- These triggers project those writes onto the new tables.
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

-- 8. Reading progress belongs to a user and a file, not to a work (RN-010,
-- DEC-030): the EPUB and the PDF of the same book keep separate positions.
-- `position` is the opaque string each reader has always produced; `locator` is
-- the typed, versioned form (spec 5.3) that readers adopt one by one.
-- `revision` counts writes, the basis for conflict detection (RF-031).
CREATE TABLE reading_progress (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	position TEXT NOT NULL DEFAULT '',
	locator JSONB,
	locator_version SMALLINT,
	percent_complete REAL NOT NULL DEFAULT 0,
	completed_at TIMESTAMPTZ,
	reading_seconds INTEGER NOT NULL DEFAULT 0,
	revision BIGINT NOT NULL DEFAULT 1,
	device VARCHAR(100),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, file_id),
	CHECK (locator IS NULL OR locator_version IS NOT NULL)
);
CREATE INDEX idx_reading_progress_file ON reading_progress(file_id);

-- Legacy progress moves to the primary file of its work. `user_progress` is kept.
INSERT INTO reading_progress (user_id, file_id, position, percent_complete, completed_at, reading_seconds, updated_at)
SELECT up.user_id, wp.file_id, up.progress, up.percent_complete,
       up.completed_at::timestamptz, up.reading_seconds, COALESCE(up.updated_at::timestamptz, now())
FROM user_progress up
JOIN work_primary wp ON wp.work_id = up.work_id
WHERE wp.file_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- 9. Notes survive their source (RF-039, DEC-039). The bibliographic reference
-- lives in the note; the link to the work no longer cascades.
ALTER TABLE notes ADD COLUMN source_work_id INTEGER;
ALTER TABLE notes ADD COLUMN source_title TEXT;
ALTER TABLE notes ADD COLUMN source_author TEXT;
ALTER TABLE notes ADD COLUMN file_id BIGINT REFERENCES files(id) ON DELETE SET NULL;

UPDATE notes n
SET source_work_id = n.work_id,
    source_title = w.original_title,
    source_author = NULLIF(p.name, 'Unknown Author')
FROM works w LEFT JOIN person p ON p.id = w.author_id
WHERE w.id = n.work_id;

ALTER TABLE notes ALTER COLUMN source_title SET NOT NULL;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_work_id_fkey;
ALTER TABLE notes ADD CONSTRAINT notes_work_id_fkey
	FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE SET NULL;

-- A note written without an explicit reference takes it from its work.
-- +goose StatementBegin
CREATE FUNCTION notes_fill_source() RETURNS trigger AS $$
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

CREATE TRIGGER notes_fill_source BEFORE INSERT ON notes
FOR EACH ROW EXECUTE FUNCTION notes_fill_source();

-- 10. Audit log: who did what to what, never secrets (plan section 10).
CREATE TABLE audit_log (
	id BIGSERIAL PRIMARY KEY,
	at TIMESTAMPTZ NOT NULL DEFAULT now(),
	actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
	actor_username VARCHAR(50),
	action VARCHAR(64) NOT NULL,
	target_type VARCHAR(32),
	target_id TEXT,
	details JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_audit_log_at ON audit_log(at DESC);
CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);

-- 11. External identities (LDAP now, OIDC later): several per account, keyed by
-- provider and a stable subject, never by username (DEC-072 to DEC-075). The
-- owner authenticates locally only (DEC-073), enforced here too.
CREATE TABLE external_identities (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	provider VARCHAR(64) NOT NULL,
	subject VARCHAR(255) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_verified_at TIMESTAMPTZ,
	UNIQUE (provider, subject)
);
CREATE INDEX idx_external_identities_user ON external_identities(user_id);

-- +goose StatementBegin
CREATE FUNCTION external_identity_not_owner() RETURNS trigger AS $$
BEGIN
	IF (SELECT role FROM users WHERE id = NEW.user_id) = 'owner' THEN
		RAISE EXCEPTION 'the owner authenticates locally only' USING ERRCODE = 'check_violation';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER external_identity_not_owner BEFORE INSERT OR UPDATE OF user_id ON external_identities
FOR EACH ROW EXECUTE FUNCTION external_identity_not_owner();

-- +goose StatementBegin
CREATE FUNCTION owner_has_no_external_identity() RETURNS trigger AS $$
BEGIN
	IF EXISTS (SELECT 1 FROM external_identities WHERE user_id = NEW.id) THEN
		RAISE EXCEPTION 'an account with an external identity cannot become the owner' USING ERRCODE = 'check_violation';
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER owner_has_no_external_identity BEFORE UPDATE OF role ON users
FOR EACH ROW WHEN (NEW.role = 'owner' AND OLD.role IS DISTINCT FROM 'owner')
EXECUTE FUNCTION owner_has_no_external_identity();

-- +goose Down
-- Reverts the structure added above. The legacy columns and tables were never
-- touched, so no legacy data is lost. Two limits: rows created only in the new
-- model (extra editions, files without a legacy column, progress per file,
-- notes of purged works) are lost, and restoring one edition per work fails if
-- a work already has several. Use a backup for anything beyond a fresh upgrade.
DROP TRIGGER IF EXISTS owner_has_no_external_identity ON users;
DROP FUNCTION IF EXISTS owner_has_no_external_identity();
DROP TABLE IF EXISTS external_identities;
DROP FUNCTION IF EXISTS external_identity_not_owner();
DROP TABLE IF EXISTS audit_log;

DROP TRIGGER IF EXISTS notes_fill_source ON notes;
DROP FUNCTION IF EXISTS notes_fill_source();
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_work_id_fkey;
DELETE FROM notes WHERE work_id IS NULL;
ALTER TABLE notes ADD CONSTRAINT notes_work_id_fkey
	FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE;
ALTER TABLE notes DROP COLUMN file_id;
ALTER TABLE notes DROP COLUMN source_author;
ALTER TABLE notes DROP COLUMN source_title;
ALTER TABLE notes DROP COLUMN source_work_id;

DROP TABLE IF EXISTS reading_progress;

DROP TRIGGER IF EXISTS works_project_update ON works;
DROP FUNCTION IF EXISTS works_project_update();
DROP TRIGGER IF EXISTS works_project_insert ON works;
DROP FUNCTION IF EXISTS works_project_insert();
DROP VIEW IF EXISTS work_primary;
DROP TABLE IF EXISTS work_contributors;
DROP TABLE IF EXISTS storage_locations;
DROP TABLE IF EXISTS files;

DROP INDEX IF EXISTS editions_one_primary_per_work;
DELETE FROM editions e WHERE NOT e.is_primary;
ALTER TABLE editions DROP COLUMN created_at;
ALTER TABLE editions DROP COLUMN is_primary;
ALTER TABLE editions DROP COLUMN isbn;
ALTER TABLE editions DROP COLUMN publication_date;
ALTER TABLE editions DROP COLUMN publisher;
ALTER TABLE editions DROP COLUMN language;
ALTER TABLE editions DROP CONSTRAINT IF EXISTS editions_work_required;
ALTER TABLE editions ADD CONSTRAINT editions_work_id_key UNIQUE (work_id);

DROP INDEX IF EXISTS idx_works_retired;
ALTER TABLE works DROP COLUMN IF EXISTS retired_by;
ALTER TABLE works DROP COLUMN IF EXISTS retired_at;
