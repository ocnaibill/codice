-- +goose Up
-- Finding a file that is another file's translation (#38): besides the sample of its words, the fingerprint of a file
-- keeps the rare names and numbers of its book (those that are in a few of its segments), which a translation keeps
-- and another book does not. They are how the files that may be translations of each other are found cheaply, before
-- they are read against each other (equivalence.ReadParallel).
ALTER TABLE text_fingerprints ADD COLUMN names TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX text_fingerprints_names ON text_fingerprints USING GIN (names);

ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check
	CHECK (reason IN ('isbn', 'title_author', 'manual', 'content', 'translation'));

-- +goose Down
DELETE FROM duplicate_candidates WHERE reason = 'translation';
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check
	CHECK (reason IN ('isbn', 'title_author', 'manual', 'content'));
DROP INDEX text_fingerprints_names;
ALTER TABLE text_fingerprints DROP COLUMN names;
