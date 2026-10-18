-- +goose Up
-- A reading device writes the annotations of its owner (DEC-199): a highlight it made is kept under a reference of its own, so that sending it
-- again changes the same note and deleting it deletes only that one. The exact points the device gave are kept, to be given back as they came
-- (the locator of the Códice has the paragraph; the device draws from the exact start and end).
ALTER TABLE notes ADD COLUMN device_ref TEXT;
ALTER TABLE notes ADD COLUMN device_pos0 TEXT;
ALTER TABLE notes ADD COLUMN device_pos1 TEXT;
ALTER TABLE notes ADD COLUMN device_name TEXT;
ALTER TABLE notes ADD COLUMN device_at TIMESTAMPTZ;
CREATE UNIQUE INDEX idx_notes_device_ref ON notes (user_id, file_id, device_ref) WHERE device_ref IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_notes_device_ref;
ALTER TABLE notes DROP COLUMN IF EXISTS device_at;
ALTER TABLE notes DROP COLUMN IF EXISTS device_name;
ALTER TABLE notes DROP COLUMN IF EXISTS device_pos1;
ALTER TABLE notes DROP COLUMN IF EXISTS device_pos0;
ALTER TABLE notes DROP COLUMN IF EXISTS device_ref;
