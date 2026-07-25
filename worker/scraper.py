import re
import sys
import requests

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

class MetadataScraper:
    def __init__(self):
        self.google_url = "https://www.googleapis.com/books/v1/volumes"
        self.openlibrary_url = "https://openlibrary.org/search.json"
        print("🌐 Multi-provider Metadata Scraper initialized...")

    def clean_query(self, raw_title):
        """Cleans raw filename or extracted title for better search results."""
        if not raw_title:
            return ""

        # 1. Remove leading timestamps (e.g., 1784988520_)
        title = re.sub(r'^\d+_', '', raw_title)

        # 2. Remove file extensions
        title = re.sub(r'\.(pdf|epub|cbz)$', '', title, flags=re.IGNORECASE)

        # 3. Replace hyphens and underscores with spaces
        title = re.sub(r'[-_]', ' ', title)

        # 4. Remove common noise words
        title = re.sub(r'\b(compress|scan|digital|vol|livro)\b', '', title, flags=re.IGNORECASE)

        # 5. Remove alphanumeric hashes (5+ chars with digits, e.g. 3fcf3k, 5ffccdf4f2531)
        title = re.sub(r'\b(?=[a-z0-9]*\d)[a-z0-9]{5,}\b', '', title, flags=re.IGNORECASE)

        # 6. Collapse multiple spaces
        return ' '.join(title.split())

    def fetch_metadata(self, raw_title):
        """Fetches official book metadata from web providers (Google Books & OpenLibrary)."""
        query = self.clean_query(raw_title)
        if not query:
            return None

        print(f"🔍 Searching web metadata for: '{query}'...")

        # 1. Try Google Books API
        result = self._fetch_google_books(query)
        if result:
            return result

        # 2. Fallback to OpenLibrary API
        result = self._fetch_open_library(query)
        if result:
            return result

        print("   ❌ No better web metadata found across providers. Keeping local metadata.")
        return None

    def _fetch_google_books(self, query):
        try:
            response = requests.get(self.google_url, params={"q": query, "maxResults": 1}, timeout=4)
            if response.status_code == 200:
                data = response.json()
                if "items" in data and len(data["items"]) > 0:
                    volume_info = data["items"][0]["volumeInfo"]
                    title = volume_info.get("title")
                    authors = volume_info.get("authors", [])
                    author = authors[0] if authors else None
                    image_links = volume_info.get("imageLinks", {})
                    cover_url = image_links.get("thumbnail") or image_links.get("smallThumbnail")

                    if cover_url:
                        cover_url = cover_url.replace("http://", "https://")

                    print(f"   ✨ Found Google Books metadata: '{title}' by {author}")
                    return {"title": title, "author": author, "cover_url": cover_url}
        except Exception as e:
            print(f"   ⚠️ Google Books API skipped: {e}")
        return None

    def _fetch_open_library(self, query):
        try:
            response = requests.get(self.openlibrary_url, params={"q": query, "limit": 1}, timeout=4)
            if response.status_code == 200:
                data = response.json()
                docs = data.get("docs", [])
                if docs:
                    doc = docs[0]
                    title = doc.get("title")
                    authors = doc.get("author_name", [])
                    author = authors[0] if authors else None
                    cover_i = doc.get("cover_i")
                    cover_url = f"https://covers.openlibrary.org/b/id/{cover_i}-L.jpg" if cover_i else None

                    print(f"   ✨ Found OpenLibrary metadata: '{title}' by {author}")
                    return {"title": title, "author": author, "cover_url": cover_url}
        except Exception as e:
            print(f"   ⚠️ OpenLibrary API skipped: {e}")
        return None
