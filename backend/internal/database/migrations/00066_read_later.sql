-- +goose Up
-- The lists the Códice keeps for a person by itself (DEC-152): today "Ler depois", the works a person put aside to read later. It is a personal
-- collection like the others, with a name the Códice gives it and that a person cannot change, and that does not go away: the works in it do.
ALTER TABLE collections
	ADD COLUMN system_key VARCHAR(16),
	ADD CONSTRAINT collections_system_is_personal CHECK (system_key IS NULL OR kind = 'personal');
-- One of each for a person.
CREATE UNIQUE INDEX collections_one_system_per_owner ON collections(owner_id, system_key) WHERE system_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS collections_one_system_per_owner;
ALTER TABLE collections
	DROP CONSTRAINT IF EXISTS collections_system_is_personal,
	DROP COLUMN IF EXISTS system_key;
