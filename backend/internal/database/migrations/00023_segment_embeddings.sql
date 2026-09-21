-- +goose Up
-- Optional semantic evidence for equivalent positions (#31). Vectors remain derived data: they are
-- tied to the exact published segment and disappear with it. JSON keeps the base PostgreSQL image;
-- matching the few hundred segments of one destination happens in the API process.
CREATE TABLE document_segment_embeddings (
	document_segment_id BIGINT PRIMARY KEY REFERENCES document_segments(id) ON DELETE CASCADE,
	provider VARCHAR(32) NOT NULL,
	model TEXT NOT NULL,
	revision TEXT NOT NULL,
	preprocessing_version SMALLINT NOT NULL,
	dimensions SMALLINT NOT NULL CHECK (dimensions > 0),
	normalized BOOLEAN NOT NULL DEFAULT TRUE,
	embedding JSONB NOT NULL CHECK (jsonb_typeof(embedding) = 'array'),
	embedded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE text_embedding_status (
	file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
	generation INTEGER NOT NULL,
	provider VARCHAR(32) NOT NULL,
	model TEXT NOT NULL,
	revision TEXT NOT NULL,
	preprocessing_version SMALLINT NOT NULL,
	segment_count INTEGER NOT NULL,
	embedded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE equivalent_position_acceptances DROP CONSTRAINT equivalent_position_acceptances_method_check;
ALTER TABLE equivalent_position_acceptances ADD CONSTRAINT equivalent_position_acceptances_method_check
	CHECK (method IN ('text', 'anchors', 'structure', 'semantic'));

-- +goose Down
UPDATE equivalent_position_acceptances SET method = 'anchors' WHERE method = 'semantic';
ALTER TABLE equivalent_position_acceptances DROP CONSTRAINT equivalent_position_acceptances_method_check;
ALTER TABLE equivalent_position_acceptances ADD CONSTRAINT equivalent_position_acceptances_method_check
	CHECK (method IN ('text', 'anchors', 'structure'));
DROP TABLE IF EXISTS text_embedding_status;
DROP TABLE IF EXISTS document_segment_embeddings;
