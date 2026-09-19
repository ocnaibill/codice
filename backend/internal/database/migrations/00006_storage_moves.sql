-- +goose Up
-- Managed storage layout (DEC-036, DEC-037, DEC-065).
--
-- 1. A location can be in the middle of a move. The intended destination is
--    written BEFORE the file is touched, so a crash between moving the file and
--    confirming it in the database can be settled afterwards.
ALTER TABLE storage_locations DROP CONSTRAINT IF EXISTS storage_locations_state_check;
ALTER TABLE storage_locations ADD CONSTRAINT storage_locations_state_check
	CHECK (state IN ('ok', 'missing', 'trashed', 'moving', 'conflict'));
ALTER TABLE storage_locations ADD COLUMN moving_to TEXT;

-- 2. When the file was last put at its layout path. NULL means it is still
--    where ingestion left it and has not been organized yet.
ALTER TABLE files ADD COLUMN organized_at TIMESTAMPTZ;

-- 3. The queue can be asked for some job types only, and the limit on
--    simultaneous jobs counts only those types. The Python worker takes
--    'ingest'; the API process takes the file-system jobs. One busy kind does
--    not starve the other.
DROP FUNCTION IF EXISTS jobs_claim(TEXT, INTEGER, INTEGER);
-- +goose StatementBegin
CREATE FUNCTION jobs_claim(p_owner TEXT, p_lease_seconds INTEGER, p_max_running INTEGER, p_types TEXT[] DEFAULT NULL)
RETURNS SETOF jobs AS $$
DECLARE
	j jobs;
BEGIN
	PERFORM pg_advisory_xact_lock(hashtext('jobs_claim'));

	UPDATE jobs SET state = 'cancelled', lease_owner = NULL, lease_expires_at = NULL,
	       finished_at = now(), updated_at = now()
	WHERE state = 'running' AND lease_expires_at < now() AND cancel_requested;

	UPDATE jobs SET state = 'failed', error_kind = 'temporary',
	       last_error = 'the worker stopped answering and no attempts are left',
	       lease_owner = NULL, lease_expires_at = NULL, finished_at = now(), updated_at = now()
	WHERE state = 'running' AND lease_expires_at < now() AND attempts >= max_attempts;

	IF (SELECT count(*) FROM jobs
	    WHERE state = 'running' AND lease_expires_at >= now()
	      AND (p_types IS NULL OR type = ANY (p_types))) >= p_max_running THEN
		RETURN;
	END IF;

	SELECT * INTO j FROM jobs
	WHERE ((state = 'pending' AND run_at <= now())
	    OR (state = 'running' AND lease_expires_at < now()))
	  AND (p_types IS NULL OR type = ANY (p_types))
	ORDER BY priority DESC, run_at, id
	FOR UPDATE SKIP LOCKED
	LIMIT 1;
	IF NOT FOUND THEN
		RETURN;
	END IF;

	UPDATE jobs
	SET state = 'running', attempts = attempts + 1, lease_owner = p_owner,
	    lease_expires_at = now() + make_interval(secs => p_lease_seconds),
	    started_at = now(), updated_at = now()
	WHERE id = j.id
	RETURNING * INTO j;
	RETURN NEXT j;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- 4. When a file has been analysed its metadata is known, so it is put in its
--    place in the layout. The job is created here, by the queue itself, so the
--    worker does not need to know about it.
-- +goose StatementBegin
CREATE FUNCTION jobs_organize_after_ingest() RETURNS trigger AS $$
BEGIN
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('organize', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER jobs_organize_after_ingest AFTER UPDATE OF state ON jobs
FOR EACH ROW WHEN (NEW.type = 'ingest' AND NEW.state = 'succeeded' AND OLD.state IS DISTINCT FROM 'succeeded' AND NEW.work_id IS NOT NULL)
EXECUTE FUNCTION jobs_organize_after_ingest();

-- +goose Down
DROP TRIGGER IF EXISTS jobs_organize_after_ingest ON jobs;
DROP FUNCTION IF EXISTS jobs_organize_after_ingest();
DROP FUNCTION IF EXISTS jobs_claim(TEXT, INTEGER, INTEGER, TEXT[]);
-- +goose StatementBegin
CREATE FUNCTION jobs_claim(p_owner TEXT, p_lease_seconds INTEGER, p_max_running INTEGER)
RETURNS SETOF jobs AS $$
DECLARE
	j jobs;
BEGIN
	PERFORM pg_advisory_xact_lock(hashtext('jobs_claim'));
	UPDATE jobs SET state = 'cancelled', lease_owner = NULL, lease_expires_at = NULL,
	       finished_at = now(), updated_at = now()
	WHERE state = 'running' AND lease_expires_at < now() AND cancel_requested;
	UPDATE jobs SET state = 'failed', error_kind = 'temporary',
	       last_error = 'the worker stopped answering and no attempts are left',
	       lease_owner = NULL, lease_expires_at = NULL, finished_at = now(), updated_at = now()
	WHERE state = 'running' AND lease_expires_at < now() AND attempts >= max_attempts;
	IF (SELECT count(*) FROM jobs WHERE state = 'running' AND lease_expires_at >= now()) >= p_max_running THEN
		RETURN;
	END IF;
	SELECT * INTO j FROM jobs
	WHERE (state = 'pending' AND run_at <= now())
	   OR (state = 'running' AND lease_expires_at < now())
	ORDER BY priority DESC, run_at, id
	FOR UPDATE SKIP LOCKED
	LIMIT 1;
	IF NOT FOUND THEN
		RETURN;
	END IF;
	UPDATE jobs
	SET state = 'running', attempts = attempts + 1, lease_owner = p_owner,
	    lease_expires_at = now() + make_interval(secs => p_lease_seconds),
	    started_at = now(), updated_at = now()
	WHERE id = j.id
	RETURNING * INTO j;
	RETURN NEXT j;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
ALTER TABLE files DROP COLUMN IF EXISTS organized_at;
UPDATE storage_locations SET state = 'ok', moving_to = NULL WHERE state IN ('moving', 'conflict');
ALTER TABLE storage_locations DROP COLUMN IF EXISTS moving_to;
ALTER TABLE storage_locations DROP CONSTRAINT IF EXISTS storage_locations_state_check;
ALTER TABLE storage_locations ADD CONSTRAINT storage_locations_state_check CHECK (state IN ('ok', 'missing', 'trashed'));
