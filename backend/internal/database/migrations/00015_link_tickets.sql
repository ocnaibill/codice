-- +goose Up
-- Linking a directory identity to an existing local account (DEC-075). When a
-- person proves the directory password, no session is opened yet: they get a
-- short-lived ticket and must also prove the local password. Only after both are
-- proven is the identity linked. The ticket is single-use, and a wrong local
-- password counts against it, so it cannot be used to guess the password.
CREATE TABLE link_tickets (
	id BIGSERIAL PRIMARY KEY,
	token_hash CHAR(64) NOT NULL UNIQUE,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	provider VARCHAR(64) NOT NULL,
	subject VARCHAR(255) NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	expires_at TIMESTAMPTZ NOT NULL,
	attempts SMALLINT NOT NULL DEFAULT 0,
	used_at TIMESTAMPTZ
);
CREATE INDEX idx_link_tickets_user ON link_tickets(user_id);

-- +goose Down
DROP TABLE IF EXISTS link_tickets;
