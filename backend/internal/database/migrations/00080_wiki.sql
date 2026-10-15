-- +goose Up
-- The automatic wiki of a work or a series (DEC-177 to DEC-183, RF-050 to RF-052).
--
-- A wiki is generated outside the request path (the lab today, the worker later) and arrives as one bundle that replaces the
-- project's content at once. Everything a page can show carries the point of reading from which it may be shown: a book of the
-- project (its position) and a chapter of that book (DEC-179). The server compares those points with the reader's own and never
-- sends what lies beyond it. A point in a side book (a collection of short stories beside the main line) is shown only when the
-- reader asks for the side books.
CREATE TABLE wiki_projects (
	id BIGSERIAL PRIMARY KEY,
	-- A series is an official collection; a single book is a work. Exactly one of the two.
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	-- ready: complete. partial: some step did not finish (a search that failed for a book). generating: a run is replacing it.
	state VARCHAR(10) NOT NULL DEFAULT 'ready' CHECK (state IN ('generating', 'ready', 'partial', 'failed')),
	-- pending: a new book updated the wiki and owner or admins have not reviewed it yet (DEC-180).
	review VARCHAR(8) NOT NULL DEFAULT 'none' CHECK (review IN ('none', 'pending')),
	-- Who wrote it (DEC-177): shown on every page. remote: the text of the books left the server to be written.
	provider VARCHAR(64) NOT NULL,
	model VARCHAR(128) NOT NULL,
	remote BOOLEAN NOT NULL,
	format_version SMALLINT NOT NULL,
	generated_at TIMESTAMPTZ NOT NULL,
	imported_by UUID REFERENCES users(id) ON DELETE SET NULL,
	imported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((collection_id IS NULL) <> (work_id IS NULL))
);
CREATE UNIQUE INDEX wiki_projects_collection ON wiki_projects (collection_id) WHERE collection_id IS NOT NULL;
CREATE UNIQUE INDEX wiki_projects_work ON wiki_projects (work_id) WHERE work_id IS NOT NULL;

-- The books of a project, in reading order. The file is the one whose text the wiki was written from: the reader's place in it is
-- what decides how far the wiki goes.
CREATE TABLE wiki_books (
	project_id BIGINT NOT NULL REFERENCES wiki_projects(id) ON DELETE CASCADE,
	position SMALLINT NOT NULL CHECK (position >= 0),
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	file_id BIGINT REFERENCES files(id) ON DELETE SET NULL,
	side BOOLEAN NOT NULL DEFAULT false,
	title TEXT NOT NULL,
	PRIMARY KEY (project_id, position)
);
CREATE INDEX wiki_books_work ON wiki_books (work_id);

