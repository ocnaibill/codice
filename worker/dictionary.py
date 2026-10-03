"""Installs a dictionary the owner asked for (#109, DEC-115).

A dictionary is third-party data: the Wiktionary, extracted by Wiktextract and published at kaikki.org as one JSON
object per line (compressed). The owner clicks "install" and the API queues a job; this module downloads the file from
the address the server's catalog gave (and nowhere else), reads it line by line, keeps what the card needs and puts it
in PostgreSQL. All of it goes in one transaction: a package is all in or not at all, and the old one keeps working
until the new one is complete.
"""
import gzip
import hashlib
import json
import os
import re
import shutil
import tempfile
import unicodedata
import urllib.error
import urllib.request
import zlib
from urllib.parse import urlparse

import psycopg2
import psycopg2.extras

from runner import Cancelled

# The only host a package is downloaded from (the server's catalog says the same, internal/dictionary).
ALLOWED_HOSTS = {'kaikki.org'}
USER_AGENT = 'Codice-dictionary-installer/1 (+https://github.com/ocnaibill/codice)'

# The languages of the library: the words of these are kept from a package that does not say otherwise, and the translations
# into them are kept from every package (the rest of the file is let go).
LANGS = ('pt', 'en', 'es', 'fr', 'de', 'it', 'ja', 'zh')
_CODE = re.compile(r'^[a-z]{2,3}$')
MAX_HEADWORDS = 20

MAX_SENSES = 40
MAX_GLOSS = 600
MAX_EXAMPLES = 2
MAX_EXAMPLE = 300
MAX_FORMS = 80
MAX_FORM = 40
MAX_TRANSLATIONS = 40
MAX_LINE = 5_000_000  # a line longer than this is not an entry
BATCH = 2000
CHUNK = 1 << 20


# ── What a word reads as ───────────────────────────────────────────────────────────────────────────────────────────
# A selected word is looked up by what it reads as with no case and no accent, so "Ação" finds "ação" and "ACAO" too.
# An accent that is only a mark on a Latin letter goes; a mark that makes a letter of another script (the dakuten of か
# and が) stays, because は and ば are other letters. Said again in internal/dictionary/normalize.go and tested against
# the same cases (backend/internal/dictionary/testdata/normalize.json).

def _strip_marks(char: str) -> str:
    decomposed = unicodedata.normalize('NFD', char)
    if len(decomposed) > 1 and unicodedata.name(decomposed[0], '').startswith('LATIN'):
        return decomposed[0]
    return char


def normalize(text: str) -> str:
    """What a word reads as: the form it is looked up by."""
    folded = unicodedata.normalize('NFKC', str(text)).replace('’', "'").replace('ʼ', "'").casefold()
    letters = ''.join(_strip_marks(c) for c in folded)
    # The edges of what was selected are not part of the word: a full stop, a quotation mark, a parenthesis.
    start, end = 0, len(letters)
    while start < end and unicodedata.category(letters[start])[0] not in 'LNM':
        start += 1
    while end > start and unicodedata.category(letters[end - 1])[0] not in 'LNM':
        end -= 1
    return letters[start:end]


# ── What of an entry is kept ───────────────────────────────────────────────────────────────────────────────────────

def _clip(text, limit):
    text = str(text).strip()
    return text if len(text) <= limit else text[: limit - 1].rstrip() + '…'


def _strings(value, limit):
    return [t for t in (value or []) if isinstance(t, str) and t.strip()][:limit]


def _keep_form(form) -> bool:
    """A form is a word: not a dash, a note about the table, or a sentence."""
    return (isinstance(form, str) and 0 < len(form) <= MAX_FORM and any(c.isalpha() for c in form)
            and len(form.split()) <= 3)


