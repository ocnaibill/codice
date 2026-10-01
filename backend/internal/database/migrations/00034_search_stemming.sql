-- +goose Up
-- Search by the stem of a word, in the language of the edition (#41): "correr" finds "corrida", and "run" finds
-- "running". The search as it was (accents and case ignored, no stemming) stays in `tsv`, for the phrases between
-- quotes, which must stay exact, for what is typed without accents, and for the files whose language is not known;
-- the stemmed form is `tsv_stem`.
--
-- Everything here is derived from the files, like the rest of the text: nothing stored is changed.

-- One stemming configuration per language the library is expected to hold (the Latin languages and English, DEC-089).
-- The dictionary is the stemmer alone, with no list of stopwords: the stopwords of the language would drop "de" and
-- "o" from the index and from the query, and then a query made only of them (or "to be or not to be") would find
-- nothing. The stemmer sees the word with its accents: taking them out first would make "ação" and "ações" two
-- different stems ("aca" and "aco"), because it knows the endings of the accented word and not of the stripped one.
-- A search typed without accents still finds the words as they are, through `tsv`.
-- +goose StatementBegin
DO $$
DECLARE
	lang TEXT;
BEGIN
	FOREACH lang IN ARRAY ARRAY['portuguese', 'english', 'spanish', 'french', 'italian', 'catalan', 'romanian'] LOOP
		EXECUTE format('CREATE TEXT SEARCH DICTIONARY codice_%s_stem (TEMPLATE = snowball, Language = %s)', lang, lang);
		EXECUTE format('CREATE TEXT SEARCH CONFIGURATION codice_%s (COPY = %s)', lang, lang);
		EXECUTE format('ALTER TEXT SEARCH CONFIGURATION codice_%s ALTER MAPPING REPLACE %s_stem WITH codice_%s_stem', lang, lang, lang);
	END LOOP;
END
$$;
-- +goose StatementEnd

-- The configuration for the language of an edition (what the file declares, or what an administrator confirmed): its
-- primary subtag, in the two-letter or the three-letter form ("pt", "pt-BR", "por"). A language that is not here, or
-- none, is searched as before.
-- +goose StatementBegin
CREATE FUNCTION codice_search_config(p_language TEXT) RETURNS regconfig AS $$
	SELECT CASE
		WHEN l IN ('pt', 'por') THEN 'codice_portuguese'::regconfig
		WHEN l IN ('en', 'eng') THEN 'codice_english'::regconfig
		WHEN l IN ('es', 'spa') THEN 'codice_spanish'::regconfig
		WHEN l IN ('fr', 'fra', 'fre') THEN 'codice_french'::regconfig
		WHEN l IN ('it', 'ita') THEN 'codice_italian'::regconfig
		WHEN l IN ('ca', 'cat') THEN 'codice_catalan'::regconfig
		WHEN l IN ('ro', 'ron', 'rum') THEN 'codice_romanian'::regconfig
		ELSE 'codice_simple'::regconfig
	END
	FROM (SELECT lower(split_part(replace(COALESCE(p_language, ''), '_', '-'), '-', 1)) AS l) AS t
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

-- Which configuration each segment is indexed with. Set before the generated column exists, so that the rows are
-- written once.
ALTER TABLE document_segments ADD COLUMN search_config regconfig NOT NULL DEFAULT 'codice_simple'::regconfig;
UPDATE document_segments s SET search_config = codice_search_config(e.language)
FROM files f JOIN editions e ON e.id = f.edition_id
WHERE f.id = s.file_id AND codice_search_config(e.language) <> 'codice_simple'::regconfig;

ALTER TABLE document_segments
	ADD COLUMN tsv_stem TSVECTOR GENERATED ALWAYS AS (to_tsvector(search_config, text)) STORED;
CREATE INDEX document_segments_tsv_stem ON document_segments USING GIN (tsv_stem);

-- The worker writes segments without knowing about this: the language is taken from the edition of the file as the
-- row goes in, and followed when the language of the edition changes or the file goes to another edition (joining
-- versions). The vector is generated, so it is made again by itself.
-- +goose StatementBegin
CREATE FUNCTION document_segments_set_config() RETURNS trigger AS $$
BEGIN
	NEW.search_config := codice_search_config(
		(SELECT e.language FROM files f JOIN editions e ON e.id = f.edition_id WHERE f.id = NEW.file_id));
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER document_segments_set_config BEFORE INSERT ON document_segments
	FOR EACH ROW EXECUTE FUNCTION document_segments_set_config();

-- +goose StatementBegin
CREATE FUNCTION editions_language_changed() RETURNS trigger AS $$
BEGIN
	UPDATE document_segments SET search_config = codice_search_config(NEW.language)
	WHERE file_id IN (SELECT id FROM files WHERE edition_id = NEW.id)
	  AND search_config <> codice_search_config(NEW.language);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER editions_language_changed AFTER UPDATE OF language ON editions
	FOR EACH ROW WHEN (OLD.language IS DISTINCT FROM NEW.language) EXECUTE FUNCTION editions_language_changed();

-- +goose StatementBegin
CREATE FUNCTION files_edition_changed() RETURNS trigger AS $$
DECLARE
	config regconfig;
BEGIN
	SELECT codice_search_config(e.language) INTO config FROM editions e WHERE e.id = NEW.edition_id;
	UPDATE document_segments SET search_config = COALESCE(config, 'codice_simple'::regconfig)
	WHERE file_id = NEW.id AND search_config <> COALESCE(config, 'codice_simple'::regconfig);
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER files_edition_changed AFTER UPDATE OF edition_id ON files
	FOR EACH ROW WHEN (OLD.edition_id IS DISTINCT FROM NEW.edition_id) EXECUTE FUNCTION files_edition_changed();

-- +goose Down
DROP TRIGGER IF EXISTS files_edition_changed ON files;
DROP FUNCTION IF EXISTS files_edition_changed();
DROP TRIGGER IF EXISTS editions_language_changed ON editions;
DROP FUNCTION IF EXISTS editions_language_changed();
DROP TRIGGER IF EXISTS document_segments_set_config ON document_segments;
DROP FUNCTION IF EXISTS document_segments_set_config();
DROP INDEX IF EXISTS document_segments_tsv_stem;
ALTER TABLE document_segments DROP COLUMN IF EXISTS tsv_stem;
ALTER TABLE document_segments DROP COLUMN IF EXISTS search_config;
DROP FUNCTION IF EXISTS codice_search_config(TEXT);
-- +goose StatementBegin
DO $$
DECLARE
	lang TEXT;
BEGIN
	FOREACH lang IN ARRAY ARRAY['portuguese', 'english', 'spanish', 'french', 'italian', 'catalan', 'romanian'] LOOP
		EXECUTE format('DROP TEXT SEARCH CONFIGURATION IF EXISTS codice_%s', lang);
		EXECUTE format('DROP TEXT SEARCH DICTIONARY IF EXISTS codice_%s_stem', lang);
	END LOOP;
END
$$;
-- +goose StatementEnd
