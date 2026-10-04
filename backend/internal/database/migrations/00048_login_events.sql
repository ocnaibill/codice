-- +goose Up
-- The record of sign-ins (DEC-121, issue #137): who tried to get in, from where, by which way, and how it went, the
-- failures included, for the owner and the administrators to read. It is a log, not state: it is kept for a time the owner
-- sets (90 days by default) and cut to a size, and it is left out of the backups (like the sessions it is made of).
--
-- A run of the same failure (same address, same account, same way) is ONE row with a count, so that someone guessing
-- passwords fills one line, not the table.
CREATE TABLE login_events (
	id BIGSERIAL PRIMARY KEY,
	at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	count INTEGER NOT NULL DEFAULT 1 CHECK (count >= 1),
	result VARCHAR(24) NOT NULL CHECK (result IN
		('success', 'bad_password', 'unknown_user', 'blocked', 'directory_unavailable', 'rate_limited', 'link_offered', 'bad_app_token')),
	method VARCHAR(8) NOT NULL CHECK (method IN ('local', 'ldap', 'invite', 'setup', 'app')),
	-- The account, when there is one. Deleting the account deletes its record: it is personal data (DEC-060).
	user_id UUID REFERENCES users(id) ON DELETE CASCADE,
	-- The name that was typed, ONLY for a name with no account, short and plain (internal/logins.TypedName): what was
	-- typed in a field for a name can be a password.
	typed_name VARCHAR(24),
	ip VARCHAR(45),
	user_agent VARCHAR(255)
);
CREATE INDEX login_events_last_at ON login_events (last_at DESC, id DESC);
CREATE INDEX login_events_user ON login_events (user_id, last_at DESC) WHERE user_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS login_events;
