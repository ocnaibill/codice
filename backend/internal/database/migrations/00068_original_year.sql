-- +goose Up
-- The year a work was first published (DEC-156): the work's own, not the one of the edition in hand (editions.publication_date). It comes from a
-- provider that knows the work (the first publication year the Open Library keeps, the publication date of Wikidata) or is written by hand, and it
-- is confirmed and locked like the other descriptive fields. A negative year is before the common era.
ALTER TABLE works
	ADD COLUMN original_year SMALLINT,
	ADD COLUMN original_year_lock BOOLEAN NOT NULL DEFAULT FALSE,
	ADD CONSTRAINT works_original_year_range CHECK (original_year IS NULL OR (original_year BETWEEN -3000 AND 9999 AND original_year <> 0));

-- +goose Down
ALTER TABLE works
	DROP CONSTRAINT IF EXISTS works_original_year_range,
	DROP COLUMN IF EXISTS original_year_lock,
	DROP COLUMN IF EXISTS original_year;
