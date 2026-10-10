-- +goose Up
-- Where a chapter of a series sits (DEC-169): the number of the bound volume (tankōbon) that collected it, and the story arc it belongs to. Both are
-- chosen by hand, in ranges of chapters or one work at a time, and may be empty; neither depends on the format of the file.
ALTER TABLE works ADD COLUMN volume_number REAL CHECK (volume_number > 0 AND volume_number < 10000);
ALTER TABLE works ADD COLUMN story_arc VARCHAR(255);

-- +goose Down
ALTER TABLE works DROP COLUMN IF EXISTS story_arc;
ALTER TABLE works DROP COLUMN IF EXISTS volume_number;
