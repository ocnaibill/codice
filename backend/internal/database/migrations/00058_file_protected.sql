-- +goose Up
-- A PDF that asks for a password to open is kept as it came and opens in the reader with the password, but the server cannot read its
-- text nor draw its cover. The analysis of the file says so, once, and the sheet and the notices say it in words.
ALTER TABLE files ADD COLUMN protected BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE files DROP COLUMN protected;
