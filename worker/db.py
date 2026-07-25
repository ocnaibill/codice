import os
import psycopg2

class CodiceDatabase:
    def __init__(self):
        # Reads database URL from .env or defaults to local Docker compose setup
        self.db_url = os.getenv(
            "DATABASE_URL", 
            "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
        )

    def update_work_metadata(self, work_id, metadata):
        """Updates work record with extracted metadata and links generated cover URL."""
        print(f"🗄️ Saving metadata to database for Work ID: {work_id}...")
        
        try:
            with psycopg2.connect(self.db_url) as conn:
                with conn.cursor() as cur:
                    # 1. Update title in works table
                    cur.execute("""
                        UPDATE works 
                        SET original_title = %s 
                        WHERE id = %s
                    """, (metadata['title'], work_id))
                    
                    # 2. Insert or update edition record with cover URL
                    cur.execute("""
                        INSERT INTO editions (work_id, title, cover_url) 
                        VALUES (%s, %s, %s)
                        ON CONFLICT (work_id) 
                        DO UPDATE SET cover_url = EXCLUDED.cover_url;
                    """, (work_id, metadata['title'], metadata['cover_url']))
                    
            print(f"✅ Metadata and cover saved to PostgreSQL for Work ID: {work_id}!")
        except Exception as e:
            print(f"❌ Error saving to database: {e}")
            raise e