-- The chapters of a book as the wiki counts them (from 1). first_sequence is the first segment of the book's file in it, which is
-- how a place in the file (the reader's) becomes a chapter. editorial: after the story (other books advertised, acknowledgements).
CREATE TABLE wiki_chapters (
	project_id BIGINT NOT NULL,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL CHECK (chapter >= 1),
	title TEXT NOT NULL,
	part TEXT NOT NULL DEFAULT '',
	first_sequence INTEGER,
	editorial BOOLEAN NOT NULL DEFAULT false,
	PRIMARY KEY (project_id, book, chapter),
	FOREIGN KEY (project_id, book) REFERENCES wiki_books (project_id, position) ON DELETE CASCADE
);

-- Every entity the registry found (the graph has them all); page says whether it has a page of its own.
CREATE TABLE wiki_entities (
	id BIGSERIAL PRIMARY KEY,
	project_id BIGINT NOT NULL REFERENCES wiki_projects(id) ON DELETE CASCADE,
	slug VARCHAR(160) NOT NULL,
	name TEXT NOT NULL,
	kind VARCHAR(12) NOT NULL CHECK (kind IN ('person', 'place', 'organization', 'concept', 'object', 'event', 'other')),
	page BOOLEAN NOT NULL DEFAULT false,
	UNIQUE (project_id, slug)
);

-- Where an entity appears: an entity is not shown before its first appearance, and the graph sizes it by how often it appeared so far.
CREATE TABLE wiki_appearances (
	entity_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	PRIMARY KEY (entity_id, book, chapter)
);

-- Other names, each from the point it was first used ("Anastássia" is a spoiler until she is ordained).
CREATE TABLE wiki_aliases (
	entity_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	alias TEXT NOT NULL,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	PRIMARY KEY (entity_id, alias)
);

-- The entity's situation at the end of a chapter it appears in ("Situação neste ponto", the timeline, the part in progress).
CREATE TABLE wiki_states (
	entity_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	text TEXT NOT NULL,
	PRIMARY KEY (entity_id, book, chapter)
);

-- The written sections of a page (DEC-178). who: "Quem é", rewritten at the end of each part; the one shown is the last whose point
-- the reader reached. bio: the biography of one part, shown once the part is read. personality: one section, each sentence shown
-- from the point of its last reference.
CREATE TABLE wiki_sections (
	id BIGSERIAL PRIMARY KEY,
	entity_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	kind VARCHAR(12) NOT NULL CHECK (kind IN ('who', 'bio', 'personality')),
	-- who and bio: the book, the chapters of the part (first and last) and its name. personality: none.
	book SMALLINT,
	chapter_from SMALLINT,
	chapter_to SMALLINT,
	part TEXT NOT NULL DEFAULT '',
	position SMALLINT NOT NULL,
	CHECK ((kind = 'personality') = (book IS NULL))
);
CREATE INDEX wiki_sections_entity ON wiki_sections (entity_id, kind);

CREATE TABLE wiki_sentences (
	id BIGSERIAL PRIMARY KEY,
	section_id BIGINT NOT NULL REFERENCES wiki_sections(id) ON DELETE CASCADE,
	paragraph SMALLINT NOT NULL DEFAULT 1,
	position SMALLINT NOT NULL,
	text TEXT NOT NULL,
	-- Marked wrong by owner or admin (RF-052): no longer shown to anyone.
	hidden_at TIMESTAMPTZ,
	hidden_by UUID REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX wiki_sentences_section ON wiki_sentences (section_id);

-- What a sentence rests on (DEC-178): the chapter, the fact and, when it came from the search in the text, the literal quote.
-- sequence is the segment of the book's file the quote is in, for "open in the reader".
CREATE TABLE wiki_references (
	id BIGSERIAL PRIMARY KEY,
	sentence_id BIGINT NOT NULL REFERENCES wiki_sentences(id) ON DELETE CASCADE,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	fact TEXT NOT NULL DEFAULT '',
	quote TEXT,
	sequence INTEGER
);
CREATE INDEX wiki_references_sentence ON wiki_references (sentence_id);

-- The relation box of a page, by group, each from the point it was first recorded.
CREATE TABLE wiki_relations (
	entity_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	other_id BIGINT REFERENCES wiki_entities(id) ON DELETE SET NULL,
	other_name TEXT NOT NULL,
	grp VARCHAR(12) NOT NULL CHECK (grp IN ('family', 'mentors', 'romance', 'allies', 'enemies', 'places', 'things', 'others')),
	role TEXT NOT NULL DEFAULT '',
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	position SMALLINT NOT NULL
);
CREATE INDEX wiki_relations_entity ON wiki_relations (entity_id);

-- The edges of the graph: every relation the map recorded, with its words and its point.
CREATE TABLE wiki_edges (
	project_id BIGINT NOT NULL REFERENCES wiki_projects(id) ON DELETE CASCADE,
	a_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	b_id BIGINT NOT NULL REFERENCES wiki_entities(id) ON DELETE CASCADE,
	label TEXT NOT NULL,
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL
);
CREATE INDEX wiki_edges_project ON wiki_edges (project_id);

-- Identities the generator could not prove (DEC-181): owner or admins confirm, keep apart or split. Decisions are kept and win
-- over later generations; the evidence is what made the pair worth asking about.
CREATE TABLE wiki_suggestions (
	id BIGSERIAL PRIMARY KEY,
	project_id BIGINT NOT NULL REFERENCES wiki_projects(id) ON DELETE CASCADE,
	kind VARCHAR(8) NOT NULL CHECK (kind IN ('merge', 'split')),
	entity_slug VARCHAR(160) NOT NULL,
	other_slug VARCHAR(160),
	evidence TEXT NOT NULL DEFAULT '',
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	state VARCHAR(8) NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'accepted', 'declined')),
	decided_by UUID REFERENCES users(id) ON DELETE SET NULL,
	decided_at TIMESTAMPTZ
);
CREATE INDEX wiki_suggestions_project ON wiki_suggestions (project_id, state);

-- +goose Down
DROP TABLE IF EXISTS wiki_suggestions;
DROP TABLE IF EXISTS wiki_edges;
DROP TABLE IF EXISTS wiki_relations;
DROP TABLE IF EXISTS wiki_references;
DROP TABLE IF EXISTS wiki_sentences;
DROP TABLE IF EXISTS wiki_sections;
DROP TABLE IF EXISTS wiki_states;
DROP TABLE IF EXISTS wiki_aliases;
DROP TABLE IF EXISTS wiki_appearances;
DROP TABLE IF EXISTS wiki_entities;
DROP TABLE IF EXISTS wiki_chapters;
DROP TABLE IF EXISTS wiki_books;
DROP TABLE IF EXISTS wiki_projects;
