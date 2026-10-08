-- +goose Up
-- Personal collections (#207, DEC-130): the lists a person keeps, private to them, with an order of their own and a work in as
-- many as they like.
--
-- A work that leaves the library for good does not take its place out of a list: the list keeps what the person saw of it
-- (title and author), as the notes do (DEC-039). So the membership of a personal collection may point at no work any more
-- (`work_id` is NULL, `label` and `author_label` say what it was). The membership of an official collection never does: it is
-- a copy of the series of the work, and goes with the work. The rows get a number of their own, since the work is no longer
-- what tells them apart.
ALTER TABLE collection_works DROP CONSTRAINT collection_works_pkey;
ALTER TABLE collection_works ADD COLUMN id BIGSERIAL PRIMARY KEY;
ALTER TABLE collection_works ALTER COLUMN work_id DROP NOT NULL;
ALTER TABLE collection_works DROP CONSTRAINT collection_works_work_id_fkey;
ALTER TABLE collection_works ADD CONSTRAINT collection_works_work_id_fkey FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE SET NULL;
ALTER TABLE collection_works ADD COLUMN label TEXT;
ALTER TABLE collection_works ADD COLUMN author_label TEXT;
-- A work is in a collection once.
CREATE UNIQUE INDEX collection_works_once ON collection_works(collection_id, work_id) WHERE work_id IS NOT NULL;
-- Only a personal membership can outlive its work.
ALTER TABLE collection_works ADD CONSTRAINT collection_works_official_has_work CHECK (NOT official OR work_id IS NOT NULL);

-- When a work is deleted for good: what the person saw of it stays in their lists, and the official membership goes.
-- (It runs before the row goes, while the authors of the work can still be read.)
-- +goose StatementBegin
CREATE FUNCTION works_keep_personal_label() RETURNS trigger AS $$
BEGIN
	DELETE FROM collection_works WHERE work_id = OLD.id AND official;
	UPDATE collection_works SET
		label = OLD.original_title,
		author_label = (
			SELECT string_agg(p.name, ', ' ORDER BY c.position, p.name)
			FROM work_contributors c JOIN person p ON p.id = c.person_id
			WHERE c.work_id = OLD.id AND c.role = 'author')
	WHERE work_id = OLD.id AND NOT official;
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER works_keep_personal_label BEFORE DELETE ON works
FOR EACH ROW EXECUTE FUNCTION works_keep_personal_label();

-- +goose Down
DROP TRIGGER IF EXISTS works_keep_personal_label ON works;
DROP FUNCTION IF EXISTS works_keep_personal_label();
DELETE FROM collection_works WHERE work_id IS NULL;
ALTER TABLE collection_works DROP CONSTRAINT IF EXISTS collection_works_official_has_work;
DROP INDEX IF EXISTS collection_works_once;
ALTER TABLE collection_works DROP COLUMN IF EXISTS author_label;
ALTER TABLE collection_works DROP COLUMN IF EXISTS label;
ALTER TABLE collection_works DROP CONSTRAINT collection_works_work_id_fkey;
ALTER TABLE collection_works ADD CONSTRAINT collection_works_work_id_fkey FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE;
ALTER TABLE collection_works ALTER COLUMN work_id SET NOT NULL;
ALTER TABLE collection_works DROP COLUMN id;
ALTER TABLE collection_works ADD PRIMARY KEY (collection_id, work_id);
