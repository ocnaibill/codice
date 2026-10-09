-- +goose Up
-- Rules that put works in categories (DEC-140): a term, which a tag of the work has to be (without regard to case or accents, and
-- each part of a tag written "Fiction / Science Fiction / General" counts on its own). Rules are applied to the library on someone's
-- say, after a preview of what they would do; they only add, they never take a work out of a category.
CREATE TABLE category_rules (
	id BIGSERIAL PRIMARY KEY,
	category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
	term VARCHAR(100) NOT NULL CHECK (btrim(term) <> ''),
	-- The term as it is compared: lower case, no accents, single spaces.
	term_key VARCHAR(100) NOT NULL,
	created_by UUID REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (category_id, term_key)
);
CREATE INDEX idx_category_rules_key ON category_rules(term_key);

-- Where a link came from: put by a person, or by the rules.
ALTER TABLE work_categories ADD COLUMN source VARCHAR(8) NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'rule'));

-- A work that someone took out of a category: the rules do not put it back.
CREATE TABLE work_category_exclusions (
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
	PRIMARY KEY (work_id, category_id)
);

-- +goose Down
DROP TABLE work_category_exclusions;
ALTER TABLE work_categories DROP COLUMN source;
DROP TABLE category_rules;
