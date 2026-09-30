-- +goose Up
-- Putting the files of one book under one work by hand (#37): a pair an admin joined or declared to be
-- different is remembered in the candidate table, so the scan never proposes it again, and an edition
-- remembers the work it came from, so separating it can put it back where it was.
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check
	CHECK (reason IN ('isbn', 'title_author', 'manual'));
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_state_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_state_check
	CHECK (state IN ('pending', 'dismissed', 'linked'));

ALTER TABLE editions ADD COLUMN former_work_id INTEGER REFERENCES works(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE editions DROP COLUMN former_work_id;
DELETE FROM duplicate_candidates WHERE reason = 'manual' OR state = 'linked';
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_state_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_state_check CHECK (state IN ('pending', 'dismissed'));
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check CHECK (reason IN ('isbn', 'title_author'));
