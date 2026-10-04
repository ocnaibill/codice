-- +goose Up
-- "Exportar meus dados" (proposta de telas, 3.5): a person asks for a file with everything that is theirs. The file is made by a
-- job, kept for 24 hours, and only that person can take it. This table says where each request is; the file itself is not in the
-- database (and not in the backups: it is as old as the day).
CREATE TABLE data_exports (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	state VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'ready', 'failed')),
	bytes BIGINT,
	-- What went wrong, for the logs and the owner of the server; the person is told only that it failed.
	error TEXT,
	requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	ready_at TIMESTAMPTZ,
	expires_at TIMESTAMPTZ,
	downloaded_at TIMESTAMPTZ,
	-- A request that a newer one replaced: its file is gone and the screen does not show it, but the row stays for the hour, because the
	-- limit on requests counts rows.
	superseded BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX data_exports_user ON data_exports (user_id, requested_at DESC);

-- +goose Down
DROP TABLE IF EXISTS data_exports;
