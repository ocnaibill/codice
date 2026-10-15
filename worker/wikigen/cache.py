"""What the model answered, kept by the hash of what was asked (llm.cache_key).

Re-running a stage asks the same questions in the same words, so a run that was stopped, a book that came into the series, or a lower
cut for full pages only pays for the questions that are new. The answer is the JSON that was checked, never the failures.
"""
import json
import threading


class MemoryCache:
    """For tests, and for a run with no database."""

    def __init__(self):
        self.items = {}
        self.lock = threading.Lock()

    def get(self, key):
        with self.lock:
            value = self.items.get(key)
        return json.loads(value) if value is not None else None

    def put(self, key, data, model=''):
        with self.lock:
            self.items.setdefault(key, json.dumps(data, ensure_ascii=False))


class DbCache:
    """The table wiki_llm_cache. `work_id` and `collection_id` say whose wiki asked, so that deleting the wiki takes the answers with it."""

    def __init__(self, db, work_id=None, collection_id=None):
        self.db, self.work_id, self.collection_id = db, work_id, collection_id

    def get(self, key):
        row = self.db.fetchone('SELECT response FROM wiki_llm_cache WHERE key = %s', (key,))
        return row[0] if row else None

    def put(self, key, data, model=''):
        self.db.execute(
            'INSERT INTO wiki_llm_cache (key, response, model, work_id, collection_id) VALUES (%s, %s::jsonb, %s, %s, %s) '
            'ON CONFLICT (key) DO NOTHING',
            (key, json.dumps(data, ensure_ascii=False), model, self.work_id, self.collection_id))
