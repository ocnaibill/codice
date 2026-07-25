import os
import psycopg2

class CodiceDatabase:
    def __init__(self):
        # Reads database URL from .env or defaults to local Docker compose setup
        self.db_url = os.getenv(
            "DATABASE_URL", 
            "postgres://codice_user:codice_secret@localhost:5432/codice_db"
        )

    def update_work_content(self, work_id, markdown_text):
        """Saves extracted Markdown content to the corresponding work record."""
        print(f"🗄️ Connecting to database to update Work ID: {work_id}...")
        
        try:
            with psycopg2.connect(self.db_url) as conn:
                with conn.cursor() as cur:
                    cur.execute("""
                        UPDATE works 
                        SET content = %s 
                        WHERE id = %s
                    """, (markdown_text, work_id))
                    
            print(f"✅ Markdown content saved to PostgreSQL for Work ID: {work_id}!")
        except Exception as e:
            print(f"❌ Error saving to database: {e}")
            raise e
