-- +goose Up
-- 1. Possible duplicates by title, author or ISBN (DEC-029, RF-008). The same bytes
--    are refused at ingestion; these are different files that may be the same
--    work (another format, edition or translation). Nothing is ever merged by
--    itself: an admin decides. A decision is remembered, so a pair the admin
--    dismissed is not proposed again.
CREATE TABLE duplicate_candidates (
	id BIGSERIAL PRIMARY KEY,
	work_a INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	work_b INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	reason VARCHAR(16) NOT NULL CHECK (reason IN ('isbn', 'title_author')),
	state VARCHAR(12) NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'dismissed')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	decided_at TIMESTAMPTZ,
	decided_by UUID,
	CHECK (work_a < work_b),
	UNIQUE (work_a, work_b)
);
CREATE INDEX idx_duplicate_candidates_pending ON duplicate_candidates(state) WHERE state = 'pending';

-- 2. Which pages of a document have no useful text (RF-019). A page counts as
--    having text when it yields enough real characters, not merely "some". OCR
--    itself is not run here; this only says where it would be needed, so a mixed
--    PDF can be processed on the pages that need it.
CREATE TABLE text_layers (
	file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
	page_count INTEGER NOT NULL,
	pages_without_text INTEGER[] NOT NULL DEFAULT '{}',
	needs_ocr BOOLEAN NOT NULL DEFAULT FALSE,
	detected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3. After a file is analysed its metadata is known: apply the layout and look
--    for duplicates. Both jobs are created by the queue itself.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION jobs_organize_after_ingest() RETURNS trigger AS $$
BEGIN
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('organize', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('dedupe', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION jobs_organize_after_ingest() RETURNS trigger AS $$
BEGIN
	INSERT INTO jobs (type, work_id, payload, priority)
	VALUES ('organize', NEW.work_id, '{}', 0)
	ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
DROP TABLE IF EXISTS text_layers;
DROP TABLE IF EXISTS duplicate_candidates;
