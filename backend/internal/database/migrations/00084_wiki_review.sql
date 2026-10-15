-- +goose Up
-- What an update of a wiki changed, for the staff to review (DEC-180, RF-052): the entities that are new and the sections that were
-- added to the ones that already existed. It is worked out when the bundle replaces the older content, from the two.
ALTER TABLE wiki_projects ADD COLUMN changes JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE wiki_projects DROP COLUMN changes;
