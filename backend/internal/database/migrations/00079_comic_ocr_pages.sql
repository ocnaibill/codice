-- +goose Up
-- The pages of comics and manga read by OCR (DEC-176, #27).
--
-- Like ocr_pages for the PDFs (00036), a page is read once and kept: it is the expensive part, and the text the search sees is made from
-- these rows each time the text of the file is read. What differs is that a comic has balloons, not lines of a page: each row keeps the
-- regions the engine found, with their boxes in the pixels of the page and the text of each, in no order. The order in which they are read
-- is computed when the text is made, from the direction the file declares, or the series says (DEC-166), so a direction chosen later
-- only makes the text again and reads nothing again.
--
-- The text is recognised, not the faithful transcription of the original: the model (mobile or server) and the engine version are
-- recorded with every page. A page that could not be read is kept as failed, with the reason, and is not tried again by itself.
CREATE TABLE comic_ocr_pages (
	file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	page INTEGER NOT NULL CHECK (page >= 0),
	source_sha256 CHAR(64),
	-- done: there are regions of speech. blank: the page was read and has none (a plate, a page of art). failed: it could not be read.
	state VARCHAR(8) NOT NULL CHECK (state IN ('done', 'blank', 'failed')),
	-- [{"box": [x, y, w, h], "text": "...", "conf": 97.5}], in the pixels of the page (width and height below).
	regions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(regions) = 'array'),
	width INTEGER,
	height INTEGER,
	model VARCHAR(16) NOT NULL,
	engine VARCHAR(32) NOT NULL,
	engine_version VARCHAR(32) NOT NULL DEFAULT '',
	seconds REAL,
	error TEXT,
	recognized_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (file_id, page),
	CHECK (state <> 'done' OR jsonb_array_length(regions) > 0)
);

CREATE INDEX comic_ocr_pages_model ON comic_ocr_pages (model) WHERE state IN ('done', 'blank');

-- +goose Down
DROP TABLE IF EXISTS comic_ocr_pages;
