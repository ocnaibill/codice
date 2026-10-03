-- +goose Up
-- The color of a highlight (and of a note on a passage): one of four, the ones the reader paints the passage with. It is the
-- person's choice and says nothing else; terracotta is what a highlight has been until now.
ALTER TABLE notes ADD COLUMN color TEXT NOT NULL DEFAULT 'terracotta'
	CHECK (color IN ('terracotta', 'sepia', 'sage', 'indigo'));

-- +goose Down
ALTER TABLE notes DROP COLUMN IF EXISTS color;
