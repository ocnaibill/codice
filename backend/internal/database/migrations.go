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
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS format VARCHAR(16);`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS page_count INTEGER DEFAULT 0;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS title_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS author_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS series_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS cover_lock BOOLEAN NOT NULL DEFAULT FALSE;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_type VARCHAR(16) DEFAULT 'page';`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS progress_percent REAL DEFAULT 0;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;`,
		`ALTER TABLE works ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;`,
		`CREATE INDEX IF NOT EXISTS idx_works_media_status ON works(media_status);`,
		`CREATE INDEX IF NOT EXISTS idx_works_series ON works(series);`,

		// 7. Migration 009: work_identifiers and media_pages tables
		`CREATE TABLE IF NOT EXISTS work_identifiers (
			id SERIAL PRIMARY KEY,
			work_id INT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			identifier_type VARCHAR(32) NOT NULL,
			identifier_value VARCHAR(128) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(work_id, identifier_type, identifier_value)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_work_identifiers_work_id ON work_identifiers(work_id);`,
		`CREATE INDEX IF NOT EXISTS idx_work_identifiers_type_value ON work_identifiers(identifier_type, identifier_value);`,
		`CREATE TABLE IF NOT EXISTS media_pages (
			id SERIAL PRIMARY KEY,
			work_id INT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			page_number INT NOT NULL,
			file_name VARCHAR(512),
			media_type VARCHAR(16) DEFAULT 'image',
			page_type VARCHAR(32) DEFAULT 'story',
			width INT,
			height INT,
			file_size INT,
			file_hash VARCHAR(64),
			UNIQUE(work_id, page_number)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_media_pages_work_id ON media_pages(work_id);`,
		`COMMENT ON COLUMN works.isbn IS 'Denormalized convenience field. Canonical identifiers are in work_identifiers table.';`,

		// 8. Migration 010: Widen isbn column — the extractor could
		// previously store non-ISBN identifiers (UUIDs, URNs) that
		// exceeded VARCHAR(32). The extractor is now fixed, but this
		// migration protects against edge cases.
		`ALTER TABLE works ALTER COLUMN isbn TYPE VARCHAR(64);`,

		// 9. Migration 011: Reading progress detail, favorites, and notes.
		// percent_complete/completed_at let the dashboard show real
		// progress bars and "completed this month" counts.
		// reading_seconds accumulates active reading time reported by
		// the reader's heartbeat, so "time spent reading" is real
		// instead of invented.
		`ALTER TABLE user_progress ADD COLUMN IF NOT EXISTS percent_complete REAL NOT NULL DEFAULT 0;`,
		`ALTER TABLE user_progress ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;`,
		`ALTER TABLE user_progress ADD COLUMN IF NOT EXISTS reading_seconds INTEGER NOT NULL DEFAULT 0;`,

		`CREATE TABLE IF NOT EXISTS favorites (
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, work_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_favorites_user_id ON favorites(user_id);`,

		`CREATE TABLE IF NOT EXISTS notes (
			id SERIAL PRIMARY KEY,
			user_id UUID REFERENCES users(id) ON DELETE CASCADE,
			work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
			quote TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_notes_user_id_created_at ON notes(user_id, created_at DESC);`,

		// 10. Migration 012: roles owner / admin / reader (spec DEC-014, DEC-022).
		// Before this, the setup wizard created an 'admin'. If no owner exists
		// yet, the earliest admin becomes the owner; later admins stay admins.
		// The statements are idempotent and safe to run on every startup.
		`UPDATE users SET role = 'reader' WHERE role IS NULL;`,
		`ALTER TABLE users ALTER COLUMN role SET DEFAULT 'reader';`,
		`ALTER TABLE users ALTER COLUMN role SET NOT NULL;`,
		`UPDATE users SET role = 'owner'
			WHERE id = (SELECT id FROM users WHERE role = 'admin' ORDER BY created_at, id LIMIT 1)
			  AND NOT EXISTS (SELECT 1 FROM users WHERE role = 'owner');`,
		`ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;`,
		`ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('owner', 'admin', 'reader'));`,
		// At most one owner: a partial unique index on a constant expression.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_single_owner ON users ((role)) WHERE role = 'owner';`,
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