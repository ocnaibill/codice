-- +goose Up
-- The answer to "find this person on Wikidata" (DEC-168): staff ask from the page of a person, the worker searches by name and keeps here the
-- candidates for the page to show, so that a human chooses which one is the author. One row per person, the last search; nothing is linked by it.
CREATE TABLE person_profile_searches (
	person_id INTEGER PRIMARY KEY REFERENCES person(id) ON DELETE CASCADE,
	-- What was searched for: the name of the person, or what staff typed instead.
	query TEXT NOT NULL,
	-- pending: asked, not answered; done: answered (results may be empty); off: Wikidata is turned off; failed: it did not answer.
	state VARCHAR(8) NOT NULL CHECK (state IN ('pending', 'done', 'off', 'failed')),
	results JSONB NOT NULL DEFAULT '[]'::jsonb,
	requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	finished_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS person_profile_searches;
