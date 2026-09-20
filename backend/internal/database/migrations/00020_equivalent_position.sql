-- +goose Up
-- What was accepted when a person asked to continue a book in another version, in another
-- format, edition or language (RF-042, DEC-030). The suggestion itself is worked out live and
-- never stored; this is the record of what was found and chosen, kept for history (nothing
-- reads it back to decide anything): the two locators, the version of each source, the method
-- and confidence of the match, and when it was accepted.
CREATE TABLE equivalent_position_acceptances (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	source_file_id BIGINT REFERENCES files(id) ON DELETE SET NULL,
	source_locator JSONB,
	source_locator_version SMALLINT,
	destination_file_id BIGINT REFERENCES files(id) ON DELETE SET NULL,
	destination_locator JSONB NOT NULL,
	destination_locator_version SMALLINT NOT NULL,
	method VARCHAR(16) NOT NULL CHECK (method IN ('text', 'anchors', 'structure')),
	confidence VARCHAR(8) NOT NULL CHECK (confidence IN ('low', 'medium', 'high')),
	match_precision VARCHAR(16) NOT NULL CHECK (match_precision IN ('passage', 'chapter')),
	accepted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_equivalent_position_user_work ON equivalent_position_acceptances(user_id, work_id);

-- +goose Down
DROP TABLE IF EXISTS equivalent_position_acceptances;
