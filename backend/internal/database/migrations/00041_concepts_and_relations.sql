-- +goose Up
-- The manual graph (#83, RF-026, DEC-109): a person's concepts and the typed relations they draw between works,
-- concepts and notes, by hand and with no model. All of it is personal (RN-006): every row has its owner and every
-- query is scoped to them.

-- A concept is a named thing the person keeps: a name, a description and the other names it goes by. The names of a
-- person's concepts and their aliases are all different, by what they read as with no case, accent or punctuation
-- (concept_keys), so that [[IA]] finds one concept and a name cannot be two.
CREATE TABLE concepts (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name VARCHAR(120) NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	aliases TEXT[] NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX concepts_user ON concepts (user_id, lower(name));

CREATE TABLE concept_keys (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	key TEXT NOT NULL,
	concept_id BIGINT NOT NULL REFERENCES concepts(id) ON DELETE CASCADE,
	PRIMARY KEY (user_id, key)
);
CREATE INDEX concept_keys_concept ON concept_keys (concept_id);

-- A relation joins two nodes of the person: a work, a concept or a note. The nodes are not foreign keys, because a
-- relation outlives the work it points to (DEC-039: what is personal outlives the work): the label of each end is
-- kept on the relation, and the node is looked up when it is read, to say whether it is still there. What the person
-- deletes by hand (a note, a concept) takes its relations with it, by the triggers below.
-- The type is one of a fixed list that the server knows (internal/graph): adding one later is no migration.
CREATE TABLE relations (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	source_kind VARCHAR(8) NOT NULL CHECK (source_kind IN ('work', 'concept', 'note')),
	source_id BIGINT NOT NULL,
	type VARCHAR(24) NOT NULL,
	target_kind VARCHAR(8) NOT NULL CHECK (target_kind IN ('work', 'concept', 'note')),
	target_id BIGINT NOT NULL,
	-- manual: drawn by the person. wikilink: derived from a [[Concept]] in a note (#21), taken away with it.
	origin VARCHAR(10) NOT NULL DEFAULT 'manual' CHECK (origin IN ('manual', 'wikilink')),
	comment TEXT NOT NULL DEFAULT '',
	-- What each end was called when the relation was drawn, for when the node is gone.
	source_label JSONB NOT NULL DEFAULT '{}',
	target_label JSONB NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK (NOT (source_kind = target_kind AND source_id = target_id))
);
-- The same relation twice is one. For a type with no direction the pair counts once whichever end is the source.
CREATE UNIQUE INDEX relations_directed ON relations (user_id, type, source_kind, source_id, target_kind, target_id);
CREATE UNIQUE INDEX relations_undirected ON relations (user_id, type,
	LEAST(source_kind || ':' || source_id, target_kind || ':' || target_id),
	GREATEST(source_kind || ':' || source_id, target_kind || ':' || target_id))
	WHERE type IN ('related', 'in_dialogue_with', 'opposes');
CREATE INDEX relations_source ON relations (user_id, source_kind, source_id);
CREATE INDEX relations_target ON relations (user_id, target_kind, target_id);

-- +goose StatementBegin
CREATE FUNCTION relations_of_deleted_node() RETURNS trigger AS $$
BEGIN
	DELETE FROM relations
	WHERE (source_kind = TG_ARGV[0] AND source_id = OLD.id) OR (target_kind = TG_ARGV[0] AND target_id = OLD.id);
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER notes_take_their_relations AFTER DELETE ON notes
	FOR EACH ROW EXECUTE FUNCTION relations_of_deleted_node('note');
CREATE TRIGGER concepts_take_their_relations AFTER DELETE ON concepts
	FOR EACH ROW EXECUTE FUNCTION relations_of_deleted_node('concept');

-- +goose Down
DROP TRIGGER IF EXISTS concepts_take_their_relations ON concepts;
DROP TRIGGER IF EXISTS notes_take_their_relations ON notes;
DROP FUNCTION IF EXISTS relations_of_deleted_node();
DROP TABLE IF EXISTS relations;
DROP TABLE IF EXISTS concept_keys;
DROP TABLE IF EXISTS concepts;
