-- +goose Up
-- The title of an edition is, to begin with, what the file brought (its own title, or its file name). When an owner or an admin
-- writes it, it is theirs: it is the name the work goes by while that edition is the one being read (#185, DEC-131).
ALTER TABLE editions ADD COLUMN title_manual BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE editions DROP COLUMN title_manual;
