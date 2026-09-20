-- +goose Up
-- The text of the files, kept where it can be searched (RF-018, RF-019, spec 5.3 and 16.2).
--
-- A DocumentSegment is a piece of a file's text with its own address (a locator), so a hit can open
-- the file at the right place. What was extracted is kept as extracted: the search finds through an
-- index that ignores accents and case, and the text a person is shown is never the normalized one.
--
-- Everything here is derived from the files: it can be thrown away and made again, which is why the
-- backup leaves it out and a restore asks for it to be extracted anew.

-- Accent- and case-insensitive search, in any language: "acao" finds "Ação". There is no stemming,
-- so "correr" does not find "corrida": a linguistic configuration per language can be added later
-- as another column, without touching what is stored (the extraction is versioned).
CREATE EXTENSION IF NOT EXISTS unaccent;
CREATE TEXT SEARCH CONFIGURATION codice_simple (COPY = simple);
ALTER TEXT SEARCH CONFIGURATION codice_simple
	ALTER MAPPING FOR hword, hword_part, word WITH unaccent, simple;

-- One row per file: which extraction is published, how it went and from which content it came.
CREATE TABLE text_extractions (
	file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
	-- The published generation of this file's segments; 0 while nothing is published.
	generation INTEGER NOT NULL DEFAULT 0,
	-- The version of the extraction (its rules and limits). A file extracted with an older one is
	-- extracted again.
	extractor_version SMALLINT NOT NULL,
	-- The hash of the file the text came from: a file that changed no longer matches its text.
	source_sha256 CHAR(64),
	-- ready: there is text. empty: the file has no usable text (a scan, waiting for OCR).
	-- unsupported: this kind of file has no text to read (a comic, an audiobook).
	-- failed: it could not be read, and nothing was ever published.
	status VARCHAR(12) NOT NULL CHECK (status IN ('ready', 'empty', 'unsupported', 'failed')),
	origin VARCHAR(8) NOT NULL DEFAULT 'native' CHECK (origin IN ('native', 'ocr', 'mixed')),
	segment_count INTEGER NOT NULL DEFAULT 0,
	char_count BIGINT NOT NULL DEFAULT 0,
	language VARCHAR(16),
	-- The last attempt that did not work. A published extraction stays published when a later one fails.
	error TEXT,
	extracted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE document_segments (
	id BIGSERIAL PRIMARY KEY,
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	generation INTEGER NOT NULL,
	-- The order of reading inside the file.
	sequence INTEGER NOT NULL,
	origin VARCHAR(8) NOT NULL DEFAULT 'native' CHECK (origin IN ('native', 'ocr')),
	-- Where it is, in words: the chapter or section, when the file says.
	section TEXT,
	text TEXT NOT NULL,
	locator JSONB NOT NULL,
	locator_version SMALLINT NOT NULL,
	tsv TSVECTOR GENERATED ALWAYS AS (to_tsvector('codice_simple'::regconfig, text)) STORED,
	UNIQUE (file_id, generation, sequence)
);
CREATE INDEX document_segments_tsv ON document_segments USING GIN (tsv);

-- Publishing is one step. The worker writes a new generation of a file's segments, invisible because
-- it is not the published one, and this makes it the published one and drops the older ones, all at
-- once: a search sees the old text or the new, never half of either, and a worker that dies in
-- the middle leaves nothing that is searchable.

-- Returns the number of the generation to write, after clearing what an earlier attempt left.
-- +goose StatementBegin
CREATE FUNCTION text_extraction_begin(p_file_id BIGINT) RETURNS INTEGER AS $$
DECLARE
	published INTEGER;
BEGIN
	SELECT generation INTO published FROM text_extractions WHERE file_id = p_file_id;
	published := COALESCE(published, 0);
	DELETE FROM document_segments WHERE file_id = p_file_id AND generation > published;
	RETURN published + 1;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Makes generation p_generation the published one. A status of ready needs segments; the others
-- need none, and any written are dropped.
-- +goose StatementBegin
CREATE FUNCTION text_extraction_publish(
	p_file_id BIGINT, p_generation INTEGER, p_extractor_version INTEGER, p_source_sha256 TEXT,
	p_status TEXT, p_origin TEXT, p_language TEXT
) RETURNS INTEGER AS $$
DECLARE
	published INTEGER;
	n INTEGER;
	chars BIGINT;
BEGIN
	SELECT generation INTO published FROM text_extractions WHERE file_id = p_file_id FOR UPDATE;
	published := COALESCE(published, 0);
	IF p_generation <> published + 1 THEN
		RAISE EXCEPTION 'generation % is not the next one after %', p_generation, published;
	END IF;

	IF p_status <> 'ready' THEN
		DELETE FROM document_segments WHERE file_id = p_file_id AND generation = p_generation;
	END IF;
	SELECT count(*), COALESCE(sum(length(text)), 0) INTO n, chars
	FROM document_segments WHERE file_id = p_file_id AND generation = p_generation;
	IF p_status = 'ready' AND n = 0 THEN
		RAISE EXCEPTION 'nothing to publish as ready';
	END IF;

	INSERT INTO text_extractions (file_id, generation, extractor_version, source_sha256, status, origin,
	                              segment_count, char_count, language, error, extracted_at)
	VALUES (p_file_id, p_generation, p_extractor_version, p_source_sha256, p_status, p_origin, n, chars,
	        NULLIF(p_language, ''), NULL, now())
	ON CONFLICT (file_id) DO UPDATE SET
		generation = EXCLUDED.generation, extractor_version = EXCLUDED.extractor_version,
		source_sha256 = EXCLUDED.source_sha256, status = EXCLUDED.status, origin = EXCLUDED.origin,
		segment_count = EXCLUDED.segment_count, char_count = EXCLUDED.char_count,
		language = EXCLUDED.language, error = NULL, extracted_at = now();

	DELETE FROM document_segments WHERE file_id = p_file_id AND generation <> p_generation;
	RETURN n;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Records an attempt that failed. What was published before stays published.
-- +goose StatementBegin
CREATE FUNCTION text_extraction_fail(p_file_id BIGINT, p_extractor_version INTEGER, p_source_sha256 TEXT, p_error TEXT)
RETURNS VOID AS $$
BEGIN
	DELETE FROM document_segments WHERE file_id = p_file_id
		AND generation > COALESCE((SELECT generation FROM text_extractions WHERE file_id = p_file_id), 0);
	INSERT INTO text_extractions (file_id, generation, extractor_version, source_sha256, status, error, extracted_at)
	VALUES (p_file_id, 0, p_extractor_version, p_source_sha256, 'failed', p_error, now())
	ON CONFLICT (file_id) DO UPDATE SET error = EXCLUDED.error;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- The text is extracted after the file is analysed, as a job of its own and after the work that people
-- asked for: reading never waits for it, and a work that is READY stays READY while it runs.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION jobs_organize_after_ingest() RETURNS trigger AS $$
BEGIN
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('organize', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('dedupe', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('extract_text', NEW.work_id, '{}', -10)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- The files that are already here get their text too, once, and after everything else.
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION jobs_organize_after_ingest() RETURNS trigger AS $$
BEGIN
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('organize', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('dedupe', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
DELETE FROM jobs WHERE type = 'extract_text';
DROP FUNCTION IF EXISTS text_extraction_fail(BIGINT, INTEGER, TEXT, TEXT);
DROP FUNCTION IF EXISTS text_extraction_publish(BIGINT, INTEGER, INTEGER, TEXT, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS text_extraction_begin(BIGINT);
DROP TABLE IF EXISTS document_segments;
DROP TABLE IF EXISTS text_extractions;
DROP TEXT SEARCH CONFIGURATION IF EXISTS codice_simple;
DROP EXTENSION IF EXISTS unaccent;
