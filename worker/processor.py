import os
import sys
import zipfile
import fitz  # PyMuPDF

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

class CodiceExtractor:
    def __init__(self, covers_dir=None, allowed_dirs=None):
        base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
        env_storage = os.getenv("CODICE_STORAGE_PATH")
        if env_storage:
            storage_path = os.path.abspath(env_storage)
        else:
            storage_path = os.path.abspath(os.path.join(base_dir, "backend", "uploads"))

        if covers_dir is None:
            self.covers_dir = os.path.join(storage_path, "covers")
        else:
            self.covers_dir = os.path.abspath(covers_dir)

        os.makedirs(self.covers_dir, exist_ok=True)
        print(f"🧠 Universal extraction engine initialized (Covers Dir: {self.covers_dir})...")

        if allowed_dirs is None:
            self.allowed_dirs = [
                storage_path,
                os.path.abspath(os.path.join(base_dir, "backend", "uploads")),
                os.path.abspath(os.path.join(base_dir, "uploads"))
            ]
        else:
            self.allowed_dirs = [os.path.abspath(d) for d in allowed_dirs]

    def is_safe_path(self, file_path):
        """Check if file path is strictly within allowed upload directories."""
        abs_target = os.path.abspath(file_path)
        for allowed in self.allowed_dirs:
            if abs_target.startswith(allowed + os.sep) or abs_target == allowed:
                return True
        return False

    def process_file(self, file_path):
        """Main router method: dispatches file to appropriate handler by extension."""
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File not found: {file_path}")

        if not self.is_safe_path(file_path):
            raise ValueError(f"Access denied to file outside allowed upload directory: {file_path}")

        _, ext = os.path.splitext(file_path.lower())

        if ext == '.pdf':
            return self._process_pdf(file_path)
        elif ext == '.epub':
            return self._process_epub(file_path)
        elif ext == '.cbz':
            return self._process_cbz(file_path)
        else:
            raise ValueError(f"Unsupported format: {ext}")

    def _process_pdf(self, file_path):
        """Processes PDF documents specifically."""
        print(f"📄 Processing PDF file: {file_path}")
        doc = fitz.open(file_path)
        try:
            meta = doc.metadata or {}
            title = meta.get("title") or self._fallback_title(file_path)
            author = meta.get("author") or "Unknown Author"
            page_count = len(doc)
            cover_url = self._generate_cover(doc, file_path)

            return {
                "title": title,
                "author": author,
                "page_count": page_count,
                "cover_url": cover_url
            }
        finally:
            doc.close()

    def _process_epub(self, file_path):
        """Processes EPUB documents specifically."""
        print(f"📚 Processing EPUB file: {file_path}")
        doc = fitz.open(file_path)
        try:
            meta = doc.metadata or {}
            title = meta.get("title") or self._fallback_title(file_path)
            author = meta.get("author") or "Unknown Author"
            page_count = len(doc)
            cover_url = self._generate_cover(doc, file_path)

            return {
                "title": title,
                "author": author,
                "page_count": page_count,
                "cover_url": cover_url
            }
        finally:
            doc.close()

    def _process_cbz(self, file_path):
        """Processes Comic Book / Manga (CBZ) archives by extracting the first page image."""
        print(f"🖼️ Processing Comic/Manga archive (CBZ): {file_path}")
        
        title = self._fallback_title(file_path)
        author = "Unknown Author"
        page_count = 0
        cover_url = None

        with zipfile.ZipFile(file_path, 'r') as archive:
            valid_exts = ('.jpg', '.jpeg', '.png', '.webp')
            images = [f for f in archive.namelist() if f.lower().endswith(valid_exts)]
            images.sort()
            page_count = len(images)

            if page_count > 0:
                first_image = images[0]
                cover_filename = f"cover_{os.path.basename(file_path)}.jpg"
                cover_path = os.path.join(self.covers_dir, cover_filename)

                with open(cover_path, 'wb') as f:
                    f.write(archive.read(first_image))

                cover_url = f"http://localhost:8080/covers/{cover_filename}"

        return {
            "title": title,
            "author": author,
            "page_count": page_count,
            "cover_url": cover_url
        }

    def _generate_cover(self, doc, file_path):
        """Utility method to render first page as JPG cover."""
        cover_filename = f"cover_{os.path.basename(file_path)}.jpg"
        cover_path = os.path.join(self.covers_dir, cover_filename)

        if len(doc) > 0:
            page = doc.load_page(0)
            pix = page.get_pixmap(matrix=fitz.Matrix(2, 2))
            pix.save(cover_path)

        return f"http://localhost:8080/covers/{cover_filename}"

    def _fallback_title(self, file_path):
        """Generates clean title from filename if metadata is empty."""
        filename = os.path.basename(file_path)
        clean_name = filename.split("_", 1)[-1] if "_" in filename else filename
        return os.path.splitext(clean_name)[0]