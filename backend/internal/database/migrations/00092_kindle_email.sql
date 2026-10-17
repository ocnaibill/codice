-- +goose Up
-- The address a person's e-reader takes books at (a Kindle has one of its own, "name_abc@kindle.com"). It is the person's, and only they see it.
ALTER TABLE users ADD COLUMN kindle_email TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS kindle_email;
