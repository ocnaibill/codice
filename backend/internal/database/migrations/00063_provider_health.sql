-- +goose Up
-- How each metadata provider answered the last time the worker asked it (DEC-144), for the administration to say it: the key was refused, the
-- quota is used up, the service did not answer. One row per provider, written by the worker at every request; it never holds a key or a title.
CREATE TABLE provider_health (
	provider VARCHAR(32) PRIMARY KEY,
	-- ok: it answered; key: the key was refused (401, 403); quota: too many requests (429); down: it did not answer (network, 5xx);
	-- error: it answered with another error.
	state VARCHAR(8) NOT NULL CHECK (state IN ('ok', 'key', 'quota', 'down', 'error')),
	status INTEGER NOT NULL DEFAULT 0,
	problem TEXT NOT NULL DEFAULT '',
	checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_ok_at TIMESTAMPTZ,
	-- How many searches in a row it answered, well, with nothing at all.
	empty_streak INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE provider_health;
