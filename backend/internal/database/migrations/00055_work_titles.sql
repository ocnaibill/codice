-- +goose Up
-- Alternative titles of a work (#185, DEC-131): the other names it goes by, in other languages or in other places, to be found by
-- them in the search and to be recognised as the same work by the duplicate detection. The main title stays on the work
-- (works.original_title); the titles of the editions (editions.title, each with its language) are alternative titles of the work
-- already, and are read from there, not copied here.
CREATE TABLE work_titles (
	id BIGSERIAL PRIMARY KEY,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	title VARCHAR(512) NOT NULL,
	-- The language the title is in, as a code (pt, pt-BR, ja…), or NULL when it is not said.
	language VARCHAR(16),
	-- Where it came from: 'manual' (an admin) or the name of the provider whose suggestion was accepted.
	source VARCHAR(32) NOT NULL DEFAULT 'manual',
	-- The title as compared (case, accents, punctuation and a trailing note in parentheses do not tell two titles apart).
	title_key TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (work_id, title_key)
);

-- +goose Down
DROP TABLE IF EXISTS work_titles;
