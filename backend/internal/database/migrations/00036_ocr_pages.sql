-- +goose Up
-- The pages of a PDF that have no text layer, read by OCR (#24, RF-019, DEC-044).
--
-- A page is recognised once and kept: reading a scanned book takes minutes to hours, so what was recognised is
-- the expensive part and the text of the file is made from it, not the other way round. One row per page (an index
-- from 0, as in the locator of a PDF) holds what the engine found, how, and when; the segments the search sees are
-- built from these rows every time the text of the file is read, so reading the native text again (a new version of
-- the extractor, a file that changed) never loses what OCR had done. A page that could not be recognised is kept as
-- failed, with the reason, and is not tried again by itself.
--
-- The text is recognised, not the faithful transcription of the original: the engine, its version and the language
-- it was told are recorded with every page, and the pages of a file whose bytes changed (another source_sha256) are
-- not used.
CREATE TABLE ocr_pages (
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	page INTEGER NOT NULL CHECK (page >= 0),
	source_sha256 CHAR(64),
	-- done: there is text. blank: the page was read and has none (a blank page, a plate). failed: it could not be read.
	state VARCHAR(8) NOT NULL CHECK (state IN ('done', 'blank', 'failed')),
	text TEXT NOT NULL DEFAULT '',
	engine VARCHAR(32) NOT NULL,
	engine_version VARCHAR(32) NOT NULL DEFAULT '',
	language VARCHAR(32) NOT NULL DEFAULT '',
	dpi SMALLINT,
	error TEXT,
	recognized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (file_id, page),
	CHECK (state <> 'done' OR text <> '')
);

-- +goose Down
DROP TABLE IF EXISTS ocr_pages;
