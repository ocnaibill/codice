-- +goose Up
-- The place a person was born in, from Wikidata (DEC-160): the name of the place (in Portuguese when the entity has one), kept with the profile.
-- `place_read` says the place was asked for: the profiles read before this column existed have it false, and are asked again for the place alone.
ALTER TABLE person_profile
	ADD COLUMN born_place VARCHAR(255),
	ADD COLUMN place_read BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE person_profile
	DROP COLUMN IF EXISTS place_read,
	DROP COLUMN IF EXISTS born_place;
