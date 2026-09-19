-- +goose Up
-- Invitations (DEC-055, DEC-059, RF-048): a single-use link, valid for seven days,
-- that creates one account. Only a hash of the secret is stored, so a copy of the
-- database cannot be used to redeem a link. Who may issue which role is decided
-- by the API from the issuer's current role; the database only guarantees that
-- an invitation never grants owner.
CREATE TABLE invitations (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	token_hash CHAR(64) NOT NULL UNIQUE,
	role VARCHAR(20) NOT NULL CHECK (role IN ('admin', 'reader')),
	email VARCHAR(255),
	created_by UUID REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	expires_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ,
	revoked_by UUID REFERENCES users(id) ON DELETE SET NULL,
	used_at TIMESTAMPTZ,
	used_by UUID REFERENCES users(id) ON DELETE SET NULL,
	CHECK (used_at IS NULL OR revoked_at IS NULL)
);
CREATE INDEX idx_invitations_created_at ON invitations(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS invitations;
