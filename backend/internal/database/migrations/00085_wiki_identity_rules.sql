-- +goose Up
-- What the staff decided about who is who (DEC-181), kept for the work or series and not for one generation: a new generation replaces
-- the project, and these are applied to it again. An entity is found again by what it is called (name or alias, folded), since its
-- slug may change when the books change.
--   same:     the two are one entity (the second goes into the first).
--   distinct: the two are not the same, whatever the notes seem to say; the generator never joins them.
--   mention:  the name b is not one of entity a (a mention that is another entity's: "deus" in "Deus da mãe" that is Luc).
CREATE TABLE wiki_identity_rules (
	id BIGSERIAL PRIMARY KEY,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	kind VARCHAR(8) NOT NULL CHECK (kind IN ('same', 'distinct', 'mention')),
	a_name TEXT NOT NULL,
	a_key TEXT NOT NULL,
	b_name TEXT NOT NULL,
	b_key TEXT NOT NULL,
	decided_by UUID REFERENCES users(id) ON DELETE SET NULL,
	decided_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((collection_id IS NULL) <> (work_id IS NULL)),
	CHECK (a_key <> '' AND b_key <> '')
);
CREATE UNIQUE INDEX wiki_identity_rules_work ON wiki_identity_rules (work_id, kind, a_key, b_key) WHERE work_id IS NOT NULL;
CREATE UNIQUE INDEX wiki_identity_rules_collection ON wiki_identity_rules (collection_id, kind, a_key, b_key) WHERE collection_id IS NOT NULL;

-- +goose Down
DROP TABLE wiki_identity_rules;
