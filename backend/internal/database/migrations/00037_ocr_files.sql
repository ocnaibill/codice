-- +goose Up
-- The language a scanned PDF is read in, and how that was decided (#24): the language its edition declares, the one found
-- by reading a few of its pages and telling the language from what came out, the owner's default when that could not be
-- told, or the one somebody on the staff said is the right one. It is decided once per version of the file and kept, so that
-- reading the pages that failed later, or the rest of a book, is in the same language as the pages already read.
CREATE TABLE ocr_files (
	file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
	source_sha256 CHAR(64),
	-- The codes of the engine, joined by "+" (por, eng, por+eng).
	language VARCHAR(32) NOT NULL,
	source VARCHAR(10) NOT NULL CHECK (source IN ('declared', 'detected', 'default', 'manual')),
	decided_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS ocr_files;
