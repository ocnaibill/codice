-- +goose Up
-- Password reset without e-mail (DEC-063, RF-049). The person asks; the owner or
-- an admin approves and hands the link over outside the system. Approving makes a
-- single-use link valid for one hour; only its hash is stored. A person has at
-- most one open request, so repeated requests consolidate into one.
CREATE TABLE password_resets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	-- NULL while the request waits; then what was decided.
	decision VARCHAR(10) CHECK (decision IN ('approved', 'rejected', 'expired')),
	decided_by UUID,
	decided_at TIMESTAMPTZ,
	token_hash CHAR(64) UNIQUE,
	token_expires_at TIMESTAMPTZ,
	used_at TIMESTAMPTZ,
	CHECK (decision IS DISTINCT FROM 'approved' OR (token_hash IS NOT NULL AND token_expires_at IS NOT NULL)),
	CHECK (used_at IS NULL OR decision = 'approved')
);
CREATE UNIQUE INDEX password_resets_one_open_per_user ON password_resets(user_id) WHERE decision IS NULL;
CREATE INDEX idx_password_resets_created_at ON password_resets(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS password_resets;
