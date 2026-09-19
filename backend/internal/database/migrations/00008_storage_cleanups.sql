-- +goose Up
-- Origins that must still be removed after a file was moved into the managed
-- storage (DEC-033). The row is written in the same transaction that switches the
-- file to its managed location, so a crash between the two cannot lose track of
-- the original; it is deleted once the original is really gone. The hash lets a
-- retry confirm it is still the same file before deleting anything.
CREATE TABLE storage_cleanups (
	id BIGSERIAL PRIMARY KEY,
	path TEXT NOT NULL UNIQUE,
	sha256 CHAR(64) NOT NULL,
	file_id BIGINT REFERENCES files(id) ON DELETE SET NULL,
	reason TEXT NOT NULL DEFAULT 'waiting to be removed',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	attempted_at TIMESTAMPTZ
);

-- +goose Down
DROP TABLE IF EXISTS storage_cleanups;
