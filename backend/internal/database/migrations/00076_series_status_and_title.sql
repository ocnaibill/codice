-- +goose Up
-- What a person says of a series besides its name (DEC-170): how it stands in publication ("ongoing", "finished", "hiatus" or "cancelled") and its
-- title in the script of its language (ベルセルク), which the name of the collection, in the alphabet of the library, does not carry. Both are
-- chosen by hand; NULL is not said.
ALTER TABLE collections ADD COLUMN publication_status VARCHAR(10) CHECK (publication_status IN ('ongoing', 'finished', 'hiatus', 'cancelled'));
ALTER TABLE collections ADD COLUMN original_title VARCHAR(255);

-- +goose Down
ALTER TABLE collections DROP COLUMN IF EXISTS original_title;
ALTER TABLE collections DROP COLUMN IF EXISTS publication_status;
