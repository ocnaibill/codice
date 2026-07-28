import os
import psycopg2


class CodiceDatabase:
    """PostgreSQL connection wrapper for the Códice worker."""

    def __init__(self):
        self.db_url = os.getenv(
            "DATABASE_URL",
            "postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable"
        )

    def execute(self, query, params=None):
        """Execute a query (INSERT, UPDATE, DELETE)."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)

    def fetchone(self, query, params=None):
        """Execute a query and return one row."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)
                return cur.fetchone()

    def fetchall(self, query, params=None):
        """Execute a query and return all rows."""
        with psycopg2.connect(self.db_url) as conn:
            with conn.cursor() as cur:
                cur.execute(query, params)
                return cur.fetchall()