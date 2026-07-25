import os
import re
import sys
import requests

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

class MetadataScraper:
    def __init__(self, covers_dir=None):
        base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
        if covers_dir is None:
            self.covers_dir = os.path.join(base_dir, "backend", "uploads", "covers")
        else:
            self.covers_dir = os.path.abspath(covers_dir)

        os.makedirs(self.covers_dir, exist_ok=True)
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

    def download_cover(self, image_url, original_filename):
        """Downloads cover image from web provider and caches it locally on server disk."""
        if not image_url:
            return None

        print("   🖼️ Downloading high-res cover image from web provider...")
        try:
            response = requests.get(image_url, stream=True, timeout=5)
            if response.status_code == 200:
                safe_base = os.path.splitext(original_filename)[0]
                safe_base = re.sub(r'[^a-zA-Z0-9]', '_', safe_base)[:40]
                
                cover_filename = f"official_{safe_base}.jpg"
                cover_path = os.path.join(self.covers_dir, cover_filename)
                
                with open(cover_path, 'wb') as f:
                    for chunk in response.iter_content(8192):
                        f.write(chunk)
                        
                print(f"   ✅ Local cover saved: {cover_filename}")
                return f"http://localhost:8080/covers/{cover_filename}"
        except Exception as e:
            print(f"   ⚠️ Failed to save web cover image locally: {e}")

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

                    raw_categories = volume_info.get("categories", [])
                    extracted_tags = []
                    for cat in raw_categories:
                        parts = [p.strip() for p in cat.split('/')]
                        extracted_tags.extend(parts)
                    clean_tags = list(dict.fromkeys([t for t in extracted_tags if t]))[:4]

                    print(f"   ✨ Found Google Books metadata: '{title}' by {author} (Tags: {clean_tags})")
                    return {"title": title, "author": author, "cover_url": cover_url, "tags": clean_tags}
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

                    raw_subjects = doc.get("subject", [])
                    clean_tags = list(dict.fromkeys([s.strip() for s in raw_subjects if s and len(s.strip()) <= 30]))[:4]

                    print(f"   ✨ Found OpenLibrary metadata: '{title}' by {author} (Tags: {clean_tags})")
                    return {"title": title, "author": author, "cover_url": cover_url, "tags": clean_tags}
        except Exception as e:
            print(f"   ⚠️ OpenLibrary API skipped: {e}")
        return None
