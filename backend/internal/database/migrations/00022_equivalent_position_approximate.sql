-- +goose Up
-- A position estimated inside the right chapter (DEC-088) is not a passage found and not the start of
-- the chapter: the history records it as what it was.
ALTER TABLE equivalent_position_acceptances DROP CONSTRAINT IF EXISTS equivalent_position_acceptances_match_precision_check;
ALTER TABLE equivalent_position_acceptances ADD CONSTRAINT equivalent_position_acceptances_match_precision_check
	CHECK (match_precision IN ('passage', 'chapter', 'approximate'));

-- +goose Down
UPDATE equivalent_position_acceptances SET match_precision = 'chapter' WHERE match_precision = 'approximate';
ALTER TABLE equivalent_position_acceptances DROP CONSTRAINT IF EXISTS equivalent_position_acceptances_match_precision_check;
ALTER TABLE equivalent_position_acceptances ADD CONSTRAINT equivalent_position_acceptances_match_precision_check
	CHECK (match_precision IN ('passage', 'chapter'));
