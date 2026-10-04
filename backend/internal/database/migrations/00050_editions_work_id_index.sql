-- +goose Up
-- Found by the scale test (benchmarks/scale): `editions` was only indexed on its primary key and on "the one primary edition of
-- a work", so every lookup "the editions of this work" (the catalog card, the duplicate detector, the sheet of a work) read the
-- whole table. With 10 000 works the last page of the library took 4.2 s (0.5 s with the index), and loading the works for the
-- duplicate check took 5.8 s for every imported file (0.14 s). The cost grew with the square of the library.
CREATE INDEX IF NOT EXISTS idx_editions_work_id ON editions (work_id);

-- +goose Down
DROP INDEX IF EXISTS idx_editions_work_id;
