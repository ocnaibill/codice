-- +goose Up
-- The identifier a reference source has for a person (#63): Open Library says "OL79034A" for Frank Herbert, in
-- whatever way a file writes the name. It is kept only when an administrator accepts the author the source
-- suggested for a work, so a person holds a key a human confirmed for that name. A person may hold the keys of
-- several sources. Two people holding the same key are very probably one, which is a better reason to propose
-- a merge than sharing the same words: the pair says why it was proposed.
CREATE TABLE person_authority (
	person_id INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE,
	scheme VARCHAR(32) NOT NULL,
	value VARCHAR(64) NOT NULL,
	source VARCHAR(64) NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (person_id, scheme, value)
);
CREATE INDEX idx_person_authority_key ON person_authority (scheme, value);

ALTER TABLE person_merge_candidates
	ADD COLUMN reason VARCHAR(12) NOT NULL DEFAULT 'words' CHECK (reason IN ('words', 'authority')),
	ADD COLUMN evidence JSONB;

-- +goose Down
ALTER TABLE person_merge_candidates DROP COLUMN evidence, DROP COLUMN reason;
DROP TABLE person_authority;
