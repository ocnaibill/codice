-- +goose Up
-- Recoverable trash for managed files (DEC-041 to DEC-043, RF-045).
--
-- Sending a file to the trash keeps its work, edition, file and location in the
-- database (the location is marked 'trashed'), so it can be restored with its
-- history. The bytes move under <storage>/.trash. Only emptying the trash, or
-- the optional automatic cleanup, deletes anything for good.
CREATE TABLE trash_items (
	id BIGSERIAL PRIMARY KEY,
	-- A trashed file keeps its record. An orphan (bytes the catalog did not know)
	-- has no file.
	file_id BIGINT UNIQUE REFERENCES files(id) ON DELETE CASCADE,
	kind VARCHAR(8) NOT NULL DEFAULT 'file' CHECK (kind IN ('file', 'orphan')),
	original_path TEXT NOT NULL,
	trash_path TEXT NOT NULL UNIQUE,
	size_bytes BIGINT NOT NULL DEFAULT 0,
	trashed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	trashed_by UUID,
	-- Set on entry from the policy in force then (DEC-043); NULL means never.
	purge_after TIMESTAMPTZ
);
CREATE INDEX idx_trash_items_purge_after ON trash_items(purge_after) WHERE purge_after IS NOT NULL;

-- Instance settings the owner controls. The trash policy lives here:
-- {"enabled": false, "days": 30}, off by default.
CREATE TABLE settings (
	key TEXT PRIMARY KEY,
	value JSONB NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_by UUID
);

-- +goose Down
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS trash_items;
UPDATE storage_locations SET state = 'ok' WHERE state = 'trashed';
