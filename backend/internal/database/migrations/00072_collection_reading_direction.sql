-- +goose Up
-- How a series is read (DEC-166): the direction a person chose for the whole collection ("ltr", "rtl" or "webtoon"), which a comic opens in
-- when its file does not declare one and before the type of the work says it. NULL is not said: the file, the type and what the device
-- remembers decide, as before.
ALTER TABLE collections ADD COLUMN reading_direction VARCHAR(8) CHECK (reading_direction IN ('ltr', 'rtl', 'webtoon'));

-- +goose Down
ALTER TABLE collections DROP COLUMN IF EXISTS reading_direction;
