-- +goose Up
-- Which address a login came from and when the session was last used (DEC-121): the list of sessions of an account
-- ("Sessões e dispositivos") shows both, so a person can tell their own devices from one that is not theirs and end it.
-- Both are empty for a session made before this: the list says "não registrado".
ALTER TABLE sessions ADD COLUMN ip VARCHAR(45);
ALTER TABLE sessions ADD COLUMN last_seen_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS last_seen_at;
ALTER TABLE sessions DROP COLUMN IF EXISTS ip;
