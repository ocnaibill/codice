-- +goose Up
-- What the staff writes by hand in the wiki (DEC-186): entities, sections of a page, ties, states of an entity at a chapter, edges of the
-- graph. They are kept for the work or series, not for one generation: a new generation replaces the project, and these are put on it
-- again. Everything has the point (book, chapter) from which it may be shown, so the cut of DEC-179 holds for what is written by hand.
CREATE TABLE wiki_manual_items (
	id BIGSERIAL PRIMARY KEY,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	kind VARCHAR(10) NOT NULL CHECK (kind IN ('entity', 'section', 'relation', 'state', 'edge')),
	-- The entity it is of (found again by its name or alias, folded) and, for a tie or an edge, the other one.
	entity_key TEXT NOT NULL,
	entity_name TEXT NOT NULL,
	other_key TEXT NOT NULL DEFAULT '',
	other_name TEXT NOT NULL DEFAULT '',
	-- entity: {kind, aliases}. section: {kind, part, paragraphs}. relation: {group, role}. state: {text}. edge: {label}.
	data JSONB NOT NULL DEFAULT '{}',
	book SMALLINT NOT NULL,
	chapter SMALLINT NOT NULL,
	created_by UUID REFERENCES users(id) ON DELETE SET NULL,
	author_name TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((collection_id IS NULL) <> (work_id IS NULL))
);
CREATE INDEX wiki_manual_items_work ON wiki_manual_items (work_id) WHERE work_id IS NOT NULL;
CREATE INDEX wiki_manual_items_collection ON wiki_manual_items (collection_id) WHERE collection_id IS NOT NULL;

-- The rows of the project that came from an item: removing the item removes them.
ALTER TABLE wiki_entities ADD COLUMN manual_item_id BIGINT REFERENCES wiki_manual_items(id) ON DELETE CASCADE;
ALTER TABLE wiki_sections ADD COLUMN manual_item_id BIGINT REFERENCES wiki_manual_items(id) ON DELETE CASCADE;
ALTER TABLE wiki_relations ADD COLUMN manual_item_id BIGINT REFERENCES wiki_manual_items(id) ON DELETE CASCADE;
ALTER TABLE wiki_states ADD COLUMN manual_item_id BIGINT REFERENCES wiki_manual_items(id) ON DELETE CASCADE;
ALTER TABLE wiki_edges ADD COLUMN manual_item_id BIGINT REFERENCES wiki_manual_items(id) ON DELETE CASCADE;
-- Who wrote a section, as it is said on the page ("escrito à mão por ...").
ALTER TABLE wiki_sections ADD COLUMN manual_by TEXT;

-- +goose Down
ALTER TABLE wiki_sections DROP COLUMN manual_by;
ALTER TABLE wiki_edges DROP COLUMN manual_item_id;
ALTER TABLE wiki_states DROP COLUMN manual_item_id;
ALTER TABLE wiki_relations DROP COLUMN manual_item_id;
ALTER TABLE wiki_sections DROP COLUMN manual_item_id;
ALTER TABLE wiki_entities DROP COLUMN manual_item_id;
DROP TABLE wiki_manual_items;
