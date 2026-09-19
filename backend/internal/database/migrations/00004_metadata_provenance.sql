-- +goose Up
-- Metadata governance (DEC-019, DEC-026, RF-009, RN-008).
--
-- 1. A lock per editable field, so a value a person confirmed is never
--    overwritten by automatic extraction. title, author, series and cover
--    already had one.
ALTER TABLE works ADD COLUMN IF NOT EXISTS isbn_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS publisher_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS language_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS publication_date_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS description_lock BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Where the current value of a field came from: 'file' (read from the file
--    itself), 'manual' (an admin), or the name of the provider whose suggestion
--    was accepted. Values that predate this table have no entry: their origin
--    is unknown, so automatic extraction leaves them alone.
CREATE TABLE work_field_sources (
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	field VARCHAR(32) NOT NULL,
	source VARCHAR(32) NOT NULL,
	actor_id UUID,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (work_id, field)
);

-- 3. Suggestions from external providers, waiting for an admin. One row per
--    work, field, source and value: a rejected suggestion is remembered and is
--    not proposed again unless the provider returns a different value.
CREATE TABLE metadata_candidates (
	id BIGSERIAL PRIMARY KEY,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	field VARCHAR(32) NOT NULL,
	value TEXT NOT NULL,
	source VARCHAR(32) NOT NULL,
	evidence JSONB NOT NULL DEFAULT '{}',
	state VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'accepted', 'rejected')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	decided_at TIMESTAMPTZ,
	decided_by UUID,
	UNIQUE (work_id, field, source, value)
);
CREATE INDEX idx_metadata_candidates_pending ON metadata_candidates(work_id) WHERE state = 'pending';

-- +goose Down
DROP TABLE IF EXISTS metadata_candidates;
DROP TABLE IF EXISTS work_field_sources;
ALTER TABLE works DROP COLUMN IF EXISTS description_lock;
ALTER TABLE works DROP COLUMN IF EXISTS publication_date_lock;
ALTER TABLE works DROP COLUMN IF EXISTS language_lock;
ALTER TABLE works DROP COLUMN IF EXISTS publisher_lock;
ALTER TABLE works DROP COLUMN IF EXISTS isbn_lock;
