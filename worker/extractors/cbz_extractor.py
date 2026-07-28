"""CBZ/CBR metadata extractor using zipfile + ComicInfo.xml parsing.

Inspired by Komga's ComicInfo.xml parsing approach:
reads the XML metadata embedded in CBZ/CBR archives.
"""
import os
import zipfile
import xml.etree.ElementTree as ET
from .base import BaseExtractor, ExtractedMetadata


class CbzExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.cbz') or file_path.lower().endswith('.cbr')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(
            format='cbz' if file_path.lower().endswith('.cbz') else 'cbr'
        )

        try:
            with zipfile.ZipFile(file_path, 'r') as zf:
                # 1. Try to parse ComicInfo.xml
                comicinfo_present = any(f.lower() == 'comicinfo.xml' for f in zf.namelist())
                if comicinfo_present:
                    # Find actual ComicInfo.xml (case-insensitive)
                    for name in zf.namelist():
                        if name.lower() == 'comicinfo.xml':
                            try:
                                data = zf.read(name)
                                self._parse_comicinfo(data, meta)
                            except Exception as e:
                                print(f"   ⚠️ ComicInfo.xml parse error: {e}")
                            break

                # 2. Count image pages and store page metadata
                valid_exts = ('.jpg', '.jpeg', '.png', '.webp')
                images = sorted([
                    f for f in zf.namelist()
                    if f.lower().endswith(valid_exts) and not f.endswith('/')
                ])
                meta.page_count = len(images)
                meta.raw['pages'] = [
                    {'page_number': i, 'file_name': name}
                    for i, name in enumerate(images)
                ]

                # 3. Extract cover from first image
                if images and not meta.cover_path:
                    first_image = images[0]
                    try:
                        img_data = zf.read(first_image)
                        meta.cover_path = self._save_cover(img_data, file_path, covers_dir)
                    except Exception as e:
                        print(f"   ⚠️ Cover extraction error: {e}")

        except Exception as e:
            print(f"   ⚠️ CBZ extraction error: {e}")

        # If ComicInfo.xml was parsed and provided a title, keep it
        # Otherwise fallback to filename
        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"

        return meta

    def _parse_comicinfo(self, data: bytes, meta: ExtractedMetadata):
        """Parse ComicInfo.xml and populate metadata fields."""
        print(f"   📄 ComicInfo.xml: {len(data)} bytes")
        root = ET.fromstring(data)
        print(f"   🏷️ ComicInfo.xml root tag: {root.tag}")

        def text(tag):
            elem = root.find(tag)
            if elem is not None:
                print(f"   🔍 ComicInfo.xml found '{tag}': '{elem.text.strip() if elem.text else 'None'}'")
            return elem.text.strip() if elem is not None and elem.text else ''

        title = text('Title')
        if title:
            meta.title = title

        series = text('Series')
        if series:
            meta.series = series

        series_index_str = text('Number')
        if series_index_str:
            try:
                meta.series_index = float(series_index_str)
            except ValueError:
                pass

        author = text('Writer') or text('Penciller') or text('Artist')
        if author:
            meta.author = author

        publisher = text('Publisher')
        if publisher:
            meta.publisher = publisher

        description = text('Summary')
        if description:
            meta.description = description

        language = text('LanguageISO') or text('Language')
        if language:
            meta.language = language

        isbn = text('ISBN')
        if isbn:
            meta.isbn = isbn

        date = text('Year') or text('Month') or text('Day')
        if date:
            meta.publication_date = date

        # Parse genre tags
        genre = text('Genre')
        if genre:
            meta.tags = [g.strip() for g in genre.split(',') if g.strip()]

        # Check for <Pages> tag
        pages_str = text('Pages')
        if pages_str:
            try:
                meta.page_count = int(pages_str)
            except ValueError:
                pass