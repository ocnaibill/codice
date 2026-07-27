"""CBR metadata extractor using rarfile for real RAR archives.

Inspired by Komga's rarfile-based extraction for CBR files.
"""
import os
from typing import Optional
from .base import BaseExtractor, ExtractedMetadata


class CbrExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.cbr')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(format='cbr')

        try:
            import rarfile
            with rarfile.RarFile(file_path) as rf:
                # 1. Try to parse ComicInfo.xml
                for name in rf.namelist():
                    if name.lower() == 'comicinfo.xml':
                        try:
                            data = rf.read(name)
                            self._parse_comicinfo_xml(data, meta)
                        except Exception as e:
                            print(f"   ⚠️ ComicInfo.xml parse error: {e}")
                        break

                # 2. Count image pages
                valid_exts = ('.jpg', '.jpeg', '.png', '.webp')
                images = sorted([
                    f for f in rf.namelist()
                    if f.lower().endswith(valid_exts) and not f.endswith('/')
                ])
                meta.page_count = len(images)

                # 3. Extract cover from first image
                if images and not meta.cover_path:
                    try:
                        img_data = rf.read(images[0])
                        meta.cover_path = self._save_cover(img_data, file_path, covers_dir)
                    except Exception as e:
                        print(f"   ⚠️ Cover extraction error: {e}")

        except ImportError:
            print("   ⚠️ rarfile not installed. Install with: pip install rarfile")
            meta.title = self._fallback_title(file_path)
            meta.author = "Unknown Author"
            return meta
        except Exception as e:
            print(f"   ⚠️ CBR extraction error: {e}")

        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"

        return meta

    def _parse_comicinfo_xml(self, data: bytes, meta: ExtractedMetadata):
        """Reuse ComicInfo.xml parsing from XML standard."""
        import xml.etree.ElementTree as ET
        root = ET.fromstring(data)

        def text(tag):
            elem = root.find(tag)
            return elem.text.strip() if elem is not None and elem.text else ''

        title = text('Title')
        if title:
            meta.title = title
        series = text('Series')
        if series:
            meta.series = series
        num = text('Number')
        if num:
            try:
                meta.series_index = float(num)
            except ValueError:
                pass
        author = text('Writer') or text('Penciller') or text('Artist')
        if author:
            meta.author = author
        publisher = text('Publisher')
        if publisher:
            meta.publisher = publisher
        desc = text('Summary')
        if desc:
            meta.description = desc
        lang = text('LanguageISO') or text('Language')
        if lang:
            meta.language = lang
        genre = text('Genre')
        if genre:
            meta.tags = [g.strip() for g in genre.split(',') if g.strip()]