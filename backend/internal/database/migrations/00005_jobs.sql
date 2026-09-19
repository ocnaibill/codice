-- +goose Up
-- Jobs: PostgreSQL is the source of truth for asynchronous work (DEC-066 to
-- DEC-069). Redis only wakes workers up; it can be restarted or emptied without
-- losing anything, because a worker also polls this table.
--
-- The state machine lives here, as functions, so there is one implementation
-- that every worker (whatever its language) calls and that is tested once.
CREATE TABLE jobs (
	id BIGSERIAL PRIMARY KEY,
	type VARCHAR(32) NOT NULL,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	payload JSONB NOT NULL DEFAULT '{}',
	priority SMALLINT NOT NULL DEFAULT 0,
	state VARCHAR(12) NOT NULL DEFAULT 'pending'
		CHECK (state IN ('pending', 'running', 'succeeded', 'failed', 'cancelled')),
	attempts INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 3,
	run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	lease_owner TEXT,
	lease_expires_at TIMESTAMPTZ,
	cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
	last_error TEXT,
	error_kind VARCHAR(12) CHECK (error_kind IN ('temporary', 'permanent')),
	created_by UUID,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ
);
-- At most one live job of a kind per work: enqueueing twice is not a way to
-- process the same file twice at once.
CREATE UNIQUE INDEX jobs_one_active_per_work ON jobs (type, work_id) WHERE state IN ('pending', 'running');
CREATE INDEX jobs_claimable ON jobs (priority DESC, run_at, id) WHERE state IN ('pending', 'running');
CREATE INDEX jobs_state_created ON jobs (state, created_at DESC);

-- Takes the next job for a worker, or nothing. Manual work goes first (higher
-- priority), then the oldest. A job whose worker vanished (lease expired) is
-- taken over, unless it has no attempts left, in which case it fails; one that
-- was being cancelled ends cancelled. At most p_max_running jobs run at once.
-- +goose StatementBegin
CREATE FUNCTION jobs_claim(p_owner TEXT, p_lease_seconds INTEGER, p_max_running INTEGER)
RETURNS SETOF jobs AS $$
DECLARE
	j jobs;
BEGIN
	-- One claim at a time, so the running limit cannot be exceeded by a race.
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

-- Extends the lease. Returns 'ok', 'cancel' (someone asked to stop: finish
-- cooperatively) or 'lost' (this worker no longer owns the job).
-- +goose StatementBegin
CREATE FUNCTION jobs_heartbeat(p_id BIGINT, p_owner TEXT, p_lease_seconds INTEGER)
RETURNS TEXT AS $$
DECLARE
	cancelling BOOLEAN;
BEGIN
	UPDATE jobs SET lease_expires_at = now() + make_interval(secs => p_lease_seconds), updated_at = now()
	WHERE id = p_id AND lease_owner = p_owner AND state = 'running'
	RETURNING cancel_requested INTO cancelling;
	IF NOT FOUND THEN
		RETURN 'lost';
	END IF;
	RETURN CASE WHEN cancelling THEN 'cancel' ELSE 'ok' END;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Only the current owner can finish a job; a worker that lost its lease cannot
-- overwrite the result of whoever took over.
-- +goose StatementBegin
CREATE FUNCTION jobs_complete(p_id BIGINT, p_owner TEXT) RETURNS BOOLEAN AS $$
BEGIN
	UPDATE jobs SET state = 'succeeded', lease_owner = NULL, lease_expires_at = NULL,
	       last_error = NULL, error_kind = NULL, finished_at = now(), updated_at = now()
	WHERE id = p_id AND lease_owner = p_owner AND state = 'running';
	RETURN FOUND;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Records a failure. A permanent error is never retried. A temporary one is
-- retried up to max_attempts times, waiting 30 seconds, 2 minutes and 10
-- minutes; after that the job stays failed until an admin reruns it (DEC-068).
-- Returns 'retry', 'failed' or 'lost'. The message is cut so a stack trace or
-- a huge payload cannot fill the table.
-- +goose StatementBegin
CREATE FUNCTION jobs_fail(p_id BIGINT, p_owner TEXT, p_kind TEXT, p_error TEXT)
RETURNS TEXT AS $$
DECLARE
	tries INTEGER;
	most INTEGER;
	msg TEXT := left(coalesce(p_error, ''), 500);
BEGIN
	SELECT attempts, max_attempts INTO tries, most FROM jobs
	WHERE id = p_id AND lease_owner = p_owner AND state = 'running' FOR UPDATE;
	IF NOT FOUND THEN
		RETURN 'lost';
	END IF;

	IF p_kind = 'permanent' OR tries >= most THEN
		UPDATE jobs SET state = 'failed', error_kind = CASE WHEN p_kind = 'permanent' THEN 'permanent' ELSE 'temporary' END,
		       last_error = msg, lease_owner = NULL, lease_expires_at = NULL,
		       finished_at = now(), updated_at = now()
		WHERE id = p_id;
		RETURN 'failed';
	END IF;

	UPDATE jobs SET state = 'pending', error_kind = 'temporary', last_error = msg,
	       run_at = now() + CASE tries WHEN 1 THEN interval '30 seconds'
	                                   WHEN 2 THEN interval '2 minutes'
	                                   ELSE interval '10 minutes' END,
	       lease_owner = NULL, lease_expires_at = NULL, updated_at = now()
	WHERE id = p_id;
	RETURN 'retry';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- The worker saw the cancel request and stopped without publishing anything.
-- +goose StatementBegin
CREATE FUNCTION jobs_cancel_ack(p_id BIGINT, p_owner TEXT) RETURNS BOOLEAN AS $$
BEGIN
	UPDATE jobs SET state = 'cancelled', lease_owner = NULL, lease_expires_at = NULL,
	       finished_at = now(), updated_at = now()
	WHERE id = p_id AND lease_owner = p_owner AND state = 'running';
	RETURN FOUND;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS jobs_cancel_ack(BIGINT, TEXT);
DROP FUNCTION IF EXISTS jobs_fail(BIGINT, TEXT, TEXT, TEXT);
DROP FUNCTION IF EXISTS jobs_complete(BIGINT, TEXT);
DROP FUNCTION IF EXISTS jobs_heartbeat(BIGINT, TEXT, INTEGER);
DROP FUNCTION IF EXISTS jobs_claim(TEXT, INTEGER, INTEGER);
DROP TABLE IF EXISTS jobs;
