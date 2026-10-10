"""Searching the providers again for a work that is already in the library (DEC-143).

Someone from the staff asks for it, from the page of the work. The file is not read again: what is asked is what the work says now (its title,
its author, the ISBN of its edition), which a person may have corrected, and the answers come back as suggestions, like the ones the first
analysis gave, for the owner or an admin to decide on: keep what is there, add what is missing or change it. Fields a person confirmed get
suggestions too, as the one who asked wants to be able to change them. What was rejected before is not offered again."""
import json
import os

from analyzer import Analyzer
from pipeline import suggest
from providers.text import tokens


def title_of(analyzer: Analyzer, title):
    """The title to search for: the work's own, and when it is still the name of the file (nothing read it yet) that name without the extension."""
    if analyzer._is_placeholder('title', title):
        return os.path.splitext(title)[0].replace('_', ' ').strip()
    return (title or '').strip()


def with_series(title, series):
    """The title with the series of the work in parentheses when the title does not say it ("A nuvem" of "Scythe" is searched as "A nuvem (Scythe)"):
    what a title says in parentheses is also searched by (providers.query.parenthetical), and the series is what finds a book that the title alone
    does not. The title that was in the file said it, and is not kept once someone accepts a cleaner one."""
    series = (series or '').strip()
    if not title or not series or not set(tokens(series)) - set(tokens(title)):
        return title
    return f'{title} ({series})'


class MetadataRematch:
    def __init__(self, db, analyzer: Analyzer, registry):
        self.db, self.analyzer, self.registry = db, analyzer, registry

    def run(self, job_id, work_id, checkpoint=lambda: None) -> dict:
        """Asks the providers that are on and stores what they answer. The outcome is written into the job, where the page of the work reads it:
        {found, source, title, new} (`new` is how many suggestions were not there before)."""
        row = self.db.fetchone(
            """SELECT COALESCE((SELECT file_format FROM work_primary WHERE work_id = w.id), ''), w.retired_at IS NOT NULL
               FROM works w WHERE w.id = %s""", (work_id,))
        if not row or row[1]:
            raise ValueError('the work does not exist')
        values = self.analyzer._load_state(work_id)['values']
        title = with_series(title_of(self.analyzer, values.get('title')), values.get('series'))
        result = {'found': False, 'new': 0}
        if title:
            checkpoint()
            best = self.registry.search_best(title, row[0] or 'default', author=values.get('author') or None, isbn=values.get('isbn') or None)
            if best:
                source, stored, _ = suggest(self.analyzer, work_id, best, title, include_locked=True)
                result = {'found': True, 'source': source, 'title': best.title, 'new': stored}
        print(f"   🔁 Searched again for '{title}': {result}")
        self.db.execute("UPDATE jobs SET payload = payload || %s::jsonb WHERE id = %s", (json.dumps({'result': result}), job_id))
        return result
