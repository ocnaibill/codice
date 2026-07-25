-- Adds unique constraint to work_id in editions table to allow UPSERT operations
ALTER TABLE editions ADD CONSTRAINT unique_edition_work_id UNIQUE (work_id);
