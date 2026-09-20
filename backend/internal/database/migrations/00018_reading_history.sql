-- +goose Up
-- Which version of a work counts, and how many times it was finished (DEC-079, DEC-080).
--
-- last_opened_at: when the person last opened this file. Not the same as updated_at, which also
-- moves when time is counted or a file is marked finished, and which an open-and-close does not touch.
ALTER TABLE reading_progress ADD COLUMN last_opened_at TIMESTAMPTZ;
UPDATE reading_progress SET last_opened_at = updated_at WHERE position <> '' OR locator IS NOT NULL OR percent_complete > 0;

-- Every time a file was finished, kept even after it is reopened to be read again. completed_at on
-- reading_progress is the current state; this is the history the "finished N times" count comes from.
-- The format is copied, so the history still says "in EPUB" if the file is removed later.
CREATE TABLE reading_completions (
	id BIGSERIAL PRIMARY KEY,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	file_id BIGINT REFERENCES files(id) ON DELETE SET NULL,
	format VARCHAR(16) NOT NULL DEFAULT '',
	completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reading_completions_user_work ON reading_completions(user_id, work_id);
CREATE INDEX idx_reading_completions_user_month ON reading_completions(user_id, completed_at);

INSERT INTO reading_completions (user_id, work_id, file_id, format, completed_at)
SELECT rp.user_id, e.work_id, rp.file_id, COALESCE(f.format, ''), rp.completed_at
FROM reading_progress rp
JOIN files f ON f.id = rp.file_id
JOIN editions e ON e.id = f.edition_id
WHERE rp.completed_at IS NOT NULL;

-- A completion is recorded when a file goes from not finished to finished, by whatever route: the
-- reader reaching the end, the explicit action, or the first readers' plain-text write.
-- +goose StatementBegin
CREATE FUNCTION reading_record_completion() RETURNS trigger AS $$
BEGIN
	IF NEW.completed_at IS NOT NULL AND (TG_OP = 'INSERT' OR OLD.completed_at IS NULL) THEN
		INSERT INTO reading_completions (user_id, work_id, file_id, format, completed_at)
		SELECT NEW.user_id, e.work_id, NEW.file_id, COALESCE(f.format, ''), NEW.completed_at
		FROM files f JOIN editions e ON e.id = f.edition_id
		WHERE f.id = NEW.file_id;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER reading_progress_completion
AFTER INSERT OR UPDATE OF completed_at ON reading_progress
FOR EACH ROW EXECUTE FUNCTION reading_record_completion();

-- "The whole work is finished": takes every version out of Continue Reading without saying the
-- versions that were not read to the end were. It goes away when the person reads on.
CREATE TABLE work_reading_state (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	finished_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, work_id)
);

-- +goose Down
DROP TABLE IF EXISTS work_reading_state;
DROP TRIGGER IF EXISTS reading_progress_completion ON reading_progress;
DROP FUNCTION IF EXISTS reading_record_completion();
DROP TABLE IF EXISTS reading_completions;
ALTER TABLE reading_progress DROP COLUMN IF EXISTS last_opened_at;
