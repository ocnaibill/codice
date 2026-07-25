-- Adds file_path column to works table for storing relative original filename
ALTER TABLE works ADD COLUMN IF NOT EXISTS file_path VARCHAR(255);
