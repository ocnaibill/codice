-- Migration 007: Users table with UUID, per-user reading progress, and admin seed
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255),
    sso_id VARCHAR(255) UNIQUE,
    role VARCHAR(20) DEFAULT 'reader',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_progress (
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
    progress VARCHAR(255) NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, work_id)
);

-- Drop deprecated global reading_progress column from works
ALTER TABLE works DROP COLUMN IF EXISTS reading_progress;

-- Seed default initial admin user if not exists
INSERT INTO users (username, email, role, password_hash) 
VALUES ('admin', 'admin@codice.local', 'admin', 'CHANGE_LATER')
ON CONFLICT (username) DO NOTHING;
