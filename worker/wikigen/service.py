"""The generation of the wiki as a service of the worker: it takes the queue of `wiki_runs` (DEC-185), one run at a time, and runs the
stages. It never decides what is generated: the administration asks, and this does it.

A run is held by one worker with a heartbeat; a worker that dies leaves it behind, and the next one takes it up (a resume). What the
model answered is kept by what was asked (cache.py), so taking up a run only pays for what is left.
"""
import hashlib
import json
import os
import threading
import time

import psycopg2

from . import epub
from .cache import DbCache
from .generate import RunFailed, generate
from .llm import Fatal, Llm, Provider, Stop, Usage
from wiki_secret import NoMasterKey, Unreadable, open_sealed

STAGES = ('text', 'map', 'identity', 'pages', 'import')
HEARTBEAT = 15          # seconds between a run saying it is alive
STALE = 120             # seconds without a heartbeat before another worker takes a run up
DEFAULT_MIN_CHAPTERS = 5
MAX_ERRORS = 10


class Control:
    """Asks the database, at most every two seconds, whether the run was asked to pause or cancel."""

    def __init__(self, db, run_id, owner):
        self.db, self.run_id, self.owner = db, run_id, owner
        self.lock = threading.Lock()
        self.checked = 0.0
        self.reason = None

    def __call__(self):
        if self.reason:
            raise Stop(self.reason)
        now = time.time()
        if now - self.checked < 2:
            return
        with self.lock:
            if time.time() - self.checked < 2:
                return
            self.checked = time.time()
            row = self.db.fetchone('SELECT pause_requested, cancel_requested, state, lease_owner FROM wiki_runs WHERE id = %s', (self.run_id,))
            if not row or row[2] != 'running' or row[3] != self.owner:
                self.reason = 'lost'          # taken up by another worker, or removed
            elif row[1]:
                self.reason = 'cancelled'
            elif row[0]:
                self.reason = 'paused'
        if self.reason:
            raise Stop(self.reason)


