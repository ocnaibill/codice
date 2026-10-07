-- +goose Up
-- Collections (#184, DEC-130): the franchises and series of the library ("official"), and the ones each person keeps
-- ("personal", private).
--
-- The official collection of a work is the one its series metadata names: works.series and works.series_index stay the
-- one truth, and the membership below follows them (a trigger, so the worker, the handlers and a migration all get it).
-- A person who adds or removes a work by hand writes those two fields and locks them (series_lock), which is what keeps
-- the automatic analysis from undoing it (#205). Nothing is ever written to the file of a book.
CREATE TABLE collections (
	id BIGSERIAL PRIMARY KEY,
	kind VARCHAR(10) NOT NULL CHECK (kind IN ('official', 'personal')),
	owner_id UUID REFERENCES users(id) ON DELETE CASCADE,
	name VARCHAR(512) NOT NULL,
	-- 'metadata': born from the series of a work. 'manual': made by a person.
	origin VARCHAR(10) NOT NULL DEFAULT 'metadata' CHECK (origin IN ('metadata', 'manual')),
	-- Set when a person renames it (or changes it otherwise): from then on it is shown even with no works in it.
	edited_at TIMESTAMPTZ,
	-- "Deleting" a collection retires it: its works are not touched, and it can be restored.
	retired_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((kind = 'personal') = (owner_id IS NOT NULL))
);
CREATE INDEX idx_collections_owner ON collections(owner_id) WHERE owner_id IS NOT NULL;

-- The texts that lead to an official collection: the one it was born from, and later the names it was given. A work
-- whose series text has one of these keys belongs to the collection, however it was renamed since.
CREATE TABLE collection_keys (
	key TEXT PRIMARY KEY,
	collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE
);
CREATE INDEX idx_collection_keys_collection ON collection_keys(collection_id);

CREATE TABLE collection_works (
	collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	-- Copied from collections.kind, so that the rule below can be an index.
	official BOOLEAN NOT NULL,
	-- The number in the series (official) or the place the person gave (personal). NULL: no number.
	position REAL,
	added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (collection_id, work_id)
);
CREATE INDEX idx_collection_works_work ON collection_works(work_id);
-- A work belongs to one official collection at most (DEC-130).
CREATE UNIQUE INDEX collection_works_one_official ON collection_works(work_id) WHERE official;

-- What two writings of a series name have in common: case, accents and runs of spaces do not tell them apart.
-- +goose StatementBegin
CREATE FUNCTION series_key(t TEXT) RETURNS TEXT AS $$
	SELECT NULLIF(btrim(regexp_replace(lower(unaccent(t)), '\s+', ' ', 'g')), '')
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd

-- Puts a work in the official collection its series names (creating the collection if there is none), or takes it out of
-- every one when it has no series. A retired collection takes no work: the ones the text leads to stay out until it is
-- restored.
-- +goose StatementBegin
CREATE FUNCTION collection_sync_work(p_work INTEGER, p_series TEXT, p_index REAL) RETURNS VOID AS $$
DECLARE
	k TEXT := series_key(p_series);
	pos REAL := NULLIF(p_index, 0);
	cid BIGINT;
	cur BIGINT;
	gone TIMESTAMPTZ;
BEGIN
	SELECT collection_id INTO cur FROM collection_works WHERE work_id = p_work AND official;

	IF k IS NULL THEN
		IF cur IS NOT NULL THEN
			DELETE FROM collection_works WHERE work_id = p_work AND official;
		END IF;
		RETURN;
	END IF;

	-- Two works with the same new series, written at the same moment, must not make two collections.
	PERFORM pg_advisory_xact_lock(hashtextextended('collection:' || k, 0));
	SELECT collection_id INTO cid FROM collection_keys WHERE key = k;
	IF cid IS NULL THEN
		INSERT INTO collections (kind, name) VALUES ('official', btrim(regexp_replace(p_series, '\s+', ' ', 'g')))
		RETURNING id INTO cid;
		INSERT INTO collection_keys (key, collection_id) VALUES (k, cid);
	END IF;

	SELECT retired_at INTO gone FROM collections WHERE id = cid;
	IF gone IS NOT NULL THEN
		IF cur IS NOT NULL THEN
			DELETE FROM collection_works WHERE work_id = p_work AND official;
		END IF;
		RETURN;
	END IF;

	IF cur IS DISTINCT FROM cid THEN
		DELETE FROM collection_works WHERE work_id = p_work AND official;
		INSERT INTO collection_works (collection_id, work_id, official, position) VALUES (cid, p_work, TRUE, pos);
	ELSE
		UPDATE collection_works SET position = pos
		WHERE collection_id = cid AND work_id = p_work AND position IS DISTINCT FROM pos;
	END IF;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION works_sync_collection() RETURNS trigger AS $$
BEGIN
	PERFORM collection_sync_work(NEW.id, NEW.series, NEW.series_index);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER works_sync_collection AFTER INSERT OR UPDATE OF series, series_index ON works
FOR EACH ROW EXECUTE FUNCTION works_sync_collection();

-- The series that exist now become collections.
SELECT collection_sync_work(id, series, series_index) FROM works WHERE series IS NOT NULL ORDER BY id;

-- +goose Down
DROP TRIGGER IF EXISTS works_sync_collection ON works;
DROP FUNCTION IF EXISTS works_sync_collection();
DROP FUNCTION IF EXISTS collection_sync_work(INTEGER, TEXT, REAL);
DROP FUNCTION IF EXISTS series_key(TEXT);
DROP TABLE IF EXISTS collection_works;
DROP TABLE IF EXISTS collection_keys;
DROP TABLE IF EXISTS collections;
