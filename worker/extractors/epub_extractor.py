"""EPUB metadata extractor using zipfile + lxml for OPF parsing.

Inspired by Calibre-Web's EPUB extraction: parses the OPF file
inside the EPUB container for Dublin Core metadata, and extracts
the cover image from the manifest.
"""
import os
import zipfile
from typing import Optional
from .base import BaseExtractor, ExtractedMetadata


class EpubExtractor(BaseExtractor):
    def can_extract(self, file_path: str) -> bool:
        return file_path.lower().endswith('.epub')

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(format='epub')

        try:
            with zipfile.ZipFile(file_path, 'r') as zf:
                # Find and parse OPF file
                opf_path = self._find_opf(zf)
                if opf_path:
                    self._parse_opf(zf, opf_path, meta, file_path, covers_dir)
                else:
                    # Fallback: try to find container.xml via XML parser
                    try:
                        from lxml import etree
                        container_data = zf.read('META-INF/container.xml')
                        container_root = etree.fromstring(container_data, parser=etree.XMLParser(recover=True))
                        ns = {'c': 'urn:oasis:names:tc:opendocument:xmlns:container'}
                        rootfile = container_root.find('.//c:rootfile', ns)
                        if rootfile is not None:
                            opf_path = rootfile.get('full-path')
                            self._parse_opf(zf, opf_path, meta, file_path, covers_dir)
                    except (KeyError, Exception):
                        pass

                # Count pages (xhtml files)
                page_count = sum(
                    1 for f in zf.namelist()
                    if f.endswith('.xhtml') or f.endswith('.html') or f.endswith('.htm')
                )
                meta.page_count = max(page_count, 1)

        except Exception as e:
            print(f"   ⚠️ EPUB extraction error: {e}")

        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"

        return meta

    def _find_opf(self, zf: zipfile.ZipFile) -> Optional[str]:
        for name in zf.namelist():
            if name.endswith('.opf'):
                return name
        return None

    def _parse_opf(self, zf, opf_path, meta, file_path, covers_dir):
        try:
            from lxml import etree
            ns = {
                'dc': 'http://purl.org/dc/elements/1.1/',
                'opf': 'http://www.idpf.org/2007/opf',
            }

            opf_data = zf.read(opf_path)
            root = etree.fromstring(opf_data, parser=etree.XMLParser(recover=True, encoding='utf-8'))

            # Dublin Core metadata
            for elem in root.iter():
                tag = elem.tag.split('}')[-1] if '}' in elem.tag else elem.tag
                text = (elem.text or '').strip()
                if not text:
                    continue

                if tag == 'title' and not meta.title:
                    meta.title = text
                elif tag == 'creator' and not meta.author:
                    meta.author = text
                elif tag == 'publisher' and not meta.publisher:
                    meta.publisher = text
                elif tag == 'language' and not meta.language:
                    meta.language = text
                elif tag == 'identifier':
                    value = text.lower()
                    if 'isbn' in value:
                        meta.isbn = text
                    elif not meta.isbn:
                        meta.isbn = text
                elif tag == 'description' and not meta.description:
                    meta.description = text
                elif tag == 'date' and not meta.publication_date:
                    meta.publication_date = text
                elif tag == 'subject':
                    meta.tags.append(text)

            # Try to extract cover image
            self._extract_cover(zf, root, opf_path, file_path, covers_dir, meta)

        except Exception as e:
            print(f"   ⚠️ OPF parsing error: {e}")

    def _extract_cover(self, zf, root, opf_path, file_path, covers_dir, meta):
        """Extract cover image from EPUB manifest."""
        import re
        opf_dir = os.path.dirname(opf_path)

        # Find cover image in manifest
        cover_id = None
        for meta_elem in root.iter():
            tag = meta_elem.tag.split('}')[-1] if '}' in meta_elem.tag else meta_elem.tag
            if tag == 'meta':
                name = meta_elem.get('name', '')
                content = meta_elem.get('content', '')
                if name.lower() in ('cover', 'cover-image'):
                    cover_id = content

        if cover_id:
            # Find the cover in manifest
            for child in root.iter():
                tag = child.tag.split('}')[-1] if '}' in child.tag else child.tag
                if tag == 'item':
                    item_id = child.get('id', '')
                    href = child.get('href', '')
                    media_type = child.get('media-type', '')
                    if (item_id == cover_id or item_id == 'cover-image') and href:
                        full_path = os.path.join(opf_dir, href).replace('\\', '/')
                        try:
                            image_data = zf.read(full_path)
                            meta.cover_path = self._save_cover(image_data, file_path, covers_dir)
                        except KeyError:
                            pass

        # Fallback: look for common cover filenames
        if not meta.cover_path:
            for name in zf.namelist():
                basename = os.path.basename(name).lower()
                if 'cover' in basename and any(basename.endswith(ext) for ext in ['.jpg', '.jpeg', '.png']):
                    try:
                        image_data = zf.read(name)
                        meta.cover_path = self._save_cover(image_data, file_path, covers_dir)
                        break
                    except KeyError:
                        pass