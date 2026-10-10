-- +goose Up
-- A fourth unit of a work of a series (DEC-164): "extra", said "Complementar" on the screens. It is a work that belongs to the
-- collection but not to its sequence (a companion book, a guide, an art book): it has its own section on the page, and it is
-- left out of the progress of the series and of "go on".
ALTER TABLE works DROP CONSTRAINT IF EXISTS works_unit_check;
ALTER TABLE works ADD CONSTRAINT works_unit_check CHECK (unit IN ('volume', 'chapter', 'oneshot', 'extra'));

-- +goose Down
UPDATE works SET unit = NULL WHERE unit = 'extra';
ALTER TABLE works DROP CONSTRAINT IF EXISTS works_unit_check;
ALTER TABLE works ADD CONSTRAINT works_unit_check CHECK (unit IN ('volume', 'chapter', 'oneshot'));
