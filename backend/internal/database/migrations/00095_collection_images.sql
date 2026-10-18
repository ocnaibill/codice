-- +goose Up
-- The cover and the banner of a series, chosen by hand by the staff (DEC-201). Until somebody chooses, the cover of a series is the one of its
-- first work that has one, and it has no banner; a null here means "automatic". The files live with the other covers, under names that start
-- with cover_manual_, so that every backup keeps them.
ALTER TABLE collections ADD COLUMN cover_url TEXT;
ALTER TABLE collections ADD COLUMN banner_url TEXT;

-- +goose Down
ALTER TABLE collections DROP COLUMN IF EXISTS banner_url;
ALTER TABLE collections DROP COLUMN IF EXISTS cover_url;
