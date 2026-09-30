-- +goose Up
-- How a person's name is written and how it is shown are two things (#64, DEC-094): "Herbert, Frank" and
-- "Frank Herbert" are both right. A person keeps the name people say (person.name), and, only when it is
-- known for certain which words are the surname, the surname and the given names too. How a name is shown
-- (given names first, or surname first) and how works are sorted by author is a preference: the library has a
-- default (settings key name_order) and each account may choose its own.
ALTER TABLE person ADD COLUMN family_name VARCHAR(255);
ALTER TABLE person ADD COLUMN given_name VARCHAR(255);
ALTER TABLE person ADD CONSTRAINT person_name_parts CHECK (given_name IS NULL OR family_name IS NOT NULL);
ALTER TABLE users ADD COLUMN name_order VARCHAR(12) CHECK (name_order IN ('given_first', 'family_first'));

-- What is already known for certain: the aliases that were written the catalogue's way with a role
-- ("Herbert, Frank, author") say which words are the surname (same rule as internal/people.Parse).
-- +goose StatementBegin
CREATE FUNCTION pg_temp.is_role(t TEXT) RETURNS BOOLEAN AS $$
	SELECT btrim(lower(unaccent(t)), ' .;:()[]') IN (
		'author', 'autor', 'autora', 'auteur', 'autore',
		'editor', 'editora', 'editeur', 'editore',
		'illustrator', 'ilustrador', 'ilustradora', 'illustrateur', 'illustratore',
		'translator', 'tradutor', 'tradutora', 'traductor', 'traductora', 'traducteur', 'traduttore',
		'narrator', 'narrador', 'narradora', 'narrateur', 'contributor', 'colaborador')
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION pg_temp.catalogue_split(raw TEXT) RETURNS TEXT[] AS $$
DECLARE
	work TEXT := btrim(regexp_replace(raw, '\s+', ' ', 'g'));
	paren TEXT[];
	parts TEXT[];
	n INTEGER;
BEGIN
	paren := regexp_match(work, '^(.+?)\s*\(([^()]*)\)$');
	IF paren IS NOT NULL AND pg_temp.is_role(paren[2]) THEN
		work := btrim(paren[1]);
		parts := ARRAY(SELECT btrim(p) FROM unnest(string_to_array(work, ',')) AS p WHERE btrim(p) <> '');
		IF array_length(parts, 1) = 2 THEN RETURN parts; END IF;
		RETURN NULL;
	END IF;
	parts := ARRAY(SELECT btrim(p) FROM unnest(string_to_array(work, ',')) AS p WHERE btrim(p) <> '');
	n := COALESCE(array_length(parts, 1), 0);
	IF n = 3 AND pg_temp.is_role(parts[3]) THEN RETURN ARRAY[parts[1], parts[2]]; END IF;
	RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

UPDATE person p SET family_name = s.sp[1], given_name = s.sp[2]
FROM (
	SELECT DISTINCT ON (person_id) person_id, pg_temp.catalogue_split(alias) AS sp
	FROM person_alias WHERE pg_temp.catalogue_split(alias) IS NOT NULL
	ORDER BY person_id, alias
) s
WHERE s.person_id = p.id AND p.family_name IS NULL
  AND p.name = s.sp[2] || ' ' || s.sp[1]; -- the alias must be this person's name written the other way round

DROP FUNCTION pg_temp.catalogue_split(TEXT);
DROP FUNCTION pg_temp.is_role(TEXT);

-- +goose Down
ALTER TABLE users DROP COLUMN name_order;
ALTER TABLE person DROP CONSTRAINT person_name_parts;
ALTER TABLE person DROP COLUMN given_name;
ALTER TABLE person DROP COLUMN family_name;
DELETE FROM settings WHERE key = 'name_order';
