-- +goose Up
-- People who may be the same (#36): "Herbert, Frank" and "Frank Herbert" are made of the same words, and a file
-- that does not say what the words are (no role) does not say which is the surname. The system only proposes;
-- an administrator decides, and a pair they said are not the same is not proposed again. A merge is done in
-- the same way as joining two works: nothing of what was written is lost (the merged name becomes an alias).
CREATE TABLE person_merge_candidates (
	id BIGSERIAL PRIMARY KEY,
	person_a INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE,
	person_b INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE,
	state VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'dismissed')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	decided_at TIMESTAMPTZ,
	decided_by UUID,
	CHECK (person_a < person_b),
	UNIQUE (person_a, person_b)
);
CREATE INDEX idx_person_merge_candidates_pending ON person_merge_candidates (state) WHERE state = 'pending';

-- +goose Down
DROP TABLE person_merge_candidates;
