-- +goose Up
-- A name that is made of several words does not always have a surname to tell apart: a mononym with a title, a pen name, an
-- organisation, a collective. When someone from the staff says so, the person stops being offered in the list of names whose
-- surname is still to be told apart (#64, DEC-094, DEC-139). The name stays as it is, and is shown as it is whatever the order
-- a person prefers.
ALTER TABLE person ADD COLUMN name_undivided BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE person DROP COLUMN name_undivided;