def trim_entry(raw, headwords=LANGS, translations_to=LANGS):
    """The entry as it is kept, or None if it is not for this dictionary: a word of a language that is not kept, or no sense.
    The translations kept are the ones into the languages of the library, and into the languages whose words are kept."""
    targets = set(translations_to) | set(headwords)
    if not isinstance(raw, dict):
        return None
    lang, word = raw.get('lang_code'), raw.get('word')
    if lang not in headwords or not isinstance(word, str) or not word.strip():
        return None
    senses = []
    for sense in raw.get('senses') or []:
        if not isinstance(sense, dict):
            continue
        glosses = [_clip(g, MAX_GLOSS) for g in _strings(sense.get('glosses'), 6)]
        form_of = [{'word': x['word']} for x in (sense.get('form_of') or []) if isinstance(x, dict) and isinstance(x.get('word'), str) and x['word']][:4]
        alt_of = [{'word': x['word']} for x in (sense.get('alt_of') or []) if isinstance(x, dict) and isinstance(x.get('word'), str) and x['word']][:4]
        if not glosses and not form_of and not alt_of:
            continue
        kept = {'glosses': glosses}
        tags = _strings(sense.get('tags'), 8)
        if tags:
            kept['tags'] = tags
        if form_of:
            kept['form_of'] = form_of
        if alt_of:
            kept['alt_of'] = alt_of
        examples = []
        for example in sense.get('examples') or []:
            if len(examples) >= MAX_EXAMPLES:
                break
            if isinstance(example, dict) and isinstance(example.get('text'), str) and example['text'].strip():
                item = {'text': _clip(example['text'], MAX_EXAMPLE)}
                if isinstance(example.get('translation'), str) and example['translation'].strip():
                    item['translation'] = _clip(example['translation'], MAX_EXAMPLE)
                examples.append(item)
        if examples:
            kept['examples'] = examples
        senses.append(kept)
        if len(senses) >= MAX_SENSES:
            break
    if not senses:
        return None

    data = {'senses': senses}
    forms, seen = [], set()
    for form in raw.get('forms') or []:
        if not isinstance(form, dict) or not _keep_form(form.get('form')):
            continue
        tags = _strings(form.get('tags'), 8)
        key = (form['form'], tuple(tags))
        if key in seen:
            continue
        seen.add(key)
        forms.append({'form': form['form'], **({'tags': tags} if tags else {})})
        if len(forms) >= MAX_FORMS:
            break
    if forms:
        data['forms'] = forms
    translations = []
    for t in raw.get('translations') or []:
        if isinstance(t, dict) and t.get('lang_code') in targets and isinstance(t.get('word'), str) and t['word'].strip():
            item = {'lang': t['lang_code'], 'word': t['word']}
            if isinstance(t.get('sense'), str) and t['sense'].strip():
                item['sense'] = _clip(t['sense'], 120)
            translations.append(item)
            if len(translations) >= MAX_TRANSLATIONS:
                break
    if translations:
        data['translations'] = translations
    for sound in raw.get('sounds') or []:
        if isinstance(sound, dict):
            pron = sound.get('ipa') or sound.get('zh_pron')
            if isinstance(pron, str) and pron.strip():
                data['ipa'] = _clip(pron, 80)
                break
    return {'lang': lang, 'word': word, 'pos': raw.get('pos') if isinstance(raw.get('pos'), str) else '', 'data': data}


def rows_of(package_id, entry):
    """What goes in the tables for one kept entry: (entry row, form rows, link rows)."""
    lang, word, pos, data = entry['lang'], entry['word'], entry['pos'], entry['data']
    norm = normalize(word)
    if not norm:
        return None, [], []
    entry_row = (package_id, lang, word, norm, pos, psycopg2.extras.Json(data, dumps=lambda d: json.dumps(d, ensure_ascii=False)))
    form_rows = []
    for form in data.get('forms', []):
        form_norm = normalize(form['form'])
        if form_norm and form_norm != norm:
            form_rows.append((package_id, lang, form_norm, form['form'], word, pos, form.get('tags', [])))
    link_rows = []
    for t in data.get('translations', []):
        link_norm = normalize(t['word'])
        if link_norm:
            link_rows.append((package_id, t['lang'], link_norm, t['word'], lang, word, pos, t.get('sense', '')))
    return entry_row, form_rows, link_rows


# ── The installation ───────────────────────────────────────────────────────────────────────────────────────────────

class PermanentDownloadError(ValueError):
    """A download that will fail the same way every time (the address is gone, the file is not what it says)."""


