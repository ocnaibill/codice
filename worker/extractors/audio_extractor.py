"""Audiobook metadata extractor using mutagen for ID3/MP4 tags.

Inspired by Calibre-Web's mutagen-based audiobook extraction.
Supports MP3 (ID3), M4A/M4B (MP4), FLAC, OGG formats.
"""
import os
from .base import BaseExtractor, ExtractedMetadata


class AudiobookExtractor(BaseExtractor):
    AUDIO_EXTS = {'.mp3', '.m4a', '.m4b', '.ogg', '.wav', '.flac'}

    def can_extract(self, file_path: str) -> bool:
        ext = os.path.splitext(file_path)[1].lower()
        return ext in self.AUDIO_EXTS

    def extract(self, file_path: str, covers_dir: str) -> ExtractedMetadata:
        meta = ExtractedMetadata(
            format=os.path.splitext(file_path)[1].lstrip('.'),
        )

        try:
            import mutagen
            from mutagen import File as MutagenFile

            audio = MutagenFile(file_path, easy=True)
            if audio is None:
                raise ValueError("Unrecognized audio format")

            title = str(audio.get('title', [''])[0]) if audio.get('title') else ''
            if title:
                meta.title = title
            else:
                meta.title = self._fallback_title(file_path)

            artist = str(audio.get('artist', [''])[0]) if audio.get('artist') else ''
            if artist:
                meta.author = artist
            else:
                meta.author = 'Unknown Artist'

            album = str(audio.get('album', [''])[0]) if audio.get('album') else ''
            if album:
                meta.series = album

            date = str(audio.get('date', [''])[0]) if audio.get('date') else ''
            if date:
                meta.publication_date = date

            genre = str(audio.get('genre', [''])[0]) if audio.get('genre') else ''
            if genre:
                meta.tags = [g.strip() for g in genre.split('/') if g.strip()]

            # Get duration from the non-easy API
            if hasattr(audio, 'info') and hasattr(audio.info, 'length'):
                meta.page_count = int(audio.info.length)  # seconds as page count proxy

        except ImportError:
            print("   ⚠️ mutagen not installed. Install with: pip install mutagen")
            meta.title = self._fallback_title(file_path)
            meta.author = 'Unknown Artist'
        except Exception as e:
            print(f"   ⚠️ Audiobook extraction error: {e}")
            meta.title = self._fallback_title(file_path)
            meta.author = 'Unknown Artist'

        return meta