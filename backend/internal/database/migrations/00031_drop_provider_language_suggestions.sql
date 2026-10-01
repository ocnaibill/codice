-- +goose Up
-- The language of a file is read from the file, not from a provider (DEC-096): a provider says the language of
-- some edition of the work (Open Library listed "cat" for an English EPUB), not of this file. What the providers
-- already suggested and nobody decided is rejected so it does not wait in the queue; the rejection is remembered,
-- so the same value is not proposed again. What the system detected from the text is not touched.
UPDATE metadata_candidates SET state = 'rejected', decided_at = now()
WHERE field = 'language' AND state = 'pending' AND source <> 'detected';

-- +goose Down
-- A rejected suggestion is not brought back: it was wrong.
SELECT 1;
