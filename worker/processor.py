import os
import fitz  # PyMuPDF

class CodiceExtractor:
    def __init__(self, covers_dir=None, allowed_dirs=None):
        # Resolve absolute path for covers directory
        base_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
        if covers_dir is None:
            self.covers_dir = os.path.join(base_dir, "backend", "uploads", "covers")
        else:
            self.covers_dir = os.path.abspath(covers_dir)

        os.makedirs(self.covers_dir, exist_ok=True)
        print("🧠 PyMuPDF extraction engine initialized...")

        # Allowed directories for file safety validation
        if allowed_dirs is None:
            self.allowed_dirs = [
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

    def process_pdf(self, file_path):
        """Extracts basic metadata and generates cover image from first page."""
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"File not found: {file_path}")

        if not self.is_safe_path(file_path):
            raise ValueError(f"Access denied to file outside allowed upload directory: {file_path}")

        print(f"⚙️ Analyzing document: {file_path}")
        
        doc = fitz.open(file_path)
        
        # 1. Metadata Extraction
        meta = doc.metadata or {}
        title = meta.get("title")
        author = meta.get("author")
        page_count = len(doc)
        
        # Fallback if title metadata is missing or blank
        if not title or not title.strip():
            filename = os.path.basename(file_path)
            title = os.path.splitext(filename)[0]

        if not author or not author.strip():
            author = "Unknown Author"

        # 2. Cover Generation (First Page Rasterization)
        cover_filename = f"cover_{os.path.basename(file_path)}.jpg"
        cover_path = os.path.join(self.covers_dir, cover_filename)
        
        if page_count > 0:
            page = doc.load_page(0)
            # Matrix(2, 2) scales resolution up for high quality rendering
            pix = page.get_pixmap(matrix=fitz.Matrix(2, 2))
            pix.save(cover_path)
        
        doc.close()
        
        # Public URL served by backend
        cover_url = f"http://localhost:8080/covers/{cover_filename}"

        return {
            "title": title,
            "author": author,
            "page_count": page_count,
            "cover_url": cover_url
        }