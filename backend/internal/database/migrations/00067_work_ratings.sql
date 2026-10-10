-- +goose Up
-- The stars a person gives a work (DEC-154): from 1 to 5, one per person and work, that only the person sees and changes (DEC-017).
CREATE TABLE work_ratings (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	stars SMALLINT NOT NULL CHECK (stars BETWEEN 1 AND 5),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, work_id)
);
CREATE INDEX idx_work_ratings_work ON work_ratings(work_id);

-- +goose Down
DROP TABLE IF EXISTS work_ratings;
