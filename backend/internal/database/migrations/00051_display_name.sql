-- +goose Up
-- How a person wants to be called (#179): the name of the greeting of the home ("Bom dia, Ana"). It is the person's own and it
-- is not the user name they sign in with, which does not change. NULL means "call me by my user name". The asking is recorded
-- apart: it happens once, at the first sign-in after this exists (an answer of "my user name" is an answer, and is not asked again).
ALTER TABLE users ADD COLUMN display_name VARCHAR(60);
ALTER TABLE users ADD COLUMN display_name_asked_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS display_name_asked_at;
ALTER TABLE users DROP COLUMN IF EXISTS display_name;
