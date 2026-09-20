-- +goose Up
-- Marginalia (RF-016, RF-017): a note is no longer only a saved quotation. It has a kind
-- (a quotation kept as a highlight, a note in the person's own words, a bookmark that is only
-- a place), a body in Markdown, personal tags, and an address inside a file (a locator, spec
-- 5.3). The reference to the work's title and author already lives on the note, so all of it
-- outlives the work (DEC-039).
ALTER TABLE notes ADD COLUMN kind VARCHAR(12) NOT NULL DEFAULT 'note'
	CHECK (kind IN ('note', 'highlight', 'bookmark'));
ALTER TABLE notes ADD COLUMN body TEXT NOT NULL DEFAULT '';
ALTER TABLE notes ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE notes ADD COLUMN locator JSONB;
ALTER TABLE notes ADD COLUMN locator_version SMALLINT;
ALTER TABLE notes ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- What existed were saved quotations.
UPDATE notes SET kind = 'highlight', updated_at = created_at;

-- A bookmark is only a place: it has neither quotation nor text.
ALTER TABLE notes ALTER COLUMN quote DROP NOT NULL;
ALTER TABLE notes ADD CONSTRAINT notes_has_content
	CHECK (kind = 'bookmark' OR COALESCE(quote, '') <> '' OR body <> '');
ALTER TABLE notes ADD CONSTRAINT notes_bookmark_has_place
	CHECK (kind <> 'bookmark' OR locator IS NOT NULL);
ALTER TABLE notes ADD CONSTRAINT notes_locator_has_version
	CHECK (locator IS NULL OR locator_version IS NOT NULL);

CREATE INDEX idx_notes_user_source ON notes(user_id, source_work_id);
CREATE INDEX idx_notes_tags ON notes USING GIN (tags);

-- +goose Down
DROP INDEX IF EXISTS idx_notes_tags;
DROP INDEX IF EXISTS idx_notes_user_source;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_locator_has_version;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_bookmark_has_place;
ALTER TABLE notes DROP CONSTRAINT IF EXISTS notes_has_content;
DELETE FROM notes WHERE kind = 'bookmark';
-- The person's own words go back into the only text column the old shape had.
UPDATE notes SET quote = CASE WHEN COALESCE(quote, '') = '' THEN body ELSE quote END WHERE quote IS NULL OR quote = '';
ALTER TABLE notes ALTER COLUMN quote SET NOT NULL;
ALTER TABLE notes DROP COLUMN IF EXISTS updated_at;
ALTER TABLE notes DROP COLUMN IF EXISTS locator_version;
ALTER TABLE notes DROP COLUMN IF EXISTS locator;
ALTER TABLE notes DROP COLUMN IF EXISTS tags;
ALTER TABLE notes DROP COLUMN IF EXISTS body;
ALTER TABLE notes DROP COLUMN IF EXISTS kind;
