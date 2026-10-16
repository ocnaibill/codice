-- +goose Up
-- An app token has a scope: 'app' is the long token every OPDS client has always had, and 'koreader' is the short one made to be typed
-- on an e-ink keyboard (DEC-190). The scope says how the token was made; both open the catalog, and only the second is meant for KOSync.
ALTER TABLE app_tokens ADD COLUMN scope TEXT NOT NULL DEFAULT 'app' CHECK (scope IN ('app', 'koreader'));

-- +goose Down
ALTER TABLE app_tokens DROP COLUMN IF EXISTS scope;
