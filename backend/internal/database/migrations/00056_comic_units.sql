-- +goose Up
-- What a comic or a manga work is (#187, DEC-134): which unit of a series it is, and whether it is a comic or a manga. Both are
-- chosen by hand by owner and admin, may be empty, and do not depend on the format of the file (a manga can be a PDF).
ALTER TABLE works ADD COLUMN unit VARCHAR(8) CHECK (unit IN ('volume', 'chapter', 'oneshot'));
ALTER TABLE works ADD COLUMN comic_kind VARCHAR(8) CHECK (comic_kind IN ('comic', 'manga'));

-- +goose Down
ALTER TABLE works DROP COLUMN IF EXISTS comic_kind;
ALTER TABLE works DROP COLUMN IF EXISTS unit;
