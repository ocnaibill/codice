-- +goose Up
-- The generator of the wiki (DEC-185, DEC-186): what the model answered, kept by what was asked, and the bundle a run hands to the backend.

-- Every answer of the model is kept by the hash of the request (model, prompt, example, size). Taking a run up again, asking again for a
-- lower cut of full pages, or adding a book to a series then pays only for what is new. Only answers that were checked are kept. The scope
-- says whose wiki asked, so that deleting the wiki takes the answers (they hold quotes of the books) with it.
CREATE TABLE wiki_llm_cache (
	key VARCHAR(64) PRIMARY KEY,
	response JSONB NOT NULL,
	model VARCHAR(128) NOT NULL DEFAULT '',
	work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
	collection_id BIGINT REFERENCES collections(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX wiki_llm_cache_work ON wiki_llm_cache (work_id) WHERE work_id IS NOT NULL;
CREATE INDEX wiki_llm_cache_collection ON wiki_llm_cache (collection_id) WHERE collection_id IS NOT NULL;

-- The bundle a run made, from the moment the worker is done (stage import) until the backend has stored it. It lives apart from the run
-- so that listing the queue stays light.
CREATE TABLE wiki_run_bundles (
	run_id BIGINT PRIMARY KEY REFERENCES wiki_runs(id) ON DELETE CASCADE,
	bundle JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE wiki_run_bundles;
DROP TABLE wiki_llm_cache;
