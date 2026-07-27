package database

import (
	"database/sql"
	"log"
)

// RunAutoMigrations executes all DDL schema statements on startup to guarantee database readiness
func RunAutoMigrations(db *sql.DB) error {
	log.Println("🔄 Checking and applying database auto-migrations...")

	statements := []string{
		// 1. Core Library Tables
		`CREATE TABLE IF NOT EXISTS person (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS works (
			id SERIAL PRIMARY KEY,
			original_title VARCHAR(255) NOT NULL,
			content TEXT,
			file_path VARCHAR(255),
			author_id INTEGER REFERENCES person(id) ON DELETE SET NULL
		);`,

		`CREATE TABLE IF NOT EXISTS editions (
			id SERIAL PRIMARY KEY,
			work_id INTEGER UNIQUE REFERENCES works(id) ON DELETE CASCADE,
			title VARCHAR(255) NOT NULL,
			cover_url VARCHAR(255)
		);`,

		// 2. Tags and Work Tags Pivot
		`CREATE TABLE IF NOT EXISTS tags (
			id SERIAL PRIMARY KEY,
			name VARCHAR(50) UNIQUE NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS work_tags (
			work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
			tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY (work_id, tag_id)
		);`,

		// 3. Seed initial tags
		`INSERT INTO tags (name) VALUES ('Fantasy'), ('Sci-Fi'), ('RPG'), ('Technology') ON CONFLICT DO NOTHING;`,

		// 4. Users Table with UUID
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username VARCHAR(50) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255),
			sso_id VARCHAR(255) UNIQUE,
			role VARCHAR(20) DEFAULT 'reader',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,

		// 5. User Progress Table
		`CREATE TABLE IF NOT EXISTS user_progress (
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
			progress VARCHAR(255) NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, work_id)
		);`,

		// 6. Migration 008: Metadata columns for extraction pipeline
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS media_status VARCHAR(20) NOT NULL DEFAULT 'UNKNOWN';`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS media_error TEXT;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS media_analysis_started_at TIMESTAMP;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS media_analysis_completed_at TIMESTAMP;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS series VARCHAR(512);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS series_index REAL DEFAULT 0;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS isbn VARCHAR(32);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS language VARCHAR(16);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS publisher VARCHAR(256);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS publication_date VARCHAR(32);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS description TEXT;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS page_count INTEGER DEFAULT 0;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS title_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS author_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS series_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS cover_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_type VARCHAR(16) DEFAULT 'page';`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_percent REAL DEFAULT 0;`,
		`CREATE INDEX IF NOT EXISTS idx_works_media_status ON works(media_status);`,
		`CREATE INDEX IF NOT EXISTS idx_works_series ON works(series);`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("❌ Migration statement error: %v", err)
			return err
		}
	}

	log.Println("✅ Database auto-migrations completed successfully!")
	return nil
}