-- +goose Up
-- A position carries two clocks (spec 18.1): the server's, when it was received
-- (updated_at, already there), and the client's, when the reader says it happened.
-- Devices disagree about the time, so the second is kept only as information: the
-- order of writes is the revision, never a clock.
ALTER TABLE reading_progress ADD COLUMN client_updated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE reading_progress DROP COLUMN IF EXISTS client_updated_at;
