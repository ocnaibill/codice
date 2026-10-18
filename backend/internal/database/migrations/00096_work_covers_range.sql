-- +goose Up
-- The part of the story a work contains (DEC-202): "covers chapter X to Y". A chapter covers itself (its number in the series), a volume
-- covers the chapters it collected, a complete edition covers them all. Both are chosen by hand, may be empty, and come together: a range has
-- a first and a last, the last not before the first. Decimals are allowed (10.5), as in the number of a chapter.
ALTER TABLE works ADD COLUMN covers_from REAL;
ALTER TABLE works ADD COLUMN covers_to REAL;
ALTER TABLE works ADD CONSTRAINT works_covers_range CHECK (
	(covers_from IS NULL AND covers_to IS NULL)
	OR (covers_from > 0 AND covers_to >= covers_from AND covers_to < 10000)
);

-- +goose Down
ALTER TABLE works DROP CONSTRAINT IF EXISTS works_covers_range;
ALTER TABLE works DROP COLUMN IF EXISTS covers_to;
ALTER TABLE works DROP COLUMN IF EXISTS covers_from;
