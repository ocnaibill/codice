-- +goose Up
-- The device of a KOReader write is named by the person (the name of the access, "Kindle"), which is what the reader of the web says
-- ("your Kindle stopped right here"); what KOReader calls the device (its model) is kept apart, because it is what it is told back.
ALTER TABLE reading_progress ADD COLUMN kosync_device TEXT;

-- +goose Down
ALTER TABLE reading_progress DROP COLUMN IF EXISTS kosync_device;
