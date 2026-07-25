-- Adds content column to works table for storing extracted Markdown text
ALTER TABLE works ADD COLUMN IF NOT EXISTS content TEXT;
