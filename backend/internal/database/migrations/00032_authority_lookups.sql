-- +goose Up
-- The authority records already looked up in a reference source (#63): what Open Library knows about an author
-- by its key (the Wikidata, VIAF and ISNI identifiers, or that the key was merged into another). A key is looked
-- up once; one that could not be reached is tried again the next day, a few times, and then left. The result is
-- kept in person_authority, never decided on its own: two people with an identifier in common are only proposed.
CREATE TABLE authority_lookups (
	source VARCHAR(32) NOT NULL,
	key VARCHAR(64) NOT NULL,
	state VARCHAR(12) NOT NULL CHECK (state IN ('done', 'missing', 'redirect', 'failed')),
	attempts SMALLINT NOT NULL DEFAULT 1,
	attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (source, key)
);

-- +goose Down
DROP TABLE authority_lookups;
