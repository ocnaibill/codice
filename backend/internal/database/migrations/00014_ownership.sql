-- +goose Up
-- Transferring ownership (RF-038, DEC-057, DEC-058) and notices that a person
-- must see at their next sign-in (a recovery done on the server is one).
--
-- A transfer has two steps: the owner starts it, confirming with their password,
-- and the chosen account accepts by signing in again. Nothing changes until it
-- is accepted, and the owner can cancel. Only one can be pending at a time.
CREATE TABLE ownership_transfers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	from_user UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	to_user UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	-- The owner chooses, at the moment of the transfer, what they become (DEC-058).
	former_role VARCHAR(10) NOT NULL CHECK (former_role IN ('admin', 'reader')),
	state VARCHAR(10) NOT NULL DEFAULT 'pending'
		CHECK (state IN ('pending', 'accepted', 'cancelled', 'declined', 'expired')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	expires_at TIMESTAMPTZ NOT NULL,
	decided_at TIMESTAMPTZ,
	CHECK (from_user <> to_user)
);
CREATE UNIQUE INDEX ownership_one_pending ON ownership_transfers ((true)) WHERE state = 'pending';

CREATE TABLE security_notices (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	kind VARCHAR(40) NOT NULL,
	details JSONB NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	acknowledged_at TIMESTAMPTZ
);
CREATE INDEX idx_security_notices_open ON security_notices(user_id) WHERE acknowledged_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS security_notices;
DROP TABLE IF EXISTS ownership_transfers;
