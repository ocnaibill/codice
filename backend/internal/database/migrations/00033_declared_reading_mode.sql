-- +goose Up
-- How a comic file says it is meant to be read (#19): right to left (its ComicInfo.xml says so) or as a strip to
-- scroll (its pages are several times taller than wide). It is a fact of the file, found by the worker; empty when
-- the file says nothing, which is the usual case. What the person chooses in the reader still comes first.
ALTER TABLE files ADD COLUMN declared_mode VARCHAR(8) CHECK (declared_mode IN ('rtl', 'webtoon'));

-- +goose Down
ALTER TABLE files DROP COLUMN declared_mode;