class Progress:
    """What the run says about itself: the stage, the count of each stage, where it is, the last errors and what it spent."""

    def __init__(self, db, run_id, usage, log=print):
        self.db, self.run_id, self.usage, self.log = db, run_id, usage, log
        self.lock = threading.Lock()
        self.stage = None
        self.counts = {}
        self.place = ''
        self.errors = []
        self.flushed = 0.0

    def begin(self, stage, total=0):
        with self.lock:
            self.stage = stage
            self.counts.setdefault(stage, {'done': 0, 'total': total})
            self.counts[stage]['total'] = total
        self.flush(force=True)

    def tick(self, stage=None, place=None):
        with self.lock:
            c = self.counts[stage or self.stage]
            c['done'] += 1
            if place is not None:
                self.place = place
        self.flush()

    def where(self, place):
        with self.lock:
            self.place = place
        self.flush()

    def error(self, message):
        with self.lock:
            self.errors = (self.errors + [{'at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), 'message': str(message)[:300]}])[-MAX_ERRORS:]
        self.flush(force=True)

    def flush(self, force=False):
        if not force and time.time() - self.flushed < 1.0:
            return
        with self.lock:
            self.flushed = time.time()
            counts = {k: dict(v) for k, v in self.counts.items()}
            payload = dict(counts, usage=self.usage.snapshot())
            row = (self.stage, json.dumps(payload), self.place, json.dumps(self.errors), self.run_id)
        self.db.execute('UPDATE wiki_runs SET stage = %s, progress = %s::jsonb, place = %s, errors = %s::jsonb, updated_at = now() '
                        'WHERE id = %s AND state = \'running\'', row)


class WikiService:
    def __init__(self, db, storage_root, owner=None, log=print, parallel=None):
        self.db, self.storage_root, self.log = db, storage_root, log
        self.owner = owner or f'wiki-{os.getpid()}-{int(time.time())}'
        self.parallel = parallel or int(os.getenv('WIKI_PARALLEL', '4'))
        self.llm_factory = None   # tests give their own

    # ---------------------------------------------------------------- the queue
    def tick(self):
        """One step of the loop: take up what a dead worker left, then run the next of the queue. True when it worked."""
        self.db.execute(
            "UPDATE wiki_runs SET state = 'queued', lease_owner = NULL, resumes = resumes + 1, updated_at = now() "
            "WHERE state = 'running' AND stage IS DISTINCT FROM 'import' AND heartbeat_at < now() - make_interval(secs => %s)", (STALE,))
        self.enqueue_new_books()
        if self.provider() is None:
            return False
        try:
            row = self.db.fetchone(
                "UPDATE wiki_runs SET state = 'running', lease_owner = %s, started_at = COALESCE(started_at, now()), heartbeat_at = now(), "
                "pause_requested = false, updated_at = now() "
                "WHERE id = (SELECT id FROM wiki_runs WHERE state = 'queued' ORDER BY created_at, id LIMIT 1 FOR UPDATE SKIP LOCKED) "
                "RETURNING id, work_id, collection_id, origin", (self.owner,))
        except psycopg2.errors.UniqueViolation:
            return False   # another worker has one running
        if not row:
            return False
        self.execute(*row)
        return True

    def run_forever(self, stop, interval=5.0):
        while not stop.is_set():
            try:
                worked = self.tick()
            except Exception as err:   # the database is away: wait for it
                self.log(f'   ⚠️ wiki: {type(err).__name__}: {err}')
                worked = False
            if not worked:
                stop.wait(interval)

    def enqueue_new_books(self):
        """A book that came into a series that has a wiki asks for its update (DEC-180): a run of origin new_book, unless the series is
        an exception, has a run live, or the last one asked this way failed or was cancelled in the last day."""
        self.db.execute("""
            INSERT INTO wiki_runs (collection_id, origin)
            SELECT p.collection_id, 'new_book'
            FROM wiki_projects p
            JOIN collections c ON c.id = p.collection_id AND c.kind = 'official' AND c.retired_at IS NULL
            WHERE p.collection_id IS NOT NULL AND p.state IN ('ready', 'partial')
              AND NOT EXISTS (SELECT 1 FROM wiki_exclusions x WHERE x.collection_id = p.collection_id)
              AND NOT EXISTS (SELECT 1 FROM wiki_runs r WHERE r.collection_id = p.collection_id
                              AND (r.state IN ('queued', 'running', 'paused')
                                   OR (r.origin = 'new_book' AND r.state IN ('failed', 'cancelled') AND r.created_at > now() - interval '1 day')))
              AND EXISTS (
                SELECT 1 FROM collection_works cw JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL
                WHERE cw.collection_id = p.collection_id AND cw.official
                  AND NOT EXISTS (SELECT 1 FROM wiki_books b WHERE b.project_id = p.id AND b.work_id = cw.work_id)
                  AND EXISTS (SELECT 1 FROM editions e JOIN files f ON f.edition_id = e.id JOIN text_extractions te ON te.file_id = f.id
                              WHERE e.work_id = cw.work_id AND lower(f.format) = 'epub' AND f.availability = 'available' AND te.status = 'ready'))
            ON CONFLICT DO NOTHING""")

    # ---------------------------------------------------------------- the provider and the settings
    def provider(self):
        row = self.db.fetchone("SELECT value FROM settings WHERE key = 'wiki.provider'")
        if not row or not row[0]:
            return None
        v = row[0]
        key = ''
        if v.get('keySealed'):
            try:
                key = open_sealed(v['keySealed'])
            except NoMasterKey:
                raise RunFailed('Falta a chave mestra deste servidor (CODICE_SECRET_KEY): o worker não consegue abrir a chave do provedor.')
            except Unreadable:
                raise RunFailed('A chave guardada não abre com a chave mestra deste servidor: digite a chave do provedor de novo.')
        return Provider(v.get('kind', 'openai'), v.get('baseUrl', ''), v.get('model', ''), key)

    def min_chapters(self):
        row = self.db.fetchone("SELECT value FROM settings WHERE key = 'wiki.settings'")
        try:
            n = int((row[0] or {}).get('minChapters', DEFAULT_MIN_CHAPTERS)) if row else DEFAULT_MIN_CHAPTERS
        except (TypeError, ValueError):
            n = DEFAULT_MIN_CHAPTERS
        return max(1, n)

    # ---------------------------------------------------------------- one run
    def execute(self, run_id, work_id, collection_id, origin):
        control = Control(self.db, run_id, self.owner)
        usage = Usage()
        progress = Progress(self.db, run_id, usage, self.log)
        beating = threading.Event()
        threading.Thread(target=self.beat, args=(run_id, beating), daemon=True).start()
        try:
            provider = self.provider()
            if provider is None:
                # the provider was turned off between the claim and now: back to the queue, nothing lost
                self.db.execute("UPDATE wiki_runs SET state = 'queued', lease_owner = NULL WHERE id = %s", (run_id,))
                return
            self.db.execute('UPDATE wiki_runs SET provider = %s, model = %s WHERE id = %s', (provider.kind, provider.model, run_id))
            cache = DbCache(self.db, work_id, collection_id)
            llm = (self.llm_factory or Llm)(provider, cache=cache, control=control, usage=usage)
            self.log(f'\n📚 Wiki run {run_id} ({origin}, {"series " + str(collection_id) if collection_id else "work " + str(work_id)})')
            bundle, partial = self.pipeline(llm, control, progress, work_id, collection_id, provider)
            body = json.dumps(bundle, ensure_ascii=False)
            self.db.execute('INSERT INTO wiki_run_bundles (run_id, bundle) VALUES (%s, %s::jsonb) '
                            'ON CONFLICT (run_id) DO UPDATE SET bundle = EXCLUDED.bundle', (run_id, body))
            progress.begin('import', 1)
            # from here the backend takes it: it imports the bundle and closes the run
            self.db.execute("UPDATE wiki_runs SET stage = 'import', heartbeat_at = now(), updated_at = now() WHERE id = %s", (run_id,))
            self.log(f'   📦 bundle of run {run_id}: {len(bundle["entities"])} entities, {len(body) // 1024} KB, waiting for the import')
        except Stop as stop:
            if stop.message:
                progress.error(stop.message)
            progress.flush(force=True)
            if stop.reason == 'paused':
                self.db.execute("UPDATE wiki_runs SET state = 'paused', lease_owner = NULL, pause_requested = false, updated_at = now() "
                                "WHERE id = %s AND lease_owner = %s", (run_id, self.owner))
            elif stop.reason == 'cancelled':
                self.db.execute("UPDATE wiki_runs SET state = 'cancelled', lease_owner = NULL, finished_at = now(), updated_at = now() "
                                "WHERE id = %s AND lease_owner = %s", (run_id, self.owner))
            # 'lost': another worker holds it now; nothing to write
        except (Fatal, RunFailed) as err:
            self.fail(run_id, str(err), progress)
        except Exception as err:   # a defect: the run says so, and the worker goes on
            self.log(f'   ❌ wiki run {run_id}: {type(err).__name__}: {err}')
            self.fail(run_id, f'Erro inesperado no gerador ({type(err).__name__}): {str(err)[:200]}', progress)
        finally:
            beating.set()

    def beat(self, run_id, done):
        """Says the run is alive while a request is slow: a long answer must not let another worker take it up."""
        while not done.wait(HEARTBEAT):
            try:
                self.db.execute('UPDATE wiki_runs SET heartbeat_at = now() WHERE id = %s AND lease_owner = %s', (run_id, self.owner))
            except Exception:
                pass

    def fail(self, run_id, message, progress):
        progress.error(message)
        self.db.execute("UPDATE wiki_runs SET state = 'failed', lease_owner = NULL, finished_at = now(), updated_at = now() WHERE id = %s", (run_id,))
        self.log(f'   ❌ wiki run {run_id}: {message}')

    # ---------------------------------------------------------------- the stages
    def load_books(self, work_id, collection_id, progress):
        if collection_id:
            rows = self.db.fetchall("""
                SELECT cw.work_id, w.original_title, w.spin_off FROM collection_works cw JOIN works w ON w.id = cw.work_id
                WHERE cw.collection_id = %s AND cw.official AND w.retired_at IS NULL
                ORDER BY cw.position NULLS LAST, w.series_index, w.id""", (collection_id,))
        else:
            rows = self.db.fetchall('SELECT id, original_title, FALSE FROM works WHERE id = %s AND retired_at IS NULL', (work_id,))
        books = []
        for wid, title, spin_off in rows:
            file = self.db.fetchone("""
                SELECT f.id, f.sha256, l.mode, l.root, l.path
                FROM editions e JOIN files f ON f.edition_id = e.id
                JOIN text_extractions te ON te.file_id = f.id AND te.status = 'ready'
                LEFT JOIN LATERAL (SELECT mode, root, path FROM storage_locations WHERE file_id = f.id ORDER BY id LIMIT 1) l ON TRUE
                LEFT JOIN work_primary wp ON wp.file_id = f.id
                WHERE e.work_id = %s AND lower(f.format) = 'epub' AND f.availability = 'available'
                ORDER BY (wp.file_id IS NOT NULL) DESC, f.id LIMIT 1""", (wid,))
            if not file:
                continue
            file_id, sha, mode, root, path = file
            from textindex.store import resolve
            full = resolve(self.storage_root, mode, root, path)
            if not full or not os.path.exists(full):
                continue
            if not sha:
                h = hashlib.sha256()
                with open(full, 'rb') as f:
                    for block in iter(lambda: f.read(1 << 20), b''):
                        h.update(block)
                sha = h.hexdigest()
            read = epub.read(full)
            if not read['chapters']:
                continue
            # a work the person marked as outside the main line (DEC-188) has a line of its own and does not count in the order
            books.append({'id': file_id, 'work_id': wid, 'titulo': title or read['title'], 'kind': 'side' if spin_off else 'main', 'sha256': sha,
                          'chapters': {c['index']: c for c in read['chapters']}, 'mapa': {}})
        if not books:
            raise RunFailed('Nenhum livro em EPUB com o texto lido: não há o que gerar.')
        return books

    def pipeline(self, llm, control, progress, work_id, collection_id, provider):
        progress.begin('text', 1)
        books = self.load_books(work_id, collection_id, progress)
        progress.tick('text')
        for b in books:
            self.log(f"   📖 {b['titulo']} ({b['kind']}): {len(b['chapters'])} capítulos")
        bundle, partial = generate(llm, books, self.min_chapters(), progress, control, self.parallel, provider.kind, provider.model,
                                   self.is_remote(provider))
        self.log(f'   ✅ {len(bundle["entities"])} entities, {len(bundle["edges"])} edges' + (' (partial)' if partial else ''))
        return bundle, partial

    @staticmethod
    def is_remote(provider):
        from urllib.parse import urlparse
        import ipaddress
        host = (urlparse(provider.base_url).hostname or '').lower()
        try:
            ip = ipaddress.ip_address(host)
            return not (ip.is_loopback or ip.is_private or ip.is_link_local)
        except ValueError:
            pass
        if host == 'localhost' or '.' not in host:
            return False
        return not host.endswith(('.local', '.lan', '.internal', '.home.arpa', '.localhost'))
