"""ComicVine API metadata provider for comics (CBZ/CBR).

Requires COMICVINE_API_KEY. ComicVine's search by an issue's name answers the same issue whatever the number the file has ("Absolute Batman
001" and "Absolute Batman 012" both gave "Absolute Zero"), so it is asked in two steps, the way ComicVine keeps its data: the volume (the
series as a publisher printed it) first, then the issue of that volume with the number of the file. Several volumes have the same name (a
publisher's, a translation, a mini-series), and the right one is the one that has an issue with that number. The credits of an issue (who wrote
it, who drew it) are a request of their own, made only for the answer that was chosen. Only the title leaves the server (DEC-097).
"""
import os
from typing import List, Optional

from .base import BaseProvider, Credit, MetadataRecord
from .http import get_json
from .match import TITLE_MIN
from .query import read_file_title
from .text import closeness, main_title

BASE = 'https://comicvine.gamespot.com/api'
VOLUMES = 10        # the volumes that the title finds
TRIED = 2           # the volumes whose issue is asked for
SERIES_ANSWERS = 3  # the volumes given as answers when there is no issue to give
MAX_CREDITS = 8
# Who wrote it comes first, then who drew it.
_ROLES = ('writer', 'penciller', 'artist', 'inker', 'colorist', 'letterer', 'editor', 'cover')


def _role_rank(role):
    words = [w.strip() for w in (role or '').lower().split(',')]
    return min([_ROLES.index(w) for w in words if w in _ROLES] or [len(_ROLES)])


class ComicVineProvider(BaseProvider):
    def __init__(self):
        self.api_key = os.getenv('COMICVINE_API_KEY', '')
        self.base_url = BASE

    @property
    def name(self) -> str:
        return 'ComicVine'

    @property
    def id(self) -> str:
        return 'comicvine'

    def _get(self, path, **params):
        """The `results` of a request, or None when ComicVine did not answer or said why not (its own status is in the body, apart from HTTP's)."""
        reply = get_json(self.name, f'{self.base_url}/{path}', params={'api_key': self.api_key, 'format': 'json', **params}, secrets=(self.api_key,))
        if not reply.ok or not isinstance(reply.data, dict):
            return None
        if reply.data.get('error') not in (None, 'OK'):
            print(f"   ⚠️ ComicVine: {reply.data.get('error')}")
            return None
        return reply.data.get('results')

    @staticmethod
    def _cover(image):
        image = image or {}
        return image.get('super_url') or image.get('original_url') or image.get('medium_url')

    def _series(self, volume: dict) -> MetadataRecord:
        """The answer about a volume as a whole, when there is no issue to give."""
        record = MetadataRecord(source=self.name)
        record.title = record.series = volume.get('name')
        record.publisher = (volume.get('publisher') or {}).get('name')
        record.publication_date = volume.get('start_year')
        record.description = volume.get('description') or volume.get('deck')
        record.cover_url = self._cover(volume.get('image'))
        record.prior = min((volume.get('count_of_issues') or 0) / 10, 10)
        record.raw = {'comicvine_id': f"4050-{volume.get('id')}", 'volume_id': volume.get('id'), 'issues_in_volume': volume.get('count_of_issues')}
        return record

    def _issue(self, volume: dict, issue: dict) -> MetadataRecord:
        record = self._series(volume)
        try:
            record.series_index = float(issue.get('issue_number'))
        except (TypeError, ValueError):
            pass
        record.title = issue.get('name') or f"{volume.get('name')} #{issue.get('issue_number')}"
        record.description = issue.get('description') or issue.get('deck') or record.description
        record.cover_url = self._cover(issue.get('image')) or record.cover_url
        record.publication_date = issue.get('cover_date') or record.publication_date
        record.raw.update(comicvine_id=f"4000-{issue.get('id')}", issue_id=issue.get('id'))
        return record

    def lookup(self, query) -> List[MetadataRecord]:
        """The issue the file is, in the volume that has it; when no volume close to the title has that issue, the volume itself."""
        if not self.api_key:
            print("   ⚠️ ComicVine: no API key (COMICVINE_API_KEY is empty), so it is not asked")
            return []
        title = query.search_title
        if not title:
            return []
        print(f"   🔎 ComicVine: searching for '{title}'" + (f" #{query.number}" if query.number is not None else ''))
        found = self._get('search', query=title, resources='volume', limit=VOLUMES,
                          field_list='id,name,start_year,count_of_issues,publisher,image,deck,description')
        close = [v for v in found or [] if v.get('name') and (closeness(title, v['name']) >= TITLE_MIN
                                                              or closeness(main_title(title), main_title(v['name'])) >= TITLE_MIN)]
        number = query.number

        # The volumes that can have the issue come first, the longest runs among them: a series that went on is the one people mean.
        close.sort(key=lambda v: -(v.get('count_of_issues') or 0))
        records = []
        if number is not None:
            # A volume that does not say how many issues it has may have the one we want.
            for volume in [v for v in close if v.get('count_of_issues') is None or v['count_of_issues'] >= number][:TRIED]:
                issues = self._get('issues', filter=f"volume:{volume['id']},issue_number:{number}",
                                   field_list='id,name,issue_number,cover_date,description,deck,image')
                if issues:
                    records.append(self._issue(volume, issues[0]))
        if not records:
            records = [self._series(v) for v in close[:SERIES_ANSWERS]]
        return records

    def enrich(self, record: MetadataRecord) -> MetadataRecord:
        """Who made the issue that was chosen: the credits of an issue are in its own page, not in the lists."""
        issue_id = (record.raw or {}).get('issue_id')
        if not issue_id or not self.api_key:
            return record
        detail = self._get(f'issue/4000-{issue_id}', field_list='person_credits')
        credits = [c for c in (detail or {}).get('person_credits') or [] if isinstance(c.get('name'), str) and c['name'].strip()]
        if not credits:
            return record   # the page could not be read, or says nobody: what the answer had stays
        credits.sort(key=lambda c: _role_rank(c.get('role')))
        record.credits = [Credit(c['name'].strip(), role=(c.get('role') or '').strip() or None,
                                 ids={'comicvine': str(c['id'])} if c.get('id') else {}) for c in credits[:MAX_CREDITS]]
        # Who wrote it, else who drew it (not who did the cover, the colours or the lettering).
        record.author = next((c.name for c in record.credits if _role_rank(c.role) < _ROLES.index('inker')), None)
        return record

    def search(self, query: str) -> Optional[MetadataRecord]:
        """The first answer for a text (the manual search of the registry)."""
        found = self.lookup(read_file_title(query, None, 'cbz'))
        return found[0] if found else None
