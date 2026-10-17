-- +goose Up
-- A note brought from a reading device (a Kindle's My Clippings.txt, the highlights exported by KOReader) has a key made of the passage and
-- what was written on it, so that importing the same file again, or the same passage from the other device, brings nothing twice.
ALTER TABLE notes ADD COLUMN import_key TEXT;
CREATE UNIQUE INDEX notes_import_key ON notes (user_id, work_id, import_key) WHERE import_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS notes_import_key;
ALTER TABLE notes DROP COLUMN IF EXISTS import_key;
