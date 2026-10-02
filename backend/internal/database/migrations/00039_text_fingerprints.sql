-- +goose Up
-- Telling that two files hold the same text, whatever their format or edition (#38): each file with published text
-- gets a fingerprint, a small sample of the runs of words in the body of the book (internal/fingerprint). Two files
-- that share most of it are proposed as the same work, with the evidence, for a person to decide (DEC-029).
--
-- A fingerprint is derived from the text, like the text: it is left out of the backups and made again when the
-- text is, and it is made again when the text it came from changes (the generation and the hash of the file are
-- kept to tell).
CREATE TABLE text_fingerprints (
	file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
	-- The published generation of the text, and the hash of the file it was read from, as they were when this was made.
	generation INTEGER NOT NULL,
	source_sha256 CHAR(64),
	-- The version of the method (fingerprint.Version).
	method SMALLINT NOT NULL,
	-- How many words of the body it was made from.
	words INTEGER NOT NULL,
	-- The sampled hashes, sorted. Empty when the text is too short to be told from another by it.
	sample BIGINT[] NOT NULL DEFAULT '{}'
);
CREATE INDEX text_fingerprints_sample ON text_fingerprints USING GIN (sample);

-- What made a pair worth proposing: the shares of text in common, for the person who decides.
ALTER TABLE duplicate_candidates ADD COLUMN evidence JSONB;
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check
	CHECK (reason IN ('isbn', 'title_author', 'manual', 'content'));

-- +goose Down
DELETE FROM duplicate_candidates WHERE reason = 'content';
ALTER TABLE duplicate_candidates DROP CONSTRAINT IF EXISTS duplicate_candidates_reason_check;
ALTER TABLE duplicate_candidates ADD CONSTRAINT duplicate_candidates_reason_check CHECK (reason IN ('isbn', 'title_author', 'manual'));
ALTER TABLE duplicate_candidates DROP COLUMN evidence;
DROP TABLE text_fingerprints;
