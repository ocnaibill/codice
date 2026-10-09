-- +goose Up
-- The origin of a suggestion says every provider that took part in it ("OpenLibrary + Wikidata + Wikipedia", DEC-141), and that does not
-- fit in 32 characters: the analysis failed on the insert. 64 is what the origin of an authority already had.
ALTER TABLE metadata_candidates ALTER COLUMN source TYPE VARCHAR(64);
ALTER TABLE work_field_sources ALTER COLUMN source TYPE VARCHAR(64);

-- +goose Down
-- Labels that no longer fit are cut, the way they were before the column was widened.
UPDATE metadata_candidates SET source = left(source, 32) WHERE length(source) > 32 AND NOT EXISTS (
	SELECT 1 FROM metadata_candidates o WHERE o.work_id = metadata_candidates.work_id AND o.field = metadata_candidates.field
		AND o.value = metadata_candidates.value AND o.source = left(metadata_candidates.source, 32) AND o.id <> metadata_candidates.id);
DELETE FROM metadata_candidates WHERE length(source) > 32;
UPDATE work_field_sources SET source = left(source, 32) WHERE length(source) > 32;
ALTER TABLE metadata_candidates ALTER COLUMN source TYPE VARCHAR(32);
ALTER TABLE work_field_sources ALTER COLUMN source TYPE VARCHAR(32);
