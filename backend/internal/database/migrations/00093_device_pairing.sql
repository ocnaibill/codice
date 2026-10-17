-- +goose Up
-- A reader asks to be connected with a code it shows on its screen, and a person who is signed in approves it (DEC-194). The code the
-- reader holds is kept only as a hash; the access it gets is made when it is delivered and never kept.
CREATE TABLE device_pairings (
	id BIGSERIAL PRIMARY KEY,
	device_code_hash BYTEA NOT NULL UNIQUE,
	user_code TEXT NOT NULL UNIQUE,
	name TEXT,
	platform TEXT,
	client TEXT,
	request_ip TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	expires_at TIMESTAMPTZ NOT NULL,
	last_poll_at TIMESTAMPTZ,
	approved_by UUID REFERENCES users(id) ON DELETE CASCADE,
	approved_at TIMESTAMPTZ,
	denied_at TIMESTAMPTZ,
	consumed_at TIMESTAMPTZ
);
CREATE INDEX idx_device_pairings_expires ON device_pairings(expires_at);

-- +goose Down
DROP TABLE IF EXISTS device_pairings;