def _check_url(url):
    parsed = urlparse(str(url))
    if parsed.scheme != 'https' or parsed.hostname not in ALLOWED_HOSTS or parsed.username or parsed.port:
        raise ValueError(f'Not an address a dictionary may be downloaded from: {url}')


class DictionaryImporter:
    """Downloads a package and imports it. `connect` makes a database connection (the work is done in one, and the
    progress written in another, so that it can be seen while the first is still open)."""

    PROGRESS_EVERY = 25_000  # lines

    def __init__(self, connect, workdir=None, opener=None, headwords=LANGS):
        self.connect = connect
        self.workdir = workdir
        self.opener = opener or urllib.request.urlopen
        self.headwords = tuple(headwords)

    # What the owner is told, kept in the row of the package.
    def _say(self, conn, package_id, **fields):
        names = ', '.join(f'{k} = %s' for k in fields)
        with conn.cursor() as cur:
            cur.execute(f'UPDATE dictionary_packages SET {names}, updated_at = now() WHERE id = %s', [*fields.values(), package_id])

    def retrying(self, job, message):
        conn = self.connect()
        try:
            conn.autocommit = True
            self._say(conn, job['payload'].get('package'), stage='retrying', error=message[:500])
        finally:
            conn.close()

    def fail(self, job, message):
        conn = self.connect()
        try:
            conn.autocommit = True
            self._say(conn, job['payload'].get('package'), state='failed', error=message[:500])
        finally:
            conn.close()

    def run(self, job, checkpoint):
        payload = job['payload']
        package_id, url = payload.get('package'), payload.get('url')
        if not package_id or not url:
            raise ValueError('The job names no package or address')
        _check_url(url)
        headwords = self._headwords(payload)
        progress = self.connect()
        progress.autocommit = True
        directory = tempfile.mkdtemp(prefix='codice-dict-', dir=self.workdir)
        try:
            with progress.cursor() as cur:
                cur.execute("SELECT 1 FROM dictionary_packages WHERE id = %s AND state = 'installing'", (package_id,))
                if cur.fetchone() is None:
                    raise ValueError(f'The package {package_id} is not being installed')
            path = os.path.join(directory, 'package.jsonl.gz')
            info = self._download(url, path, progress, package_id, checkpoint)
            checkpoint()
            counts = self._import(package_id, path, info, progress, checkpoint, headwords)
            return counts
        except Cancelled:
            try:
                self._say(progress, package_id, state='failed', stage='cancelled', error='Instalação cancelada.')
            except Exception as err:  # the job is cancelled all the same
                print(f'   ⚠️ could not say the installation was cancelled: {err}')
            raise
        finally:
            shutil.rmtree(directory, ignore_errors=True)
            progress.close()

    def _download(self, url, path, progress, package_id, checkpoint):
        self._say(progress, package_id, stage='downloading', progress=0, bytes_done=0, error='')
        request = urllib.request.Request(url, headers={'User-Agent': USER_AGENT})
        try:
            response = self.opener(request, timeout=60)
        except urllib.error.HTTPError as err:
            if 400 <= err.code < 500 and err.code not in (408, 429):
                raise PermanentDownloadError(f'The source answered {err.code} for the package') from err
            raise
        sha = hashlib.sha256()
        done = 0
        last = 0
        with response:
            _check_url(getattr(response, 'url', None) or url)  # a redirect leads nowhere else
            total = int(response.headers.get('Content-Length') or 0) or None
            source_date = response.headers.get('Last-Modified')
            self._say(progress, package_id, bytes_total=total, source_date=source_date)
            with open(path, 'wb') as out:
                while True:
                    chunk = response.read(CHUNK)
                    if not chunk:
                        break
                    out.write(chunk)
                    sha.update(chunk)
                    done += len(chunk)
                    checkpoint()
                    if done - last >= 4 * CHUNK:
                        last = done
                        self._say(progress, package_id, bytes_done=done, progress=min(1.0, done / total) if total else 0)
        if total and done != total:
            raise OSError(f'The download stopped at {done} of {total} bytes')
        self._say(progress, package_id, bytes_done=done, progress=1)
        return {'sha256': sha.hexdigest(), 'source_date': source_date, 'bytes': done}

    def _headwords(self, payload):
        """The languages whose words are kept: the ones the job says (the catalog's), or the languages of the library."""
        given = payload.get('headwords')
        if given is None:
            return self.headwords
        if not isinstance(given, list) or not given or len(given) > MAX_HEADWORDS or not all(isinstance(c, str) and _CODE.match(c) for c in given):
            raise ValueError(f'The job says which languages to keep in a way that is not one: {given!r}')
        return tuple(dict.fromkeys(given))

    def _import(self, package_id, path, info, progress, checkpoint, headwords):
        self._say(progress, package_id, stage='importing', progress=0)
        size = os.path.getsize(path) or 1
        conn = self.connect()
        try:
            cur = conn.cursor()
            for table in ('dictionary_entries', 'dictionary_forms', 'dictionary_links'):
                cur.execute(f'DELETE FROM {table} WHERE package_id = %s', (package_id,))
            entries, forms, links, langs, skipped = [], [], [], set(), 0
            totals = {'entries': 0, 'forms': 0, 'links': 0}

            def flush():
                if entries:
                    psycopg2.extras.execute_values(
                        cur, 'INSERT INTO dictionary_entries (package_id, lang, word, norm, pos, data) VALUES %s', entries, page_size=len(entries))
                if forms:
                    psycopg2.extras.execute_values(
                        cur, 'INSERT INTO dictionary_forms (package_id, lang, norm, form, lemma, pos, tags) VALUES %s', forms, page_size=len(forms))
                if links:
                    psycopg2.extras.execute_values(
                        cur, 'INSERT INTO dictionary_links (package_id, lang, norm, word, target_lang, target_word, pos, sense) VALUES %s', links, page_size=len(links))
                totals['entries'] += len(entries)
                totals['forms'] += len(forms)
                totals['links'] += len(links)
                entries.clear()
                forms.clear()
                links.clear()

            raw = open(path, 'rb')
            try:
                try:
                    with gzip.GzipFile(fileobj=raw) as gz:
                        lines = 0
                        for line in gz:
                            lines += 1
                            if len(line) > MAX_LINE:
                                skipped += 1
                                continue
                            try:
                                entry = trim_entry(json.loads(line), headwords)
                            except (json.JSONDecodeError, UnicodeDecodeError):
                                skipped += 1
                                continue
                            if entry is None:
                                continue
                            entry_row, form_rows, link_rows = rows_of(package_id, entry)
                            if entry_row is None:
                                continue
                            langs.add(entry['lang'])
                            entries.append(entry_row)
                            forms.extend(form_rows)
                            links.extend(link_rows)
                            if len(entries) >= BATCH:
                                flush()
                                checkpoint()
                            if lines % self.PROGRESS_EVERY == 0:
                                self._say(progress, package_id, progress=min(1.0, raw.tell() / size), entries=totals['entries'] + len(entries))
                except (gzip.BadGzipFile, EOFError, zlib.error) as err:
                    raise ValueError(f'The file is not a complete gzip file: {err}') from err
                flush()
                if totals['entries'] == 0:
                    raise ValueError('The file has no entry of the languages kept: it is not the dictionary that was asked for')
            finally:
                raw.close()
            # Everything is in: the package says so in the same transaction, so that it is ready exactly when its data is.
            cur.execute(
                """UPDATE dictionary_packages SET state = 'ready', stage = 'done', progress = 1, entries = %s, forms = %s, links = %s,
                          languages = %s, sha256 = %s, source_date = %s, error = '', installed_at = now(), updated_at = now()
                   WHERE id = %s""",
                (totals['entries'], totals['forms'], totals['links'], sorted(langs), info['sha256'], info['source_date'], package_id))
            conn.commit()
            return {**totals, 'skipped': skipped, 'languages': sorted(langs)}
        except BaseException:
            conn.rollback()
            raise
        finally:
            conn.close()


def connect_from_env():
    """A connection factory for the worker (the same database as the rest of it)."""
    url = os.getenv('DATABASE_URL', 'postgres://codice_user:codice_secret@localhost:5432/codice_db?sslmode=disable')
    return lambda: psycopg2.connect(url)
