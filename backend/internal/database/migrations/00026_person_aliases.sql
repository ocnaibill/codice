-- +goose Up
-- A person goes by the name people say, and keeps what a file wrote as an alias (#36): "Herbert, Frank,
-- author" is Frank Herbert, and searching for the first still finds the work. The names that are stored
-- already are fixed here, with the same rule as internal/people (only a role word makes the catalogue's
-- way clear; anything else is left for a person to decide).
CREATE TABLE person_alias (
	person_id INTEGER NOT NULL REFERENCES person(id) ON DELETE CASCADE,
	alias VARCHAR(255) NOT NULL,
	PRIMARY KEY (person_id, alias)
);
CREATE INDEX idx_person_alias_lower ON person_alias (lower(alias));

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
CREATE FUNCTION pg_temp.normalize_person_name(raw TEXT) RETURNS TEXT AS $$
DECLARE
	original TEXT := btrim(regexp_replace(raw, '\s+', ' ', 'g'));
	work TEXT := original;
	had BOOLEAN := FALSE;
	paren TEXT[];
	parts TEXT[];
	n INTEGER;
BEGIN
	IF original = '' THEN RETURN original; END IF;
	paren := regexp_match(work, '^(.+?)\s*\(([^()]*)\)$');
	IF paren IS NOT NULL AND pg_temp.is_role(paren[2]) THEN
		work := btrim(paren[1]);
		had := TRUE;
	END IF;
	parts := ARRAY(SELECT btrim(p) FROM unnest(string_to_array(work, ',')) AS p WHERE btrim(p) <> '');
	n := COALESCE(array_length(parts, 1), 0);
	IF n > 1 AND pg_temp.is_role(parts[n]) THEN
		parts := parts[1:n - 1];
		n := n - 1;
		had := TRUE;
	END IF;
	IF NOT had THEN RETURN original; END IF;
	IF n = 1 THEN RETURN parts[1]; END IF;
	IF n = 2 THEN RETURN parts[2] || ' ' || parts[1]; END IF;
	RETURN original;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
DECLARE
	r RECORD;
	target INTEGER;
BEGIN
	FOR r IN SELECT id, name, pg_temp.normalize_person_name(name) AS fixed FROM person ORDER BY id LOOP
		CONTINUE WHEN r.fixed = r.name OR r.fixed = '';
		SELECT id INTO target FROM person WHERE name = r.fixed AND id <> r.id;
		IF target IS NULL THEN
			UPDATE person SET name = r.fixed WHERE id = r.id;
			INSERT INTO person_alias (person_id, alias) VALUES (r.id, r.name) ON CONFLICT DO NOTHING;
		ELSE
			-- The person already exists under the name people say: its works are this one's too.
			UPDATE work_contributors SET person_id = target
			WHERE person_id = r.id AND NOT EXISTS (
				SELECT 1 FROM work_contributors o
				WHERE o.work_id = work_contributors.work_id AND o.person_id = target AND o.role = work_contributors.role);
			INSERT INTO person_alias (person_id, alias) VALUES (target, r.name) ON CONFLICT DO NOTHING;
			INSERT INTO person_alias (person_id, alias) SELECT target, alias FROM person_alias WHERE person_id = r.id ON CONFLICT DO NOTHING;
			DELETE FROM person WHERE id = r.id; -- what is left of it (a work that had both) goes with it
		END IF;
	END LOOP;
END $$;
-- +goose StatementEnd

DROP FUNCTION pg_temp.normalize_person_name(TEXT);
DROP FUNCTION pg_temp.is_role(TEXT);

-- +goose Down
-- The names that were fixed stay as they are (the alias says what they were), and only the aliases go.
DROP TABLE person_alias;
