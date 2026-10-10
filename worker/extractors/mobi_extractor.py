"""MOBI/AZW metadata extractor via EXTH binary header parsing.

Inspired by Calibre-Web's MOBI extraction approach.
Parses the PalmDB + MOBI + EXTH headers directly from the binary file.
No external dependencies required beyond stdlib.
"""
import os
import re
import struct
from typing import Optional
from .base import BaseExtractor, ExtractedMetadata


class MobiExtractor(BaseExtractor):
    """Extracts metadata from MOBI, AZW, and AZW3 (KF8) files."""

    MOBI_EXTS = {'.mobi', '.azw', '.azw3', '.prc'}

    # EXTH record type IDs → metadata field names
    EXTH_FIELDS = {
        100: 'author',
        101: 'publisher',
        103: 'description',
        104: 'isbn',
        105: 'tag',        # Can repeat (subject/genre)
        106: 'publication_date',
        503: 'title',      # Updated title (overrides header title)
        524: 'language',
    }

    def can_extract(self, file_path: str) -> bool:
        ext = os.path.splitext(file_path)[1].lower()
        return ext in self.MOBI_EXTS

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(
            format=os.path.splitext(file_path)[1].lstrip('.').lower()
        )
        tags = []

        try:
            with open(file_path, 'rb') as f:
                # 1. Parse PalmDB header
                content = f.read()
                palm_header = self._parse_palm_header(content)
                if not palm_header:
                    raise ValueError("Invalid PalmDB header")

                records = palm_header['records']
                if not records:
                    raise ValueError("No records found in PalmDB")

                # 2. Parse MOBI header (starts at first record)
                first_record_offset = records[0]
                mobi = self._parse_mobi_header(content, first_record_offset)

                # 3. Get title from MOBI header
                if mobi.get('title'):
                    meta.title = mobi['title']

                # 4. Parse EXTH header if present
                exth_data = {}
                if mobi.get('has_exth'):
                    exth_offset = first_record_offset + 16 + mobi['header_length']
                    exth_data = self._parse_exth(content, exth_offset)

                    if 'title' in exth_data:
                        meta.title = exth_data['title']  # EXTH 503 overrides
                    if 'author' in exth_data:
                        meta.author = exth_data['author']
                    if 'publisher' in exth_data:
                        meta.publisher = exth_data['publisher']
                    if 'description' in exth_data:
                        meta.description = exth_data['description']
                    if 'isbn' in exth_data:
                        meta.isbn = exth_data['isbn']
                    if 'publication_date' in exth_data:
                        meta.publication_date = self._clean_date(exth_data['publication_date'])
                    if 'language' in exth_data:
                        meta.language = exth_data['language']
                    if 'tags' in exth_data:
                        tags = exth_data['tags']

                # 5. Extract the cover image: the one the EXTH says (counted from the first image), else the first image
                first_image = mobi.get('first_image_index')
                if first_image is not None:
                    candidates = []
                    if 'cover_offset' in exth_data:
                        candidates.append(first_image + exth_data['cover_offset'])
                    candidates.append(first_image)
                    for index in candidates:
                        if index >= len(records):
                            continue
                        img_start = records[index]
                        img_end = records[index + 1] if index + 1 < len(records) else len(content)
                        image_data = content[img_start:img_end]
                        # Verify it's actually an image (JPEG, PNG or GIF magic bytes)
                        if image_data[:2] == b'\xff\xd8' or image_data[:4] == b'\x89PNG' or image_data[:4] == b'GIF8':
                            meta.cover_path = self._save_cover(image_data, file_path, covers_dir)
                            break

        except Exception as e:
            print(f"   ⚠️ MOBI extraction error: {e}")

        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"
        if tags:
            meta.tags = tags

        return meta

    @staticmethod
    def _clean_date(text: str) -> str:
        """The date of an EXTH is a timestamp ("2024-03-15 03:00:00+00:00"); what the library keeps of a publication is the date, as far
        as it is one: "2024-03-15", "2024-03" or "2024". Anything that does not begin with one is kept as it came."""
        match = re.match(r'^\d{4}(?:-\d{2}(?:-\d{2})?)?', text.strip())
        return match.group(0) if match else text.strip()

    def _parse_palm_header(self, content: bytes) -> Optional[dict]:
        """Parse PalmDB header to get record offsets."""
        if len(content) < 78:
            return None

        # Number of records: bytes 76-77 (big-endian unsigned short)
        num_records = struct.unpack_from('>H', content, 76)[0]
        records = []

        for i in range(num_records):
            offset_pos = 78 + (i * 8)
            if offset_pos + 4 > len(content):
                break
            record_offset = struct.unpack_from('>I', content, offset_pos)[0]
            records.append(record_offset)

        return {'num_records': num_records, 'records': records}

    def _parse_mobi_header(self, content: bytes, offset: int) -> dict:
        """Parse the MOBI header: the text encoding, the full title, the first image and whether there is an EXTH.

        `offset` is where record 0 begins. It holds the PalmDOC header (16 bytes) and then the MOBI header, which begins with the magic
        'MOBI'. The fields, by their place from the magic (the MobileRead wiki gives them from the start of the record, 16 more):
        +4 header length, +12 text encoding, +68 and +72 the offset (from the start of record 0) and the length of the full title,
        +92 the index of the first image record, +112 the EXTH flags (bit 0x40: an EXTH header follows). A header shorter than a field
        does not have it (the early versions of the format)."""
        result = {}
        mobi_offset = offset + 16
        if mobi_offset + 8 > len(content) or content[mobi_offset:mobi_offset + 4] != b'MOBI':
            return result

        def u32(at: int, default=None):
            return struct.unpack_from('>I', content, mobi_offset + at)[0] if at + 4 <= header_length and mobi_offset + at + 4 <= len(content) else default

        header_length = struct.unpack_from('>I', content, mobi_offset + 4)[0]
        result['header_length'] = header_length

        # 65001 is UTF-8; 1252 is Windows-1252, which is what the older files are.
        result['encoding'] = 'utf-8' if u32(12) == 65001 else 'cp1252'

        title_offset, title_length = u32(68), u32(72)
        if title_offset and title_length and offset + title_offset + title_length <= len(content):
            raw_title = content[offset + title_offset:offset + title_offset + title_length]
            try:
                result['title'] = raw_title.decode(result['encoding']).strip()
            except (UnicodeDecodeError, LookupError):
                result['title'] = raw_title.decode('utf-8', errors='replace').strip()

        flags = u32(112, 0)
        result['has_exth'] = bool(flags & 0x40)

        first_image = u32(92)
        if first_image is not None and first_image != 0xFFFFFFFF:
            result['first_image_index'] = first_image

        return result

    def _parse_exth(self, content: bytes, offset: int) -> dict:
        """Parse EXTH header for rich metadata fields."""
        result = {}
        tags = []

        if offset + 12 > len(content):
            return result

        magic = content[offset:offset + 4]
        if magic != b'EXTH':
            return result

        # header_length = struct.unpack_from('>I', content, offset + 4)[0]
        record_count = struct.unpack_from('>I', content, offset + 8)[0]

        pos = offset + 12
        for _ in range(min(record_count, 200)):  # Safety cap
            if pos + 8 > len(content):
                break

            record_type = struct.unpack_from('>I', content, pos)[0]
            record_length = struct.unpack_from('>I', content, pos + 4)[0]

            if record_length < 8:
                break

            data_length = record_length - 8
            if pos + 8 + data_length > len(content):
                break

            raw_data = content[pos + 8:pos + 8 + data_length]

            if record_type == 201 and data_length == 4:
                # The cover: which image, counted from the first image of the book (it is rarely the first one).
                result['cover_offset'] = struct.unpack('>I', raw_data)[0]

            if record_type in self.EXTH_FIELDS:
                field_name = self.EXTH_FIELDS[record_type]
                try:
                    value = raw_data.decode('utf-8').strip()
                except UnicodeDecodeError:
                    value = raw_data.decode('cp1252', errors='replace').strip()

                if field_name == 'tag':
                    tags.append(value)
                else:
                    result[field_name] = value

            pos += record_length

        if tags:
            result['tags'] = tags

        return result