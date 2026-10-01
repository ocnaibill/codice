"""One-time repairs of what an earlier version of the worker stored wrongly.

Each runs once, on the first worker to start after the update, and is recorded in the settings so that it never
runs twice: some repairs are not safe to repeat (reading the entities of a text twice would change a text that
was meant to say "&lt;"). The check, the repair and the record are one transaction, held under a lock, so two
workers starting together do it once and a worker that dies in the middle leaves nothing half done.
"""
import json

import psycopg2

from extractors.plain_text import MARKUP, plain_description

DESCRIPTIONS_FLAG = 'maintenance.plain_descriptions'
_LOCK = 7301258


def repair_descriptions_on(conn):
    """Descriptions kept with the HTML of the file they came from (#58) become plain text. A description the
    person wrote or locked is left as it is. Returns how many were changed, or None when it was done before."""
    cur = conn.cursor()
    cur.execute("SELECT pg_advisory_xact_lock(%s)", (_LOCK,))
    cur.execute("SELECT 1 FROM settings WHERE key = %s", (DESCRIPTIONS_FLAG,))
    if cur.fetchone():
        conn.rollback()
        return None
    cur.execute("SELECT id, description FROM works WHERE NOT description_lock AND description ~ %s", (MARKUP,))
    changed = 0
    for work_id, description in cur.fetchall():
        clean = plain_description(description)
        if clean and clean != description:
            cur.execute("UPDATE works SET description = %s WHERE id = %s", (clean, work_id))
            changed += 1
    cur.execute(
        """INSERT INTO settings (key, value) VALUES (%s, %s::jsonb)
           ON CONFLICT (key) DO NOTHING""",
        (DESCRIPTIONS_FLAG, json.dumps({'done': True, 'changed': changed})))
    conn.commit()
    return changed


def repair_descriptions(db):
    """Runs the repair; a failure is told and never stops the worker."""
    try:
        conn = psycopg2.connect(db.db_url)
        try:
            changed = repair_descriptions_on(conn)
        finally:
            conn.close()
        if changed:
            print(f"   🧹 {changed} description(s) had their HTML turned into plain text")
        return changed
    except Exception as err:
        print(f"   ⚠️ Could not clean the stored descriptions ({type(err).__name__}: {err}); will try again at the next start")
        return None
