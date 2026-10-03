-- +goose Up
-- The dictionaries the owner installs (#109, DEC-115): third-party data (the Wiktionary, extracted by Wiktextract),
-- downloaded and imported by the worker when the owner asks, kept here and asked of nobody. A package is one of the
-- catalog (internal/dictionary); everything it brought goes with it when it is removed.
CREATE TABLE dictionary_packages (
	id TEXT PRIMARY KEY,
	state TEXT NOT NULL CHECK (state IN ('installing', 'ready', 'failed')),
	-- Where an installation is (queued, downloading, importing, retrying) and how far through that stage.
	stage TEXT NOT NULL DEFAULT 'queued',
	progress REAL NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 1),
	source_url TEXT NOT NULL,
	bytes_total BIGINT,
	bytes_done BIGINT NOT NULL DEFAULT 0,
	-- What was downloaded, to say which file the dictionary came from.
	sha256 TEXT,
	-- When the source says the file was made (its Last-Modified).
	source_date TEXT,
	entries INTEGER NOT NULL DEFAULT 0,
	forms INTEGER NOT NULL DEFAULT 0,
	links INTEGER NOT NULL DEFAULT 0,
	languages TEXT[] NOT NULL DEFAULT '{}',
	error TEXT NOT NULL DEFAULT '',
	job_id BIGINT,
	installed_by UUID,
	installed_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A word of a language, with what the dictionary says of it, as the Wiktionary has it: the senses (definitions or, for a
-- word of another language than the package's, the translation), the forms of the word, a few translations. `norm` is
-- what the word reads as with no case and no accent (and as written for scripts where an accent is a letter): it is
-- what a selected word is looked up by (worker/dictionary.py and internal/dictionary/normalize.go say the same).
CREATE TABLE dictionary_entries (
	id BIGSERIAL PRIMARY KEY,
	package_id TEXT NOT NULL REFERENCES dictionary_packages(id) ON DELETE CASCADE,
	lang TEXT NOT NULL,
	word TEXT NOT NULL,
	norm TEXT NOT NULL,
	pos TEXT NOT NULL DEFAULT '',
	data JSONB NOT NULL
);
CREATE INDEX dictionary_entries_lookup ON dictionary_entries (lang, norm);
CREATE INDEX dictionary_entries_package ON dictionary_entries (package_id);

-- The forms of a lemma (corro, corres, correndo of correr): a selected word that is one of them finds its lemma even
-- where the package has no entry of its own for the form.
CREATE TABLE dictionary_forms (
	package_id TEXT NOT NULL REFERENCES dictionary_packages(id) ON DELETE CASCADE,
	lang TEXT NOT NULL,
	norm TEXT NOT NULL,
	form TEXT NOT NULL,
	lemma TEXT NOT NULL,
	pos TEXT NOT NULL DEFAULT '',
	tags TEXT[] NOT NULL DEFAULT '{}'
);
CREATE INDEX dictionary_forms_lookup ON dictionary_forms (lang, norm);
CREATE INDEX dictionary_forms_package ON dictionary_forms (package_id);

-- The translations a lemma lists, turned around: the entry "correr" lists 走る (ja) as a translation, so that a selected
-- 走る finds "correr" in the language of the package. They are what a word of another language is looked up by when the
-- package has no entry of its own for it.
CREATE TABLE dictionary_links (
	package_id TEXT NOT NULL REFERENCES dictionary_packages(id) ON DELETE CASCADE,
	lang TEXT NOT NULL,
	norm TEXT NOT NULL,
	word TEXT NOT NULL,
	target_lang TEXT NOT NULL,
	target_word TEXT NOT NULL,
	pos TEXT NOT NULL DEFAULT '',
	-- The sense of the target the translation is listed under, when the source says.
	sense TEXT NOT NULL DEFAULT ''
);
CREATE INDEX dictionary_links_lookup ON dictionary_links (lang, norm);
CREATE INDEX dictionary_links_package ON dictionary_links (package_id);

-- +goose Down
DROP TABLE IF EXISTS dictionary_links;
DROP TABLE IF EXISTS dictionary_forms;
DROP TABLE IF EXISTS dictionary_entries;
DROP TABLE IF EXISTS dictionary_packages;
