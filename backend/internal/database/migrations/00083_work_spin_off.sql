-- +goose Up
-- A work that is not of the main line of its series (DEC-188): a spin-off, a book of short stories beside the series, something that is
-- not canon. The wiki (DEC-179) reads the books of a series in order and cuts what lies beyond the reader; a book outside that line has
-- a line of its own and shows only when the reader asks for it. It was a position with a fraction (3.5) that said so, which depended on
-- the order being written by hand: it is now a choice of the person, next to the series.
ALTER TABLE works ADD COLUMN spin_off BOOLEAN NOT NULL DEFAULT FALSE;

-- What the position said stays said: the works at a position with a fraction in an official series are marked, once.
UPDATE works SET spin_off = TRUE WHERE id IN (
	SELECT cw.work_id FROM collection_works cw
	WHERE cw.official AND cw.work_id IS NOT NULL AND cw.position IS NOT NULL AND cw.position <> floor(cw.position));

-- +goose Down
ALTER TABLE works DROP COLUMN spin_off;
