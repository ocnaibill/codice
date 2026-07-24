-- 1. Creating the Base Tables
CREATE TABLE series (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50), -- ex: 'manga', 'novel'
    status VARCHAR(50)
);

CREATE TABLE person (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    image_url TEXT
);

CREATE TABLE works (
    id SERIAL PRIMARY KEY,
    original_title VARCHAR(255) NOT NULL,
    author_id INT REFERENCES person(id),
    original_release_year INT,
    series_id INT REFERENCES series(id)
);

CREATE TABLE editions (
    id SERIAL PRIMARY KEY,
    work_id INT REFERENCES works(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    language VARCHAR(50),
    isbn VARCHAR(20),
    publisher_id INT REFERENCES person(id), -- Simplification: publisher as 'person' or entity
    cover_url TEXT
);

-- 2. File Table with FTS Search Engine
CREATE TABLE media_files (
    id SERIAL PRIMARY KEY,
    edition_id INT REFERENCES editions(id) ON DELETE CASCADE,
    format VARCHAR(10) NOT NULL, -- 'pdf', 'epub', 'cbz'
    file_path TEXT NOT NULL,
    full_text_md TEXT,
    
    -- PostgreSQL automatically generates this vector whenever Markdown is inserted
    search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('portuguese', coalesce(full_text_md, ''))
    ) STORED
);

-- Index for quick search in the files.
CREATE INDEX idx_media_search_vector ON media_files USING GIN (search_vector);

-- N:N relationship table (Secondary authors, Illustrators, etc.)
CREATE TABLE work_person (
    work_id INT REFERENCES works(id) ON DELETE CASCADE,
    person_id INT REFERENCES person(id) ON DELETE CASCADE,
    role VARCHAR(50), -- 'author', 'illustrator', 'translator'
    PRIMARY KEY (work_id, person_id)
);

-- 3. A Materialized View (The Global Search Engine)
CREATE MATERIALIZED VIEW library_search_index AS
SELECT 
    w.id AS work_id,
    w.original_title AS title,
    -- WEIGHT A: Book title has top priority
    setweight(to_tsvector('portuguese', coalesce(w.original_title, '')), 'A') ||
    
    -- WEIGHT B: The main author's name has high priority
    setweight(to_tsvector('portuguese', coalesce(p.name, '')), 'B') ||
    
    -- WEIGHT C: Text extracted from the PDF has normal priority
    setweight(to_tsvector('portuguese', coalesce(m.full_text_md, '')), 'C') AS search_document

FROM works w
LEFT JOIN person p ON w.author_id = p.id
LEFT JOIN editions e ON w.id = e.work_id
LEFT JOIN media_files m ON e.id = m.edition_id;

-- Creates the GIN index in the Materialized View to prevent search from freezing
CREATE INDEX idx_fts_search_document ON library_search_index USING GIN (search_document);