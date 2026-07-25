-- Migration to add reading_progress column to works table
ALTER TABLE works ADD COLUMN IF NOT EXISTS reading_progress VARCHAR(255);
