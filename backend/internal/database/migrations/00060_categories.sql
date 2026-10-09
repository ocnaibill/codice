-- +goose Up
-- Categories organise the navigation of the library by theme (DEC-024, DEC-025, DEC-140): a tree that owner and admin make as they
-- like, empty at the start, where a work may be in several. A work in a subcategory is also in the ones above it. The tags stay as
-- they are: free text from the files and the providers, for subjects that cross the categories.
CREATE TABLE categories (
	id BIGSERIAL PRIMARY KEY,
	parent_id BIGINT REFERENCES categories(id) ON DELETE RESTRICT,
	name VARCHAR(80) NOT NULL CHECK (btrim(name) <> ''),
	created_by UUID REFERENCES users(id) ON DELETE SET NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Two categories with the same name can be in different places ("Terror" among the comics and among the novels), not side by side.
CREATE UNIQUE INDEX categories_name_in_parent ON categories (COALESCE(parent_id, 0), lower(name));
CREATE INDEX idx_categories_parent ON categories(parent_id);

CREATE TABLE work_categories (
	work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
	category_id BIGINT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
	assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
	assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (work_id, category_id)
);
CREATE INDEX idx_work_categories_category ON work_categories(category_id);

-- +goose Down
DROP TABLE work_categories;
DROP TABLE categories;
