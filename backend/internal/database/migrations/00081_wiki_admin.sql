-- +goose Up
-- The administration of the automatic wiki (DEC-185): the queue of generations and the works or series that are not to have one.
-- The provider (kind, address, model and the sealed key) is the setting 'wiki.provider', owner's only (DEC-177, section 17.2).

-- A work or a series marked "não gerar wiki": left out of "Gerar para todas" and of the update a new book brings (DEC-180).
-- Generating it by a direct request on it is still possible.
CREATE TABLE wiki_exclusions (
	id BIGSERIAL PRIMARY KEY,
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	created_by UUID REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((collection_id IS NULL) <> (work_id IS NULL))
);
CREATE UNIQUE INDEX wiki_exclusions_collection ON wiki_exclusions (collection_id) WHERE collection_id IS NOT NULL;
CREATE UNIQUE INDEX wiki_exclusions_work ON wiki_exclusions (work_id) WHERE work_id IS NOT NULL;

-- A generation of the wiki of a work or a series. They run one at a time, oldest first (two runs on one key were refused by the
-- provider in the lab). The worker says where it is: the stage, how much of each stage is done, the book and chapter it is on, the
-- last errors, and how many times it took the run up again after a stop.
CREATE TABLE wiki_runs (
	id BIGSERIAL PRIMARY KEY,
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	-- manual: asked for on the work or series; batch: "Gerar para todas"; new_book: a book came into a series that has a wiki.
	origin VARCHAR(8) NOT NULL CHECK (origin IN ('manual', 'batch', 'new_book')),
	state VARCHAR(10) NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'paused', 'done', 'failed', 'cancelled')),
	-- text: the books become chapters; map: one request per chapter; identity: who is who; pages: one request per section; import.
	stage VARCHAR(10) CHECK (stage IN ('text', 'map', 'identity', 'pages', 'import')),
	-- {"map": {"done": 23, "total": 47}, ...}
	progress JSONB NOT NULL DEFAULT '{}',
	-- Where it is, as people say it: "A Nuvem, capítulo 12".
	place TEXT,
	-- The last errors, newest last (at most 10): [{"at": ..., "message": ...}].
	errors JSONB NOT NULL DEFAULT '[]',
	resumes INTEGER NOT NULL DEFAULT 0,
	-- Asked by a person, done by the worker at its next step.
	pause_requested BOOLEAN NOT NULL DEFAULT FALSE,
	cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
	-- Who wrote it: copied from the provider when the run starts.
	provider VARCHAR(64),
	model VARCHAR(128),
	lease_owner TEXT,
	heartbeat_at TIMESTAMPTZ,
	requested_by UUID REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CHECK ((collection_id IS NULL) <> (work_id IS NULL))
);
-- At most one live run for a work or a series.
CREATE UNIQUE INDEX wiki_runs_live_collection ON wiki_runs (collection_id) WHERE collection_id IS NOT NULL AND state IN ('queued', 'running', 'paused');
CREATE UNIQUE INDEX wiki_runs_live_work ON wiki_runs (work_id) WHERE work_id IS NOT NULL AND state IN ('queued', 'running', 'paused');
-- And at most one running at all.
CREATE UNIQUE INDEX wiki_runs_one_running ON wiki_runs ((true)) WHERE state = 'running';
CREATE INDEX wiki_runs_recent ON wiki_runs (created_at DESC);

-- +goose Down
DROP TABLE wiki_runs;
DROP TABLE wiki_exclusions;
