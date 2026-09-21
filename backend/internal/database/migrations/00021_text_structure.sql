-- +goose Up
-- The shape of a book (DEC-087): the nodes of its outline (chapters, parts, a preface, an appendix)
-- with the part each one is in (front, body, back), published with the text so that a reader of the
-- text never sees one generation's segments with another's outline. A segment says which node of
-- its file it is in; a file with no outline has no nodes, and that is left as it is.
ALTER TABLE text_extractions ADD COLUMN structure JSONB;
ALTER TABLE document_segments ADD COLUMN node INTEGER;

DROP FUNCTION text_extraction_publish(BIGINT, INTEGER, INTEGER, TEXT, TEXT, TEXT, TEXT);

-- +goose StatementBegin
CREATE FUNCTION text_extraction_publish(
	p_file_id BIGINT, p_generation INTEGER, p_extractor_version INTEGER, p_source_sha256 TEXT,
	p_status TEXT, p_origin TEXT, p_language TEXT, p_structure JSONB DEFAULT NULL
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
	                              segment_count, char_count, language, structure, error, extracted_at)
	VALUES (p_file_id, p_generation, p_extractor_version, p_source_sha256, p_status, p_origin, n, chars,
	        NULLIF(p_language, ''), p_structure, NULL, now())
	ON CONFLICT (file_id) DO UPDATE SET
		generation = EXCLUDED.generation, extractor_version = EXCLUDED.extractor_version,
		source_sha256 = EXCLUDED.source_sha256, status = EXCLUDED.status, origin = EXCLUDED.origin,
		segment_count = EXCLUDED.segment_count, char_count = EXCLUDED.char_count,
		language = EXCLUDED.language, structure = EXCLUDED.structure, error = NULL, extracted_at = now();

	DELETE FROM document_segments WHERE file_id = p_file_id AND generation <> p_generation;
	RETURN n;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- The text that is here is read again, once, after everything else, to learn its outline.
INSERT INTO jobs (type, work_id, payload, priority)
SELECT DISTINCT 'extract_text', e.work_id, '{}'::jsonb, -10
FROM editions e
JOIN files f ON f.edition_id = e.id
JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
JOIN text_extractions te ON te.file_id = f.id
ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;

-- +goose Down
DROP FUNCTION text_extraction_publish(BIGINT, INTEGER, INTEGER, TEXT, TEXT, TEXT, TEXT, JSONB);
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
ALTER TABLE document_segments DROP COLUMN node;
ALTER TABLE text_extractions DROP COLUMN structure;
