-- +goose Up
-- What a person says of a collection (DEC-163): a few lines under the name, on the page of the collection. Nothing is
-- written to a work or to a file; NULL is no description.
ALTER TABLE collections ADD COLUMN description TEXT;

-- +goose Down
ALTER TABLE collections DROP COLUMN IF EXISTS description;
