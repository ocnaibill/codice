"""MOBI/AZW metadata extractor via EXTH binary header parsing.

Inspired by Calibre-Web's MOBI extraction approach.
Parses the PalmDB + MOBI + EXTH headers directly from the binary file.
No external dependencies required beyond stdlib.
"""
import os
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
                        meta.publication_date = exth_data['publication_date']
                    if 'language' in exth_data:
                        meta.language = exth_data['language']
                    if 'tags' in exth_data:
                        tags = exth_data['tags']

                # 5. Extract cover image
                first_image = mobi.get('first_image_index')
                if first_image and first_image < len(records):
                    img_start = records[first_image]
                    img_end = records[first_image + 1] if first_image + 1 < len(records) else len(content)
                    image_data = content[img_start:img_end]

                    # Verify it's actually an image (JPEG or PNG magic bytes)
                    if image_data[:2] == b'\xff\xd8' or image_data[:4] == b'\x89PNG':
                        meta.cover_path = self._save_cover(image_data, file_path, covers_dir)

        except Exception as e:
            print(f"   ⚠️ MOBI extraction error: {e}")

        if not meta.title:
            meta.title = self._fallback_title(file_path)
        if not meta.author:
            meta.author = "Unknown Author"
        if tags:
            meta.tags = tags

        return meta

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
        """Parse MOBI header for title and image index."""
        result = {}

        # PalmDOC header: compression(2) + unused(2) + text_length(4) + record_count(2) + ...
        # MOBI header starts at offset + 16
        mobi_offset = offset + 16

        if mobi_offset + 132 > len(content):
            return result

        # Check 'MOBI' magic at mobi_offset
        magic = content[mobi_offset:mobi_offset + 4]
        if magic != b'MOBI':
            return result

        # Header length: offset+4, 4 bytes
        header_length = struct.unpack_from('>I', content, mobi_offset + 4)[0]
        result['header_length'] = header_length

        # Encoding: offset+8, 4 bytes (1252=cp1252, 65001=utf-8)
        encoding_raw = struct.unpack_from('>I', content, mobi_offset + 8)[0]
        result['encoding'] = 'utf-8' if encoding_raw == 65001 else 'cp1252'

        # Full title offset: mobi_offset+84, 4 bytes (relative to first record)
        title_offset = struct.unpack_from('>I', content, mobi_offset + 84)[0]
        title_length = struct.unpack_from('>I', content, mobi_offset + 88)[0]

        if title_offset and title_length:
            abs_title_offset = offset + title_offset
            if abs_title_offset + title_length <= len(content):
                raw_title = content[abs_title_offset:abs_title_offset + title_length]
                try:
                    result['title'] = raw_title.decode(result['encoding']).strip()
                except (UnicodeDecodeError, LookupError):
                    result['title'] = raw_title.decode('utf-8', errors='replace').strip()

        # EXTH flags: mobi_offset+128, 4 bytes
        exth_flags = struct.unpack_from('>I', content, mobi_offset + 128)[0]
        result['has_exth'] = bool(exth_flags & 0x40)

        # First image index: mobi_offset+108, 4 bytes
        if mobi_offset + 112 <= len(content):
            first_image = struct.unpack_from('>I', content, mobi_offset + 108)[0]
            if first_image != 0xFFFFFFFF:
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