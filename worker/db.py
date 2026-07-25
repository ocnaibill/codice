import os
import sys
import psycopg2

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

class CodiceDatabase:
    def __init__(self):
        # Reads database URL from .env or defaults to local Docker compose setup
        self.db_url = os.getenv(
            "DATABASE_URL", 
            "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
        )

    def update_work_metadata(self, work_id, metadata):
        """Updates work record, resolves author ID in person table, and links cover URL."""
        print(f"🗄️ Saving metadata to database for Work ID: {work_id}...")
        
        author_name = metadata.get('author') or 'Unknown Author'

        try:
            with psycopg2.connect(self.db_url) as conn:
                with conn.cursor() as cur:
                    # 1. Resolve Author ID in person table
                    cur.execute("SELECT id FROM person WHERE name = %s;", (author_name,))
                    row = cur.fetchone()
                    
                    if row:
                        author_id = row[0]
                        print(f"   👤 Author already exists (ID: {author_id})")
                    else:
                        cur.execute("INSERT INTO person (name) VALUES (%s) RETURNING id;", (author_name,))
                        author_id = cur.fetchone()[0]
                        print(f"   👤 Created new author record (ID: {author_id})")

                    # 2. Update works table with title and resolved author_id
                    cur.execute("""
                        UPDATE works 
                        SET original_title = %s, author_id = %s 
                        WHERE id = %s
                    """, (metadata['title'], author_id, work_id))
                    
                    # 3. Insert or update edition record with cover URL
                    cur.execute("""
                        INSERT INTO editions (work_id, title, cover_url) 
                        VALUES (%s, %s, %s)
                        ON CONFLICT (work_id) 
                        DO UPDATE SET cover_url = EXCLUDED.cover_url;
                    """, (work_id, metadata['title'], metadata['cover_url']))
                    
            print(f"✅ Metadata, author, and cover saved to PostgreSQL for Work ID: {work_id}!")
        except Exception as e:
            print(f"❌ Error saving to database: {e}")
            raise e
