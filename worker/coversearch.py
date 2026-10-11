"""Looking for covers of a work, when staff ask (DEC-174).

A cover from a provider is never put on a work by itself once the work has one (DEC-173): here a person asks, sees what the providers that are
on have, and chooses one, which the page then cuts to 2:3 and sends like a picture of their own. So what the providers answer is only kept to
be seen: each cover is downloaded to a folder that expires (the page cannot link to a provider's image, and some providers forbid it), and
nothing of the work changes. Only the title typed or read from the work, and its author and ISBN, leave the server, and only to the providers
the owner turned on."""
import hashlib
import json
import os
import time

from analyzer import Analyzer
from providers.http import get_binary
from rematch import title_of, with_series

MAX_QUERY = 200
MAX_CANDIDATES = 12
MAX_IMAGE = 4 * 1024 * 1024
KEEP_SECONDS = 6 * 3600
# What an image is, by its first bytes: the page decodes these three, and nothing else is kept.
IMAGE_TYPES = ((b'\xff\xd8\xff', 'jpg'), (b'\x89PNG\r\n\x1a\n', 'png'))


def image_extension(data):
    """"jpg", "png" or "webp" when the bytes are such an image, else None."""
    for magic, extension in IMAGE_TYPES:
        if data.startswith(magic):
            return extension
    if data[:4] == b'RIFF' and data[8:12] == b'WEBP':
        return 'webp'
    return None


class CoverSearcher:
    """Runs the job `cover_search`: {work, query}. The answer is kept in work_cover_searches for the page to read."""

    def __init__(self, db, registry, analyzer: Analyzer, cache_dir, download=get_binary, clock=time.time):
        self.db, self.registry, self.analyzer = db, registry, analyzer
        self.cache_dir, self.download, self.clock = cache_dir, download, clock

    def _keep(self, work_id, state, results):
        self.db.execute(
            "UPDATE work_cover_searches SET state = %s, results = %s::jsonb, finished_at = now() WHERE work_id = %s",
            (state, json.dumps(results), work_id))

    def _sweep(self, work_id):
        """Takes away what an earlier search of this work left, and whatever any search left that is old: a folder that is not looked at again
        must not keep covers for ever."""
        try:
            names = os.listdir(self.cache_dir)
        except FileNotFoundError:
            return
        limit = self.clock() - KEEP_SECONDS
        for name in names:
            path = os.path.join(self.cache_dir, name)
            try:
                if name.startswith(f'{work_id}_') or os.path.getmtime(path) < limit:
                    os.remove(path)
            except OSError:
                pass

    def run(self, job_id, work_id, query, checkpoint=lambda: None):
        query = ' '.join(str(query or '').split())[:MAX_QUERY]
        if not isinstance(work_id, int):
            return {'state': 'failed'}
        row = self.db.fetchone(
            """SELECT COALESCE((SELECT file_format FROM work_primary WHERE work_id = w.id), ''), w.retired_at IS NOT NULL
               FROM works w WHERE w.id = %s""", (work_id,))
        if not row or row[1]:
            self._keep(work_id, 'failed', [])
            return {'state': 'failed'}
        format = row[0] or 'default'
        if not self.registry.any_on(format):
            self._keep(work_id, 'off', [])
            return {'state': 'off'}
        values = self.analyzer._load_state(work_id)['values']
        if query:
            title, author, isbn = query, None, None
        else:
            title = with_series(title_of(self.analyzer, values.get('title')), values.get('series'))
            author, isbn = values.get('author') or None, values.get('isbn') or None
        self._sweep(work_id)
        if not title:
            self._keep(work_id, 'done', [])
            return {'state': 'done', 'found': 0}
        checkpoint()
        records = self.registry.search_all(title, format, author=author, isbn=isbn)
        os.makedirs(self.cache_dir, exist_ok=True)
        results, seen = [], set()
        for record in records:
            url = getattr(record, 'cover_url', None)
            if not url or url in seen:
                continue
            seen.add(url)
            if len(results) >= MAX_CANDIDATES:
                break
            checkpoint()
            provider = self.registry.provider(getattr(record, 'provider_id', None))
            reply = self.download(provider.name if provider else (record.source or 'provider'), url, MAX_IMAGE)
            data = reply.data if reply.ok and isinstance(reply.data, bytes) else None
            extension = image_extension(data) if data else None
            if extension is None:
                continue
            name = f"{work_id}_{len(results)}_{hashlib.md5(url.encode()).hexdigest()[:8]}.{extension}"
            with open(os.path.join(self.cache_dir, name), 'wb') as handle:
                handle.write(data)
            results.append({'file': name, 'provider': getattr(record, 'provider_id', '') or '', 'source': record.source or '',
                            'title': (record.title or '')[:200], 'author': (record.author or '')[:200]})
        self._keep(work_id, 'done', results)
        return {'state': 'done', 'found': len(results)}
