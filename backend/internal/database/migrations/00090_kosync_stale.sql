-- +goose Up
-- A device that reads without a network says where it was only when it comes back, and what it sends carries no time (DEC-191). So the
-- server keeps, for each access (a device), the revision of a book that it saw the last time it asked, and a send from a device that did
-- not see the newest save of someone else is not written over it: it is kept here, for the person to choose.
CREATE TABLE kosync_seen (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	token_id UUID NOT NULL REFERENCES app_tokens(id) ON DELETE CASCADE,
	revision BIGINT NOT NULL,
	seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, file_id, token_id)
);

CREATE TABLE kosync_pending (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	device TEXT NOT NULL,          -- the name the person gave the access
	position TEXT NOT NULL,
	locator JSONB NOT NULL,
	locator_version SMALLINT NOT NULL,
	percent_complete REAL NOT NULL,
	kosync_progress TEXT,
	kosync_device_id TEXT,
	kosync_device TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, file_id)
);

-- +goose Down
DROP TABLE IF EXISTS kosync_pending;
DROP TABLE IF EXISTS kosync_seen;
