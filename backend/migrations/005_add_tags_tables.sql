-- Creates tags and work_tags pivot table for Many-to-Many relationship
CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS work_tags (
    work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
    tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (work_id, tag_id)
);

-- Seed initial test tags
INSERT INTO tags (name) VALUES ('Fantasy'), ('Sci-Fi'), ('RPG'), ('Technology') ON CONFLICT DO NOTHING;
