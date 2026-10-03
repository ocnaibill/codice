-- +goose Up
-- The languages a package can find a word of by the translations it lists (the English package lists Malay words under its
-- English ones): with the languages of its own entries they are what the reader's card offers to look a word up in (#109).
ALTER TABLE dictionary_packages ADD COLUMN link_languages TEXT[] NOT NULL DEFAULT '{}';
UPDATE dictionary_packages p SET link_languages = ARRAY(SELECT DISTINCT l.lang FROM dictionary_links l WHERE l.package_id = p.id ORDER BY 1);

-- +goose Down
ALTER TABLE dictionary_packages DROP COLUMN IF EXISTS link_languages;
