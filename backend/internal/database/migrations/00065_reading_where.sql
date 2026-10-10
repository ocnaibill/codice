-- +goose Up
-- Where the person is, in words, to say it on the card of "continue reading" (DEC-148): the chapter they are in, and which unit of how many:
-- the page of a PDF or of a comic, the position of an EPUB (which has no pages). The reader is the one that knows both, so it sends them with
-- the locator; they are the place that locator points to, and move with it: a save that sends neither leaves neither.
ALTER TABLE reading_progress
	ADD COLUMN chapter VARCHAR(200),
	ADD COLUMN unit_index INTEGER,
	ADD COLUMN unit_total INTEGER,
	ADD CONSTRAINT reading_progress_unit_check CHECK (
		(unit_index IS NULL AND unit_total IS NULL) OR (unit_index >= 1 AND unit_total >= unit_index)
	);

-- +goose Down
ALTER TABLE reading_progress
	DROP CONSTRAINT IF EXISTS reading_progress_unit_check,
	DROP COLUMN IF EXISTS unit_total,
	DROP COLUMN IF EXISTS unit_index,
	DROP COLUMN IF EXISTS chapter;
