-- +goose Up
-- How a person wants the text of a book to look (#106): the page color, the font, the size, the space between lines, the margins
-- and whether the lines are justified. It is the person's own (an account chooses it, and no other sees it), and it follows them
-- from one device to another; what it holds is checked by internal/reading, not here.
ALTER TABLE users ADD COLUMN reader_prefs JSONB;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS reader_prefs;
