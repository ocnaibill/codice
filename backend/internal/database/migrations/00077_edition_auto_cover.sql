-- +goose Up
-- The cover the file (or a provider) gave an edition, kept apart from the one shown (DEC-173). `cover_url` is what is shown; when a person
-- chooses a cover by hand it is that one, and the worker, which honours the lock, keeps writing here what it finds, so that "back to the
-- cover of the file" has something to go back to without reading the file again.
ALTER TABLE editions ADD COLUMN auto_cover_url TEXT;
UPDATE editions SET auto_cover_url = cover_url WHERE cover_url IS NOT NULL;

-- +goose Down
ALTER TABLE editions DROP COLUMN IF EXISTS auto_cover_url;
