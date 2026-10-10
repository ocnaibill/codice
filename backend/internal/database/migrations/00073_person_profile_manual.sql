-- +goose Up
-- A profile written by hand (DEC-167): owner and admin can write the description, the years, the biography and the photo of a person themselves,
-- for an author the providers do not know. The profile then has no Wikidata identifier, and `manual` says nobody else touches it: the worker
-- neither reads it again nor copies it to another person that holds the same identifier.
ALTER TABLE person_profile ALTER COLUMN wikidata_id DROP NOT NULL;
ALTER TABLE person_profile ADD COLUMN manual BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
DELETE FROM person_profile WHERE wikidata_id IS NULL;
ALTER TABLE person_profile DROP COLUMN IF EXISTS manual;
ALTER TABLE person_profile ALTER COLUMN wikidata_id SET NOT NULL;
