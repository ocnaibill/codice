-- +goose Up
-- The answer to "find covers for this work" (DEC-174): staff ask from the metadata of a work, the worker asks the providers that are on and keeps
-- here what it found, for the page to show, so that a person chooses. One row per work, the last search; nothing is put on the work by it.
CREATE TABLE work_cover_searches (
	work_id INTEGER PRIMARY KEY REFERENCES works(id) ON DELETE CASCADE,
	-- What was searched for: what staff typed, or empty when the work's own title, author and ISBN were used.
	query TEXT NOT NULL DEFAULT '',
	-- pending: asked, not answered; done: answered (results may be empty); off: no provider is on; failed: the work could not be searched.
	state VARCHAR(8) NOT NULL CHECK (state IN ('pending', 'done', 'off', 'failed')),
	-- [{file, provider, source, title, author}]: file is the name of the picture in the folder that expires.
	results JSONB NOT NULL DEFAULT '[]'::jsonb,
	requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	finished_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS work_cover_searches;
