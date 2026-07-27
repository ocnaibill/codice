-- Migration 008: Add metadata columns for the new extraction pipeline
-- Adds columns for media status lifecycle, series info, identifiers, and locks

-- Media status lifecycle (inspired by Komga)
ALTER TABLE works ADD COLUMN IF NOT EXISTS media_status VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN';
ALTER TABLE works ADD COLUMN IF NOT EXISTS media_error TEXT;
ALTER TABLE works ADD COLUMN IF NOT EXISTS media_analysis_started_at TIMESTAMP;
ALTER TABLE works ADD COLUMN IF NOT EXISTS media_analysis_completed_at TIMESTAMP;

-- Series info
ALTER TABLE works ADD COLUMN IF NOT EXISTS series VARCHAR(512);
ALTER TABLE works ADD COLUMN IF NOT EXISTS series_index REAL DEFAULT 0;

-- Rich metadata fields
ALTER TABLE works ADD COLUMN IF NOT EXISTS isbn VARCHAR(32);
ALTER TABLE works ADD COLUMN IF NOT EXISTS language VARCHAR(16);
ALTER TABLE works ADD COLUMN IF NOT EXISTS publisher VARCHAR(256);
ALTER TABLE works ADD COLUMN IF NOT EXISTS publication_date VARCHAR(32);
ALTER TABLE works ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE works ADD COLUMN IF NOT EXISTS page_count INTEGER DEFAULT 0;

-- Metadata lock columns (inspired by Komga: fields with lock=True are not overwritten on rescan)
ALTER TABLE works ADD COLUMN IF NOT EXISTS title_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS author_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS series_lock BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE works ADD COLUMN IF NOT EXISTS cover_lock BOOLEAN NOT NULL DEFAULT FALSE;

-- Progress structured fields (replacing the old string-based progress)
ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_type VARCHAR(16) DEFAULT 'page';  -- page, cfi, time
ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_percent REAL DEFAULT 0;

-- Index for media status queries
CREATE INDEX IF NOT EXISTS idx_works_media_status ON works(media_status);
CREATE INDEX IF NOT EXISTS idx_works_series ON works(series);