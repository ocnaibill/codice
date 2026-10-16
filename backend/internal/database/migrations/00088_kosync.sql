-- +goose Up
-- KOSync, the protocol KOReader syncs reading progress with (DEC-190).
-- A book is known to KOReader by a hash of a few samples of its file; the worker fills this in for every file.
ALTER TABLE files ADD COLUMN koreader_md5 CHAR(32);
CREATE INDEX idx_files_koreader_md5 ON files (koreader_md5) WHERE koreader_md5 IS NOT NULL;

-- KOReader sends the MD5 of what was typed as the password, and a person may type it as it is shown (in groups, in capitals): the token
-- keeps the SHA-256 of the MD5 of each way of typing it, so that what the device sends is found without the token being kept.
ALTER TABLE app_tokens ADD COLUMN kosync_hashes TEXT[];

-- What KOReader said last, as it said it, so that it is given back as it was (the place is the one it sent and the device is known to be
-- the same), until the person reads somewhere else, which clears both.
ALTER TABLE reading_progress ADD COLUMN kosync_progress TEXT, ADD COLUMN kosync_device_id TEXT;

-- +goose Down
ALTER TABLE reading_progress DROP COLUMN IF EXISTS kosync_device_id, DROP COLUMN IF EXISTS kosync_progress;
ALTER TABLE app_tokens DROP COLUMN IF EXISTS kosync_hashes;
DROP INDEX IF EXISTS idx_files_koreader_md5;
ALTER TABLE files DROP COLUMN IF EXISTS koreader_md5;
